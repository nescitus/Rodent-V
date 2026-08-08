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
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// Global search state — shared across all threads.
// Per-thread state lives in SearchState (see thread.go).
var (
	softTimeLimit int64     // allocated soft move time in ms; stop between iterations
	hardTimeLimit int64     // allocated hard move time in ms (-1 = unlimited); drop-dead limit
	pondering     bool      // true while in ponder mode (ignore clock)
	rootDepth     int       // current iterative deepening depth (main thread drives)
	abortFlag     int32     // set to 1 atomically to stop all threads
	numThreads    int   = 1 // number of search threads (1 = single-threaded)
)

// lmr[depth][moveIndex] holds the ply reduction for a quiet move tried
// at position moveIndex in the move list when depth plies remain.
var lmr [64][64]int

func init() {
	initLMRTable()
}

// initLMRTable pre-computes the reduction for every (depth, moveIndex) pair.
// Entries where depth < 3 or moveIndex < 4 are left at zero (no reduction for
// the first few moves or near the leaves, where late move pruning takes over).
// All other entries use the logarithmic formula + a linear coefficient
func initLMRTable() {
	for depth := 0; depth < 64; depth++ {
		for moveCount := 0; moveCount < 64; moveCount++ {

			// no lmr here
			if depth < minLmrDepth || moveCount < minLmrMove {
				lmr[depth][moveCount] = 0
				continue
			}

			// log component
			raw := math.Log(float64(depth)) * math.Log(float64(moveCount)) / lmrDivisor

			// linear component
			reduction := int(raw + lmrLinear)

			lmr[depth][moveCount] = limitValue(reduction, 1, lmrMax)
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
		h.clearHistory()
		h.resetForSearch(p)
		refresh(p, &h.accStack[0])
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
				iterScore = ss.search(p, 0, -inf, inf, rootDepth, false, pvs[pvIdx][:])
			} else {
				score := scores[pvIdx]
				// Score-adaptive initial delta: balanced positions get a tight
				// window; large scores widen it to reduce retry churn.
				delta := 25 + score*score/16384
				alpha := max(-inf, score-delta)
				beta := min(inf, score+delta)

				for {
					iterScore = ss.search(p, 0, alpha, beta, rootDepth, false, pvs[pvIdx][:])
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

		if pvs[0][0] != 0 && pvs[0][0] == lastBestMove {
			bestMoveStability++
		} else {
			bestMoveStability = 1
			lastBestMove = pvs[0][0]
		}
	}

	// Stop all helpers and wait for them to exit.
	atomic.StoreInt32(&abortFlag, 1)
	wg.Wait()

	if pvs[0][0] != 0 {
		best := moveToStr(pvs[0][0])
		if pvs[0][1] != 0 {
			ponder := moveToStr(pvs[0][1])
			fmt.Printf("bestmove %s ponder %s\n", best, ponder)
		} else {
			fmt.Printf("bestmove %s\n", best)
		}
	} else {
		fmt.Println("bestmove 0000")
	}
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

	// Quiescence search entry point
	if depth <= 0 {
		return ss.quiesce(p, ply, alpha, beta, pv)
	}

	// Set selDepth (max distance from root
	// achieved in this search)
	ss.nodes++
	if ply > ss.selDepth {
		ss.selDepth = ply
	}

	// Check for timeout
	ss.checkTime()
	if ss.isAbortingSearch() {
		return 0
	}

	// Root node warrants extra treatment,
	// because we need to return a move after search
	isRoot := ply == 0

	// Check for draw
	if !isRoot {
		pv[0] = 0

		// Check all draw conditions before generating any moves so that
		// pv[0] == 0 is guaranteed when we return, preventing the PV from
		// extending past a drawing position.
		if ss.isDraw(p) {
			ss.checkTime() // sets abort flag - helps in case of many draws in a row
			return 0
		}
	}

	// --- Mate distance pruning ---
	// Tighten the window based on the best/worst mate reachable from this ply.
	// If we already have a proven mate, skip searching slower mates.
	if !isRoot {
		alpha = max(alpha, -mate+ply)
		beta = min(beta, mate-ply-1)

		if alpha >= beta {
			return alpha
		}
	}

	// Are we in pv-node? Principal variation nodes return exact
	// scores (no cutoffs) and see less pruning.
	isPv := beta > alpha+1

	// --- Transposition-table probe ---
	ttMove := 0
	ttFlag := 0
	ttDepth := 0
	score := 0

	// We don't do tt cutoffs in pv nodes to avoid truncating main line.
	// This decision is debatable. If we change it, we also must guard
	// against returning at root.
	if ss.tt.probe(p.key, &ttMove, &score, &ttFlag, &ttDepth, alpha, beta, depth, ply) {
		if !isPv && ss.excludedMove[ply] == 0 {
			return score
		}
	}

	// Save TT score before NMP/LMR overwrites the score variable.
	ttScore := score

	// Overflow guard.
	if plyLimitReached(ply) {
		return ss.staticEval(p, ply)
	}

	// Are we in check in this node?
	nodeInCheck := p.inCheck()

	// Cache the static evaluation; shared by RFP, improving, and null-move pruning.
	// Store in evalStack for the improving heuristic (ply-2 comparison).
	// When in check we store a sentinel so the improving check below works correctly.

	// We also adjust static eval by a dynamic, search-dependent correction factor.
	// This factor rises when in search current position overperforms its static eval
	// and falls in the opposite case.

	rawEval := 0       // raw score returned by eval function
	correctedEval := 0 // eval after corrhist correction, later used by that heuristic
	staticEval := 0    // fully adjusted static eval, used throughout search

	if !nodeInCheck {
		rawEval = ss.staticEval(p, ply)
		correction := 0
		if adjustEvalByCorrhist {
			correction = ss.getCorrection(p)
		}
		staticEval = rawEval + correction
		correctedEval = staticEval
		ss.evalStack[ply] = staticEval
	} else {
		ss.evalStack[ply] = noEval
	}

	// --- TT-adjusted eval ---
	// When the TT holds a score that is consistent with the direction of the
	// static eval's likely error, use it as a sharper proxy for the true value.
	// A LOWER-bound score >= staticEval means the position is probably better
	// than the static eval; an UPPER-bound score <= staticEval means it's
	// probably worse.  Using this refined estimate makes RFP, razoring, and
	// NMP trigger on more accurate information.
	// Guard: only valid when not in check (staticEval is meaningless then)
	// and when there is no excluded move (singular extension sub-search).
	if adjustEvalByTT {
		staticEval = ss.adjustedStaticEval(ply, staticEval, ttScore, ttFlag, nodeInCheck)
	}

	// Set improving flag. We change some prunings and reductions
	// based on whether position gets better.
	improving := ss.isImproving(ply, staticEval, nodeInCheck)

	// --- Node level pruning ---
	// gatekeeping reverse futility pruning, razoring and null move
	if !isPv && !nodeInCheck && !wasNull && !isMateScore(beta) && ss.excludedMove[ply] == 0 {

		// --- Reverse futility pruning ---
		// If the static eval beats beta by a depth-scaled margin, the position
		// is already so good that a full search is unlikely to fall below beta.
		// The mate guard prevents pruning when beta is a mate score, where the
		// static eval is unreliable.  We return the margin-adjusted value rather
		// than the raw static eval to avoid inflating the returned score.

		// Use a tighter margin when improving (position is gaining ground).
		// Wider margin when not improving avoids over-pruning a deteriorating line.
		rfpDepthMargin := rfpMargin
		if improving {
			rfpDepthMargin = rfpImpMargin
		}

		if useRFP && p.canNullMove() && depth <= rfpMaxDepth && !isMating(beta) &&
			staticEval-rfpDepthMargin*depth >= beta {

			return staticEval - rfpDepthMargin*depth
		}

		// --- Razoring ---
		// If static eval is far below alpha at shallow depth, the position is
		// unlikely to improve enough with quiet moves. Drop into qsearch; if
		// qsearch also fails low, return immediately without searching further.
		if useRazoring && depth <= maxRazorDepth &&
			staticEval <= alpha-razorMargin*depth {
			s := ss.quiesce(p, ply, alpha, beta, pv)
			if s <= alpha {
				return s
			}
		}

		// --- Null-move pruning ---
		// Skip if: depth <= 1 (too shallow to be reliable), the position
		// is already beyond beta, we are in check, or only pawns remain.
		// Reduction = base + depth/depthDiv + min((eval-beta)/evalDiv, maxEvalBonus).
		if useNULL && depth >= nmpMinDepth && p.canNullMove() && beta <= staticEval {

			ss.contValid[ply] = false

			reduction := nmpBaseReduction + depth/nmpDepthReduction

			// We need to prepare child accumulator, but we don't need a pointer,
			// because null move does not change accumulator state.
			ss.prepareChildAccumulator(ply)

			var nullPv [maxPly]int
			oldEP := makeNullMove(p)
			score = -ss.search(p, ply+1, -beta, -beta+1, depth-1-reduction, true, nullPv[:])
			unmakeNullMove(p, oldEP)

			if ss.isAbortingSearch() {
				return 0
			}

			// Do not return an unproven mate score.
			if isMating(score) {
				score = beta
			}

			// Null-move verification searches the original position
			// at the same ply.
			if useVerification && depth-reduction >= minVerDepth && score >= beta {
				score = ss.search(p, ply, alpha, beta, depth-reduction-verReduction, true, pv)

				if ss.isAbortingSearch() {
					return 0
				}
			}

			if score >= beta {
				return score
			}
		}
	}

	// --- ProbCut ---
	// If a tactical move already exceeds beta by a safety margin at reduced
	// depth, assume the full node will also fail high and prune the rest.
	if useProbcut && !isPv && !nodeInCheck && depth >= probcutMinDepth && !isMating(beta) &&
		ss.excludedMove[ply] == 0 {
		probcutBeta := min(beta+probcutMargin, mate-maxPly)
		probcutDepth := depth - 1 - probcutReduction
		probcutPicker := &ss.moveBuffers[ply]
		initQSearch(p, probcutPicker)
		var probcutPv [maxPly]int

		for {
			move := probcutPicker.nextCapture()
			if move == 0 {
				break
			}

			if isBadCapture(p, move) {
				continue
			}

			// We are about to move. Prepare NNUE accumulator for the next ply.
			childAcc := ss.prepareChildAccumulator(ply)

			// doMove() executes a move and creates pointer
			// to data required by NNUE accumulator
			u := ss.doMove(p, ply, move)

			// Skip illegal move.
			if p.selfInCheck() {
				ss.undoMove(p, ply)
				continue
			}

			// Update NNUE accumulator once we know that move is legal.
			if childAcc != nil {
				childAcc.applyPendingChanges(p, u)
			}

			score = -ss.quiesce(p, ply+1, -probcutBeta, -probcutBeta+1, probcutPv[:])

			// ProbCut re-search if quiesce returns good score
			if score >= probcutBeta && probcutDepth > 0 {
				score = -ss.search(p, ply+1, -probcutBeta, -probcutBeta+1, probcutDepth, false, probcutPv[:])
			}

			ss.undoMove(p, ply)

			if ss.isAbortingSearch() {
				return 0
			}

			if score >= probcutBeta {
				return score
			}
		}
	}

	// --- Internal iterative reduction ---
	// Without a TT move the move picker has no good hint; search one
	// ply shallower so the cost of poor ordering is contained.
	// Skipped in check: evasions must be searched at full depth.
	if useIIR && ttMove == 0 && depth >= 4 && !nodeInCheck {
		depth--
	}

	// Init before main loop.
	bestMove := 0
	origAlpha := alpha
	bestScore := -inf
	picker := &ss.moveBuffers[ply]
	movesTried := 0      // move counter (useful for pruning/reduction decisions).
	quietTried := 0      // quiet move counter (as above)
	quietsMadeCount := 0 // index to ss.quietsMade;
	// ss.quietsMade tracks quiet moves that were
	// fully searched without causing a beta cutoff.
	// On a cutoff we apply a malus to all of them.

	initMovePicker(p, picker, ss, ttMove, ply)
	var childPv [maxPly]int

	// --- Main move loop ---
	for {
		move, stage := picker.nextMove()
		if move == 0 {
			break
		}

		// Skip the move excluded during a singular extension search.
		if move == ss.excludedMove[ply] {
			continue
		}

		// Skip excluded root moves for MultiPV support
		if isRoot {
			skip := false
			for _, excluded := range ss.excludedRootMoves {
				if move == excluded {
					skip = true
					break
				}
			}
			if skip {
				continue
			}
		}

		// --- Singular extension ---
		// When the TT move is reliable and deep enough, check if it's the
		// only good move by searching without it at reduced depth. If all
		// other moves fail below a margin, extend this move by +1 ply.
		// Negative extension: if the position isn't singular AND the TT
		// score already beats beta, reduce aggressively (multi-cut signal).
		extension := 0
		if useSingularExt &&
			move == ttMove &&
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

		// Capture moving piece type before making a move — after the call
		// the square may hold a promoted piece rather than the original pawn.
		movedPiece := p.typeAt(moveFrom(move))

		// We are about to move. Prepare NNUE accumulator for the next ply.
		childAcc := ss.prepareChildAccumulator(ply)

		// doMove() executes a move and creates pointer
		// to data required by NNUE accumulator
		u := ss.doMove(p, ply, move)

		// Skip illegal move.
		if p.selfInCheck() {
			ss.undoMove(p, ply)
			continue
		}

		// Record this move in the cont hist context stack so child nodes
		// can look it up as their "1-ply back" context.
		// p.side has flipped after making a move, so the mover was p.side^1.
		ss.recordContHistContext(ply, p.side^1, movedPiece, moveTo(move))

		// Does the move give check?
		givesCheck := p.inCheck()

		// Extend by one ply for moves that give check, plus singular extension.
		newDepth := depth - 1 + extension

		if givesCheck {
			newDepth++
		}

		// Late move pruning: skip quiet moves beyond the threshold.
		// Moves that give check are exempt — they may be the only defence
		// or the only way to continue a mating attack.
		// When improving we allow more moves (position is trending up, so
		// later moves are more likely to be relevant).
		if depth < 10 { // table size limit

			lmpThreshold := LMPnormalStep
			if improving {
				lmpThreshold = LMPimprovingStep
			}

			if useLmpTable {
				lmpThreshold = lmp[0][depth]
				if improving {
					lmpThreshold = lmp[1][depth]
				}
			}
			if useLMP && stage == StageQuiet && !isPv && !nodeInCheck && depth < LMPdepth &&
				quietTried > lmpThreshold && !givesCheck {
				ss.undoMove(p, ply)
				continue
			}
		}

		// Futility pruning: at shallow depth, skip late quiet moves that
		// cannot plausibly raise alpha even with a generous margin.
		if useFutility && stage == StageQuiet && !isPv && !nodeInCheck && !givesCheck &&
			ss.excludedMove[ply] == 0 && depth <= fpMaxDepth &&
			quietTried > 0 && !isMating(alpha) &&
			staticEval+fpMargin*depth <= alpha {
			ss.undoMove(p, ply)
			continue
		}

		// Update nnue accumulator now that we know
		// that move is legal and hasn't been pruned.
		if ss.isUsingNNUE {
			childAcc.applyPendingChanges(p, u)
		}

		// Count quiet moves
		if stage == StageQuiet {
			quietTried++
		}

		// --- Late move reduction ---
		isReduced := false

		// LMR of quiet nodes
		if useLMR && stage == StageQuiet && depth >= minLmrDepth &&
			!nodeInCheck && movesTried >= 4 &&
			singleOptionValue[NodesLimit] == 0 {
			// Read base reduction value.
			reduction := lmr[min(depth, 63)][min(movesTried, 63)]
			if reduction > 0 {
				// Reduce more in zero window nodes.
				if !isPv {
					reduction++
				}
				// Not improving: position is trending down, reduce more aggressively.
				if !improving && LMRnonImproving {
					reduction++
				}
				// Moves that give check are reduced half the amount.
				if givesCheck {
					reduction /= 2
				}
				// Reduction cannot exceed actual depth.
				if reduction > newDepth-1 {
					reduction = newDepth - 1
				}

				score = -ss.search(p, ply+1, -alpha-1, -alpha, newDepth-reduction, false, childPv[:])

				if score <= alpha {
					isReduced = true
				}
			}
		}

		// LMR of bad captures
		if useLMR && stage == StageBadCaptures && depth >= minLmrDepth &&
			!nodeInCheck && !givesCheck && movesTried >= 4 &&
			singleOptionValue[NodesLimit] == 0 {
			reduction := 1
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

		// Principal Variation Search:
		// First move: full window.
		// Subsequent moves: zero-width window first; re-search with full window
		// only if the ZW fails high AND we are at a PV node.
		if !isReduced {
			if movesTried == 0 {
				score = -ss.search(p, ply+1, -beta, -alpha, newDepth, false, childPv[:])
			} else {
				score = -ss.search(p, ply+1, -alpha-1, -alpha, newDepth, false, childPv[:])
				if score > alpha && isPv {
					score = -ss.search(p, ply+1, -beta, -alpha, newDepth, false, childPv[:])
				}
			}
		}

		ss.undoMove(p, ply)

		movesTried++

		if ss.isAbortingSearch() {
			return 0
		}

		// Beta cutoff: this move is "too good"; the opponent won't allow
		// reaching this position, so we can stop searching.
		if score >= beta {

			// We want to try moves that may cause a beta cutoff as early
			// as possible. History move ordering tries to increase chances
			// of such an occurence.

			// Award a bonus to a quiet move that caused a cutoff.
			// Apply a malus to every quiet move that was searched before
			// this one — they failed to produce a cutoff and should be
			// tried later in future sibling nodes.
			if isQuiet(p, move) {
				ss.updateHistory(p, move, depth, ply, ss.quietsMade[ply][:quietsMadeCount])
			}

			// TT save.
			if ss.excludedMove[ply] == 0 {
				ss.tt.store(p.key, move, score, LOWER, depth, ply)
			}

			// Update correction history on beta cutoff.
			if !nodeInCheck && isQuiet(p, move) && score > correctedEval {
				ss.addCorrection(p, depth, score-correctedEval)
			}

			return score
		}

		// Record this quiet as searched-but-failed so we can penalise it
		// if a later move causes a cutoff.
		if stage == StageQuiet && quietsMadeCount < maxMoves {
			ss.quietsMade[ply][quietsMadeCount] = move
			quietsMadeCount++
		}

		if score > bestScore {
			bestScore = score
			bestMove = move

			if score > alpha {
				alpha = score

				// bestMove = move
				buildPV(pv, childPv[:], move)

				// In MultiPV mode, lines are reported together as a
				// batch once every line has finished searching this
				if isRoot && ss.multiPVCount <= 1 {
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

	// Store the result in the TT with the appropriate bound type.
	// Skip during singular extension sub-searches: their partial results
	// (with one move excluded) must not corrupt the main TT entries.
	bound := UPPER

	if ss.excludedMove[ply] == 0 {
		if bestScore > origAlpha {
			bound = EXACT

			if isQuiet(p, bestMove) {
				ss.updateHistory(p, bestMove, depth, ply, nil)
			}

			ss.tt.store(p.key, bestMove, bestScore, EXACT, depth, ply)
		} else {
			ss.tt.store(p.key, 0, bestScore, UPPER, depth, ply)
		}
	}

	// Update correction history: adjust the correction table when the
	// search result disagrees with the raw static eval.  Skip when in check
	// (no reliable eval), when the best move is tactical, or when the bound
	// direction is consistent with the eval (no useful correction signal).
	if !nodeInCheck &&
		ss.excludedMove[ply] == 0 &&
		!(bestMove != 0 && !isQuiet(p, bestMove)) &&
		!((bound == LOWER && bestScore <= correctedEval) ||
			(bound == UPPER && bestScore >= correctedEval)) {

		ss.addCorrection(p, depth, bestScore-correctedEval)
	}

	return bestScore
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

	// Test for timeout.
	ss.checkTime()
	if ss.isAbortingSearch() {
		return 0
	}

	pv[0] = 0

	// Draw.
	if ss.isDraw(p) {
		return 0
	}

	// Guard against search stacks overflow.
	if plyLimitReached(ply) {
		return ss.staticEval(p, ply)
	}

	// TT probe: use depth=0 for all qsearch entries.
	ttMove := 0
	ttScore := 0
	ttFlag := 0
	ttDepth := 0

	if ss.tt.probe(p.key, &ttMove, &ttScore, &ttFlag, &ttDepth, alpha, beta, 0, ply) {
		return ttScore
	}

	inCheck := p.inCheck()
	picker := &ss.moveBuffers[ply]
	var childPv [maxPly]int

	// Stand-pat: outside of check we may decline all captures.
	// In check we must find an evasion, so stand-pat is illegal.
	best := -inf

	if !inCheck {
		rawQEval := ss.staticEval(p, ply)

		// Adjust eval using correction history
		if adjustEvalByCorrhist {
			best = rawQEval + ss.getCorrection(p)
		} else {
			best = rawQEval
		}

		if best >= beta {
			ss.tt.store(p.key, 0, best, LOWER, 0, ply)
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
				futility += pieceValue[promType(move)] -
					pieceValue[P]
			}

			if futility <= alpha {
				best = max(best, futility)
				continue
			}
		}

		// We are about to move. Prepare NNUE accumulator for the next ply.
		childAcc := ss.prepareChildAccumulator(ply)

		// doMove() executes a move and creates pointer
		// to data required by NNUE accumulator.
		u := ss.doMove(p, ply, move)

		// Skip illegal move.
		if p.selfInCheck() {
			ss.undoMove(p, ply)
			continue
		}

		movesTried++
		// QS LMP: cap captures tried outside of check to prevent explosion
		// in pathological positions with many equal-value captures.
		// Commented: only uncomment when testing positions like: 1b2kBbK/2BbB1B1/2B2bb1/B2b4/bbb1b3/BBb2BBB/BB3b2/BB1bb2b w - - 0 1
		// if !inCheck && movesTried > qsLmpLimit {
		// 	break
		// }

		// Update NNUE accumulator once we know move is legal
		if childAcc != nil {
			childAcc.applyPendingChanges(p, u)
		}

		score := -ss.quiesce(p, ply+1, -beta, -alpha, childPv[:])

		ss.undoMove(p, ply)

		if ss.isAbortingSearch() {
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

// isDraw detects draws in search
func (ss *SearchState) isDraw(p *Pos) bool {
	return ss.isRepetition(p) || p.isInsufficientMaterial() || p.clock >= 100
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

	// Keeping screen clean in datagen mode.
	if datagenMode {
		return
	}

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
	if !IsBenchMode {
		fmt.Printf("info depth %d seldepth %d multipv %d time %d nodes %d nps %d hashfull %d score %s %d pv %s\n",
			rootDepth, ss.selDepth, ss.multiPVIdx, elapsed, ss.nodes, nps, hashfull, scoreType, score, pvString(pv))
	}
}

// checkTime tests periodically (every 1024 nodes) whether
// the allocated time has expired.  If so, it sets abortFlag so the
// search unwinds cleanly.  Skipped during depth-1 searches (the
// engine must always return at least one move) and when pondering.
func (ss *SearchState) checkTime() {

	if datagenMode {
		return
	}

	if ss.nodes&timeoutTestPeriod != 0 || rootDepth == 1 {
		return
	}

	// Nodes limit for weaker personalities
	if singleOptionValue[NodesLimit] > 0 && ss.nodes >= int64(singleOptionValue[NodesLimit]) {
		atomic.StoreInt32(&abortFlag, 1)
	}

	// Timeout
	if !pondering && hardTimeLimit >= 0 {
		if time.Now().UnixMilli()-ss.searchStart >= hardTimeLimit {
			atomic.StoreInt32(&abortFlag, 1)
		}
	}
}

// Return eval adjusted by transposition table score
func (ss *SearchState) adjustedStaticEval(ply, staticEval, ttScore, ttFlag int, nodeInCheck bool) int {
	if nodeInCheck || ss.excludedMove[ply] != 0 || ttFlag == 0 {
		return staticEval
	}

	if ttFlag == EXACT ||
		(ttFlag == LOWER && ttScore >= staticEval) ||
		(ttFlag == UPPER && ttScore <= staticEval) {
		return ttScore
	}

	return staticEval
}

// Are we better than 2 plies ago (4 if 2 plies ago we were in check)?
func (ss *SearchState) isImproving(ply, staticEval int, nodeInCheck bool) bool {
	if nodeInCheck {
		return false
	}
	if ply >= 2 && ss.evalStack[ply-2] != noEval {
		return staticEval > ss.evalStack[ply-2]
	}
	if ply >= 4 && ss.evalStack[ply-4] != noEval {
		return staticEval > ss.evalStack[ply-4]
	}
	return true
}

// Does score show we are delivering checkmate?
func isMating(score int) bool {
	return score >= mate-maxPly
}

// Does score show we are being checkmated?
func isMated(score int) bool {
	return score <= -mate+maxPly
}

// Is score in the checkate range?
func isMateScore(score int) bool {
	return isMating(score) || isMated(score)
}

// Are we in the process of aborting search?
func (ss *SearchState) isAbortingSearch() bool {
	// For datagen
	if ss.nodesLimit > 0 && ss.nodes >= ss.nodesLimit {
		return true
	}

	// Timeout
	return atomic.LoadInt32(&abortFlag) != 0
}

// Is search stack about to overflow?
func plyLimitReached(ply int) bool {
	return ply >= maxPly-1
}
