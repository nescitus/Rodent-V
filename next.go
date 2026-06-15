// ================================================================
// S8  MOVE ORDERING
// ================================================================
//
//   Alpha-beta search is critically sensitive to the order in which
//   moves are tried.  The ideal order is best-first: if the best
//   move is always tried first, every subsequent move causes a
//   beta cutoff and is pruned immediately.  In practice we can't
//   know the best move ahead of time, but heuristics get us close.
//
//   ORDERING PIPELINE (phases 0 -> 7)
//   ----------------------------------
//   0. TT move:       the best move stored from a previous search
//                     of this position. Often produces a cutoff
//                     immediately.
//   1. Generate caps: fill the move list with captures/promotions.
//   2. Good captures: highest-value victims first (MVV-LVA), skipping
//                     captures that lose material by SEE.
//   3. Killer 1:      quiet move that caused a beta cutoff at this
//                     ply in a sibling branch.
//   4. Killer 2:      second killer at this ply.
//   5. Generate quiet: fill the move list with non-captures.
//   6. Quiet moves:   ordered by history heuristic score.
//   7. Bad captures:  exchanges that lose material, deferred to last.
//
//   HISTORY HEURISTIC
//   -----------------
//   histTable[fromSq][toSq] stores a score for each (origin, destination)
//   square pair.  On a beta cutoff the cutoff move receives a depth*depth
//   bonus; every other quiet move tried in that node receives the same
//   value as a malus (penalty).  Values are kept in [-maxHist, +maxHist]
//   via a gravity formula so recent signal gradually displaces old signal.
//   Indexing by from/to squares (4096 entries) rather than by piece type
//   (768 entries) gives finer resolution: moves with the same destination
//   but different origin squares get independent entries.
//
//   KILLER HEURISTIC
//   -----------------
//   killerMoves[ply][0..1] stores the two most recent quiet moves
//   that caused a beta cutoff at a given search depth.  Killers
//   from sibling nodes are often good moves in the current node too.
//

package main

// maxHist caps the history table values so old signal can be displaced
// by newer, deeper results.  The gravity update formula keeps every
// entry in the range [-maxHist, +maxHist].
const maxHist = 16384

// Correction history constants.
const (
	corrHistSize        = 16384              // number of entries per side
	corrHistGrain       = 256                // scaling factor for diff
	corrHistWeightScale = 256                // weight divisor
	corrHistMax         = corrHistGrain * 32 // absolute clamp
)

// Heuristic tables are now per-thread fields inside SearchState (thread.go).

type MoveGenStage int

const (
	StageTTMove MoveGenStage = iota
	StageGenCaptures
	StageGoodCaptures
	StageKiller1
	StageKiller2
	StageGenQuiet
	StageQuiet
	StageBadCaptures
	StageDone
)

// MovePicker holds the state for iterating through moves in priority
// order.  The search constructs one per node and calls nextMove()
// until it returns 0.
type MovePicker struct {
	ss       *SearchState // owning thread's state (for history lookup)
	p        *Pos
	phase    MoveGenStage  // which pipeline stage we are in (0-7)
	ttMove   int           // the transposition table move hint
	killer1  int           // first killer move for this ply
	killer2  int           // second killer move for this ply
	ply      int           // search ply, for continuation history lookup
	cur      int           // next index to yield from move[]
	end      int           // one-past-last valid index in move[]
	move     [maxMoves]int // move buffer (reused for captures then quiets)
	value    [maxMoves]int // ordering score for each move in move[]
	badCount int           // number of bad captures saved in badCaps[]
	badCur   int           // next index to yield from badCaps[]
	badCaps  [maxMoves]int // bad captures deferred to phase 7
}

// initMovePicker initialises the picker for a new node.
// ss is the owning thread's SearchState (for history tables and killers).
// transMove is the move hint from the TT (0 if none).
func initMovePicker(p *Pos, m *MovePicker, ss *SearchState, transMove, ply int) {
	m.ss = ss
	m.p = p
	m.phase = 0
	m.ttMove = transMove
	m.killer1 = ss.killerMoves[ply][0]
	m.killer2 = ss.killerMoves[ply][1]
	m.ply = ply
}

