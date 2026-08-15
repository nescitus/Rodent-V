// ================================================================
// S13  SEARCH STATE  (per-thread search context)
// ================================================================
//
//   SearchState holds all mutable state that belongs to one search
//   thread.  In single-threaded mode there is exactly one instance.
//   In lazy SMP each helper goroutine owns its own SearchState while
//   sharing only the transposition table and abortFlag with the main
//   thread.
//
//   LIFETIME
//   --------
//   One SearchState is created per thread slot and reused across
//   moves of the same game.  Heuristic tables (history, correction)
//   are NOT reset between moves so ordering signal accumulates.
//   clearHistory() is called only on ucinewgame.
//   resetForSearch() resets progress counters and per-ply context
//   before each think() call.
//
//   LAZY SMP
//   --------
//   All helper threads search the exact same root position from
//   depth 1 upward, sharing results through the TT.  No work
//   splitting or synchronisation beyond the atomic abortFlag is
//   needed.  Typical scaling: ~1.6x at 4 threads, ~2.3x at 8.

package main

import (
	"time"
)

// SearchState holds all per-thread mutable search context.
type SearchState struct {
	isUsingNNUE bool
	// kbnModeActive is true while in a KBN vs K endgame; forces pure HCE.
	kbnModeActive bool
	isMainThread  bool    // true only for states[0]; lazy-SMP helpers never report UCI "info" lines
	tt            *TTable // pointer to transposition table

	// evalHash/pawnHash are nil for the normal UCI/search path, which
	// falls back to the shared global tables (evalHashFor/pawnHashFor).
	// Batch workers use private tables instead: shared unsynchronized
	// reads/writes could tear a multiword entry and return the wrong score
	// for a key, which datagen could then write into training data.
	evalHash *evalHashTable
	pawnHash *pawnHashTable

	// evalMode selects which function staticEval() dispatches to.  Was
	// previously a method value stored in a func-typed field, which made
	// every evaluation an indirect (non-inlinable) call; a small enum
	// switched on directly lets the compiler inline the chosen path.
	evalMode       EvalMode
	nnueGeneration uint64 // network generation used by the previous search

	// ---- Progress (reset each think) ----
	nodes          int64 // nodes searched by this thread
	nodesLimit     int64 // max nodes to search before aborting (0 = no limit)
	aborting       bool  // thread-local cached abort flag to avoid atomic contention
	selDepth       int   // maximum ply reached this search
	searchStart    int64 // Unix ms at the start of think()
	rootHistLen    int   // p.histLen when think() began (repetition detection)
	completedDepth int   // last completed depth for thread voting
	completedScore int   // score from last completed depth
	completedMove  int   // best move from last completed depth

	// ---- MultiPV State ----
	multiPVIdx        int   // current multipv index (1-based)
	multiPVCount      int   // number of pv lines requested this think() call
	excludedRootMoves []int // slice of root moves to skip

	// ---- Per-ply context (indexed by ply, reset each think) ----
	accStack     [maxPly]Accumulator   // NNUE accumulator uses copy/makes
	updateStack  [maxPly]Update        // data for lazy nnue accumulator updates
	revertStack  [maxPly]Revert        // data for reverting board state
	evalStack    [maxPly]int           // static eval at each ply; noEval when in check
	contSide     [maxPly]int           // side that made the move reaching this ply
	contPiece    [maxPly]int           // piece type (0-5) of that move
	contTo       [maxPly]int           // destination square of that move
	contValid    [maxPly]bool          // false for null moves and unvisited plies
	excludedMove [maxPly]int           // singular extension: excluded move (0 = none)
	quietsMade   [maxPly][maxMoves]int // quiet moves tried so far at a current ply

	// ---- Heuristic tables (persist across moves of the same game) ----
	histTable       [2][64][64]int            // butterfly history [side][from][to]
	contHistMain    [2][6][64][2][6][64]int16 // continuation history
	killerMoves     [maxPly][2]int            // two killers per ply
	moveBuffers     [maxPly]MovePicker        // pre-allocated pickers (one per ply)
	pawnCorrHist    [2][corrHistSize]int16    // pawn correction history
	nonPawnCorrHist [2][2][corrHistSize]int16 // non-pawn correction history
	minorCorrHist   [2][2][corrHistSize]int16 // knight-bishop-king correction history
	majorCorrHist   [2][2][corrHistSize]int16 // rook-queen-king correction history

	// ---- NNUE Finny Cache (cached accumulator per perspective & mirror) ----
	finny [2][2]FinnyEntry // [perspective][mirror]
}

