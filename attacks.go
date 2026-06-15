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

// Detect whether a move gives check
// without making it on the board
func moveGivesCheck(p *Pos, move int) bool {
	var checks uint64

	side := p.side
	from := moveFrom(move)
	to := moveTo(move)
	fromType := p.typeAt(from) // moving piece
	toType := p.typeAt(to)     // captured piece, if any
	typeOfMove := moveType(move)

	// What piece is placed on the destination square?
	placedPiece := fromType
	if isProm(move) {
		placedPiece = promType(move)
	}

	// Locate enemy king
	kingSquare := p.kingSq[opp(side)]

	// Direct checks by a pawn
	if placedPiece == P {
		checks = shiftForward(shiftSides(squareBit(kingSquare)), opp(side))
		if checks&squareBit(to) != 0 {
			return true
		}
	}

	// Direct checks by a knight
	if placedPiece == N {
		checks = knightAtk[to]
		if checks&squareBit(kingSquare) != 0 {
			return true
		}
	}

	// Prepare occupancy after the move.
	// Remove moving piece from origin...
	occAfter := p.occupied() ^ squareBit(from)

	// ...place moving piece on destination.
	occAfter |= squareBit(to)

	// En passant: captured pawn is not on `to`, remove it explicitly.
	if typeOfMove == EP_CAP {
		dir := -8
		if side == Black {
			dir = 8
		}
		occAfter ^= squareBit(to + dir)
	}

	// For direct slider checks from the moved piece, remove that piece
	// itself from occupancy when generating attacks from `to`.
	occForMovedPiece := occAfter ^ squareBit(to)

	// Direct diagonal checks
	if placedPiece == B || placedPiece == Q {
		checks = bishopAttacks(occForMovedPiece, to)
		if checks&squareBit(kingSquare) != 0 {
			return true
		}
	}

	// Direct orthogonal checks
	if placedPiece == R || placedPiece == Q {
		checks = rookAttacks(occForMovedPiece, to)
		if checks&squareBit(kingSquare) != 0 {
			return true
		}
	}

	// Discovered checks use occupancy after the move.
	occ := occAfter

	// Diagonal discovered checks
	checks = bishopAttacks(occ, kingSquare)
	if checks&(p.pieceBB(side, B)|p.pieceBB(side, Q)) != 0 {
		return true
	}

	// Orthogonal discovered checks
	checks = rookAttacks(occ, kingSquare)
	if checks&(p.pieceBB(side, R)|p.pieceBB(side, Q)) != 0 {
		return true
	}

	// Horizontal checks discovered by castling
	// (we make and unmake a move, as it's rare enough
	// and writing out correct conditions would be hard)
	if typeOfMove == CASTLE {
		var u Undo
		makeMove(p, move, &u)
		isInCheck := p.inCheck()
		unmakeMove(p, move, &u)
		return isInCheck
	}

	// toType is intentionally kept, even though it is no longer needed in
	// this version, because it can be useful for debugging / future tweaks.
	_ = toType

	return false
}
