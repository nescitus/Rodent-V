package main

import (
	"bufio"
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"strings"
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

func runDatagen(targetPositions, threads, nodesPerMove int, bookFile string) {
	fmt.Printf("Starting datagen: %d target positions, %d threads, %d nodes/move\n", targetPositions, threads, nodesPerMove)
	allocTT(16)
	datagenMode = true

	var bookFENs []string
	if bookFile != "" {
		bf, err := os.Open(bookFile)
		if err == nil {
			scanner := bufio.NewScanner(bf)
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line != "" {
					bookFENs = append(bookFENs, line)
				}
			}
			bf.Close()
			fmt.Printf("Loaded %d positions from book %s\n", len(bookFENs), bookFile)
		} else {
			fmt.Printf("Warning: could not open book file %s, using startpos\n", bookFile)
		}
	}

	filename := fmt.Sprintf("data_%d.vf", time.Now().UnixNano())
	file, err := os.Create(filename)
	if err != nil {
		fmt.Println("Error creating file:", err)
		return
	}
	defer file.Close()

	var totalPositions int64 = 0
	var totalGames int64 = 0
	var fileMutex sync.Mutex
	var wg sync.WaitGroup
	startTime := time.Now()

	fmt.Printf("Writing viriformat binary to %s\n", filename)

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
			rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(threadID)))
			ss := new(SearchState)
			ss.tt = new(TTable)
			ss.tt.alloc(1)
			ss.isUsingNNUE = nnue.Loaded && singleOptionValue[NnuePerc] > 0

			for {
				if atomic.LoadInt64(&totalPositions) >= int64(targetPositions) {
					return
				}

				vb, posCount, played := dgPlayGame(rng, ss, nodesPerMove, bookFENs)
				if !played || posCount == 0 {
					continue
				}

				atomic.AddInt64(&totalPositions, int64(posCount))
				atomic.AddInt64(&totalGames, 1)

				fileMutex.Lock()
				file.Write(vb.buf)
				fileMutex.Unlock()
			}
		}(i)
	}

	wg.Wait()
	close(done)
	fmt.Printf("Datagen complete. Total games: %d, Total positions: %d\n", atomic.LoadInt64(&totalGames), atomic.LoadInt64(&totalPositions))
}

func dgPlayGame(rng *rand.Rand, ss *SearchState, nodesPerMove int, bookFENs []string) (ViriBuffer, int, bool) {
	var p Pos

	numRandom := 0
	if len(bookFENs) > 0 {
		fen := bookFENs[rng.Intn(len(bookFENs))]
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

		var legals []int
		for j := 0; j < total; j++ {
			move := list[j]
			var child Pos = p
			var u Update
			var r Revert
			makeMove(&child, &u, &r, move)
			if !child.selfInCheck() {
				legals = append(legals, move)
			}
		}
		if len(legals) == 0 {
			return ViriBuffer{}, 0, false
		}
		move := legals[rng.Intn(len(legals))]
		var u Update
		var r Revert
		makeMove(&p, &u, &r, move)
		if p.clock == 0 {
			p.histLen = 0
		}
	}

	// Make sure NNUE is ready
	refresh(&p, &ss.accStack[0])

	var vb ViriBuffer
	vb.WriteBoard(&p, p.clock, 1)

	var numEntries int
	drawCount := 0
	var result float64 = 0.5 // 0.5 for draw, 1.0 for White win, 0.0 for Black win

	for ply := 0; ply < 512; ply++ {
		var list [maxMoves]int
		capCount := genCaptures(&p, list[:])
		quietCount := genQuiet(&p, list[capCount:])
		total := capCount + quietCount

		hasLegal := false
		for j := 0; j < total; j++ {
			move := list[j]
			var child Pos = p
			var u Update
			var r Revert
			makeMove(&child, &u, &r, move)
			if !child.selfInCheck() {
				hasLegal = true
				break
			}
		}

		if !hasLegal {
			if p.inCheck() {
				if p.side == White {
					result = 0.0 // Black wins
				} else {
					result = 1.0 // White wins
				}
			}
			break
		}

		if p.isInsufficientMaterial() || p.clock >= 100 || isRepetitionDG(&p) {
			result = 0.5
			break
		}

		bestMove, score := runDatagenSearch(&p, ss, nodesPerMove)
		if bestMove == 0 {
			break
		}

		whiteScore := score
		if p.side == Black {
			whiteScore = -score
		}
		vb.WriteMoveEval(bestMove, whiteScore)
		numEntries++

		if score > -30000 && score < 30000 {
			if score > -10 && score < 10 {
				drawCount++
			} else {
				drawCount = 0
			}
			if drawCount >= 10 && ply >= 40 {
				result = 0.5
				break
			}
		}

		var u Update
		var r Revert
		makeMove(&p, &u, &r, bestMove)
		if p.clock == 0 {
			p.histLen = 0
		}
		refresh(&p, &ss.accStack[0])
	}

	if numEntries == 0 {
		return vb, 0, false
	}

	wdl := 1
	switch result {
	case 1.0:
		wdl = 2
	case 0.0:
		wdl = 0
	}
	vb.PatchWDL(wdl)

	return vb, numEntries, true
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
	ss.resetForSearch(p)

	// HARD nodes limit checked mid-search (8 million nodes)
	ss.nodesLimit = 8000000
	refresh(p, &ss.accStack[0])

	var pv [maxPly]int
	score := 0
	bestMove := 0

	for d := 1; d < 100; d++ {
		ss.rootDepth = d
		iterScore := ss.search(p, 0, -inf, inf, d, false, pv[:], false)

		// If we hit the 8M hard limit mid-search, we discard this depth's result
		if ss.isAbortingSearch() {
			break
		}

		if pv[0] != 0 {
			score = iterScore
			bestMove = pv[0]
		}

		// SOFT nodes limit checked only AFTER an iteration finishes
		if ss.nodes >= int64(softNodesLimit) {
			break
		}
	}

	// Reset the nodes limit to 0 so the subsequent quiesce filter doesn't instantly abort
	ss.nodesLimit = 0

	return bestMove, score
}

func countLines(filename string) (int64, error) {
	file, err := os.Open(filename)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	var count int64
	buf := make([]byte, 32*1024)
	for {
		c, err := file.Read(buf)
		count += int64(bytes.Count(buf[:c], []byte{'\n'}))
		if err != nil {
			break
		}
	}
	return count, nil
}
