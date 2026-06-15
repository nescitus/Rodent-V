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

import "time"

// SearchState holds all per-thread mutable search context.
type SearchState struct {
	// ---- Progress (reset each think) ----
	nodes       int64 // nodes searched by this thread
	selDepth    int   // maximum ply reached this search
	searchStart int64 // Unix ms at the start of think()
	rootHistLen int   // p.histLen when think() began (repetition detection)

	// ---- Per-ply context (indexed by ply, reset each think) ----
	evalStack    [maxPly]int  // static eval at each ply; noEval when in check
	contSide     [maxPly]int  // side that made the move reaching this ply
	contPiece    [maxPly]int  // piece type (0-5) of that move
	contTo       [maxPly]int  // destination square of that move
	contValid    [maxPly]bool // false for null moves and unvisited plies
	excludedMove [maxPly]int  // singular extension: excluded move (0 = none)

	// ---- Heuristic tables (persist across moves of the same game) ----
	histTable       [2][64][64]int          // butterfly history [side][from][to]
	contHistMain    [2][6][64][2][6][64]int // continuation history
	killerMoves     [maxPly][2]int          // two killers per ply
	moveBuffers     [maxPly]MovePicker      // pre-allocated pickers (one per ply)
	corrHist        [2][corrHistSize]int    // pawn correction history
	nonPawnCorrHist [2][2][corrHistSize]int // non-pawn correction history
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
	ss.contHistMain = [2][6][64][2][6][64]int{}
	ss.killerMoves = [maxPly][2]int{}
	ss.corrHist = [2][corrHistSize]int{}
	ss.nonPawnCorrHist = [2][2][corrHistSize]int{}
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
}
