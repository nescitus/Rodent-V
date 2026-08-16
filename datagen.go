package main

import (
	"bufio"
	"fmt"
	"math/rand"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

type dgEntry struct {
	fen   string
	eval  int // White perspective
	score float64
}

var datagenMode bool = false

// Scharnagl N5n table for knight placement in Chess960
var n5n = [10][2]int{
	{0, 0}, {0, 1}, {0, 2}, {0, 3},
	{1, 1}, {1, 2}, {1, 3},
	{2, 2}, {2, 3},
	{3, 3},
}

// frcBackrank generates the 8-piece backrank for a Chess960 index n (0..959).
// Index 518 produces standard chess: RNBQKBNR.
func frcBackrank(n int) [8]byte {
	n2 := n / 4
	b1 := n % 4

	n3 := n2 / 4
	b2 := n2 % 4

	n4 := n3 / 6
	q := n3 % 6

	var rank [8]byte
	rank[b1*2+1] = 'B'
	rank[b2*2] = 'B'

	empty := 0
	for i := 0; i < 8; i++ {
		if rank[i] == 0 {
			if empty == q {
				rank[i] = 'Q'
			}
			empty++
		}
	}

	knight1 := n5n[n4][0]
	knight2 := n5n[n4][1]

	empty = 0
	for i := 0; i < 8; i++ {
		if rank[i] == 0 {
			if empty == knight1 {
				rank[i] = 'N'
			}
			empty++
		}
	}

	empty = 0
	for i := 0; i < 8; i++ {
		if rank[i] == 0 {
			if empty == knight2 {
				rank[i] = 'N'
			}
			empty++
		}
	}

	first := true
	for i := 0; i < 8; i++ {
		if rank[i] == 0 {
			if first {
				rank[i] = 'R'
				first = false
			} else {
				rank[i] = 'K'
				for j := i + 1; j < 8; j++ {
					if rank[j] == 0 {
						rank[j] = 'R'
						break
					}
				}
				break
			}
		}
	}
	return rank
}

// generateDFRCFEN creates a random Double Fischer Random Chess (DFRC) starting position.
// There are 960 x 960 = 921,600 unique DFRC combinations.
func generateDFRCFEN(rng *rand.Rand) string {
	whiteRank := frcBackrank(rng.Intn(960))
	blackRank := frcBackrank(rng.Intn(960))

	var fen [64]byte
	idx := 0

	// Black backrank (lowercase)
	for i := 0; i < 8; i++ {
		c := blackRank[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		fen[idx] = c
		idx++
	}

	copy(fen[idx:], "/pppppppp/8/8/8/8/PPPPPPPP/")
	idx += len("/pppppppp/8/8/8/8/PPPPPPPP/")

	// White backrank (uppercase)
	for i := 0; i < 8; i++ {
		fen[idx] = whiteRank[i]
		idx++
	}

	copy(fen[idx:], " w KQkq - 0 1")
	idx += len(" w KQkq - 0 1")

	return string(fen[:idx])
}

// bookIndex holds the raw book file in a single contiguous byte buffer
// and an index of [start, end] byte offsets for each non-empty trimmed line.
// This avoids allocating millions of individual string objects (and their
// GC metadata) on large opening books.
type bookIndex struct {
	data    []byte
	offsets []uint32 // [start0, end0, start1, end1, ...]
}

func isASCIISpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\r' || b == '\n'
}

func indexBookLines(data []byte) []uint32 {
	offsets := make([]uint32, 0, 2*(len(data)/64+1))
	lineStart := 0
	for i := 0; i <= len(data); i++ {
		if i == len(data) || data[i] == '\n' {
			s, e := lineStart, i
			for s < e && isASCIISpace(data[s]) {
				s++
			}
			for e > s && isASCIISpace(data[e-1]) {
				e--
			}
			if e > s {
				offsets = append(offsets, uint32(s), uint32(e))
			}
			lineStart = i + 1
		}
	}
	return offsets
}

func loadBookIndex(path string) (*bookIndex, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return &bookIndex{data: data, offsets: indexBookLines(data)}, nil
}

func (bi *bookIndex) numLines() int {
	if bi == nil {
		return 0
	}
	return len(bi.offsets) / 2
}

func (bi *bookIndex) line(i int) string {
	s, e := bi.offsets[2*i], bi.offsets[2*i+1]
	return string(bi.data[s:e])
}

func runDatagen(targetPositions, threads, nodesPerMove int, bookFile string) {
	fmt.Printf("Starting datagen: %d target positions, %d threads, %d nodes/move\n", targetPositions, threads, nodesPerMove)
	datagenMode = true

	// datagen is a batch process, not the UCI engine: disable the soft memory limit
	// so the GC isn't driven into continuous collection loops across millions of positions.
	disableMemoryLimit()

	var book *bookIndex
	if bookFile != "" {
		b, err := loadBookIndex(bookFile)
		if err != nil {
			fmt.Printf("Warning: could not open book file %s, using pure DFRC\n", bookFile)
		} else {
			book = b
			fmt.Printf("Loaded %d positions from book %s (compact buffer)\n", book.numLines(), bookFile)
		}
	} else {
		fmt.Println("Using pure DFRC (Double Fischer Random Chess, 921,600 opening combinations)")
	}

	baseTimestamp := time.Now().UnixNano()
	var totalPositions int64 = 0
	var totalGames int64 = 0
	var wg sync.WaitGroup
	startTime := time.Now()

	fmt.Printf("Writing lock-free viriformat binaries per thread (prefix: data_%d_t*.vf)\n", baseTimestamp)

	done := make(chan bool)
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				p := atomic.LoadInt64(&totalPositions)
				g := atomic.LoadInt64(&totalGames)
				elapsed := time.Since(startTime).Seconds()
				posPerSec := float64(p) / elapsed
				if elapsed == 0 {
					posPerSec = 0
				}
				fmt.Printf("Progress: %d / %d positions, %d games, %.0f pos/sec\n", p, targetPositions, g, posPerSec)
			}
		}
	}()

	for i := 0; i < threads; i++ {
		wg.Add(1)
		go func(threadID int) {
			defer wg.Done()
			threadFilename := fmt.Sprintf("data_%d_t%d.vf", baseTimestamp, threadID)
			file, err := os.Create(threadFilename)
			if err != nil {
				fmt.Printf("Error creating thread file %s: %v\n", threadFilename, err)
				return
			}
			defer file.Close()

			writer := bufio.NewWriterSize(file, 64*1024)
			defer writer.Flush()

			rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(threadID)))
			ss := new(SearchState)
			ss.tt = new(TTable)
			ss.tt.alloc(1)
			ss.isUsingNNUE = nnue.Loaded && singleOptionValue[NnuePerc] > 0

			var vb ViriBuffer // Reused across games to eliminate per-game heap allocations

			for {
				if atomic.LoadInt64(&totalPositions) >= int64(targetPositions) {
					return
				}

				posCount, played := dgPlayGame(rng, ss, &vb, nodesPerMove, book)
				if !played || posCount == 0 {
					continue
				}

				atomic.AddInt64(&totalPositions, int64(posCount))
				atomic.AddInt64(&totalGames, 1)

				writer.Write(vb.buf)
			}
		}(i)
	}

	wg.Wait()
	close(done)
	fmt.Printf("Datagen complete. Total games: %d, Total positions: %d\n", atomic.LoadInt64(&totalGames), atomic.LoadInt64(&totalPositions))
}