// nextMove returns the next move to try, in priority order, plus the
// stage from which it came. When all moves are exhausted, it returns
// (0, StageDone).
//
// The goto-based phase loop is the idiomatic way to implement a
// state machine in Go when each phase may immediately fall through
// to the next.
func (m *MovePicker) nextMove() (int, MoveGenStage) {
startPhase:
	switch m.phase {
	case StageTTMove:
		move := m.ttMove
		m.phase = StageGenCaptures
		if move != 0 && isLegal(m.p, move) {
			return move, StageTTMove
		}
		goto startPhase

	case StageGenCaptures:
		m.end = genCaptures(m.p, m.move[:])
		m.cur = 0
		m.badCount = 0
		scoreCaptures(m)
		m.phase = StageGoodCaptures
		goto startPhase

	case StageGoodCaptures:
		for m.cur < m.end {
			move := m.pickBest()
			if move == m.ttMove {
				continue
			}
			if isBadCapture(m.p, move) {
				m.badCaps[m.badCount] = move
				m.badCount++
				continue
			}
			return move, StageGoodCaptures
		}
		m.phase = StageKiller1
		goto startPhase

	case StageKiller1:
		move := m.killer1
		m.phase = StageKiller2
		if move != 0 && move != m.ttMove &&
			m.p.board[moveTo(move)] == NO_PC && isLegal(m.p, move) {
			return move, StageKiller1
		}
		goto startPhase

	case StageKiller2:
		move := m.killer2
		m.phase = StageGenQuiet
		if move != 0 && move != m.ttMove &&
			m.p.board[moveTo(move)] == NO_PC && isLegal(m.p, move) {
			return move, StageKiller2
		}
		goto startPhase

	case StageGenQuiet:
		m.end = genQuiet(m.p, m.move[:])
		m.cur = 0
		scoreQuiet(m)
		m.phase = StageQuiet
		goto startPhase

	case StageQuiet:
		for m.cur < m.end {
			move := m.pickBest()
			if move == m.ttMove || move == m.killer1 || move == m.killer2 {
				continue
			}
			return move, StageQuiet
		}
		m.badCur = 0
		m.phase = StageBadCaptures
		goto startPhase

	case StageBadCaptures:
		if m.badCur < m.badCount {
			move := m.badCaps[m.badCur]
			m.badCur++
			return move, StageBadCaptures
		}
	}

	m.phase = StageDone
	return 0, StageDone
}

func (m *MovePicker) nextCaptureOrCheck() (int, MoveGenStage) {
startPhase:
	switch m.phase {
	case StageTTMove:
		move := m.ttMove
		m.phase = StageGenCaptures
		if move != 0 && isLegal(m.p, move) {
			return move, StageTTMove
		}
		goto startPhase

	case StageGenCaptures:
		m.end = genCaptures(m.p, m.move[:])
		m.cur = 0
		m.badCount = 0
		scoreCaptures(m)
		m.phase = StageGoodCaptures
		goto startPhase

	case StageGoodCaptures:
		for m.cur < m.end {
			move := m.pickBest()
			if move == m.ttMove {
				continue
			}
			if isBadCapture(m.p, move) {
				m.badCaps[m.badCount] = move
				m.badCount++
				continue
			}
			return move, StageGoodCaptures
		}
		m.phase = StageGenQuiet
		goto startPhase

	case StageGenQuiet:
		m.end = genChecks(m.p, m.move[:])
		m.cur = 0
		scoreQuiet(m)
		m.phase = StageQuiet
		goto startPhase

	case StageQuiet:
		for m.cur < m.end {
			move := m.pickBest()
			if move == m.ttMove || move == m.killer1 || move == m.killer2 {
				continue
			}
			return move, StageQuiet
		}
		m.badCur = 0
		m.phase = StageBadCaptures
		goto startPhase

	case StageBadCaptures:
		if m.badCur < m.badCount {
			move := m.badCaps[m.badCur]
			m.badCur++
			return move, StageBadCaptures
		}
	}

	m.phase = StageDone
	return 0, StageDone
}

// ---- Capture-only picker for quiescence search ----

