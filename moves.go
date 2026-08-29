// ================================================================
// S4  MAKE / UNMAKE MOVE
// ================================================================
//
//   makeMove() and unmakeMove() are the engine's most critical
//   functions.  Every field in Pos that is kept incrementally
//   (bitboards, piece array, material, PST score, Zobrist key,
//   castling rights, en-passant square) must be updated here.
//
//   DESIGN PRINCIPLE
//   ----------------
//   We do NOT copy the full position before each move.  Instead,
//   makeMove() saves just the fields that cannot be reconstructed
//   from the move alone into an Undo struct, then unmakeMove()
//   restores them.  This is the "incremental update" style, and it
//   is faster than full copying because most fields (bitboards, PST
//   scores, material) can be recomputed by reversing the same
//   arithmetic.
//
//   NNUE accumulator, on the other hand, is updated on making a move
//   and copied on unmaking.
//
//   MOVE TYPES
//   ----------
//   NORMAL: move the piece, remove a captured piece (if any).
//   CASTLE: move king AND rook atomically; the rook lands on the
//           square between king's old and new position.
//   EP_CAP: the captured pawn is NOT on "to" but one square
//           behind it (to ^ 8 toggles rank by 1).
//   EP_SET: double pawn push; set epSquare for next ply.
//   ?_PROM: the pawn on "from" becomes a new piece on "to".
//
//   NULL MOVE
//   ---------
//   makeNullMove() passes the turn without moving any piece.  It is
//   used in null-move pruning (see search.go).  The Zobrist key still
//   gets the sideKey XOR; the en-passant key is cleared if needed.
//

package main

// makeMove applies move to position p. The Update record
// contains information about nnue update that should be
// applied before executing any other move (or discarded
// if a move made is pruned or proven illegal before that)
// plus data for unmaking a move

