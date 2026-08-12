// ================================================================
// S10 STATIC EXCHANGE EVALUATION (SEE)
// ================================================================
//
//   swap(p, from, to) computes the net material gain or loss of
//   capturing on square "to" with the piece on "from", assuming both
//   sides recapture with their least valuable available attacker at
//   each step.
//
//   WHY SEE?
//   --------
//   Move ordering sends captures to the front of the search (good for
//   pruning), but sending losing captures first wastes time on moves
//   that immediately fail low.  SEE lets us cheaply classify a capture
//   as "good" (gain >= 0) or "bad" (gain < 0) before committing to a
//   makeMove / unmakeMove cycle.
//
//   ALGORITHM
//   ---------
//   We simulate the exchange sequence on a scratch board:
//
//   1. score[0] = value of the piece currently on "to" (our gain if
//      we capture and the opponent declines to recapture).
//   2. Remove the capturing piece from the occupancy bitboard.
//   3. Find the opponent's least-valuable piece that attacks "to."
//      If none, the exchange is over.
//   4. score[ply] = value of the piece we just placed on "to" minus
//      score[ply-1] (what the opponent gains by taking it).
//   5. Remove that attacker; add any X-ray attackers it may have been
//      blocking (sliders behind it now become visible).
//   6. Alternate sides; increment ply.
//
//   After the loop, work backwards: at each step, the player chooses
//   whether to continue the exchange or stop.  They stop if continuing
//   would make them worse off:
//
//       score[ply-1] = -max(-score[ply-1], score[ply])
//
//   This is the negamax form of "take the best of (stop, continue)."
//
//   KING HANDLING
//   -------------
//   If the current side would have to expose its king, we record INF
//   as the gain (we would rather stop) and break immediately.  This
//   prevents SEE from considering suicidal king captures.
//

package main

// swap returns the static exchange evaluation for capturing the piece
// on "to" with the piece on "from."  A positive return value means
// the capture is expected to gain material.
func swap(p *Pos, from, to int) int {
	var score [32]int
	score[0] = pieceValue[p.typeAt(to)]

	attType := p.typeAt(from)
	occ := p.occupied() ^ squareBit(from) // remove the initial attacker

	// Gather all attackers to "to" (both sides), then exclude the
	// capturing piece which is no longer on the board.
	attackers := attacksTo(p, to)
	attackers |= (bishopAttacks(occ, to) & (p.typeBB[B] | p.typeBB[Q])) |
		(rookAttacks(occ, to) & (p.typeBB[R] | p.typeBB[Q]))
	attackers &= occ

	side := opp(p.side) // opponent moves first (they recapture)
	ply := 1

	for attackers&p.colorBB[side] != 0 {
		if attType == K {
			// King cannot capture if the square is defended. Stop here.
			score[ply] = inf
			ply++
			break
		}
		// Score this ply: gain from taking the previous piece,
		// minus what the opponent gained before us.
		score[ply] = tpValue(attType) - score[ply-1]

		// Find the least valuable attacker for the current side.
		var candidates uint64
		attType = P
		for attType <= K {
			candidates = p.pieceBB(side, attType) & attackers
			if candidates != 0 {
				break
			}
			attType++
		}

		// Remove one attacker (the LSB of candidates) and reveal any
		// X-ray attackers behind it.  Only recompute rays that pass
		// through both the removed square and the target square —
		// a removed piece can only unblock sliders it was aligned with.
		lsbBit := candidates & (^candidates + 1)
		sq := lsb(uint64(lsbBit))
		occ ^= lsbBit
		toBit := squareBit(to)
		if lineMasks[0][sq]&toBit != 0 || lineMasks[1][sq]&toBit != 0 {
			attackers |= rookAttacks(occ, to) & (p.typeBB[R] | p.typeBB[Q])
		}
		if lineMasks[2][sq]&toBit != 0 || lineMasks[3][sq]&toBit != 0 {
			attackers |= bishopAttacks(occ, to) & (p.typeBB[B] | p.typeBB[Q])
		}
		attackers &= occ

		side ^= 1
		ply++
	}

	// Work backwards: each side chooses optimally whether to stop or
	// continue.  The negamax form:
	//   score[ply-1] = -max(-score[ply-1], score[ply])
	//                = min(score[ply-1], -score[ply])
	for ply--; ply > 0; ply-- {
		score[ply-1] = -maxOf(-score[ply-1], score[ply])
	}
	return score[0]
}

// tpValue is a local alias for pieceValue to avoid repetition in swap.
// (swap uses this in a tight loop, and the name reads more naturally.)
func tpValue(t int) int { return pieceValue[t] }