func dgPlayGame(rng *rand.Rand, ss *SearchState, vb *ViriBuffer, nodesPerMove int, book *bookIndex) (int, bool) {
	var p Pos

	numRandom := 0
	if book != nil && book.numLines() > 0 {
		fen := book.line(rng.Intn(book.numLines()))
		parseFEN(&p, fen)
		numRandom = rng.Intn(10) // 0 to 9 random moves from book pos
	} else {
		// Pure DFRC mode: 50% standard startpos, 50% random DFRC (921,600 positions)
		if rng.Intn(2) == 0 {
			parseFEN(&p, startFEN)
		} else {
			parseFEN(&p, generateDFRCFEN(rng))
		}
		numRandom = 8 + rng.Intn(3) // 8 to 10 random moves
	}

	for i := 0; i < numRandom; i++ {
		var list [maxMoves]int
		capCount := genCaptures(&p, list[:])
		quietCount := genQuiet(&p, list[capCount:])
		total := capCount + quietCount

		var legals [maxMoves]int
		legalCount := 0
		for j := 0; j < total; j++ {
			move := list[j]
			var u Update
			var r Revert
			makeMove(&p, &u, &r, move)
			legal := !p.selfInCheck()
			unmakeMove(&p, &u, &r)
			if legal {
				legals[legalCount] = move
				legalCount++
			}
		}
		if legalCount == 0 {
			return 0, false
		}
		move := legals[rng.Intn(legalCount)]
		var u Update
		var r Revert
		makeMove(&p, &u, &r, move)
		if p.clock == 0 {
			p.histLen = 0
		}
	}

	// New game: clear TT entries and reset Finny cache
	ss.tt.clear()
	ss.finny = [2][2][NNUEInputBuckets]FinnyEntry{}
	refresh(&p, &ss.accStack[0])

	vb.Reset()
	vb.WriteBoard(&p, p.clock, 1)

	var numEntries int
	drawCount := 0
	resignCount := 0
	var result float64 = 0.5 // 0.5 for draw, 1.0 for White win, 0.0 for Black win

	for ply := 0; ply < 512; ply++ {
		if p.isInsufficientMaterial() || p.clock >= 100 || isRepetitionDG(&p) {
			result = 0.5
			break
		}

		bestMove, score := runDatagenSearch(&p, ss, nodesPerMove)
		if bestMove == 0 {
			if p.inCheck() {
				if p.side == White {
					result = 0.0 // Black wins
				} else {
					result = 1.0 // White wins
				}
			} else {
				result = 0.5 // Stalemate
			}
			break
		}

		whiteScore := score
		if p.side == Black {
			whiteScore = -score
		}

		absScore := score
		if absScore < 0 {
			absScore = -absScore
		}

		// Initial position verification filter (skip blundered/unbalanced random openings)
		if ply == 0 && absScore > 400 {
			return 0, false
		}

		vb.WriteMoveEval(bestMove, whiteScore)
		numEntries++

		if absScore < 30000 {
			// Draw adjudication: if score remains within [-30, 30] cp for 8 half-moves after ply 40
			if absScore <= 30 {
				drawCount++
			} else {
				drawCount = 0
			}
			if drawCount >= 8 && ply >= 40 {
				result = 0.5
				break
			}

			// Resignation / overwhelming lead adjudication (|score| >= 650 cp for 5 half-moves)
			if absScore >= 650 {
				resignCount++
				if resignCount >= 5 {
					if score > 0 {
						if p.side == White {
							result = 1.0
						} else {
							result = 0.0
						}
					} else {
						if p.side == White {
							result = 0.0
						} else {
							result = 1.0
						}
					}
					break
				}
			} else {
				resignCount = 0
			}
		}

		var u Update
		var r Revert
		makeMove(&p, &u, &r, bestMove)
		if p.clock == 0 {
			p.histLen = 0
		}
		if ss.isUsingNNUE {
			ss.accStack[0].applyPendingChanges(&ss.accStack[0], &p, &u, ss)
		}
	}

	if numEntries == 0 {
		return 0, false
	}

	wdl := 1
	switch result {
	case 1.0:
		wdl = 2
	case 0.0:
		wdl = 0
	}
	vb.PatchWDL(wdl)

	return numEntries, true
}

