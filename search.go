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
//   The re-search is triggered whenever score > alpha, with no upper
//   beta guard.
//
//   ITERATIVE DEEPENING + ASPIRATION WINDOWS
//   ------------------------------------------
//   We don't search directly to the maximum depth.  Instead we start
//   at depth 1 and deepen one ply at a time.  Each shallower search
//   populates the TT with good move hints that guide the deeper
//   search.  It also lets us output progress and stop cleanly when
//   time expires.  From depth 4 onward each iteration is searched
//   inside a narrow aspiration window centred on the previous score;
//   on a fail-low or fail-high the window doubles and the search is
//   retried.  This generates many more cutoffs at the root without
//   risking correctness.
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
//   taken.  When entered while in check, stand-pat is illegal and the
//   full move pipeline is used so quiet evasions are included; no
//   legal move means checkmate.
//
//   REVERSE FUTILITY PRUNING (RFP)
//   --------------------------------
//   Also called "static null-move pruning".  If the static evaluation
//   already exceeds beta by a depth-scaled margin, the position is so
//   good that even a pessimistic adjustment won't fall below beta.  We
//   return the static eval immediately without searching further.
//   Applied only at non-PV nodes, when not in check, at shallow depth.
//
//   LATE MOVE PRUNING (LMP)
//   -----------------------
//   At shallow depths (depth < 4) on non-PV nodes, quiet moves beyond
//   the first 4*depth+1 are skipped entirely rather than just reduced.
//   quietTried tracks only quiet moves so the threshold is independent
//   of how many captures were searched first.  Moves that give check
//   are exempt: a quiet check can be the only defensive resource or
//   the only way out of a mating attack.  LMP is also skipped when the
//   node itself is in check, since evasions must be fully searched.
//
//   INTERNAL ITERATIVE REDUCTION (IIR)
//   ------------------------------------
//   When no TT move is available, move ordering will be poor and a
//   full-depth search is likely wasteful.  We reduce depth by one ply
//   instead.  The shallower search populates the TT; the next iterative-
//   deepening iteration will find the hint and search at full depth with
//   good ordering.  Applied at depth >= 4; skipped in check since
//   evasions must always be searched at full depth.
//
//   LATE MOVE REDUCTIONS (LMR)
//   --------------------------
//   Moves tried late in the list (after the killer and good captures)
//   are unlikely to be best.  We search them at reduced depth first;
//   if the reduced search surprisingly raises alpha, we re-search at
//   full depth.  The reduction table lmr[depth][moveIndex] is filled
//   once at startup using the formula log(depth)*log(moveIndex)/1.8,
//   clamped to [1, 5].  An extra ply is added when not in a PV node.
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
	"sync"
	"sync/atomic"
	"time"
)

// Global search state — shared across all threads.
// Per-thread state lives in SearchState (see thread.go).
var (
	timeLimit  int64     // allocated move time in ms (-1 = unlimited); set by UCI
	pondering  bool      // true while in ponder mode (ignore clock)
	rootDepth  int       // current iterative deepening depth (main thread drives)
	abortFlag  int32     // set to 1 atomically to stop all threads
	numThreads int   = 1 // number of search threads (1 = single-threaded)
)

// lmr[depth][moveIndex] holds the ply reduction for a quiet move tried
// at position moveIndex in the move list when depth plies remain.
var lmr [64][64]int

func init() {
	initLMRTable()
}