// initQSearch prepares m to iterate only over captures, for use in
// quiesce().  Bad captures are skipped entirely.
func initQSearch(p *Pos, m *MovePicker) {
	m.p = p
	m.end = genCaptures(p, m.move[:])
	scoreCaptures(m)
	m.cur = 0
}

// nextCapture returns the next good capture, or 0 when done.
func (m *MovePicker) nextCapture() int {
	for m.cur < m.end {
		move := m.pickBest()
		return move
	}
	return 0
}

// ---- Scoring helpers ----

// scoreCaptures assigns MVV-LVA scores to every move in m.move[0..end].
func scoreCaptures(m *MovePicker) {
	for i := 0; i < m.end; i++ {
		m.value[i] = mvvLva(m.p, m.move[i])
	}
}

// scoreQuiet assigns history heuristic scores to quiet moves,
// including 1-ply and 2-ply continuation history.
func scoreQuiet(m *MovePicker) {
	ss := m.ss
	side := m.p.side
	ply := m.ply
	for i := 0; i < m.end; i++ {
		mv := m.move[i]
		from := moveFrom(mv)
		to := moveTo(mv)
		pt := m.p.typeAt(from) // piece type (0-5)
		score := ss.histTable[side][from][to]
		if ply >= 1 && ss.contValid[ply-1] {
			score += ss.contHistMain[ss.contSide[ply-1]][ss.contPiece[ply-1]][ss.contTo[ply-1]][side][pt][to]
		}
		if ply >= 2 && ss.contValid[ply-2] {
			score += ss.contHistMain[ss.contSide[ply-2]][ss.contPiece[ply-2]][ss.contTo[ply-2]][side][pt][to]
		}
		m.value[i] = score
	}
}

// pickBest finds the highest-valued entry in move[cur..end], swaps it
// into position cur, advances cur, and returns the move.  One swap per
// call instead of the bubble approach's up to n-1 swaps.
func (m *MovePicker) pickBest() int {
	best := m.cur
	for i := m.cur + 1; i < m.end; i++ {
		if m.value[i] > m.value[best] {
			best = i
		}
	}
	m.value[m.cur], m.value[best] = m.value[best], m.value[m.cur]
	m.move[m.cur], m.move[best] = m.move[best], m.move[m.cur]
	m.cur++
	return m.move[m.cur-1]
}

// isBadCapture returns true if a capture loses material after the
// full exchange sequence on the target square.  En-passant is never
// considered a bad capture (the pawn trade is always approximately
// equal).
func isBadCapture(p *Pos, move int) bool {
	from := moveFrom(move)
	to := moveTo(move)
	// If the victim is at least as valuable as the attacker, the
	// capture is at least equal, so it can't be "bad."
	if pieceValue[p.typeAt(to)] >= pieceValue[p.typeAt(from)] {
		return false
	}
	if moveType(move) == EP_CAP {
		return false // pawn x pawn is always fair
	}
	return swap(p, from, to) < 0
}

// mvvLva scores a capture move for ordering:
//
//   Most Valuable Victim, Least Valuable Aggressor.
//
//   Regular captures: (victimType * 6) + (5 - attackerType)
//   Promotions:       promType - 5  (queen promotion = -1, etc.)
//   En passant / EP_SET: score 5 (pawn exchange, relatively neutral).
//
// Higher scores are tried first.
func mvvLva(p *Pos, move int) int {
	if p.board[moveTo(move)] != NO_PC {
		return p.typeAt(moveTo(move))*6 + 5 - p.typeAt(moveFrom(move))
	}
	if isProm(move) {
		return promType(move) - 5 // queen=-1, rook=-2, bishop=-3, knight=-4
	}
	return 5 // default (e.g. quiet ep-set)
}

// getCorrection returns the total correction value for the position,
// combining pawn and non-pawn correction history.
func (ss *SearchState) getCorrection(p *Pos) int {
	side := p.side
	pawnIdx := int((p.pawnKey[White] ^ p.pawnKey[Black]) % corrHistSize)
	corr := ss.corrHist[side][pawnIdx] / corrHistGrain
	corr += ss.nonPawnCorrHist[White][side][int(p.nonPawnKey[White]%corrHistSize)] / corrHistGrain
	corr += ss.nonPawnCorrHist[Black][side][int(p.nonPawnKey[Black]%corrHistSize)] / corrHistGrain
	return corr
}

