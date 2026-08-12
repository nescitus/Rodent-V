// ================================================================
// S9  HISTORY & KILLER HEURISTICS
// ================================================================
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

// histBonus returns the bonus/malus value for a history update at the
// given depth.  depth*depth + 64*depth grows fast enough to matter
// against maxHist=16384, with the cap preventing any single cutoff
// from dominating accumulated signal.
func histBonus(depth int) int {
	return min(depth*depth+64*depth, maxHist/4)
}

// histUpdate applies a signed bonus to a history entry using the new gravity formula.
// s += (32 * bonus) - (s * abs(bonus)) / gravityDiv
// gravityDiv = 512 + (abs(bonus) >> 4)
func histUpdate(entry *int, bonus int) {
	absBonus := bonus
	if absBonus < 0 {
		absBonus = -absBonus
	}
	gravityDiv := 512 + (absBonus >> 4)
	s := *entry
	s += (32 * bonus) - (s*absBonus)/gravityDiv
	*entry = min(max(s, -maxHist), maxHist)
}

func histUpdateInt16(entry *int16, bonus int) {
	absBonus := bonus
	if absBonus < 0 {
		absBonus = -absBonus
	}
	gravityDiv := 512 + (absBonus >> 4)
	s := int(*entry)
	s += (32 * bonus) - (s*absBonus)/gravityDiv
	*entry = int16(min(max(s, -maxHist), maxHist))
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
			histUpdateInt16(&ss.contHistMain[ss.contSide[ply-1]][ss.contPiece[ply-1]][ss.contTo[ply-1]][side][pt][to], delta)
		}
		if ply >= 2 && ss.contValid[ply-2] {
			histUpdateInt16(&ss.contHistMain[ss.contSide[ply-2]][ss.contPiece[ply-2]][ss.contTo[ply-2]][side][pt][to], delta)
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