func makeMove(p *Pos, u *Update, r *Revert, move int) {
	side := p.side
	enemy := opp(side)

	r.oldKey = p.key
	r.oldPawnKey = p.pawnKey
	r.oldNonPawnKey = p.nonPawnKey
	r.oldMinorKey = p.minorKey
	r.oldMajorKey = p.majorKey
	r.oldCastleRights = p.castleRights
	r.oldEpSquare = p.epSquare
	r.oldClock = p.clock
	r.oldHistLen = p.histLen
	r.flag = moveType(move)

	u.dirty = true // accumulator update has not been applied
	u.from = moveFrom(move)
	u.to = moveTo(move)
	u.movingType = p.typeAt(u.from)
	u.captType = p.typeAt(u.to)
	u.color = side
	u.flag = uNORMAL

	// Append current key to the repetition history.
	p.keyHist[p.histLen] = p.key
	p.histLen++

	if moveType(move) == CASTLE {
		u.captType = NO_TP // FRC Castling is not a capture, even if it lands on a friendly rook
	}

	// 50-move clock: resets on pawn moves and captures.
	if u.movingType == P || u.captType != NO_TP {
		p.clock = 0
	} else {
		p.clock++
	}

	// Update castling rights using dynamic castleMask.
	p.key ^= zobCastle[p.castleRights]
	p.castleRights &= p.castleMask[u.from] & p.castleMask[u.to]
	p.key ^= zobCastle[p.castleRights]

	// Clear old en-passant key contribution.
	if p.epSquare != NO_SQ {
		p.key ^= zobEP[fileOf(p.epSquare)]
		p.epSquare = NO_SQ
	}

	// --- Move the piece from -> to ---
	if moveType(move) != CASTLE {
		p.board[u.from] = NO_PC
		p.board[u.to] = makePiece(side, u.movingType)
	
		hashDelta := zobPiece[makePiece(side, u.movingType)][u.from] ^
			zobPiece[makePiece(side, u.movingType)][u.to]
	
		p.key ^= hashDelta
		if u.movingType == P {
			p.pawnKey[side] ^= hashDelta
		} else {
			p.nonPawnKey[side] ^= hashDelta
			if u.movingType == N || u.movingType == B || u.movingType == K {
				p.minorKey[side] ^= hashDelta
			}
			if u.movingType == R || u.movingType == Q || u.movingType == K {
				p.majorKey[side] ^= hashDelta
			}
		}
	
		p.colorBB[side] ^= squareBit(u.from) | squareBit(u.to)
		p.typeBB[u.movingType] ^= squareBit(u.from) | squareBit(u.to)
	
		// --- Update king square ---
		if u.movingType == K {
			p.kingSq[side] = u.to
		}
	}

	// --- Handle a normal capture at "to" ---
	if u.captType != NO_TP {
		u.capSq = u.to
		hashDelta := zobPiece[makePiece(enemy, u.captType)][u.to]
		p.key ^= hashDelta
		if u.captType == P {
			p.pawnKey[enemy] ^= hashDelta
		} else if u.captType != K {
			p.nonPawnKey[enemy] ^= hashDelta
		}
		if u.captType == N || u.movingType == B {
			p.minorKey[side] ^= hashDelta
		}
		if u.captType == R || u.movingType == Q {
			p.majorKey[side] ^= hashDelta
		}
		p.colorBB[enemy] ^= squareBit(u.to)
		p.typeBB[u.captType] ^= squareBit(u.to)
		p.count[enemy][u.captType]--
		if isProm(move) {
			u.flag = uPROMCAPT
		} else {
			u.flag = uCAPTURE
		}
	}

	// --- Special move type handling ---
	switch moveType(move) {
	case NORMAL:
		// Nothing extra to do.

	case CASTLE:
		// In FRC, from is king (e.g. E1) and to is rook (e.g. H1).
		// We must move King to kingDst and Rook to rookDst.
		// We remove both first, then place both, to handle overlapping sqs.
		u.flag = uCASTLE
		
		var kingDst, rookDst int
		if side == White {
			if u.to == p.castlingRookSq[White][0] { kingDst, rookDst = G1, F1 } else { kingDst, rookDst = C1, D1 }
		} else {
			if u.to == p.castlingRookSq[Black][0] { kingDst, rookDst = G8, F8 } else { kingDst, rookDst = C8, D8 }
		}
		u.rookFrom = u.to
		u.rookTo = rookDst
		u.to = kingDst
		
		// 1. Remove King
		p.board[u.from] = NO_PC
		p.colorBB[side] ^= squareBit(u.from)
		p.typeBB[K] ^= squareBit(u.from)
		hashDeltaKFrom := zobPiece[makePiece(side, K)][u.from]
		p.key ^= hashDeltaKFrom
		p.nonPawnKey[side] ^= hashDeltaKFrom
		p.minorKey[side] ^= hashDeltaKFrom
		p.majorKey[side] ^= hashDeltaKFrom
		
		// 2. Remove Rook
		p.board[u.rookFrom] = NO_PC
		p.colorBB[side] ^= squareBit(u.rookFrom)
		p.typeBB[R] ^= squareBit(u.rookFrom)
		hashDeltaRFrom := zobPiece[makePiece(side, R)][u.rookFrom]
		p.key ^= hashDeltaRFrom
		p.nonPawnKey[side] ^= hashDeltaRFrom
		p.majorKey[side] ^= hashDeltaRFrom
		
		// 3. Place King
		p.board[kingDst] = makePiece(side, K)
		p.colorBB[side] ^= squareBit(kingDst)
		p.typeBB[K] ^= squareBit(kingDst)
		hashDeltaKTo := zobPiece[makePiece(side, K)][kingDst]
		p.key ^= hashDeltaKTo
		p.nonPawnKey[side] ^= hashDeltaKTo
		p.minorKey[side] ^= hashDeltaKTo
		p.majorKey[side] ^= hashDeltaKTo
		p.kingSq[side] = kingDst
		
		// 4. Place Rook
		p.board[u.rookTo] = makePiece(side, R)
		p.colorBB[side] ^= squareBit(u.rookTo)
		p.typeBB[R] ^= squareBit(u.rookTo)
		hashDeltaRTo := zobPiece[makePiece(side, R)][u.rookTo]
		p.key ^= hashDeltaRTo
		p.nonPawnKey[side] ^= hashDeltaRTo
		p.majorKey[side] ^= hashDeltaRTo

	case EP_CAP:
		// The captured pawn sits one square behind "to" (XOR 8 flips rank).
		u.flag = uEP_CAP
		capSq := u.to ^ 8
		u.capSq = capSq
		p.board[capSq] = NO_PC
		p.key ^= zobPiece[makePiece(enemy, P)][capSq]
		p.pawnKey[enemy] = p.pawnKey[enemy] ^ zobPiece[makePiece(enemy, P)][capSq]
		p.colorBB[enemy] ^= squareBit(capSq)
		p.typeBB[P] ^= squareBit(capSq)
		p.count[enemy][P]--

	case EP_SET:
		// Double pawn push: record the en-passant square if an enemy
		// pawn can actually capture there next move.
		epSq := u.to ^ 8
		if pawnAtk[side][epSq]&p.pieceBB(enemy, P) != 0 {
			p.epSquare = epSq
			p.key ^= zobEP[fileOf(epSq)]
		}

	case N_PROM, B_PROM, R_PROM, Q_PROM:
		// Promotion: change piece on target square
		if u.flag != uPROMCAPT {
			u.flag = uPROMO
		}
		promotedType := promType(move)
		u.prom = promotedType
		p.board[u.to] = makePiece(side, promotedType)
		p.key ^= zobPiece[makePiece(side, P)][u.to] ^
			zobPiece[makePiece(side, promotedType)][u.to]
		p.pawnKey[side] ^= zobPiece[makePiece(side, P)][u.to]
		p.nonPawnKey[side] ^= zobPiece[makePiece(side, promotedType)][u.to]
		if promotedType == B || promotedType == N {
			p.minorKey[side] ^= zobPiece[makePiece(side, promotedType)][u.to]
		}
		if promotedType == R || promotedType == Q {
			p.majorKey[side] ^= zobPiece[makePiece(side, promotedType)][u.to]
		}
		p.typeBB[P] ^= squareBit(u.to)
		p.typeBB[promotedType] ^= squareBit(u.to)
		p.count[side][promotedType]++
		p.count[side][P]--
	}

	p.side ^= 1
	p.key ^= sideKey
}

