// ================================================================
// SEARCH ROOT
// ================================================================
//
// This file contains functions handling the beginning of the search.
// It deals with aspiration windows, stopping search due to reaching
// soft time limit, displaying alternative lines in multi-pv mode,
// launching multiple threads and finally choosing the best move in
// multi-threaded mode by the means of thread voting.

package main

import (
	"fmt"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// think is the top-level search entry point called from the UCI loop.
// It performs iterative deepening from depth 1 to maxDepth, outputting
// UCI info lines and finally "bestmove" when done or time expires.
//
// From depth 5 onward each iteration is searched inside an aspiration window
// centred on the previous score.  The initial delta scales with the score
// magnitude so volatile (unbalanced) positions get a wider starting window.
// On fail-low beta is first collapsed to the midpoint before alpha widens,
// avoiding a needlessly large high-side window.  The delta grows by 50% on
// each failure (smoother than doubling) until the window opens fully.
func think(p *Pos, states []*SearchState, maxDepth int) {
	engineSide = p.side
	configureEngineStrength()
	atomic.StoreInt32(&abortFlag, 0)
	ss := states[0]
	ss.tt.newDate()
	ss.resetForSearch(p)
	refresh(p, &ss.accStack[0])

	// Emit info about node limit
	if singleOptionValue[NodesLimit] > 0 {
		fmt.Println("info string search limited to ", singleOptionValue[NodesLimit], " nodes")
	}

	// Launch lazy SMP helper threads (depth 1..INF until abortFlag fires).
	var wg sync.WaitGroup
	for i := 1; i < numThreads && i < len(states); i++ {
		h := states[i]
		if h == nil {
			h = new(SearchState)
			h.tt = &mainTT
			states[i] = h
		}
		h.resetForSearch(p)
		refresh(p, &h.accStack[0])
		h.searchStart = ss.searchStart // helpers share the same clock origin
		pCopy := *p
		threadID := i
		wg.Add(1)
		go func(hs *SearchState, pos Pos, id int) {
			defer wg.Done()
			var pv [maxPly]int
			score := 0
			startDepth := 1 + (id % 2)

			for d := startDepth; atomic.LoadInt32(&abortFlag) == 0 && d <= maxDepth; d++ {
				var iterScore int
				if d < 5 {
					iterScore = hs.search(&pos, 0, -inf, inf, d, false, pv[:], false)
				} else {
					delta := 25 + score*score/16384
					alpha := max(-inf, score-delta)
					beta := min(inf, score+delta)

					for {
						iterScore = hs.search(&pos, 0, alpha, beta, d, false, pv[:], false)
						if hs.isAbortingSearch() {
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

				if hs.isAbortingSearch() {
					break
				}

				if pv[0] != 0 {
					score = iterScore
					hs.completedDepth = d
					hs.completedScore = score
					hs.completedMove = pv[0]
				}
			}
		}(h, pCopy, threadID)
	}

	numPV := singleOptionValue[MultiPV]
	if numPV < 1 {
		numPV = 1
	}
	ss.multiPVCount = numPV
	pvs := make([][maxPly]int, numPV)
	scores := make([]int, numPV)

	// Scratch buffers for rankPVsByScore, allocated once and reused every
	// depth to avoid per-iteration garbage; unused (and left nil) in the
	// default single-PV case, where there is nothing to rank.
	var rankOrder []int
	var rankPVsScratch [][maxPly]int
	var rankScoresScratch []int
	if numPV > 1 {
		rankOrder = make([]int, numPV)
		rankPVsScratch = make([][maxPly]int, numPV)
		rankScoresScratch = make([]int, numPV)
	}

	var lastBestMove int
	var bestMoveStability int

	for rootDepth = 1; rootDepth <= maxDepth; rootDepth++ {
		// Before starting a new depth, check elapsed time against dynamic soft time limit.
		if rootDepth > 1 && !pondering && hardTimeLimit >= 0 {
			elapsed := time.Now().UnixMilli() - ss.searchStart
			adjustedSoft := softTimeLimit
			if bestMoveStability >= 4 {
				// Stable best move over 4+ consecutive iterations -> reduce soft budget by 25%
				adjustedSoft = softTimeLimit * 75 / 100
			} else if bestMoveStability <= 1 {
				// Unstable best move (changed on previous iteration) -> extend soft budget by 50%
				adjustedSoft = softTimeLimit * 150 / 100
				if adjustedSoft > hardTimeLimit {
					adjustedSoft = hardTimeLimit
				}
			}
			if elapsed >= adjustedSoft || elapsed >= hardTimeLimit {
				break
			}
		}
		ss.excludedRootMoves = ss.excludedRootMoves[:0] // Reset for this depth

		depthPVs := 0
		for pvIdx := 0; pvIdx < numPV; pvIdx++ {
			ss.multiPVIdx = pvIdx + 1
			var iterScore int

			//printMemory(rootDepth)

			if rootDepth < 5 {
				// Aspiration windows are unreliable at shallow depths.
				iterScore = ss.search(p, 0, -inf, inf, rootDepth, false, pvs[pvIdx][:], false)
			} else {
				score := scores[pvIdx]
				// Score-adaptive initial delta: balanced positions get a tight
				// window; large scores widen it to reduce retry churn.
				delta := 25 + score*score/16384
				alpha := max(-inf, score-delta)
				beta := min(inf, score+delta)

				for {
					iterScore = ss.search(p, 0, alpha, beta, rootDepth, false, pvs[pvIdx][:], false)
					if ss.isAbortingSearch() {
						break
					}
					if iterScore <= alpha {
						// Fail low: collapse beta to the midpoint before widening
						// alpha so the high side doesn't grow unnecessarily.
						beta = (alpha + beta) / 2
						alpha = max(-inf, alpha-delta)
					} else if iterScore >= beta {
						// Fail high: widen the window above and retry.
						beta = min(inf, beta+delta)
					} else {
						break // score is inside the window
					}
					delta += delta / 2 // proportional widening (×1.5)
				}
			}

			if ss.isAbortingSearch() {
				break
			}

			// If no legal moves were found (fewer legal moves than requested PVs)
			if pvs[pvIdx][0] == 0 {
				break
			}

			scores[pvIdx] = iterScore
			ss.excludedRootMoves = append(ss.excludedRootMoves, pvs[pvIdx][0])
			depthPVs++
		}

		if ss.isAbortingSearch() {
			break
		}

		// In single-PV mode there is nothing to rank
		if numPV > 1 {
			// Rank the completed lines by score so index 0 is always the best-scoring line.
			rankPVsByScore(pvs, scores, depthPVs, rankOrder, rankPVsScratch, rankScoresScratch)
			// UCI requires all requested MultiPV lines to be sent together.
			for i := 0; i < depthPVs; i++ {
				ss.multiPVIdx = i + 1
				ss.reportInfo(scores[i], pvs[i][:])
			}
		}

		if pvs[0][0] != 0 {
			ss.completedDepth = rootDepth
			ss.completedScore = scores[0]
			ss.completedMove = pvs[0][0]

			if pvs[0][0] == lastBestMove {
				bestMoveStability++
			} else {
				bestMoveStability = 1
				lastBestMove = pvs[0][0]
			}
		}
	}

	// Stop all helpers and wait for them to exit.
	atomic.StoreInt32(&abortFlag, 1)
	wg.Wait()

	bestMove, _, _ := selectBestThreadMove(states, numThreads)
	if bestMove == 0 {
		bestMove = pvs[0][0]
	}

	if bestMove != 0 {
		best := moveToStr(bestMove)
		ponderMove := 0
		if bestMove == pvs[0][0] && pvs[0][1] != 0 {
			ponderMove = pvs[0][1]
		}
		if ponderMove != 0 {
			ponder := moveToStr(ponderMove)
			fmt.Printf("bestmove %s ponder %s\n", best, ponder)
		} else {
			fmt.Printf("bestmove %s\n", best)
		}
	} else {
		fmt.Println("bestmove 0000")
	}
}

// selectBestThreadMove tallies depth-and-score weighted votes across all active threads.
func selectBestThreadMove(states []*SearchState, activeCount int) (int, int, int) {
	if activeCount <= 1 || len(states) == 0 || states[0] == nil {
		if len(states) > 0 && states[0] != nil {
			return states[0].completedMove, states[0].completedScore, states[0].completedDepth
		}
		return 0, 0, 0
	}

	minScore := 999999
	hasValidThread := false
	for i := 0; i < activeCount && i < len(states); i++ {
		st := states[i]
		if st == nil || st.completedDepth == 0 || st.completedMove == 0 {
			continue
		}
		hasValidThread = true
		if st.completedScore < minScore {
			minScore = st.completedScore
		}
	}

	if !hasValidThread {
		return states[0].completedMove, states[0].completedScore, states[0].completedDepth
	}

	type moveVote struct {
		weight int64
		thread int
	}
	votes := make(map[int]moveVote)
	bestMove := states[0].completedMove
	var maxWeight int64 = -1

	for i := 0; i < activeCount && i < len(states); i++ {
		st := states[i]
		if st == nil || st.completedDepth == 0 || st.completedMove == 0 {
			continue
		}

		mv := st.completedMove
		weight := int64(st.completedScore-minScore+50) * int64(st.completedDepth)

		entry := votes[mv]
		entry.weight += weight
		if entry.thread == 0 || st.completedDepth > states[entry.thread].completedDepth {
			entry.thread = i
		}
		votes[mv] = entry

		if entry.weight > maxWeight {
			maxWeight = entry.weight
			bestMove = mv
		}
	}

	bestThread := votes[bestMove].thread
	if bestThread >= 0 && bestThread < len(states) && states[bestThread] != nil {
		return bestMove, states[bestThread].completedScore, states[bestThread].completedDepth
	}
	return bestMove, states[0].completedScore, states[0].completedDepth
}

// rankPVsByScore reorders the first n entries of pvs/scores so they are
// sorted by score in descending order (best line first), preserving the
// relative order of equal scores.
//
// order, pvsScratch, and scoresScratch are caller-owned scratch buffers
// (capacity >= n) reused across calls to avoid per-depth allocation.
func rankPVsByScore(pvs [][maxPly]int, scores []int, n int, order []int, pvsScratch [][maxPly]int, scoresScratch []int) {
	order = order[:n]
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool {
		return scores[order[i]] > scores[order[j]]
	})

	for rank, idx := range order {
		pvsScratch[rank] = pvs[idx]
		scoresScratch[rank] = scores[idx]
	}
	copy(pvs[:n], pvsScratch[:n])
	copy(scores[:n], scoresScratch[:n])
}

// a small test function to print informations about memory usage
func printMemory(depth int) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	fmt.Printf(
		"info string depth %d heap=%dMB sys=%dMB allocs=%d\n",
		depth,
		m.HeapAlloc/(1024*1024),
		m.Sys/(1024*1024),
		m.Mallocs-m.Frees,
	)
}
