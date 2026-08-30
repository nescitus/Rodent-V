package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Piece values for material delta calculations (Centipawns)
var pieceMatVal = [6]int{100, 300, 300, 500, 900, 0} // P, N, B, R, Q, K

// computeBoardMaterial returns (stm_material - opp_material) and total_material on board.
func computeBoardMaterial(p *Pos) (int, int) {
	stm := p.side
	opp := opp(stm)

	stmMat := 0
	oppMat := 0

	for pt := 0; pt < 5; pt++ {
		stmMat += p.count[stm][pt] * pieceMatVal[pt]
		oppMat += p.count[opp][pt] * pieceMatVal[pt]
	}

	return stmMat - oppMat, stmMat + oppMat
}

// isAggressivePosition evaluates if a position matches Patricia Filter 9, Filter 10, or Combined criteria.
func isAggressivePosition(p *Pos, eval int, result float64, mode string) bool {
	matDiff, totalMat := computeBoardMaterial(p)

	// Filter out dry, liquidated endgames (< 30 pawns / 3000 cp total material)
	if totalMat < 3000 {
		return false
	}

	absEval := eval
	if absEval < 0 {
		absEval = -absEval
	}

	// Filter 10: Winning Sacrifices & Knockout Attacks (Down material or equal, but eval is +400 cp winning and won the game)
	isFilter10 := false
	if (eval > matDiff+400 && matDiff <= 50 && result >= 0.99) ||
		(eval < matDiff-400 && matDiff >= -50 && result <= 0.01) {
		isFilter10 = true
	}

	if mode == "10" {
		return isFilter10
	}

	// Filter 9: Dynamic Initiative / Extreme Positional Compensation
	// Position where eval diverges wildly from material balance (e.g. piece down for monster king attack)
	margin := 300
	if dynamicMargin := (absEval * 3) / 4; dynamicMargin > margin {
		margin = dynamicMargin
	}

	isFilter9 := false
	if eval > matDiff+margin || eval < matDiff-margin {
		isFilter9 = true
	}

	if mode == "9" {
		return isFilter9
	}

	// Combined Mode (default / "all"): Match either Filter 10 (winning sacrifices) or Filter 9 (heavy initiative)
	return isFilter10 || isFilter9
}

// =================================================================================
// 1. POSITION-LEVEL EXTRACTION FROM VIRIFORMAT (.vf) TO TEXT (<fen> | <score> | <wdl>)
// =================================================================================

type VfTextChunk struct {
	seq  int64
	data []byte
}

type VfTextResult struct {
	seq          int64
	lines        []string
	scannedGames int
	scannedPos   int
	matchedPos   int
}