func isPromotionFlag(flag int) bool {
	return (flag == Q_PROM || flag == R_PROM || flag == B_PROM || flag == N_PROM)
}

func unmakeMove(p *Pos, u *Update, r *Revert) {
	side := u.color
	enemy := opp(side)

	from := u.from
	to := u.to

	fromBB := squareBit(from)
	toBB := squareBit(to)

	// The piece currently on "to" may be a promoted piece rather
	// than the original moving pawn.
	pieceOnTo := u.movingType
	if isPromotionFlag(r.flag) {
		pieceOnTo = u.prom
	}

	if u.flag == uCASTLE {
		kingDst := u.to
		
		p.board[kingDst] = NO_PC
		p.colorBB[side] ^= squareBit(kingDst)
		p.typeBB[K] ^= squareBit(kingDst)
		p.kingSq[side] = from
		
		p.board[u.rookTo] = NO_PC
		p.colorBB[side] ^= squareBit(u.rookTo)
		p.typeBB[R] ^= squareBit(u.rookTo)
		
		p.board[from] = makePiece(side, K)
		p.colorBB[side] ^= squareBit(from)
		p.typeBB[K] ^= squareBit(from)
		
		p.board[u.rookFrom] = makePiece(side, R)
		p.colorBB[side] ^= squareBit(u.rookFrom)
		p.typeBB[R] ^= squareBit(u.rookFrom)
	} else {
		p.board[to] = NO_PC
		p.board[from] = makePiece(side, u.movingType)
	
		p.colorBB[side] ^= fromBB | toBB
	
		if pieceOnTo == u.movingType {
			p.typeBB[u.movingType] ^= fromBB | toBB
		} else {
			// Promotion: remove promoted piece from "to" and restore pawn on "from".
			p.typeBB[pieceOnTo] ^= toBB
			p.typeBB[u.movingType] ^= fromBB
	
			p.count[side][pieceOnTo]--
			p.count[side][P]++
		}
	
		if u.movingType == K {
			p.kingSq[side] = from
		}
	
		// --- Restore destination/captured piece ---
		switch u.flag {
		case uEP_CAP:
			// Destination was empty before the move.
			p.board[to] = NO_PC
	
			capSq := u.capSq
			capBB := squareBit(capSq)
	
			p.board[capSq] = makePiece(enemy, P)
			p.colorBB[enemy] ^= capBB
			p.typeBB[P] ^= capBB
			p.count[enemy][P]++
	
		default:
			if u.captType != NO_TP {
				p.board[to] = makePiece(enemy, u.captType)
				p.colorBB[enemy] ^= toBB
				p.typeBB[u.captType] ^= toBB
				p.count[enemy][u.captType]++
			} else {
				p.board[to] = NO_PC
			}
		}
	}

	// --- Restore all scalar/hash state exactly ---
	p.side = side
	p.key = r.oldKey
	p.pawnKey = r.oldPawnKey
	p.nonPawnKey = r.oldNonPawnKey
	p.minorKey = r.oldMinorKey
	p.majorKey = r.oldMajorKey
	p.castleRights = r.oldCastleRights
	p.epSquare = r.oldEpSquare
	p.clock = r.oldClock
	p.histLen = r.oldHistLen
}

// ================================================================
// NULL MOVE
// ================================================================
//
//   A null move simply flips the side to move without touching any
//   piece.  It is used by null-move pruning: if the position is so
//   good that even "doing nothing" causes a beta cutoff, we can
//   prune the branch.
//
//   Only the Zobrist key and en-passant state need updating; the
//   rest of the position is unchanged. Even nnue accumulator stays
//   as is.
//

// makeNullMove passes the turn without moving.
func makeNullMove(p *Pos) int {
	oldEP := p.epSquare

	p.keyHist[p.histLen] = p.key
	p.histLen++
	p.clock++

	if oldEP != NO_SQ {
		p.key ^= zobEP[fileOf(oldEP)]
		p.epSquare = NO_SQ
	}

	p.side ^= 1
	p.key ^= sideKey

	return oldEP
}

func unmakeNullMove(p *Pos, oldEP int) {
	p.side ^= 1
	p.key ^= sideKey

	if oldEP != NO_SQ {
		p.epSquare = oldEP
		p.key ^= zobEP[fileOf(oldEP)]
	}

	p.clock--
	p.histLen--
}