// updateCorrEntry applies a weighted update to a single correction history entry.
func updateCorrEntry(entry *int, newWeight, scaledDiff int) {
	update := (*entry)*(corrHistWeightScale-newWeight) + scaledDiff*newWeight
	*entry = max(-corrHistMax, min(corrHistMax, update/corrHistWeightScale))
}

// addCorrection updates all correction history entries for the position.
func (ss *SearchState) addCorrection(p *Pos, depth, diff int) {
	side := p.side
	newWeight := min(16, 1+depth)
	scaledDiff := diff * corrHistGrain
	pawnIdx := int((p.pawnKey[White] ^ p.pawnKey[Black]) % corrHistSize)
	updateCorrEntry(&ss.corrHist[side][pawnIdx], newWeight, scaledDiff)
	updateCorrEntry(&ss.nonPawnCorrHist[White][side][int(p.nonPawnKey[White]%corrHistSize)], newWeight, scaledDiff)
	updateCorrEntry(&ss.nonPawnCorrHist[Black][side][int(p.nonPawnKey[Black]%corrHistSize)], newWeight, scaledDiff)
}

// histBonus returns the bonus/malus value for a history update at the
// given depth.  depth*depth + 64*depth grows fast enough to matter
// against maxHist=16384, with the cap preventing any single cutoff
// from dominating accumulated signal.
func histBonus(depth int) int {
	return min(depth*depth+64*depth, maxHist/4)
}

// histUpdate applies a signed delta to a history entry using the gravity
// formula: entry += delta - entry*|delta|/maxHist.  This keeps the value
// inside [-maxHist, +maxHist] and lets recent signal gradually displace
// old signal instead of accumulating without bound.
func histUpdate(entry *int, delta int) {
	abs := delta
	if abs < 0 {
		abs = -abs
	}
	*entry += delta - (*entry)*abs/maxHist
}

// isQuiet returns true if move is a quiet move (no capture, no promotion, no EP).
func isQuiet(p *Pos, move int) bool {
	return p.board[moveTo(move)] == NO_PC && !isProm(move) && moveType(move) != EP_CAP
}

// updateHistory records that a quiet move caused a beta cutoff, and
// penalises all quiet moves that were tried before it (quietsTried).
// The bonus rewards the cutoff move; the malus makes failed quiets
// less likely to be tried early in future sibling nodes.
// Killers store the two most recent cutoff moves at this ply.
func (ss *SearchState) updateHistory(p *Pos, move, depth, ply int, quietsTried []int) {
	bonus := histBonus(depth)
	malus := -bonus
	side := p.side

	// Helper: update cont hist entries for a move at 1-ply and 2-ply back.
	updateCont := func(mv, delta int) {
		pt := p.typeAt(moveFrom(mv))
		to := moveTo(mv)
		if ply >= 1 && ss.contValid[ply-1] {
			histUpdate(&ss.contHistMain[ss.contSide[ply-1]][ss.contPiece[ply-1]][ss.contTo[ply-1]][side][pt][to], delta)
		}
		if ply >= 2 && ss.contValid[ply-2] {
			histUpdate(&ss.contHistMain[ss.contSide[ply-2]][ss.contPiece[ply-2]][ss.contTo[ply-2]][side][pt][to], delta)
		}
	}

	// Reward the cutoff move.
	histUpdate(&ss.histTable[side][moveFrom(move)][moveTo(move)], bonus)
	updateCont(move, bonus)

	// Penalise all quiets that were searched but did not cut off.
	for _, tried := range quietsTried {
		if tried == move {
			continue
		}
		histUpdate(&ss.histTable[side][moveFrom(tried)][moveTo(tried)], malus)
		updateCont(tried, malus)
	}

	// Update killers.
	if move != ss.killerMoves[ply][0] {
		ss.killerMoves[ply][1] = ss.killerMoves[ply][0]
		ss.killerMoves[ply][0] = move
	}
}