// initLMRTable pre-computes the reduction for every (depth, moveIndex) pair.
// Entries where depth < 3 or moveIndex < 4 are left at zero (no reduction for
// the first few moves or near the leaves).  All other entries use the
// logarithmic formula log(depth)*log(moveIndex)/1.8, rounded and clamped to
// [1, 5] so reductions stay meaningful but never collapse a search entirely.
func initLMRTable() {
	for depth := 0; depth < 64; depth++ {
		for moveIndex := 0; moveIndex < 64; moveIndex++ {
			if depth < 3 || moveIndex < 4 {
				lmr[depth][moveIndex] = 0
				continue
			}

			raw := math.Log(float64(depth)) * math.Log(float64(moveIndex)) / 1.8
			reduction := int(raw + 0.5) // round to nearest

			if reduction < 1 {
				reduction = 1
			} else if reduction > 5 {
				reduction = 5
			}

			lmr[depth][moveIndex] = reduction
		}
	}
}

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
	ttDate = (ttDate + 1) & 255
	atomic.StoreInt32(&abortFlag, 0)

	ss := states[0]
	ss.resetForSearch(p)

	// Launch lazy SMP helper threads (depth 1..INF until abortFlag fires).
	var wg sync.WaitGroup
	for i := 1; i < numThreads && i < len(states); i++ {
		h := states[i]
		h.clearHistory()
		h.resetForSearch(p)
		h.searchStart = ss.searchStart // helpers share the same clock origin
		pCopy := *p
		wg.Add(1)
		go func(hs *SearchState, pos Pos) {
			defer wg.Done()
			var dummyPV [maxPly]int
			for d := 1; atomic.LoadInt32(&abortFlag) == 0; d++ {
				hs.search(&pos, 0, -inf, inf, d, false, dummyPV[:])
			}
		}(h, pCopy)
	}

	var pv [maxPly]int
	score := 0

	for rootDepth = 1; rootDepth <= maxDepth; rootDepth++ {
		// Before starting a new depth, do an unthrottled time check.
		// Two stopping criteria (either is sufficient):
		//   1. Hard limit: elapsed >= timeLimit (don't start if already over).
		//   2. Half-budget rule: elapsed >= timeLimit/2. The next depth
		//      typically takes longer than all prior depths combined, so
		//      starting it when half the budget is gone almost always busts
		//      the limit. This is the key guard against time forfeits.
		if rootDepth > 1 && !pondering && timeLimit >= 0 {
			elapsed := time.Now().UnixMilli() - ss.searchStart
			if elapsed >= timeLimit || elapsed >= timeLimit/2 {
				break
			}
		}
		var iterScore int

		if rootDepth < 5 {
			// Aspiration windows are unreliable at shallow depths.
			iterScore = ss.search(p, 0, -inf, inf, rootDepth, false, pv[:])
		} else {
			// Score-adaptive initial delta: balanced positions get a tight
			// window; large scores widen it to reduce retry churn.
			delta := 25 + score*score/16384
			alpha := max(-inf, score-delta)
			beta := min(inf, score+delta)

			for {
				iterScore = ss.search(p, 0, alpha, beta, rootDepth, false, pv[:])
				if atomic.LoadInt32(&abortFlag) != 0 {
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

		if atomic.LoadInt32(&abortFlag) != 0 {
			break
		}
		score = iterScore
	}

	// Stop all helpers and wait for them to exit.
	atomic.StoreInt32(&abortFlag, 1)
	wg.Wait()

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
//
//	p: current position
//	ply: distance from the root (0 = root)
//	alpha: lower bound on the score we need to improve our line
//	beta: upper bound; a score >= beta causes a cutoff
//	depth: remaining plies to search
//	pv: principal variation output buffer
//
// Returns the score for the side to move (negamax convention).
func (ss *SearchState) search(p *Pos, ply, alpha, beta, depth int, wasNull bool, pv []int) int {
	if depth <= 0 {
		return ss.quiesceCheck(p, ply, alpha, beta, pv)
	}

	ss.nodes++
	if ply > ss.selDepth {
		ss.selDepth = ply
	}
	ss.checkTime()
	if atomic.LoadInt32(&abortFlag) != 0 {
		return 0
	}

	score := 0
	isRoot := (ply == 0)
	ttMove := 0
	if isRoot {
		ttMove = pv[0]
	}

	if !isRoot {
		pv[0] = 0
		// Check all draw conditions before generating any moves so that
		// pv[0] == 0 is guaranteed when we return, preventing the PV from
		// extending past a drawing position.
		if ss.isRepetition(p) || p.isInsufficientMaterial() || p.clock >= 100 {
			ss.checkTime() // sets abort flag - helps in case of many draws in a row
			return 0
		}
	}

	// Are we in the pv node?
	isPv := (beta > alpha+1)
	movesTried := 0

	// --- Transposition table probe ---
	ttFlag := 0
	ttDepth := 0
	if probeTT(p.key, &ttMove, &score, &ttFlag, &ttDepth, alpha, beta, depth, ply) {
		if !isPv && ss.excludedMove[ply] == 0 {
			return score
		}
	}
	ttScore := score // save TT score before NMP/LMR overwrites the score variable

	// Safeguard against hitting max ply limit
	if ply >= maxPly-1 {
		return evaluate(p)
	}

	// Are we in check in this node?
	nodeInCheck := p.inCheck()

	// Cache the static evaluation; shared by RFP, improving, and null-move pruning.
	// Store in evalStack for the improving heuristic (ply-2 comparison).
	// When in check we store a sentinel so the improving check below works correctly.
	staticEval := 0
	rawEval := 0
	if !nodeInCheck {
		rawEval = evaluate(p)
		correction := ss.getCorrection(p)
		staticEval = rawEval + correction
		ss.evalStack[ply] = staticEval
	} else {
		ss.evalStack[ply] = noEval
	}

	// --- Reverse futility pruning ---
	// If the static eval beats beta by a depth-scaled margin, the position
	// is already so good that a full search is unlikely to fall below beta.
	// The mate guard prevents pruning when beta is a mate score, where the
	// static eval is unreliable.  We return the margin-adjusted value rather
	// than the raw static eval to avoid inflating the returned score.

	// Use a tighter margin when improving (position is gaining ground).
	// Wider margin when not improving avoids over-pruning a deteriorating line.
	rfpDepthMargin := rfpMargin
	// if improving {
	// 	rfpDepthMargin = rfpImpMargin
	// }
	if !isPv && !nodeInCheck && depth <= rfpDepth && beta < mate-maxPly && useRFP &&
	    ss.excludedMove[ply] == 0 &&
		staticEval-rfpDepthMargin*depth >= beta {
		return staticEval - rfpDepthMargin*depth
	}

	// --- Null-move pruning ---
	// Skip if: depth <= 1 (too shallow to be reliable), the position
	// is already beyond beta, we are in check, or only pawns remain.
	// Reduction = base + depth/depthDiv + min((eval-beta)/evalDiv, maxEvalBonus).
	if depth > 1 && !isPv && !nodeInCheck && !wasNull && p.canNullMove() && beta <= staticEval && useNULL && ss.excludedMove[ply] == 0 {
		ss.contValid[ply] = false // null move: no valid piece context for cont hist
		reduction := nmpBaseReduction + depth/nmpDepthReduction
		var u Undo
		makeNullMove(p, &u)
		var nullPv [maxPly]int
		score = -ss.search(p, ply+1, -beta, -beta+1, depth-1-reduction, true, nullPv[:])
		unmakeNullMove(p, &u)
		if atomic.LoadInt32(&abortFlag) != 0 {
			return 0
		}

		if score >= beta {
			return score
		}
	}

	var bestMove int
	origAlpha := alpha

	// --- Main move loop ---
	bestScore := -inf
	picker := &ss.moveBuffers[ply]
	initMovePicker(p, picker, ss, ttMove, ply)
	var childPv [maxPly]int

	quietTried := 0
	// quietsMade tracks quiet moves that were fully searched without causing
	// a beta cutoff.  On a cutoff we apply a malus to all of them.
	var quietsMade [maxMoves]int
	quietsMadeCount := 0

	for {
		move, stage := picker.nextMove()
		if move == 0 {
			break
		}

		// Skip the move excluded during a singular extension search.
		if move == ss.excludedMove[ply] {
			continue
		}

		// --- Singular extension ---
		// When the TT move is reliable and deep enough, check if it's the
		// only good move by searching without it at reduced depth. If all
		// other moves fail below a margin, extend this move by +1 ply.
		// Negative extension: if the position isn't singular AND the TT
		// score already beats beta, reduce aggressively (multi-cut signal).
		extension := 0
		if useSingularExt && move == ttMove &&
			ss.excludedMove[ply] == 0 &&
			depth >= 8 &&
			ttDepth >= depth-3 &&
			ttFlag != UPPER &&
			abs(ttScore) < mate-maxPly {
			sBeta := ttScore - seBetaMargin*depth
			// The SE sub-search calls search(p, ply, ...) which reinitialises
			// moveBuffers[ply], destroying the main loop's picker state.
			// Save and restore the picker around the sub-search.
			savedPicker := *picker
			ss.excludedMove[ply] = move
			var sePv [maxPly]int
			sScore := ss.search(p, ply, sBeta-1, sBeta, (depth-1)/2, false, sePv[:])
			ss.excludedMove[ply] = 0
			*picker = savedPicker
			if sScore < sBeta {
				extension = 1
				if useDoubleExt && !isPv && sScore < sBeta-seDoubleMargin {
					extension = 2
				}
			}
		}

		// Capture piece type before makeMove — after the call the square
		// may hold a promoted piece rather than the original pawn.
		movedPiece := p.typeAt(moveFrom(move))

		var u Undo
		makeMove(p, move, &u)
		if p.selfInCheck() { // move left our king in check: illegal
			unmakeMove(p, move, &u)
			continue
		}

		// Record this move in the cont hist context stack so child nodes
		// can look it up as their "1-ply back" context.
		// p.side has flipped after makeMove, so the mover was p.side^1.
		ss.contSide[ply] = p.side ^ 1
		ss.contPiece[ply] = movedPiece
		ss.contTo[ply] = moveTo(move)
		ss.contValid[ply] = true

		givesCheck := p.inCheck()

		// Extend by one ply for moves that give check, plus singular extension.
		newDepth := depth - 1 + extension
		if givesCheck && extension == 0{
			newDepth++
		}

		// Late move pruning: skip quiet moves beyond the threshold.
		// Moves that give check are exempt — they may be the only defence
		// or the only escape from a mating attack.
		// When improving we allow more moves (position is trending up, so
		// later moves are more likely to be relevant).
		lmpThreshold := LMPnormalStep*depth + 1
		//if improving {
		//	lmpThreshold = LMPimprovingStep*depth + 1
		//}
		if useLMP && stage == StageQuiet && !isPv && !nodeInCheck && depth < 4 &&
			quietTried > lmpThreshold && !givesCheck {
			unmakeMove(p, move, &u)
			continue
		}

		if stage == StageQuiet {
			quietTried++
		}

		// Late move reduction
		isReduced := false
		if stage == StageQuiet && depth > 2 && !nodeInCheck && !givesCheck && movesTried >= 4 && useLMR {
			reduction := lmr[min(depth, 63)][min(movesTried, 63)]
			if reduction > 0 {
				if !isPv {
					reduction++
				}

				if reduction > newDepth-1 {
					reduction = newDepth - 1
				}
				score = -ss.search(p, ply+1, -alpha-1, -alpha, newDepth-reduction, false, childPv[:])
				if score <= alpha {
					isReduced = true
				}
			}
		}

		// Principal Variation Search (PVS):
		// We assume the first move is the best and search it with a full window.
		// For subsequent moves, we use a Null-Window Search (-alpha-1, -alpha)
		// to quickly prove they are worse than the PV. If a move fails high,
		// we re-search it with the full window to find its true value.
		if (!isReduced) {
			if movesTried == 0 {
				score = -ss.search(p, ply+1, -beta, -alpha, newDepth, false, childPv[:])
			} else {
				score = -ss.search(p, ply+1, -alpha-1, -alpha, newDepth, false, childPv[:])
				if score > alpha && isPv {
					score = -ss.search(p, ply+1, -beta, -alpha, newDepth, false, childPv[:])
				}
			}
		}

		unmakeMove(p, move, &u)
		movesTried++

		if atomic.LoadInt32(&abortFlag) != 0 {
			return 0
		}

		// Beta cutoff: this move is "too good"; the opponent won't allow
		// reaching this position, so we can stop searching.  Apply a malus
		// to every quiet that was searched before this one — they failed to
		// cut off and should be tried later in future sibling nodes.
		if score >= beta {
			if isQuiet(p, move) {
				ss.updateHistory(p, move, depth, ply, quietsMade[:quietsMadeCount])
			}
			if ss.excludedMove[ply] == 0 {
				storeTT(p.key, move, score, LOWER, depth, ply)
			}
			return score
		}

		// Record this quiet as searched-but-failed so we can penalise it
		// if a later move causes a cutoff.
		if stage == StageQuiet && quietsMadeCount < maxMoves {
			quietsMade[quietsMadeCount] = move
			quietsMadeCount++
		}

		if score > bestScore {
			bestScore = score
			bestMove = move
			if score > alpha {
				alpha = score
				buildPV(pv, childPv[:], move)
				if isRoot {
					ss.reportInfo(score, pv)
				}
			}
		}
	}

	// --- Handle terminal nodes ---
	if bestScore == -inf {
		// In a singular extension sub-search the excluded move may have been
		// the only legal move; that's not checkmate or stalemate, just failure.
		 if ss.excludedMove[ply] != 0 {
		 	return alpha
		 }
		if p.inCheck() {
			return -mate + ply // checkmate: prefer shorter mates
		}
		return 0 // stalemate
	}

	// Note: the 50-move draw (p.clock >= 100) is now handled at the top of
	// this function alongside repetition detection, so it never reaches here.

	// Store the result in the TT with the appropriate bound type.
	// Skip during singular extension sub-searches: their partial results
	// (with one move excluded) must not corrupt the main TT entries.
	 //bound := UPPER
	 if ss.excludedMove[ply] == 0 {
	if bestScore > origAlpha {
		if isQuiet(p, bestMove) {
			ss.updateHistory(p, bestMove, depth, ply, nil)
		}
		storeTT(p.key, bestMove, bestScore, EXACT, depth, ply)
	} else {
		storeTT(p.key, 0, bestScore, UPPER, depth, ply)
	}
	 }

	return bestScore
}

func (ss *SearchState) quiesceCheck(p *Pos, ply, alpha, beta int, pv []int) int {
	ss.nodes++
	if ply > ss.selDepth {
		ss.selDepth = ply
	}
	ss.checkTime()
	if atomic.LoadInt32(&abortFlag) != 0 {
		return 0
	}

	pv[0] = 0

	if ss.isRepetition(p) {
		return 0
	}
	if p.clock >= 100 || p.isInsufficientMaterial() {
		return 0
	}

	if ply >= maxPly-1 {
		return evaluate(p)
	}

	inCheck := p.inCheck()

	picker := &ss.moveBuffers[ply]
	var childPv [maxPly]int

	best := -inf
	if !inCheck {
		rawQEval := evaluate(p)
		best = rawQEval + ss.getCorrection(p)
		if best >= beta {
			storeTT(p.key, 0, best, LOWER, 0, ply)
			return best
		}
		if best > alpha {
			alpha = best
		}
		initMovePicker(p, picker, ss, 0, ply)
	} else {
		initMovePicker(p, picker, ss, 0, ply)
	}

	movesTried := 0
	for {
		var move int
		if inCheck {
			move, _ = picker.nextMove()
			if move == 0 {
				break
			}
		} else {
			move, _ = picker.nextCaptureOrCheck()
			if move == 0 {
				break
			}
		}

		var u Undo
		makeMove(p, move, &u)
		if p.selfInCheck() {
			unmakeMove(p, move, &u)
			continue
		}
		movesTried++
		score := -ss.quiesce(p, ply+1, -beta, -alpha, childPv[:])
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

	if inCheck && movesTried == 0 {
		return -mate + ply
	}
	return best
}

// quiesce searches captures (and, when in check, all moves) until the
// position is quiet, then returns the static evaluation.  This prevents
// the "horizon effect" where the engine ignores an imminent capture at
// the leaf.
//
// When the side to move is in check we cannot stand pat — an evasion
// must be found.  We fall through to the full move pipeline so that
// quiet evasions are included.  If no legal move exists we return a
// checkmate score.  Outside of check the standard stand-pat applies.
func (ss *SearchState) quiesce(p *Pos, ply, alpha, beta int, pv []int) int {
	ss.nodes++
	if ply > ss.selDepth {
		ss.selDepth = ply
	}
	ss.checkTime()
	if atomic.LoadInt32(&abortFlag) != 0 {
		return 0
	}

	pv[0] = 0

	if ss.isRepetition(p) {
		return 0
	}
	if p.clock >= 100 || p.isInsufficientMaterial() {
		return 0
	}

	// Safeguard against reaching max ply limit
	if ply >= maxPly-1 {
		return evaluate(p)
	}

	inCheck := p.inCheck()
	picker := &ss.moveBuffers[ply]
	var childPv [maxPly]int

	// Stand-pat: outside of check we may decline all captures.
	// In check we must find an evasion, so stand-pat is illegal.
	best := -inf
	if !inCheck {
		rawQEval := evaluate(p)
		best = rawQEval + ss.getCorrection(p)
		if best >= beta {
			storeTT(p.key, 0, best, LOWER, 0, ply)
			return best
		}
		if best > alpha {
			alpha = best
		}
		initQSearch(p, picker)
	} else {
		initMovePicker(p, picker, ss, 0, ply)
	}

	movesTried := 0
	for {
		var move int
		if inCheck {
			move, _ = picker.nextMove()
			if move == 0 {
				break
			}
		} else {
			move = picker.nextCapture()
			if move == 0 {
				break
			}

			if isBadCapture(p, move) {
				continue
			}

			// Prune non-queen promotions in QS.
			if isProm(move) && moveType(move) != Q_PROM {
				continue
			}

			// Use a higher margin for pawn captures: passers can have
			// wildly different values so we give them extra slack.
			var margin int
			if moveType(move) == EP_CAP || p.typeAt(moveTo(move)) == P {
				margin = qsFpPawnMargin
			} else {
				margin = qsFpPieceMargin
			}
			futility := best + margin
			if moveType(move) == EP_CAP {
				futility += pieceValue[P]
			} else if p.board[moveTo(move)] != NO_PC {
				futility += pieceValue[p.typeAt(moveTo(move))]
			}
			if isProm(move) {
				futility += pieceValue[promType(move)] - pieceValue[P]
			}
			if futility <= alpha {
				best = max(best, futility)
				continue
			}
		}

		var u Undo
		makeMove(p, move, &u)
		if p.selfInCheck() {
			unmakeMove(p, move, &u)
			continue
		}
		movesTried++
		// QS LMP: cap captures tried outside of check to prevent explosion
		// in pathological positions with many equal-value captures.
		// Commented: only uncomment when testing positions like: 1b2kBbK/2BbB1B1/2B2bb1/B2b4/bbb1b3/BBb2BBB/BB3b2/BB1bb2b w - - 0 1
		// if !inCheck && movesTried > qsLmpLimit {
		// 	unmakeMove(p, move, &u)
		// 	break
		// }
		score := -ss.quiesce(p, ply+1, -beta, -alpha, childPv[:])
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

	if inCheck && movesTried == 0 {
		return -mate + ply
	}
	return best
}

// isRepetition returns true if the current position is a draw by repetition.
//
// Two rules apply depending on where the prior occurrence lives:
//
//	In-tree (keyHist index >= rootHistLen): the position was created during
//	the current search. One prior occurrence is enough to return draw — the
//	opponent can always force the third occurrence on the real board.
//
//	In-history (keyHist index < rootHistLen): the position existed before the
//	search started. Strict threefold requires two prior occurrences (three
//	total) before we can claim a forced draw.
//
// We step back in increments of 2 (same side to move) and stop at the
// 50-move clock boundary, since repetitions cannot cross an irreversible move.
func (ss *SearchState) isRepetition(p *Pos) bool {
	reps := 0
	for i := 4; i <= p.clock; i += 2 {
		idx := p.histLen - i
		if idx < 0 {
			break
		}
		if p.key != p.keyHist[idx] {
			continue
		}
		if idx >= ss.rootHistLen {
			return true // in-tree: one occurrence is enough
		}
		reps++
		if reps >= 2 {
			return true // in-history: need two prior occurrences
		}
	}
	return false
}

// reportInfo outputs a UCI "info" line for the current iteration.
// Mate scores are converted to "moves to mate" format.
func (ss *SearchState) reportInfo(score int, pv []int) {
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
	elapsed := time.Now().UnixMilli() - ss.searchStart
	if elapsed <= 0 {
		elapsed = 1
	}

	// Calculate nodes per second
	nps := ss.nodes * 1000 / elapsed

	// Output
	hashfull := ttHashfull()
	fmt.Printf("info depth %d seldepth %d time %d nodes %d nps %d hashfull %d score %s %d pv %s\n",
		rootDepth, ss.selDepth, elapsed, ss.nodes, nps, hashfull, scoreType, score, pvString(pv))
}

// checkTime is called periodically (every 4096 nodes) to see whether
// the allocated time has expired.  If so, it sets abortFlag so the
// search unwinds cleanly.  Skipped during depth-1 searches (the
// engine must always return at least one move) and when pondering.
func (ss *SearchState) checkTime() {
	if ss.nodes&1023 != 0 || rootDepth == 1 {
		return
	}
	if !pondering && timeLimit >= 0 {
		if time.Now().UnixMilli()-ss.searchStart >= timeLimit {
			atomic.StoreInt32(&abortFlag, 1)
		}
	}
}
