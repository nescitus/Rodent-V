// ================================================================
// S10 STATIC EXCHANGE EVALUATION (SEE)
// ================================================================
//
//   see(p, move, threshold) evaluates whether a capture (or move)
//   gains at least the specified threshold in material.
//
//   WHY SEE?
//   --------
//   Move ordering sends captures to the front of the search (good for
//   pruning), but sending losing captures first wastes time on moves
//   that immediately fail low.  SEE lets us cheaply classify a capture
//   as "good" or "bad" before committing to a makeMove/unmakeMove cycle.
//   It is also heavily used in pruning (e.g. SEE pruning in QSearch).
//
//   ALGORITHM (Bitboard Negamax)
//   ---------
//   Unlike older iterative simulated exchanges, this implementation uses
//   a purely bitboard-based negamax algorithm for extreme performance.
//
//   1. Establish an initial 'balance' based on the target piece's value
//      minus the threshold. If balance < 0, it fails immediately.
//   2. If the balance is positive even after losing the attacking piece,
//      it succeeds immediately (early exit).
//   3. Compute an `attackers` bitboard containing all pieces attacking
//      the target square using an updated occupancy bitboard.
//   4. Loop: the opponent (and then we) takes turns recapturing with
//      their least valuable piece.
//   5. Reveal X-ray attackers (sliders behind the moved piece) using
//      magic bitboards.
//   6. The balance flips its sign at each ply (negamax: `balance = -balance - 1 - value`).
//   7. If the balance becomes >= 0, the current side stops the exchange.
//
//   KING HANDLING
//   -------------
//   If a King captures, and the square is still defended by the opponent,
//   the side flipping logic ensures it is correctly classified as a
//   losing exchange (since the King cannot be captured).
//

package main

import "math/bits"

// see returns true if the Static Exchange Evaluation (SEE) of a move
// is greater than or equal to the given threshold.
// A threshold of 0 means the move doesn't lose material.
func see(p *Pos, move int, threshold int) bool {
	from := moveFrom(move)
	to := moveTo(move)
	typeOfMove := moveType(move)
	isEp := typeOfMove == EP_CAP
	prom := 0
	if isProm(move) {
		prom = promType(move)
	}

	nextVictim := p.typeAt(from)
	if prom != 0 {
		nextVictim = prom
	}

	// Balance is the immediate gain minus the threshold.
	balance := pieceValue[p.typeAt(to)] - threshold
	if isEp {
		balance = pieceValue[P] - threshold
	} else if prom != 0 {
		balance += pieceValue[prom] - pieceValue[P]
	}

	// Best case still fails to beat the threshold
	if balance < 0 {
		return false
	}

	// Worst case is losing the moved piece (and not getting any compensation).
	balance -= pieceValue[nextVictim]

	// If the balance is positive even if losing the moved piece,
	// the exchange is guaranteed to beat the threshold.
	if balance >= 0 {
		return true
	}

	bishops := p.typeBB[B] | p.typeBB[Q]
	rooks := p.typeBB[R] | p.typeBB[Q]

	// Simulate the move by updating occupancy.
	occupied := (p.occupied() ^ squareBit(from)) | squareBit(to)
	if isEp {
		occupied ^= squareBit(p.epSquare ^ 8) // epSquare is the square behind the pawn
	}

	// Get all attackers to the destination square
	attackers := seeAttackers(p, to, occupied)

	// Filter attackers by absolute pins
	blackKingRay := LineBB[p.kingSq[Black]][to]
	whiteKingRay := LineBB[p.kingSq[White]][to]

	blackPinned := getPinned(p, Black, occupied)
	whitePinned := getPinned(p, White, occupied)

	allowed := ^(blackPinned | whitePinned) | (blackPinned & blackKingRay) | (whitePinned & whiteKingRay)
	attackers = (attackers & allowed) & occupied

	color := opp(p.side)

	for {
		myAttackers := attackers & p.colorBB[color]
		if myAttackers == 0 {
			break
		}

		// Find the least valuable attacker
		nextVictim = P
		for nextVictim <= K {
			if (myAttackers & p.typeBB[nextVictim]) != 0 {
				break
			}
			nextVictim++
		}

		// Remove this attacker from the occupied
		occupied ^= squareBit(lsb(myAttackers & p.typeBB[nextVictim]))

		// A diagonal move may reveal bishop or queen attackers
		if nextVictim == P || nextVictim == B || nextVictim == Q {
			attackers |= bishopAttacks(occupied, to) & bishops
		}
		// A straight move may reveal rook or queen attackers
		if nextVictim == R || nextVictim == Q {
			attackers |= rookAttacks(occupied, to) & rooks
		}

		attackers &= occupied
		color = opp(color)

		balance = -balance - 1 - pieceValue[nextVictim]
		if balance >= 0 {
			if nextVictim == K && (attackers & p.colorBB[color]) != 0 {
				color = opp(color)
			}
			break
		}
	}

	return color != p.side
}

// seeAttackers calculates all pieces attacking the given square with a custom occupancy bitboard.
// This is necessary for SEE to properly resolve x-ray attacks as pieces capture and leave the board.
func seeAttackers(p *Pos, sq int, occ uint64) uint64 {
	return (p.pieceBB(White, P) & pawnAtk[Black][sq]) |
		(p.pieceBB(Black, P) & pawnAtk[White][sq]) |
		(p.typeBB[N] & knightAtk[sq]) |
		((p.typeBB[B] | p.typeBB[Q]) & bishopAttacks(occ, sq)) |
		((p.typeBB[R] | p.typeBB[Q]) & rookAttacks(occ, sq)) |
		(p.typeBB[K] & kingAtk[sq])
}

// getPinned calculates the bitboard of pieces of the given side that are absolutely pinned to their king.
func getPinned(p *Pos, side int, occ uint64) uint64 {
	kingSq := p.kingSq[side]
	us := p.colorBB[side] & occ
	them := p.colorBB[opp(side)] & occ

	var pinned uint64

	// Orthogonal pinners (Rooks, Queens)
	pinners := (p.typeBB[R] | p.typeBB[Q]) & them & rookAttacks(them, kingSq)
	for pinners != 0 {
		sq := lsb(pinners)
		pinners ^= squareBit(sq)
		ray := BetweenBB[kingSq][sq] & occ
		if bits.OnesCount64(ray) == 1 && (ray&us) != 0 {
			pinned |= ray
		}
	}

	// Diagonal pinners (Bishops, Queens)
	pinners = (p.typeBB[B] | p.typeBB[Q]) & them & bishopAttacks(them, kingSq)
	for pinners != 0 {
		sq := lsb(pinners)
		pinners ^= squareBit(sq)
		ray := BetweenBB[kingSq][sq] & occ
		if bits.OnesCount64(ray) == 1 && (ray&us) != 0 {
			pinned |= ray
		}
	}

	return pinned
}
