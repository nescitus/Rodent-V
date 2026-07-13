// ================================================================
// S4  MAKE MOVE (COPY-MAKE)
// ================================================================
//
//   We use copy-make strategy. Each move creates a child board
//   on the stack, whereas the board for the current node persists
//   until overwritten by the parent. This strategy is viable
//   as long as Pos record is sufficiently small and it avoids
//   using a complex unmakeMove.
//
//   The most expensive part is nnue accumulator update. We try
//   to defer it, so makeMove() only creates Update struct
//   that remembers which accumulator fields need to be changed.
//   This trick gains roughly 27% in terms of speed.
//
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

func tryKingMove(p *Pos, move int) {

	side := p.side
	from := moveFrom(move)
	to := moveTo(move)
	captType := p.typeAt(to)

	p.board[from] = NO_PC
	p.board[to] = makePiece(side, K)
	p.colorBB[side] ^= squareBit(from) | squareBit(to)
	p.typeBB[K] ^= squareBit(from) | squareBit(to)

	p.kingSq[side] = to

	if captType != NO_TP {
		p.colorBB[opp(side)] ^= squareBit(to)
		p.typeBB[captType] ^= squareBit(to)
	}

	p.side ^= 1
}

// makeMove applies move to position p. The Update record
// contains information about nnue update that should be
// applied before executing any other move (or discarded
// if a move made is pruned or proven illegal before that)

func makeMove(p *Pos, u *Update, move int) {
	side := p.side
	u.dirty = true // accumulator update has not been applied
	u.from = moveFrom(move)
	u.to = moveTo(move)
	u.movingType = p.typeAt(u.from)
	u.captType = p.typeAt(u.to)
	u.color = side
	u.flag = moveType(move)

	// Append current key to the repetition history.
	p.keyHist[p.histLen] = p.key
	p.histLen++

	// 50-move clock: resets on pawn moves and captures.
	if u.movingType == P || u.captType != NO_TP {
		p.clock = 0
	} else {
		p.clock++
	}

	// Update castling rights: strip any rights affected by a piece
	// moving from or being captured on a corner/king square.
	p.key ^= zobCastle[p.castleRights]
	p.castleRights &= castleMask[u.from] & castleMask[u.to]
	p.key ^= zobCastle[p.castleRights]

	// Clear old en-passant key contribution.
	if p.epSquare != NO_SQ {
		p.key ^= zobEP[fileOf(p.epSquare)]
		p.epSquare = NO_SQ
	}

	// --- Move the piece from -> to ---
	p.board[u.from] = NO_PC
	p.board[u.to] = makePiece(side, u.movingType)
	p.key ^= zobPiece[makePiece(side, u.movingType)][u.from] ^
		zobPiece[makePiece(side, u.movingType)][u.to]
	if u.movingType == P {
		p.pawnKey[side] = p.pawnKey[side] ^ zobPiece[makePiece(side, u.movingType)][u.from] ^
			zobPiece[makePiece(side, u.movingType)][u.to]
	} else {
		p.nonPawnKey[side] ^= zobPiece[makePiece(side, u.movingType)][u.from] ^
			zobPiece[makePiece(side, u.movingType)][u.to]
	}
	p.colorBB[side] ^= squareBit(u.from) | squareBit(u.to)
	p.typeBB[u.movingType] ^= squareBit(u.from) | squareBit(u.to)

	// --- Update king square ---
	if u.movingType == K {
		p.kingSq[side] = u.to
	}

	// --- Handle a normal capture at "to" ---
	if u.captType != NO_TP {
		u.capSq = u.to
		p.key ^= zobPiece[makePiece(opp(side), u.captType)][u.to]
		if u.captType == P {
			p.pawnKey[opp(side)] = p.pawnKey[opp(side)] ^ zobPiece[makePiece(opp(side), u.captType)][u.to]
		} else if u.captType != K {
			p.nonPawnKey[opp(side)] ^= zobPiece[makePiece(opp(side), u.captType)][u.to]
		}
		p.colorBB[opp(side)] ^= squareBit(u.to)
		p.typeBB[u.captType] ^= squareBit(u.to)
		p.count[opp(side)][u.captType]--
	}

	// --- Special move type handling ---
	switch moveType(move) {
	case NORMAL:
		// Nothing extra to do.

	case CASTLE:
		// Move the rook atomically with the king.
		// King side: rook goes from +3 to +(-1) relative to king.
		// Queen side: rook goes from -4 to +(+1) relative to king.
		if u.to > u.from {
			u.rookFrom = u.from + 3 // H-file rook
			u.rookTo = u.to - 1     // F-file landing
		} else {
			u.rookFrom = u.from - 4 // A-file rook
			u.rookTo = u.to + 1     // D-file landing
		}

		p.board[u.rookFrom] = NO_PC
		p.board[u.rookTo] = makePiece(side, R)
		p.key ^= zobPiece[makePiece(side, R)][u.rookFrom] ^
			zobPiece[makePiece(side, R)][u.rookTo]
		p.nonPawnKey[side] ^= zobPiece[makePiece(side, R)][u.rookFrom] ^
			zobPiece[makePiece(side, R)][u.rookTo]
		p.colorBB[side] ^= squareBit(u.rookFrom) | squareBit(u.rookTo)
		p.typeBB[R] ^= squareBit(u.rookFrom) | squareBit(u.rookTo)

	case EP_CAP:
		// The captured pawn sits one square behind "to" (XOR 8 flips rank).
		capSq := u.to ^ 8
		u.capSq = capSq
		p.board[capSq] = NO_PC
		p.key ^= zobPiece[makePiece(opp(side), P)][capSq]
		p.pawnKey[opp(side)] = p.pawnKey[opp(side)] ^ zobPiece[makePiece(opp(side), P)][capSq]
		p.colorBB[opp(side)] ^= squareBit(capSq)
		p.typeBB[P] ^= squareBit(capSq)
		p.count[opp(side)][P]--

	case EP_SET:
		// Double pawn push: record the en-passant square if an enemy
		// pawn can actually capture there next move.
		epSq := u.to ^ 8
		if pawnAtk[side][epSq]&p.pieceBB(opp(side), P) != 0 {
			p.epSquare = epSq
			p.key ^= zobEP[fileOf(epSq)]
		}

	case N_PROM, B_PROM, R_PROM, Q_PROM:
		// Promotion: change piece on target square
		promotedType := promType(move)
		u.prom = promotedType
		p.board[u.to] = makePiece(side, promotedType)
		p.key ^= zobPiece[makePiece(side, P)][u.to] ^
			zobPiece[makePiece(side, promotedType)][u.to]
		p.pawnKey[side] ^= zobPiece[makePiece(side, P)][u.to]
		p.nonPawnKey[side] ^= zobPiece[makePiece(side, promotedType)][u.to]
		p.typeBB[P] ^= squareBit(u.to)
		p.typeBB[promotedType] ^= squareBit(u.to)
		p.count[side][promotedType]++
		p.count[side][P]--
	}

	p.side ^= 1
	p.key ^= sideKey
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
func makeNullMove(p *Pos) {

	p.keyHist[p.histLen] = p.key
	p.histLen++
	p.clock++ // null moves advance the 50-move clock

	if p.epSquare != NO_SQ {
		p.key ^= zobEP[fileOf(p.epSquare)]
		p.epSquare = NO_SQ
	}

	p.side ^= 1
	p.key ^= sideKey
}
