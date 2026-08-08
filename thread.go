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
//

package main

import (
	"time"
)

// SearchState holds all per-thread mutable search context.
type SearchState struct {
	isUsingNNUE  bool
	isMainThread bool    // true only for states[0]; lazy-SMP helpers never report UCI "info" lines
	tt           *TTable // pointer to transposition table

	staticEval EvalFunc

	// ---- Progress (reset each think) ----
	nodes       int64 // nodes searched by this thread
	nodesLimit  int64 // max nodes to search before aborting (0 = no limit)
	selDepth    int   // maximum ply reached this search
	searchStart int64 // Unix ms at the start of think()
	rootHistLen int   // p.histLen when think() began (repetition detection)

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
}

// resetForSearch prepares ss for a new search without losing
// accumulated history signal.  It resets progress counters,
// per-ply context, and killers (which are root-position specific).
func (ss *SearchState) resetForSearch(p *Pos) {
	ss.nodes = 0
	ss.selDepth = 0
	ss.searchStart = time.Now().UnixMilli()
	ss.rootHistLen = p.histLen
	ss.contValid = [maxPly]bool{}
	ss.killerMoves = [maxPly][2]int{}

	ss.multiPVIdx = 1
	ss.multiPVCount = 1
	ss.excludedRootMoves = ss.excludedRootMoves[:0]

	ss.isUsingNNUE = nnue.Loaded && singleOptionValue[NnuePerc] > 0
	ss.pickEvalFunction()
}

// interface for eval wrapper
type EvalFunc func(p *Pos, ply int) int

func (ss *SearchState) evalPesto(p *Pos, ply int) int {
	return evaluatePesto(p)
}

func (ss *SearchState) evalHCE(p *Pos, ply int) int {
	return evaluateHCE(p)
}

func (ss *SearchState) evalNNUE(p *Pos, ply int) int {
	return evaluateNNUE(p, &ss.accStack[ply])
}

func (ss *SearchState) evalHybrid(p *Pos, ply int) int {
	return evaluate(p, &ss.accStack[ply])
}

// which eval function we want to use? (PeSTO, HCE, NNUE, hybrid)
func (ss *SearchState) pickEvalFunction() {

	// user wants PeSTo eval
	if pestoEval {
		ss.isUsingNNUE = false // side effect, needed for speed
		ss.staticEval = ss.evalPesto
		return
	}

	// either fallback when NNUE isn't loaded or user wants HCE
	if !ss.isUsingNNUE && singleOptionValue[HcePerc] == 100 {
		ss.staticEval = ss.evalHCE
		return
	}

	// NNUE mode with no fluff
	if ss.isUsingNNUE && singleOptionValue[HcePerc] == 0 {
		ss.staticEval = ss.evalNNUE
		return
	}

	// hybrid eval
	ss.staticEval = ss.evalHybrid
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

// wrapper for preparing accumulator while hiding stack from search
func (ss *SearchState) prepareChildAccumulator(ply int) *Accumulator {
	if !ss.isUsingNNUE {
		return nil
	}

	child := &ss.accStack[ply+1]
	child.copyFrom(&ss.accStack[ply])

	return child
}

// record data needed for continuation history calculations
func (ss *SearchState) recordContHistContext(ply, side, piece, to int) {
	ss.contSide[ply] = side
	ss.contPiece[ply] = piece
	ss.contTo[ply] = to
	ss.contValid[ply] = true
}