func filterVfToText(chunk *VfTextChunk, p *Pos, mode string) *VfTextResult {
	data := chunk.data
	n := len(data)
	off := 0

	var lines []string
	gamesCount := 0
	scannedPositions := 0
	matchedPositions := 0

	for off+32 <= n {
		header := data[off : off+32]
		if !unpackViriBoard(header, p) {
			break
		}

		gamesCount++
		wdlByte := header[30]
		var result float64 = 0.5
		if wdlByte == 2 {
			result = 1.0 // White win
		} else if wdlByte == 0 {
			result = 0.0 // Black win
		}

		// Root board eval
		rootEvalWhite := int(int16(binary.LittleEndian.Uint16(header[28:30])))
		rootStmEval := rootEvalWhite
		if p.side == Black {
			rootStmEval = -rootEvalWhite
		}

		if isAggressivePosition(p, rootStmEval, result, mode) {
			lines = append(lines, fmt.Sprintf("%s | %d | %.1f", p.generateFen(), rootStmEval, result))
			matchedPositions++
		}
		scannedPositions++

		off += 32

		// Moves loop
		for off+4 <= n {
			moveRecord := data[off : off+4]
			if moveRecord[0] == 0 && moveRecord[1] == 0 && moveRecord[2] == 0 && moveRecord[3] == 0 {
				off += 4
				break
			}

			evalWhite := int(int16(binary.LittleEndian.Uint16(moveRecord[2:4])))
			stmEval := evalWhite
			if p.side == Black {
				stmEval = -evalWhite
			}

			if isAggressivePosition(p, stmEval, result, mode) {
				lines = append(lines, fmt.Sprintf("%s | %d | %.1f", p.generateFen(), stmEval, result))
				matchedPositions++
			}
			scannedPositions++

			// Decode move and execute
			vMove := binary.LittleEndian.Uint16(moveRecord[0:2])
			fr := int(vMove & 0x3F)
			to := int((vMove >> 6) & 0x3F)
			vPromo := int((vMove >> 12) & 0x3)
			vType := int((vMove >> 14) & 0x3)

			var mType int
			if vType == 0 {
				if p.typeAt(fr) == P && (to-fr == 16 || fr-to == 16) {
					mType = EP_SET
				} else {
					mType = NORMAL
				}
			} else if vType == 1 {
				mType = EP_CAP
			} else if vType == 2 {
				mType = CASTLE
				isKingside := false
				if fr < 32 {
					if to == 7 || to == 6 || to > fr {
						isKingside = true
					}
				} else {
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
			} else if vType == 3 {
				mType = N_PROM + vPromo
			}

			move := (mType << 12) | (to << 6) | fr
			var u Update
			var r Revert
			makeMove(p, &u, &r, move)
			if p.clock == 0 {
				p.histLen = 0
			}

			off += 4
		}
	}

	return &VfTextResult{
		seq:          chunk.seq,
		lines:        lines,
		scannedGames: gamesCount,
		scannedPos:   scannedPositions,
		matchedPos:   matchedPositions,
	}
}

func runAggressiveFilterVfToText(inputFile, outputFile, mode string, numThreads int) {
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
	fmt.Printf("   PATRICIA / TAL HIGH-SPEED POSITION EXTRACTION (.vf -> .txt)   \n")
	fmt.Printf("=================================================================\n")
	fmt.Printf("Input:       %s (%.2f GB)\n", inputFile, float64(totalFileSize)/(1024*1024*1024))
	fmt.Printf("Output:      %s (<fen> | <eval> | <wdl> format)\n", outputFile)
	fmt.Printf("Filter Mode: %s (9=Dynamic Initiative, 10=Winning Sacrifices, all=Combined)\n", mode)
	fmt.Printf("Threads:     %d worker threads (AVX2 Parallel)\n", numThreads)
	fmt.Printf("=================================================================\n\n")

	const gamesPerChunk = 2048
	jobsChan := make(chan *VfTextChunk, numThreads*4)
	resultsChan := make(chan *VfTextResult, numThreads*4)

	var totalReadBytes int64
	var totalScannedGames int64
	var totalScannedPositions int64
	var totalMatchedPositions int64

	startTime := time.Now()

	// 1. Worker Goroutines
	var workerWg sync.WaitGroup
	for i := 0; i < numThreads; i++ {
		workerWg.Add(1)
		go func() {
			defer workerWg.Done()
			var p Pos

			for chunk := range jobsChan {
				res := filterVfToText(chunk, &p, mode)
				atomic.AddInt64(&totalScannedGames, int64(res.scannedGames))
				atomic.AddInt64(&totalScannedPositions, int64(res.scannedPos))
				atomic.AddInt64(&totalMatchedPositions, int64(res.matchedPos))
				atomic.AddInt64(&totalReadBytes, int64(len(chunk.data)))
				resultsChan <- res
			}
		}()
	}

	// 2. Sequential Ordered Writer
	var writerWg sync.WaitGroup
	writerWg.Add(1)
	go func() {
		defer writerWg.Done()
		bufWriter := bufio.NewWriterSize(outFile, 8*1024*1024)
		defer bufWriter.Flush()

		pendingChunks := make(map[int64]*VfTextResult)
		expectedSeq := int64(0)

		for res := range resultsChan {
			pendingChunks[res.seq] = res
			for {
				if nextRes, found := pendingChunks[expectedSeq]; found {
					for _, line := range nextRes.lines {
						bufWriter.WriteString(line)
						bufWriter.WriteByte('\n')
					}
					delete(pendingChunks, expectedSeq)
					expectedSeq++
				} else {
					break
				}
			}
		}
	}()

	// 3. Live Progress Monitor
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
				procBytes := atomic.LoadInt64(&totalReadBytes)
				procPos := atomic.LoadInt64(&totalScannedPositions)
				procMatched := atomic.LoadInt64(&totalMatchedPositions)

				pct := float64(procBytes) * 100.0 / float64(totalFileSize)
				elapsed := now.Sub(startTime).Seconds()
				dt := now.Sub(lastTime).Seconds()
				dPos := procPos - lastPos

				instSpeed := float64(dPos) / dt

				etaSec := 0.0
				if pct > 0 && pct < 100 {
					remainingBytes := totalFileSize - procBytes
					bytesPerSec := float64(procBytes) / elapsed
					if bytesPerSec > 0 {
						etaSec = float64(remainingBytes) / bytesPerSec
					}
				}

				matchPct := 0.0
				if procPos > 0 {
					matchPct = float64(procMatched) * 100.0 / float64(procPos)
				}

				fmt.Printf("\r[%.1f%%] Scanned: %d | Matched: %d (%.2f%%) | Speed: %.2fM pos/s | ETA: %.0fs    ",
					pct, procPos, procMatched, matchPct, instSpeed/1_000_000, etaSec)

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
			jobsChan <- &VfTextChunk{
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
		jobsChan <- &VfTextChunk{
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
	finalGames := atomic.LoadInt64(&totalScannedGames)
	finalScannedPos := atomic.LoadInt64(&totalScannedPositions)
	finalMatched := atomic.LoadInt64(&totalMatchedPositions)

	fmt.Printf("\n\n=================================================================\n")
	fmt.Printf("Extraction Complete in %.2f seconds!\n", totalElapsed)
	fmt.Printf("Total Games Scanned:      %d\n", finalGames)
	fmt.Printf("Total Positions Scanned:  %d\n", finalScannedPos)
	fmt.Printf("Extracted Aggressive Pos: %d (%.2f%% of dataset)\n", finalMatched, float64(finalMatched)*100.0/float64(finalScannedPos))
	fmt.Printf("Throughput:               %.2f Million positions/second\n", (float64(finalScannedPos)/totalElapsed)/1_000_000)
	fmt.Printf("Output File:              %s\n", outputFile)
	fmt.Printf("=================================================================\n")
}

// =================================================================================
// 2. TEXT (.txt) PARALLEL LINE FILTERING
// =================================================================================

type AggressiveFilterChunk struct {
	seq   int64
	lines []string
}

type AggressiveFilterResult struct {
	seq     int64
	matched []string
}

func runAggressiveFilterText(inputFile, outputFile string, mode string, numThreads int) {
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
	fmt.Printf("          PATRICIA / TAL TEXT DATASET FILTER                     \n")
	fmt.Printf("=================================================================\n")
	fmt.Printf("Input:       %s (%.2f MB)\n", inputFile, float64(totalFileSize)/(1024*1024))
	fmt.Printf("Output:      %s\n", outputFile)
	fmt.Printf("Filter Mode: %s (9=Dynamic Initiative, 10=Winning Sacrifices, all=Combined)\n", mode)
	fmt.Printf("Threads:     %d worker threads\n", numThreads)
	fmt.Printf("=================================================================\n\n")

	const linesPerChunk = 2048
	jobsChan := make(chan *AggressiveFilterChunk, numThreads*4)
	resultsChan := make(chan *AggressiveFilterResult, numThreads*4)

	var totalReadLines int64
	var totalMatchedLines int64
	var totalProcessedBytes int64

	startTime := time.Now()

	// 1. Worker Goroutines
	var workerWg sync.WaitGroup
	for i := 0; i < numThreads; i++ {
		workerWg.Add(1)
		go func() {
			defer workerWg.Done()
			var p Pos
			var localMatched []string

			for chunk := range jobsChan {
				localMatched = localMatched[:0]
				chunkBytes := int64(0)

				for _, line := range chunk.lines {
					trimmed := strings.TrimSpace(line)
					chunkBytes += int64(len(line) + 1)
					if trimmed == "" || strings.HasPrefix(trimmed, "#") {
						continue
					}

					parts := strings.Split(trimmed, "|")
					if len(parts) < 3 {
						continue
					}

					fen := strings.TrimSpace(parts[0])
					evalStr := strings.TrimSpace(parts[1])
					wdlStr := strings.TrimSpace(parts[2])

					eval, err1 := strconv.Atoi(evalStr)
					result, err2 := strconv.ParseFloat(wdlStr, 64)
					if err1 != nil || err2 != nil {
						continue
					}

					parseFEN(&p, fen)

					if isAggressivePosition(&p, eval, result, mode) {
						localMatched = append(localMatched, line)
					}
				}

				atomic.AddInt64(&totalReadLines, int64(len(chunk.lines)))
				atomic.AddInt64(&totalMatchedLines, int64(len(localMatched)))
				atomic.AddInt64(&totalProcessedBytes, chunkBytes)

				outSlice := make([]string, len(localMatched))
				copy(outSlice, localMatched)
				resultsChan <- &AggressiveFilterResult{
					seq:     chunk.seq,
					matched: outSlice,
				}
			}
		}()
	}

	// 2. Sequential Ordered Writer
	var writerWg sync.WaitGroup
	writerWg.Add(1)
	go func() {
		defer writerWg.Done()
		bufWriter := bufio.NewWriterSize(outFile, 4*1024*1024)
		defer bufWriter.Flush()

		pendingChunks := make(map[int64]*AggressiveFilterResult)
		expectedSeq := int64(0)

		for res := range resultsChan {
			pendingChunks[res.seq] = res
			for {
				if nextRes, found := pendingChunks[expectedSeq]; found {
					for _, l := range nextRes.matched {
						bufWriter.WriteString(l)
						bufWriter.WriteByte('\n')
					}
					delete(pendingChunks, expectedSeq)
					expectedSeq++
				} else {
					break
				}
			}
		}
	}()

	// 3. Live Progress Monitor
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
				procLines := atomic.LoadInt64(&totalReadLines)
				procMatched := atomic.LoadInt64(&totalMatchedLines)

				pct := float64(procBytes) * 100.0 / float64(totalFileSize)
				elapsed := now.Sub(startTime).Seconds()
				dt := now.Sub(lastTime).Seconds()
				dPos := procLines - lastPos

				instSpeed := float64(dPos) / dt

				etaSec := 0.0
				if pct > 0 && pct < 100 {
					remainingBytes := totalFileSize - procBytes
					bytesPerSec := float64(procBytes) / elapsed
					if bytesPerSec > 0 {
						etaSec = float64(remainingBytes) / bytesPerSec
					}
				}

				matchPct := 0.0
				if procLines > 0 {
					matchPct = float64(procMatched) * 100.0 / float64(procLines)
				}

				fmt.Printf("\r[%.1f%%] Read: %d | Matched: %d (%.1f%%) | Speed: %.2fM lines/s | ETA: %.0fs    ",
					pct, procLines, procMatched, matchPct, instSpeed/1_000_000, etaSec)

				lastPos = procLines
				lastTime = now
			}
		}
	}()

	// 4. Main Streaming Reader Loop
	scanner := bufio.NewScanner(inFile)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	var currentLines []string
	chunkSeq := int64(0)

	for scanner.Scan() {
		line := scanner.Text()
		currentLines = append(currentLines, line)

		if len(currentLines) >= linesPerChunk {
			chunkLines := make([]string, len(currentLines))
			copy(chunkLines, currentLines)
			jobsChan <- &AggressiveFilterChunk{
				seq:   chunkSeq,
				lines: chunkLines,
			}
			chunkSeq++
			currentLines = currentLines[:0]
		}
	}

	if len(currentLines) > 0 {
		chunkLines := make([]string, len(currentLines))
		copy(chunkLines, currentLines)
		jobsChan <- &AggressiveFilterChunk{
			seq:   chunkSeq,
			lines: chunkLines,
		}
	}

	close(jobsChan)
	workerWg.Wait()
	close(resultsChan)
	writerWg.Wait()
	close(stopProgress)

	totalElapsed := time.Since(startTime).Seconds()
	finalRead := atomic.LoadInt64(&totalReadLines)
	finalMatched := atomic.LoadInt64(&totalMatchedLines)

	fmt.Printf("\n\n=================================================================\n")
	fmt.Printf("Aggressive Filtering Complete in %.2f seconds!\n", totalElapsed)
	fmt.Printf("Total Lines Read:     %d\n", finalRead)
	fmt.Printf("Aggressive Matches:   %d (%.2f%% of dataset)\n", finalMatched, float64(finalMatched)*100.0/float64(finalRead))
	fmt.Printf("Throughput:           %.2f Million lines/second\n", (float64(finalRead)/totalElapsed)/1_000_000)
	fmt.Printf("Output File:          %s\n", outputFile)
	fmt.Printf("=================================================================\n")
}

// runAggressiveFilter automatically detects file extension (.vf or .txt) and runs the matching pipeline.
func runAggressiveFilter(inputFile, outputFile string, mode string, numThreads int) {
	if numThreads <= 0 {
		numThreads = runtime.NumCPU()
	}
	if mode == "" {
		mode = "all"
	}

	inExt := strings.ToLower(filepath.Ext(inputFile))
	if inExt == ".vf" {
		if outputFile == "" {
			base := strings.TrimSuffix(inputFile, filepath.Ext(inputFile))
			outputFile = base + "_aggressive.txt"
		}
		runAggressiveFilterVfToText(inputFile, outputFile, mode, numThreads)
	} else {
		if outputFile == "" {
			base := strings.TrimSuffix(inputFile, filepath.Ext(inputFile))
			outputFile = base + "_aggressive.txt"
		}
		runAggressiveFilterText(inputFile, outputFile, mode, numThreads)
	}
}
