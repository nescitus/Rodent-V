package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"math/bits"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// GameChunk represents a contiguous batch of Viriformat serialized games for parallel processing.
type GameChunk struct {
	seq   int64
	data  []byte
	games int
	moves int
}

// unpackViriBoard initializes a Pos from a 32-byte Viriformat board header.
func unpackViriBoard(header []byte, p *Pos) bool {
	occ := binary.LittleEndian.Uint64(header[0:8])
	if bits.OnesCount64(occ) > 32 {
		return false
	}

	// Reset position fields
	p.colorBB[White] = 0
	p.colorBB[Black] = 0
	for pt := 0; pt < 6; pt++ {
		p.typeBB[pt] = 0
		p.count[White][pt] = 0
		p.count[Black][pt] = 0
	}
	for sq := 0; sq < 64; sq++ {
		p.board[sq] = NO_PC
	}
	p.castlingRookSq[White][0] = NO_SQ
	p.castlingRookSq[White][1] = NO_SQ
	p.castlingRookSq[Black][0] = NO_SQ
	p.castlingRookSq[Black][1] = NO_SQ
	p.castleRights = 0

	// First pass: reconstruct pieces on the board
	iterOcc := occ
	idx := 0
	for iterOcc != 0 {
		sq := bits.TrailingZeros64(iterOcc)
		iterOcc &= iterOcc - 1

		byteVal := header[8+(idx/2)]
		var nibble byte
		if idx%2 == 1 {
			nibble = byteVal >> 4
		} else {
			nibble = byteVal & 0x0F
		}
		idx++

		isBlack := (nibble & 8) != 0
		color := White
		if isBlack {
			color = Black
		}
		pt := int(nibble & 7)

		if pt == 5 { // King
			p.kingSq[color] = sq
			p.board[sq] = makePiece(color, K)
			p.colorBB[color] |= squareBit(sq)
			p.typeBB[K] |= squareBit(sq)
			p.count[color][K]++
		} else if pt == 6 { // Unmoved Rook with castling rights
			p.board[sq] = makePiece(color, R)
			p.colorBB[color] |= squareBit(sq)
			p.typeBB[R] |= squareBit(sq)
			p.count[color][R]++
		} else {
			p.board[sq] = makePiece(color, pt)
			p.colorBB[color] |= squareBit(sq)
			p.typeBB[pt] |= squareBit(sq)
			p.count[color][pt]++
		}
	}

	// Second pass: assign castling rights and rook squares relative to the king
	iterOcc2 := occ
	idx2 := 0
	for iterOcc2 != 0 {
		sq := bits.TrailingZeros64(iterOcc2)
		iterOcc2 &= iterOcc2 - 1

		byteVal := header[8+(idx2/2)]
		var nibble byte
		if idx2%2 == 1 {
			nibble = byteVal >> 4
		} else {
			nibble = byteVal & 0x0F
		}
		idx2++

		isBlack := (nibble & 8) != 0
		color := White
		if isBlack {
			color = Black
		}
		pt := int(nibble & 7)

		if pt == 6 { // Unmoved castling rook
			if color == White {
				if sq > p.kingSq[White] {
					p.castlingRookSq[White][0] = sq
					p.castleRights |= 1 // WK
				} else {
					p.castlingRookSq[White][1] = sq
					p.castleRights |= 2 // WQ
				}
			} else {
				if sq > p.kingSq[Black] {
					p.castlingRookSq[Black][0] = sq
					p.castleRights |= 4 // BK
				} else {
					p.castlingRookSq[Black][1] = sq
					p.castleRights |= 8 // BQ
				}
			}
		}
	}

	// Set dynamic castleMask
	for i := 0; i < 64; i++ {
		p.castleMask[i] = 15
	}
	p.castleMask[p.kingSq[White]] &^= 3
	p.castleMask[p.kingSq[Black]] &^= 12
	if sq := p.castlingRookSq[White][0]; sq != NO_SQ {
		p.castleMask[sq] &^= 1
	}
	if sq := p.castlingRookSq[White][1]; sq != NO_SQ {
		p.castleMask[sq] &^= 2
	}
	if sq := p.castlingRookSq[Black][0]; sq != NO_SQ {
		p.castleMask[sq] &^= 4
	}
	if sq := p.castlingRookSq[Black][1]; sq != NO_SQ {
		p.castleMask[sq] &^= 8
	}

	// Side to move and en-passant square
	ep := int(header[24] & 0x7F)
	if ep >= 64 {
		p.epSquare = NO_SQ
	} else {
		p.epSquare = ep
	}
	if (header[24] & 0x80) != 0 {
		p.side = Black
	} else {
		p.side = White
	}

	p.clock = int(header[25])
	p.histLen = 0

	return true
}