func isRepetitionDG(p *Pos) bool {
	end := p.histLen - p.clock
	if end < 0 {
		end = 0
	}
	for i := p.histLen - 2; i >= end; i -= 2 {
		if p.key == p.keyHist[i] {
			return true
		}
	}
	return false
}

func runDatagenSearch(p *Pos, ss *SearchState, softNodesLimit int) (int, int) {
	ss.tt.newDate()
	ss.resetForSearch(p)
	ss.nodesLimit = int64(softNodesLimit)

	var pv [maxPly]int
	score := 0
	bestMove := 0

	for d := 1; d < 100; d++ {
		var iterScore int

		if d < 5 {
			iterScore = ss.search(p, 0, -inf, inf, d, false, pv[:], false)
		} else {
			delta := 25 + score*score/16384
			alpha := max(-inf, score-delta)
			beta := min(inf, score+delta)

			for {
				iterScore = ss.search(p, 0, alpha, beta, d, false, pv[:], false)
				if ss.isAbortingSearch() {
					break
				}
				if iterScore <= alpha {
					beta = (alpha + beta) / 2
					alpha = max(-inf, alpha-delta)
				} else if iterScore >= beta {
					beta = min(inf, beta+delta)
				} else {
					break
				}
				delta += delta / 2
			}
		}

		// If we hit the hard limit mid-search, break and retain previous completed iteration
		if ss.isAbortingSearch() {
			break
		}

		if pv[0] != 0 {
			score = iterScore
			bestMove = pv[0]
		}

		// SOFT nodes limit checked AFTER an iteration finishes
		if ss.nodes >= int64(softNodesLimit) {
			break
		}
	}

	// Reset nodesLimit to 0
	ss.nodesLimit = 0

	return bestMove, score
}
