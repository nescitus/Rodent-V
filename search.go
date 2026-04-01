// ================================================================
// S11 SEARCH
// ================================================================
//
//   The search is the engine's brain.  Given a position, it finds
//   the best move by recursively exploring the game tree, bounded
//   by alpha-beta pruning.
//
//   ALGORITHM: PRINCIPAL VARIATION SEARCH (PVS)
//   --------------------------------------------
//   PVS is an enhancement of alpha-beta negamax.  The key insight:
//   after finding a move that raises alpha (a "PV move"), every other
//   move can be searched with a zero-width window (alpha, alpha+1)
//   first.  If that search fails high (score > alpha), a re-search
//   with the full window is needed.  In practice, most branches fail
//   the zero-width search immediately, saving significant time.
//
//   ITERATIVE DEEPENING
//   --------------------
//   We don't search directly to the maximum depth.  Instead we start
//   at depth 1 and deepen one ply at a time.  Each shallower search
//   populates the TT with good move hints that guide the deeper
//   search.  It also lets us output progress and stop cleanly when
//   time expires.
//
//   NULL-MOVE PRUNING
//   -----------------
//   If the position is so good that even passing the turn to the
//   opponent (a "null move") at reduced depth still causes a beta
//   cutoff, the branch can be pruned.  The reduction is 3 plies
//   (R=3).  We skip null moves in check (the engine would be in an
//   illegal state) and when only pawns + kings remain (zugzwang risk).
//
//   CHECK EXTENSION
//   ---------------
//   When a move gives check we extend the search by one extra ply.
//   Forcing moves deserve deeper investigation; cutting off a check
//   sequence prematurely can cause catastrophic horizon effect.
//
//   QUIESCENCE SEARCH
//   ------------------
//   At depth 0 we don't immediately return evaluate(). Instead we
//   enter quiesce(), which searches all captures until the position
//   is "quiet" (no captures remain or all are bad).  This prevents
//   the horizon effect where the engine stops just before a piece is
//   taken.
//
//   REPETITION DRAW
//   ---------------
//   The engine detects 3-fold repetition by checking the Zobrist key
//   against keyHist.  We look back in steps of 2 (same side to move)
//   up to clock moves (the 50-move clock resets on irreversible moves,
//   so only those positions can repeat).
//

package main

import (
	"fmt"
	"math"
	"sync/atomic"
	"time"
)

// Global search state.
var (
	timeLimit   int64  // allocated move time in ms (-1 = unlimited)
	pondering   bool   // true while in ponder mode (ignore clock)
	rootDepth   int    // current iterative deepening depth
	nodes       int64  // total nodes searched (search-goroutine only; no atomic needed)
	abortFlag   int32  // set to 1 atomically to stop the search
	searchStart int64  // Unix ms at the start of think()
)

var lmr [64][64]int

func init() {
	initLMR()
}

func initLMR() {
	for d := 0; d < 64; d++ {
		for m := 0; m < 64; m++ {
			if d < 3 || m < 4 {
				lmr[d][m] = 0
				continue
			}

			r := math.Log(float64(d)) * math.Log(float64(m)) / 1.8
			ri := int(r + 0.5) // round to nearest

			if ri < 1 {
				ri = 1
			} else if ri > 5 {
				ri = 5
			}

			lmr[d][m] = ri
		}
	}
}

// think is the top-level search entry point called from the UCI loop.
// It performs iterative deepening from depth 1 to maxDepth, outputting
// UCI info lines and finally "bestmove" when done or time expires.
func think(p *Pos, maxDepth int) {
	clearHistory()
	ttDate = (ttDate + 1) & 255
	nodes = 0
	atomic.StoreInt32(&abortFlag, 0)
	searchStart = time.Now().UnixMilli()

	var pv [maxPly]int
	for rootDepth = 1; rootDepth <= maxDepth; rootDepth++ {
		search(p, 0, -inf, inf, rootDepth, pv[:])
		if atomic.LoadInt32(&abortFlag) != 0 {
			break
		}
	}

	if pv[0] != 0 {
		best := moveToStr(pv[0])
		if pv[1] != 0 {
			ponder := moveToStr(pv[1])
			fmt.Printf("bestmove %s ponder %s\n", best, ponder)
		} else {
			fmt.Printf("bestmove %s\n", best)
		}
	} else {
		fmt.Println("bestmove 0000")
	}
}