// processGameChunk relabels all games within a chunk in-place using the worker's local Pos and Accumulator.
func processGameChunk(chunk *GameChunk, p *Pos, acc *Accumulator) {
	data := chunk.data
	n := len(data)
	off := 0

	for off+32 <= n {
		header := data[off : off+32]
		if !unpackViriBoard(header, p) {
			break
		}

		refresh(p, acc)

		// Rescore root position (from White's perspective)
		rootStmEval := acc.getEval(p, p.side)
		var rootEvalWhite int16
		if p.side == White {
			rootEvalWhite = int16(rootStmEval)
		} else {
			rootEvalWhite = int16(-rootStmEval)
		}
		binary.LittleEndian.PutUint16(data[off+28:off+30], uint16(rootEvalWhite))

		off += 32
		chunk.games++

		// Process moves until 4-byte zero terminator
		for off+4 <= n {
			moveRecord := data[off : off+4]
			if moveRecord[0] == 0 && moveRecord[1] == 0 && moveRecord[2] == 0 && moveRecord[3] == 0 {
				off += 4
				break
			}

			// In Viriformat, each move record stores the evaluation of the position BEFORE the move is executed.
			stmEval := acc.getEval(p, p.side)
			var evalWhite int16
			if p.side == White {
				evalWhite = int16(stmEval)
			} else {
				evalWhite = int16(-stmEval)
			}
			binary.LittleEndian.PutUint16(data[off+2:off+4], uint16(evalWhite))
			chunk.moves++

			// Decode Viriformat move
			vMove := binary.LittleEndian.Uint16(moveRecord[0:2])
			fr := int(vMove & 0x3F)
			to := int((vMove >> 6) & 0x3F)
			vPromo := int((vMove >> 12) & 0x3)
			vType := int((vMove >> 14) & 0x3)

			var mType int
			if vType == 0 { // Normal move
				if p.typeAt(fr) == P && (to-fr == 16 || fr-to == 16) {
					mType = EP_SET
				} else {
					mType = NORMAL
				}
			} else if vType == 1 { // En Passant capture
				mType = EP_CAP
			} else if vType == 2 { // Castling
				mType = CASTLE
				// Map castling target square to true rook square if stored as king destination
				isKingside := false
				if fr < 32 { // White
					if to == 7 || to == 6 || to > fr {
						isKingside = true
					}
				} else { // Black
					if to == 63 || to == 62 || to > fr {
						isKingside = true
					}
				}
				var trueRookSq int
				if isKingside {
					trueRookSq = p.castlingRookSq[p.side][0]
				} else {
					trueRookSq = p.castlingRookSq[p.side][1]
				}
				if trueRookSq >= 0 {
					to = trueRookSq
				}
			} else if vType == 3 { // Promotion
				mType = N_PROM + vPromo
			}

			move := (mType << 12) | (to << 6) | fr

			// Execute move and update accumulator incrementally
			var u Update
			var r Revert
			makeMove(p, &u, &r, move)
			acc.applyPendingChanges(acc, p, &u, nil)
			if p.clock == 0 {
				p.histLen = 0
			}

			off += 4
		}
	}
}