type FinnyEntry struct {
	acc        [NNUEHiddenSize]int16
	pieces     [12]uint64
	generation uint64
	valid      bool
}

// State destroyed by makeMove.
type Revert struct {
	flag            int
	oldKey          uint64
	oldPawnKey      [2]uint64
	oldNonPawnKey   [2]uint64
	oldMajorKey     [2]uint64
	oldMinorKey     [2]uint64
	oldCastleRights int
	oldEpSquare     int
	oldClock        int
	oldHistLen      int
}

// SearchResult carries the output of a completed think() call.
// The UCI loop reads this to print "bestmove".
type SearchResult struct {
	BestMove   int   // best move found (0 = none / resign)
	PonderMove int   // predicted opponent reply (0 if none)
	Score      int   // centipawn score (engine's perspective)
	Depth      int   // last completed iteration depth
	SelDepth   int   // maximum ply reached
	Nodes      int64 // total nodes across all threads
	TimeMs     int64 // wall-clock time (ms)
}

// trimIdleThreadStates releases SearchState memory for thread slots that
// are no longer in use after numThreads has been reduced.
// Must only be called while no search is

func trimIdleThreadStates(states []*SearchState, numThreads int) {
	for i := numThreads; i < len(states); i++ {
		states[i] = nil
	}
}

// clearHistory resets all heuristic tables to zero.
// Call on ucinewgame to prevent cross-game contamination.
func (ss *SearchState) clearHistory() {
	ss.histTable = [2][64][64]int{}
	ss.contHistMain = [2][6][64][2][6][64]int16{}
	ss.killerMoves = [maxPly][2]int{}
	ss.pawnCorrHist = [2][corrHistSize]int16{}
	ss.nonPawnCorrHist = [2][2][corrHistSize]int16{}
	ss.majorCorrHist = [2][2][corrHistSize]int16{}
	ss.minorCorrHist = [2][2][corrHistSize]int16{}
	ss.finny = [2][2]FinnyEntry{}
	for i := range ss.moveBuffers {
		ss.moveBuffers[i].p = nil
		ss.moveBuffers[i].ss = nil
	}
}

// resetForSearch prepares ss for a new search without losing accumulated
// history signal. It returns true when cached scores were produced by a
// different evaluation regime; the caller must then clear the hash tables
// it exclusively owns.
func (ss *SearchState) resetForSearch(p *Pos) bool {
	previousEvalMode := ss.evalMode
	previousNNUEGeneration := ss.nnueGeneration

	ss.nodes = 0
	ss.aborting = false
	ss.completedDepth = 0
	ss.completedScore = 0
	ss.completedMove = 0
	ss.selDepth = 0
	ss.searchStart = time.Now().UnixMilli()
	ss.rootHistLen = p.histLen
	ss.contValid = [maxPly]bool{}
	ss.killerMoves = [maxPly][2]int{}

	for i := range ss.moveBuffers {
		ss.moveBuffers[i].p = nil
		ss.moveBuffers[i].ss = nil
	}

	ss.multiPVIdx = 1
	ss.multiPVCount = 1
	ss.excludedRootMoves = ss.excludedRootMoves[:0]

	ss.isUsingNNUE = nnue.Loaded && singleOptionValue[NnuePerc] > 0

	// KBN vs K: switch to pure HCE so checkmateHelper's corner-driving
	// tables are used instead of NNUE, which has no understanding of the
	// correct mating corner. Hash invalidation belongs to the table owner:
	// UCI helpers share tables, while datagen and filter workers do not.
	ss.kbnModeActive = isKBNEndgame(p)
	ss.pickEvalFunction()
	ss.nnueGeneration = nnue.generation

	return ss.evalMode != previousEvalMode ||
		(ss.usesNNUE() && ss.nnueGeneration != previousNNUEGeneration)
}