// search is the recursive alpha-beta negamax function.
//
// Parameters:
//   p: current position
//   ply: distance from the root (0 = root)
//   alpha: lower bound on the score we need to improve our line
//   beta: upper bound; a score >= beta causes a cutoff
//   depth: remaining plies to search
//   pv: principal variation output buffer
//
// Returns the score for the side to move (negamax convention).
func search(p *Pos, ply, alpha, beta, depth int, pv []int) int {
	if depth <= 0 {
		return quiesce(p, ply, alpha, beta, pv)
	}

	nodes++
	checkTime()
	if atomic.LoadInt32(&abortFlag) != 0 {
		return 0
	}

	isRoot := (ply == 0)

	if !isRoot {
		pv[0] = 0
	}
	// A position repeated from earlier in the game tree is a draw.
	if !isRoot && isRepetition(p) {
		return 0
	}

	// Are we in the pv node?
	isPv := (beta > alpha + 1)
	movesTried := 0

	// --- Transposition table probe ---
	ttMove := 0
	ttFlag := 0
	score := 0
	if probeTT(p.key, &ttMove, &score, &ttFlag, alpha, beta, depth, ply) {
		if ttFlag == EXACT || !isPv {
			return score
		}
	}

	// Safeguard against hitting max ply limit
	if ply >= maxPly-1 {
		return evaluate(p)
	}

	// Are we in check in this node?
	nodeInCheck := p.inCheck()

	// --- Null-move pruning ---
	// Skip if: depth <= 1 (too shallow to be reliable), the position
	// is already beyond beta, we are in check, or only pawns remain.
	if depth > 1 && !isPv && !nodeInCheck && p.canNullMove() && beta <= evaluate(p) {
		var u Undo
		makeNullMove(p, &u)
		var nullPv [maxPly]int
		score = -search(p, ply+1, -beta, -beta+1, depth-3, nullPv[:])
		unmakeNullMove(p, &u)
		if atomic.LoadInt32(&abortFlag) != 0 {
			return 0
		}
		if score >= beta {
			return score
		}
	}

	var bestMove int;

	// --- Main move loop ---
	best := -inf
	picker := &moveBuffers[ply]
	initMovePicker(p, picker, ttMove, ply)
	var childPv [maxPly]int

	for {
		move, stage := picker.nextMove()
		if move == 0 {
			break
		}

		var u Undo
		makeMove(p, move, &u)
		if p.selfInCheck() { // move left our king in check: illegal
			unmakeMove(p, move, &u)
			continue
		}

		movesTried++

		// Extend by one ply for moves that give check.
		newDepth := depth - 1
		if p.inCheck() {
			newDepth++
		}

		// Late move reduction
		isReduced := false
		if stage == StageQuiet && depth > 2 && !nodeInCheck && !p.inCheck() && movesTried > 3 {

			reduction := lmr[min(depth, 63)][min(movesTried, 63)]

			if (reduction > 0) {
				if (!isPv) {
					reduction++
				}
				if (reduction > newDepth-1) {
					reduction = newDepth - 1
				}
				score = -search(p, ply+1, -alpha-1, -alpha, newDepth-reduction, childPv[:])
				if score <= alpha {
					isReduced = true
				}
			}
		}

		// Principal Variation Search:
		// First move: full window.
		// Subsequent moves: zero-width window first; re-search if it fails high.
		if !isReduced {
			if best == -inf {
				score = -search(p, ply+1, -beta, -alpha, newDepth, childPv[:])
			} else {
				score = -search(p, ply+1, -alpha-1, -alpha, newDepth, childPv[:])
				if score > alpha && score < beta {
					score = -search(p, ply+1, -beta, -alpha, newDepth, childPv[:])
				}
			}
		}
		unmakeMove(p, move, &u)

		if atomic.LoadInt32(&abortFlag) != 0 {
			return 0
		}

		// Beta cutoff: this move is "too good"; the opponent won't allow
		// reaching this position, so we can stop searching.
		if score >= beta {
			updateHistory(p, move, depth, ply)
			storeTT(p.key, move, score, LOWER, depth, ply)
			return score
		}

		if score > best {
			best = score
			if score > alpha {
				alpha = score
				bestMove = move
				buildPV(pv, childPv[:], move)
				if isRoot {
					reportInfo(score, pv)
				}
			}
		}
	}

	// --- Handle terminal nodes ---
	if best == -inf {
		if p.inCheck() {
			return -mate + ply // checkmate: prefer shorter mates
		}
		return 0 // stalemate
	}

	// Store the result in the TT with the appropriate bound type.
	if bestMove != 0 {
		updateHistory(p, pv[0], depth, ply)
		storeTT(p.key, pv[0], best, EXACT, depth, ply)
	} else {
		storeTT(p.key, 0, best, UPPER, depth, ply)
	}
	return best
}

