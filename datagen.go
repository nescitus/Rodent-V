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

var datagenMode bool = false

// bookIndex holds an opening-book file as one raw byte buffer plus a flat
// table of per-line [start,end) offsets, instead of one string per line.
// The reference UHO book is 2.6M lines; holding that as []string would
// leave 2.6M separate pointers for the GC to rescan on every cycle for the
// life of the run. offsets has no pointers at all, so only two allocations
// (data and offsets) are ever live regardless of book size.
type bookIndex struct {
	data    []byte
	offsets []uint32 // pairs: offsets[2*i], offsets[2*i+1] = start, end of line i
}

func isASCIISpace(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r', '\v', '\f':
		return true
	}
	return false
}

// indexBookLines finds the trimmed [start,end) byte range of every
// non-blank line in data, matching bufio.Scanner + strings.TrimSpace +
// skip-empty semantics without allocating a string per line.
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

	// datagen is a batch process, not the UCI engine: the soft memory limit
	// in updateMemoryLimit is sized for one global hash table and badly
	// undercounts N private per-thread tables plus a multi-hundred-MB
	// opening book, which previously drove the GC into near-continuous
	// collection. A batch run doesn't need a memory ceiling, so disable it
	// and let the default pacer manage the (now much smaller) live set.
	disableMemoryLimit()

	var book *bookIndex
	if bookFile != "" {
		b, err := loadBookIndex(bookFile)
		if err != nil {
			fmt.Printf("Warning: could not open book file %s, using startpos\n", bookFile)
		} else {
			book = b
			fmt.Printf("Loaded %d positions from book %s\n", book.numLines(), bookFile)
		}
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
			// Private eval/pawn hash tables, same sizing as the shared
			// global ones (eval.go's init), so 16 threads don't race on
			// the same 24-byte entries -- see evalHash/pawnHash on
			// SearchState.
			ss.evalHash = newEvalHashTable(128 * 128)
			ss.pawnHash = newPawnHashTable(64 * 128)

			var vb ViriBuffer // reused across games; Reset() keeps its backing array

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
	if book.numLines() > 0 {
		fen := book.line(rng.Intn(book.numLines()))
		parseFEN(&p, fen)
		numRandom = rng.Intn(10) // 0 to 9 random moves from book pos
	} else {
		parseFEN(&p, startFEN)
		numRandom = 8 + rng.Intn(3) // 8 to 10 random moves from start pos
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

	// New game: age out the previous game's TT entries so replacement
	// isn't purely depth-preferent (see runDatagenSearch for the per-move
	// aging that keeps this working within a game).
	ss.tt.clear()

	// Make sure NNUE is ready
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
		vb.WriteMoveEval(bestMove, whiteScore)
		numEntries++

		absScore := score
		if absScore < 0 {
			absScore = -absScore
		}

		if absScore < 30000 {
			// Draw adjudication
			if absScore <= 10 {
				drawCount++
			} else {
				drawCount = 0
			}
			if drawCount >= 10 && ply >= 40 {
				result = 0.5
				break
			}

			// Resignation / overwhelming lead adjudication
			if absScore > 1500 {
				resignCount++
				if resignCount >= 3 {
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
	// Age the TT for this move. datagen never calls think(), which is the
	// only other caller of newDate(); without this, t.date stays 0 for the
	// whole run and the replacement policy degenerates to pure
	// depth-preference, never evicting stale entries from earlier moves.
	ss.tt.newDate()
	if ss.resetForSearch(p) {
		ss.clearSearchHashes()
	}

	// Capped soft limit: cap search at 1.5x - 2x soft limit to prevent pathological node explosions
	hardLimit := int64(softNodesLimit) * 3 / 2
	if hardLimit < 7500 {
		hardLimit = 7500
	}
	ss.nodesLimit = hardLimit

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