// clearSearchHashes removes scores produced under the previous eval mode.
// The caller must have exclusive access to all three tables.
func (ss *SearchState) clearSearchHashes() {
	ss.tt.clear()
	evalHashFor(ss).clear()
	pawnHashFor(ss).clear()
}

// EvalMode selects which evaluation function ss.staticEval() dispatches
// to for the rest of the current search.
type EvalMode uint8

const (
	EvalModePesto EvalMode = iota
	EvalModeHCE
	EvalModeNNUE
	EvalModeHybrid
)

func (ss *SearchState) usesNNUE() bool {
	return ss.evalMode == EvalModeNNUE || ss.evalMode == EvalModeHybrid
}

// staticEval returns the static evaluation of p at the given ply, using
// whichever mode pickEvalFunction last selected for this thread.
func (ss *SearchState) staticEval(p *Pos, ply int) int {
	switch ss.evalMode {
	case EvalModePesto:
		return evaluatePesto(p)
	case EvalModeHCE:
		return evaluateHCE(p, ss)
	case EvalModeNNUE:
		return evaluateNNUE(p, &ss.accStack[ply], ss)
	default: // EvalModeHybrid
		return evaluate(p, &ss.accStack[ply], ss)
	}
}

// which eval function we want to use? (PeSTO, HCE, NNUE, hybrid)
func (ss *SearchState) pickEvalFunction() {

	// user wants PeSTo eval
	if pestoEval {
		ss.isUsingNNUE = false // side effect, needed for speed
		ss.evalMode = EvalModePesto
		return
	}

	// KBN vs K: HCE has dedicated corner-driving tables (checkmateHelper),
	// NNUE does not understand which corner to target.
	if ss.kbnModeActive {
		ss.isUsingNNUE = false
		ss.evalMode = EvalModeHCE
		return
	}

	// either fallback when NNUE isn't loaded or user wants HCE
	if !ss.isUsingNNUE && singleOptionValue[HcePerc] == 100 {
		ss.evalMode = EvalModeHCE
		return
	}

	// NNUE mode with no fluff
	if ss.isUsingNNUE && singleOptionValue[HcePerc] == 0 &&
		singleOptionValue[LikesClosed] == 0 &&
		singleOptionValue[KingTropism] == 0 &&
		singleOptionValue[Forwardness] == 0 {
		ss.evalMode = EvalModeNNUE
		return
	}

	// hybrid eval
	ss.evalMode = EvalModeHybrid
}

// doMove makes a move, stores all undo and NNUE update data,
// and prefetches the resulting position's TT bucket.
// It returns pointer to updateStack, required by nnue accumulator.
func (ss *SearchState) doMove(p *Pos, ply, move int) *Update {
	u := &ss.updateStack[ply]
	r := &ss.revertStack[ply]
	makeMove(p, u, r, move)
	ss.tt.prefetch(p.key)
	return u
}

// wrapper for makemove to simplify search code by hiding stacks
func (ss *SearchState) undoMove(p *Pos, ply int) {
	unmakeMove(p, &ss.updateStack[ply], &ss.revertStack[ply])
}

// prepareChildAccumulator copies the current accumulator to the next ply.
// Used only for null moves, which change no NNUE feature and so have
// nothing for applyPendingChanges to fuse the copy into -- ply+1 simply
// needs an exact copy of ply's accumulator.
func (ss *SearchState) prepareChildAccumulator(ply int) *Accumulator {
	if !ss.isUsingNNUE {
		return nil
	}

	child := &ss.accStack[ply+1]
	child.copyFrom(&ss.accStack[ply])

	return child
}

// nextPlyAccumulator returns the next ply's accumulator slot without
// copying into it. Used for real moves, where applyPendingChanges reads
// ply's accumulator as its src and writes ply+1 as dst in one fused pass
// (see nnueMove3 and friends) instead of copying first.
func (ss *SearchState) nextPlyAccumulator(ply int) *Accumulator {
	if !ss.isUsingNNUE {
		return nil
	}
	return &ss.accStack[ply+1]
}

// record data needed for continuation history calculations
func (ss *SearchState) recordContHistContext(ply, side, piece, to int) {
	ss.contSide[ply] = side
	ss.contPiece[ply] = piece
	ss.contTo[ply] = to
	ss.contValid[ply] = true
}