// quiesce searches only captures until the position is quiet, then
// returns the static evaluation.  This prevents the "horizon effect"
// where the engine ignores an imminent capture at the leaf.
//
// The "stand-pat" score is evaluate(). If it already exceeds beta,
// we assume we can stand pat (decline all captures) and cut off.
func quiesce(p *Pos, ply, alpha, beta int, pv []int) int {
	nodes++
	checkTime()
	if atomic.LoadInt32(&abortFlag) != 0 {
		return 0
	}

	pv[0] = 0

	// Repetition detection
	if isRepetition(p) {
		return 0
	}

	// Safeguard against reaching max ply limit
	if ply >= maxPly-1 {
		return evaluate(p)
	}

	// Stand-pat evaluation: if we're already beating beta, stop.
	best := evaluate(p)
	if best >= beta {
		return best
	}
	if best > alpha {
		alpha = best
	}

	picker := &moveBuffers[ply]
	initQSearch(p, picker)
	var childPv [maxPly]int

	for {
		move := picker.nextCapture()
		if move == 0 {
			break
		}

		if isBadCapture(p, move) {
			continue
		}

		var u Undo
		makeMove(p, move, &u)
		if p.selfInCheck() {
			unmakeMove(p, move, &u)
			continue
		}
		score := -quiesce(p, ply+1, -beta, -alpha, childPv[:])
		unmakeMove(p, move, &u)

		if atomic.LoadInt32(&abortFlag) != 0 {
			return 0
		}
		if score >= beta {
			return score
		}
		if score > best {
			best = score
			if score > alpha {
				alpha = score
				buildPV(pv, childPv[:], move)
			}
		}
	}
	return best
}

// isRepetition returns true if the current position has appeared at
// least once before in the game history (2-fold repetition).
// We step back in increments of 2 (same side to move) and compare
// Zobrist keys; the 50-move clock limits how far back we look.
func isRepetition(p *Pos) bool {
	for i := 4; i <= p.clock; i += 2 {
		if p.key == p.keyHist[p.histLen-i] {
			return true
		}
	}
	return false
}

// reportInfo outputs a UCI "info" line for the current iteration.
// Mate scores are converted to "moves to mate" format.
func reportInfo(score int, pv []int) {
	// If we are approaching checkmate for either side, calculate
	// distance to checkmate; in the normal scenario, switch to
	// centipawns.
	scoreType := "mate"
	if score < -maxEval {
		score = (-mate - score) / 2
	} else if score > maxEval {
		score = (mate - score + 1) / 2
	} else {
		scoreType = "cp"
	}

	// Set elapsed (time used so far), and guard
	// against division by zero in nps calculation
	elapsed := time.Now().UnixMilli() - searchStart
	if elapsed <= 0 {
		elapsed = 1
	}

	// Calculate nodes per second
	nps := nodes * 1000 / elapsed

	// Output
	fmt.Printf("info depth %d time %d nodes %d nps %d score %s %d pv %s\n",
		rootDepth, elapsed, nodes, nps, scoreType, score, pvString(pv))
}

// checkTime is called periodically (every 4096 nodes) to see whether
// the allocated time has expired.  If so, it sets abortFlag so the
// search unwinds cleanly.  Skipped during depth-1 searches (the
// engine must always return at least one move) and when pondering.
func checkTime() {
	if nodes&4095 != 0 || rootDepth == 1 {
		return
	}
	if !pondering && timeLimit >= 0 {
		if time.Now().UnixMilli()-searchStart >= timeLimit {
			atomic.StoreInt32(&abortFlag, 1)
		}
	}
}
