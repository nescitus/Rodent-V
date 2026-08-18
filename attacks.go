// ================================================================
// S3  ATTACK DETECTION
// ================================================================
//
//   Three functions answer the core question "is this square safe?":
//
//   attacksFrom(p, sq)
//       Returns a bitboard of squares that the piece ON sq can reach.
//       Used in legality checking to confirm a piece can actually
//       move to its alleged destination.
//
//   attacksTo(p, sq)
//       Returns a bitboard of ALL pieces (both sides) that attack sq.
//       Used as the starting point for static exchange evaluation
//       (see swap.go).
//
//   isAttacked(p, sq, side)
//       Boolean: is sq attacked by any piece of the given side?
//       Used for check detection and castling legality.
//
//   All three rely on the precomputed tables in tables.go.  The key
//   insight is that attack detection is symmetric for sliding pieces:
//   a bishop on A1 attacks H8 if and only if the rook on H8 would
//   see A1, so we shoot a ray FROM the target square and intersect
//   with the actual piece bitboards.
//

package main

import "math/bits"

// attacksFrom returns a bitboard of squares that the piece on sq
// can move to (ignoring legality; the caller filters illegal moves).
// Sliding piece attacks account for the current occupancy.
func attacksFrom(p *Pos, sq int) uint64 {
	occ := p.occupied()
	switch p.typeAt(sq) {
	case P:
		return pawnAtk[colorOf(p.board[sq])][sq]
	case N:
		return knightAtk[sq]
	case B:
		return bishopAttacks(occ, sq)
	case R:
		return rookAttacks(occ, sq)
	case Q:
		return queenAttacks(occ, sq)
	case K:
		return kingAtk[sq]
	}
	return 0
}

// attacksTo returns a bitboard of every piece (of either color) that
// attacks the given square.  This is the "sonar ping" function:
// shoot rays from sq in all directions and see what you hit.
func attacksTo(p *Pos, sq int) uint64 {
	occ := p.occupied()
	return (p.pieceBB(White, P) & pawnAtk[Black][sq]) | // White pawns attack from below
		(p.pieceBB(Black, P) & pawnAtk[White][sq]) | // Black pawns attack from above
		(p.typeBB[N] & knightAtk[sq]) |
		((p.typeBB[B] | p.typeBB[Q]) & bishopAttacks(occ, sq)) |
		((p.typeBB[R] | p.typeBB[Q]) & rookAttacks(occ, sq)) |
		(p.typeBB[K] & kingAtk[sq])
}

// isAttacked reports whether sq is attacked by any piece of side.
// This is a short-circuit version of attacksTo: it returns as soon
// as the first attacker is found, making it faster for check tests.
func isAttacked(p *Pos, sq, side int) bool {
	occ := p.occupied()
	return (p.pieceBB(side, P)&pawnAtk[opp(side)][sq] != 0) ||
		(p.pieceBB(side, N)&knightAtk[sq] != 0) ||
		((p.pieceBB(side, B)|p.pieceBB(side, Q))&bishopAttacks(occ, sq) != 0) ||
		((p.pieceBB(side, R)|p.pieceBB(side, Q))&rookAttacks(occ, sq) != 0) ||
		(p.pieceBB(side, K)&kingAtk[sq] != 0)
}

// CheckInfo caches target check squares and discovered check candidates
// for the current node, avoiding repeated magic bitboard attack lookups across candidate moves.
type CheckInfo struct {
	checkMask    [6]uint64
	dcCandidates uint64
	kingSq       int
	valid        bool
}

func (ci *CheckInfo) init(p *Pos) {
	side := p.side
	enemyKing := p.kingSq[opp(side)]
	ci.kingSq = enemyKing
	occ := p.occupied()

	// 1. Direct check squares for each piece type
	ci.checkMask[P] = pawnAtk[opp(side)][enemyKing]
	ci.checkMask[N] = knightAtk[enemyKing]
	bAtk := bishopAttacks(occ, enemyKing)
	rAtk := rookAttacks(occ, enemyKing)
	ci.checkMask[B] = bAtk
	ci.checkMask[R] = rAtk
	ci.checkMask[Q] = bAtk | rAtk
	ci.checkMask[K] = 0

	// 2. Discovered check candidates: friendly pieces on the rays between our sliders and enemy king
	ci.dcCandidates = 0
	ourPieces := p.colorBB[side]
	ourSliders := ourPieces & (p.typeBB[B] | p.typeBB[R] | p.typeBB[Q])

	if ourSliders != 0 {
		// Diagonal pinners/sliders towards enemy king
		diagSliders := (p.typeBB[B] | p.typeBB[Q]) & ourSliders & bishopAttacks(ourPieces, enemyKing)
		for diagSliders != 0 {
			sq := lsb(diagSliders)
			diagSliders &= diagSliders - 1
			between := BetweenBB[enemyKing][sq] & occ
			if bits.OnesCount64(between) == 1 && (between&ourPieces) != 0 {
				ci.dcCandidates |= between
			}
		}

		// Orthogonal pinners/sliders towards enemy king
		orthoSliders := (p.typeBB[R] | p.typeBB[Q]) & ourSliders & rookAttacks(ourPieces, enemyKing)
		for orthoSliders != 0 {
			sq := lsb(orthoSliders)
			orthoSliders &= orthoSliders - 1
			between := BetweenBB[enemyKing][sq] & occ
			if bits.OnesCount64(between) == 1 && (between&ourPieces) != 0 {
				ci.dcCandidates |= between
			}
		}
	}

	ci.valid = true
}

// Detect whether a move gives check without making it on the board.
// Uses precomputed CheckInfo to evaluate candidate moves with minimal O(1) bitwise operations.
func moveGivesCheck(p *Pos, move int, ci *CheckInfo) bool {
	from := moveFrom(move)
	to := moveTo(move)
	typeOfMove := moveType(move)

	// Castling is rare and involves moving both King and Rook
	if typeOfMove == CASTLE {
		var u Update
		var r Revert
		child := *p
		makeMove(&child, &u, &r, move)
		return child.inCheck()
	}

	if ci == nil {
		var localCi CheckInfo
		localCi.init(p)
		ci = &localCi
	} else if !ci.valid {
		ci.init(p)
	}

	placedPiece := p.typeAt(from)
	if isProm(move) {
		placedPiece = promType(move)
	}

	// 1. Direct check
	if ci.checkMask[placedPiece]&squareBit(to) != 0 {
		return true
	}

	// 2. Discovered check
	if ci.dcCandidates&squareBit(from) != 0 {
		// If the piece does not stay on the same line to the king, it discovered check
		if LineBB[from][ci.kingSq]&squareBit(to) == 0 {
			return true
		}
	}

	// 3. En-passant capture (removes enemy pawn off a different square)
	if typeOfMove == EP_CAP {
		side := p.side
		kingSquare := p.kingSq[opp(side)]
		occAfter := p.occupied() ^ squareBit(from) ^ squareBit(to)
		dir := -8
		if side == Black {
			dir = 8
		}
		occAfter ^= squareBit(to + dir)

		if (bishopAttacks(occAfter, kingSquare)&(p.pieceBB(side, B)|p.pieceBB(side, Q))) != 0 ||
			(rookAttacks(occAfter, kingSquare)&(p.pieceBB(side, R)|p.pieceBB(side, Q))) != 0 {
			return true
		}
	}

	return false
}