// runRelabel executes the high-throughput parallel Viriformat dataset relabeling.
func runRelabel(inputFile, outputFile string, netFile string, numThreads int) {
	if numThreads <= 0 {
		numThreads = runtime.NumCPU()
	}

	if outputFile == "" {
		ext := filepath.Ext(inputFile)
		base := strings.TrimSuffix(inputFile, ext)
		outputFile = base + "_relabeled" + ext
	}

	if netFile != "" {
		nnueLoad(netFile)
	}

	if !nnue.Loaded {
		fmt.Printf("Error: NNUE network is not loaded! Cannot proceed with relabeling.\n")
		return
	}

	inFile, err := os.Open(inputFile)
	if err != nil {
		fmt.Printf("Error opening input file '%s': %v\n", inputFile, err)
		return
	}
	defer inFile.Close()

	fileInfo, err := inFile.Stat()
	if err != nil {
		fmt.Printf("Error stating input file: %v\n", err)
		return
	}
	totalFileSize := fileInfo.Size()

	outFile, err := os.Create(outputFile)
	if err != nil {
		fmt.Printf("Error creating output file '%s': %v\n", outputFile, err)
		return
	}
	defer outFile.Close()

	fmt.Printf("=================================================================\n")
	fmt.Printf("             RODENT-V PARALLEL DATASET RELABELER                 \n")
	fmt.Printf("=================================================================\n")
	fmt.Printf("Input:    %s (%.2f GB)\n", inputFile, float64(totalFileSize)/(1024*1024*1024))
	fmt.Printf("Output:   %s\n", outputFile)
	fmt.Printf("Net:      %s (Loaded: %v)\n", nnuePath, nnue.Loaded)
	fmt.Printf("Threads:  %d worker threads (AVX2 Enabled)\n", numThreads)
	fmt.Printf("=================================================================\n\n")

	const gamesPerChunk = 2048
	jobsChan := make(chan *GameChunk, numThreads*4)
	resultsChan := make(chan *GameChunk, numThreads*4)

	var totalProcessedBytes int64
	var totalProcessedGames int64
	var totalProcessedPositions int64

	startTime := time.Now()

	// 1. Start Worker Goroutines
	var workerWg sync.WaitGroup
	for i := 0; i < numThreads; i++ {
		workerWg.Add(1)
		go func() {
			defer workerWg.Done()
			var p Pos
			var acc Accumulator

			for chunk := range jobsChan {
				processGameChunk(chunk, &p, &acc)
				atomic.AddInt64(&totalProcessedGames, int64(chunk.games))
				atomic.AddInt64(&totalProcessedPositions, int64(chunk.moves))
				atomic.AddInt64(&totalProcessedBytes, int64(len(chunk.data)))
				resultsChan <- chunk
			}
		}()
	}

	// 2. Start Ordered Writer Goroutine
	var writerWg sync.WaitGroup
	writerWg.Add(1)
	go func() {
		defer writerWg.Done()
		bufWriter := bufio.NewWriterSize(outFile, 4*1024*1024)
		defer bufWriter.Flush()

		pendingChunks := make(map[int64]*GameChunk)
		expectedSeq := int64(0)

		for chunk := range resultsChan {
			pendingChunks[chunk.seq] = chunk
			for {
				if nextChunk, found := pendingChunks[expectedSeq]; found {
					bufWriter.Write(nextChunk.data)
					delete(pendingChunks, expectedSeq)
					expectedSeq++
				} else {
					break
				}
			}
		}
	}()

	// 3. Start Live Progress Monitor
	stopProgress := make(chan struct{})
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		var lastPos int64
		var lastTime = time.Now()

		for {
			select {
			case <-stopProgress:
				return
			case now := <-ticker.C:
				procBytes := atomic.LoadInt64(&totalProcessedBytes)
				procGames := atomic.LoadInt64(&totalProcessedGames)
				procPos := atomic.LoadInt64(&totalProcessedPositions)

				pct := float64(procBytes) * 100.0 / float64(totalFileSize)
				elapsed := now.Sub(startTime).Seconds()
				dt := now.Sub(lastTime).Seconds()
				dPos := procPos - lastPos

				instSpeed := float64(dPos) / dt
				avgSpeed := float64(procPos) / elapsed

				etaSec := 0.0
				if pct > 0 && pct < 100 {
					remainingBytes := totalFileSize - procBytes
					bytesPerSec := float64(procBytes) / elapsed
					if bytesPerSec > 0 {
						etaSec = float64(remainingBytes) / bytesPerSec
					}
				}

				fmt.Printf("\r[%.1f%%] Games: %d | Positions: %d | Speed: %.2fM pos/s | Avg: %.2fM pos/s | ETA: %.0fs    ",
					pct, procGames, procPos, instSpeed/1_000_000, avgSpeed/1_000_000, etaSec)

				lastPos = procPos
				lastTime = now
			}
		}
	}()

	// 4. Main Streaming Reader Loop
	reader := bufio.NewReaderSize(inFile, 4*1024*1024)
	var headerBuf [32]byte
	var moveBuf [4]byte
	var chunkBuffer []byte
	var gameBuf []byte
	chunkSeq := int64(0)
	gamesInChunk := 0

	for {
		gameBuf = gameBuf[:0]
		_, err := io.ReadFull(reader, headerBuf[:])
		if err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			}
			fmt.Printf("\nError reading game header: %v\n", err)
			break
		}

		gameBuf = append(gameBuf, headerBuf[:]...)
		validGame := false

		for {
			_, err := io.ReadFull(reader, moveBuf[:])
			if err != nil {
				break
			}
			gameBuf = append(gameBuf, moveBuf[:]...)
			if moveBuf[0] == 0 && moveBuf[1] == 0 && moveBuf[2] == 0 && moveBuf[3] == 0 {
				validGame = true
				break
			}
		}

		if !validGame {
			break
		}

		chunkBuffer = append(chunkBuffer, gameBuf...)
		gamesInChunk++

		if gamesInChunk >= gamesPerChunk {
			chunkData := make([]byte, len(chunkBuffer))
			copy(chunkData, chunkBuffer)
			jobsChan <- &GameChunk{
				seq:  chunkSeq,
				data: chunkData,
			}
			chunkSeq++
			chunkBuffer = chunkBuffer[:0]
			gamesInChunk = 0
		}
	}

	if len(chunkBuffer) > 0 {
		chunkData := make([]byte, len(chunkBuffer))
		copy(chunkData, chunkBuffer)
		jobsChan <- &GameChunk{
			seq:  chunkSeq,
			data: chunkData,
		}
	}

	close(jobsChan)
	workerWg.Wait()
	close(resultsChan)
	writerWg.Wait()
	close(stopProgress)

	totalElapsed := time.Since(startTime).Seconds()
	finalGames := atomic.LoadInt64(&totalProcessedGames)
	finalPos := atomic.LoadInt64(&totalProcessedPositions)

	fmt.Printf("\n\n=================================================================\n")
	fmt.Printf("Relabeling Complete in %.2f seconds!\n", totalElapsed)
	fmt.Printf("Total Games:      %d\n", finalGames)
	fmt.Printf("Total Positions:  %d\n", finalPos)
	fmt.Printf("Throughput:       %.2f Million positions/second\n", (float64(finalPos)/totalElapsed)/1_000_000)
	fmt.Printf("Output File:      %s\n", outputFile)
	fmt.Printf("=================================================================\n")
}
