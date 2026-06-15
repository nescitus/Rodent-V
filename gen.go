// ================================================================
// S5  MOVE GENERATION
// ================================================================
//
//   Two functions produce all pseudo-legal moves for the side to move:
//
//   genCaptures(p, list) -> n
//       All captures, en-passant captures, and promotions (including
//       quiet promotions, which are high-value and must not be missed).
//       Returns n, the number of moves written.
//
//   genQuiet(p, list) -> n
//       All non-capturing, non-promoting moves: pawn pushes, castling,
//       and piece moves to empty squares.
//
//   "Pseudo-legal" means we generate moves without checking whether
//   they leave our own king in check.  The search loop calls
//   selfInCheck() after makeMove() and skips illegal moves.
//
//   MOVE ENCODING REMINDER
//   ----------------------
//   move = (moveType << 12) | (toSq << 6) | fromSq
//   NORMAL moves omit the type (bits 15..12 = 0).
//
//   BITBOARD ITERATION PATTERN
//   --------------------------
//   The idiomatic way to loop over set bits in a bitboard:
//
//       for bb != 0 {
//           sq := lsb(bb)        // index of lowest set bit
//           ... process sq ...
//           bb &= bb - 1         // clear that bit
//       }
//
//   This "reset LSB" trick is O(1) and avoids a separate shift.
//
//   PAWN GENERATION DETAIL
//   ----------------------
//   Pawns are the trickiest piece because they:
//     - attack diagonally but move straight
//     - have a special first-move double push
//     - promote on rank 1 (Black) or rank 8 (White)
//     - can capture en passant
//
//   We handle all cases with bitboard shifts and rank masks.
//   White's "forward" is <<8 (increasing square index); Black's is >>8.
//   Diagonal captures are +/-7 or +/-9 squares.
//

package main

// genCaptures fills list with all captures, en-passant captures, and
// promotions for the side to move, then returns the count.
func genCaptures(p *Pos, list []int) int {
	n    := 0
	side := p.side
	occ  := p.occupied()

	// ---- Pawn captures and promotions ----
	if side == White {
		// Promotion-captures to the left (toward file A).
		// Source: White pawns on rank 7, not on file A, shifted +7.
		bb := ((p.pieceBB(White, P) & ^fileABB & rank7BB) << 7) & p.colorBB[Black]
		for bb != 0 {
			to := lsb(bb)
			list[n] = (Q_PROM << 12) | (to << 6) | (to - 7); n++
			list[n] = (R_PROM << 12) | (to << 6) | (to - 7); n++
			list[n] = (B_PROM << 12) | (to << 6) | (to - 7); n++
			list[n] = (N_PROM << 12) | (to << 6) | (to - 7); n++
			bb &= bb - 1
		}
		// Promotion-captures to the right (toward file H).
		bb = ((p.pieceBB(White, P) & ^fileHBB & rank7BB) << 9) & p.colorBB[Black]
		for bb != 0 {
			to := lsb(bb)
			list[n] = (Q_PROM << 12) | (to << 6) | (to - 9); n++
			list[n] = (R_PROM << 12) | (to << 6) | (to - 9); n++
			list[n] = (B_PROM << 12) | (to << 6) | (to - 9); n++
			list[n] = (N_PROM << 12) | (to << 6) | (to - 9); n++
			bb &= bb - 1
		}
		// Quiet promotions (pawn on rank 7 pushes straight to rank 8).
		bb = ((p.pieceBB(White, P) & rank7BB) << 8) & p.empty()
		for bb != 0 {
			to := lsb(bb)
			list[n] = (Q_PROM << 12) | (to << 6) | (to - 8); n++
			list[n] = (R_PROM << 12) | (to << 6) | (to - 8); n++
			list[n] = (B_PROM << 12) | (to << 6) | (to - 8); n++
			list[n] = (N_PROM << 12) | (to << 6) | (to - 8); n++
			bb &= bb - 1
		}
		// Normal captures left (no rank-7 pawns, no file-A pawns).
		bb = ((p.pieceBB(White, P) & ^fileABB & ^rank7BB) << 7) & p.colorBB[Black]
		for bb != 0 {
			to := lsb(bb)
			list[n] = (to << 6) | (to - 7); n++
			bb &= bb - 1
		}
		// Normal captures right.
		bb = ((p.pieceBB(White, P) & ^fileHBB & ^rank7BB) << 9) & p.colorBB[Black]
		for bb != 0 {
			to := lsb(bb)
			list[n] = (to << 6) | (to - 9); n++
			bb &= bb - 1
		}
		// En passant.
		if to := p.epSquare; to != NO_SQ {
			if ((p.pieceBB(White, P)&^fileABB)<<7)&squareBit(to) != 0 {
				list[n] = (EP_CAP << 12) | (to << 6) | (to - 7); n++
			}
			if ((p.pieceBB(White, P)&^fileHBB)<<9)&squareBit(to) != 0 {
				list[n] = (EP_CAP << 12) | (to << 6) | (to - 9); n++
			}
		}
	} else {
		// Black pawns advance in the decreasing-index direction (>>).
		// Promotion-captures left (from Black's perspective: toward file A, shift >>9).
		bb := ((p.pieceBB(Black, P) & ^fileABB & rank2BB) >> 9) & p.colorBB[White]
		for bb != 0 {
			to := lsb(bb)
			list[n] = (Q_PROM << 12) | (to << 6) | (to + 9); n++
			list[n] = (R_PROM << 12) | (to << 6) | (to + 9); n++
			list[n] = (B_PROM << 12) | (to << 6) | (to + 9); n++
			list[n] = (N_PROM << 12) | (to << 6) | (to + 9); n++
			bb &= bb - 1
		}
		// Promotion-captures right (>>7).
		bb = ((p.pieceBB(Black, P) & ^fileHBB & rank2BB) >> 7) & p.colorBB[White]
		for bb != 0 {
			to := lsb(bb)
			list[n] = (Q_PROM << 12) | (to << 6) | (to + 7); n++
			list[n] = (R_PROM << 12) | (to << 6) | (to + 7); n++
			list[n] = (B_PROM << 12) | (to << 6) | (to + 7); n++
			list[n] = (N_PROM << 12) | (to << 6) | (to + 7); n++
			bb &= bb - 1
		}
		// Quiet promotions.
		bb = ((p.pieceBB(Black, P) & rank2BB) >> 8) & p.empty()
		for bb != 0 {
			to := lsb(bb)
			list[n] = (Q_PROM << 12) | (to << 6) | (to + 8); n++
			list[n] = (R_PROM << 12) | (to << 6) | (to + 8); n++
			list[n] = (B_PROM << 12) | (to << 6) | (to + 8); n++
			list[n] = (N_PROM << 12) | (to << 6) | (to + 8); n++
			bb &= bb - 1
		}
		// Normal captures left.
		bb = ((p.pieceBB(Black, P) & ^fileABB & ^rank2BB) >> 9) & p.colorBB[White]
		for bb != 0 {
			to := lsb(bb)
			list[n] = (to << 6) | (to + 9); n++
			bb &= bb - 1
		}
		// Normal captures right.
		bb = ((p.pieceBB(Black, P) & ^fileHBB & ^rank2BB) >> 7) & p.colorBB[White]
		for bb != 0 {
			to := lsb(bb)
			list[n] = (to << 6) | (to + 7); n++
			bb &= bb - 1
		}
		// En passant.
		if to := p.epSquare; to != NO_SQ {
			if ((p.pieceBB(Black, P)&^fileABB)>>9)&squareBit(to) != 0 {
				list[n] = (EP_CAP << 12) | (to << 6) | (to + 9); n++
			}
			if ((p.pieceBB(Black, P)&^fileHBB)>>7)&squareBit(to) != 0 {
				list[n] = (EP_CAP << 12) | (to << 6) | (to + 7); n++
			}
		}
	}

	// ---- Knight captures ----
	pieces := p.pieceBB(side, N)
	for pieces != 0 {
		from := lsb(pieces)
		bb   := knightAtk[from] & p.colorBB[opp(side)]
		for bb != 0 {
			to := lsb(bb)
			list[n] = (to << 6) | from; n++
			bb &= bb - 1
		}
		pieces &= pieces - 1
	}

	// ---- Bishop captures ----
	pieces = p.pieceBB(side, B)
	for pieces != 0 {
		from := lsb(pieces)
		bb   := bishopAttacks(occ, from) & p.colorBB[opp(side)]
		for bb != 0 {
			to := lsb(bb)
			list[n] = (to << 6) | from; n++
			bb &= bb - 1
		}
		pieces &= pieces - 1
	}

	// ---- Rook captures ----
	pieces = p.pieceBB(side, R)
	for pieces != 0 {
		from := lsb(pieces)
		bb   := rookAttacks(occ, from) & p.colorBB[opp(side)]
		for bb != 0 {
			to := lsb(bb)
			list[n] = (to << 6) | from; n++
			bb &= bb - 1
		}
		pieces &= pieces - 1
	}

	// ---- Queen captures ----
	pieces = p.pieceBB(side, Q)
	for pieces != 0 {
		from := lsb(pieces)
		bb   := queenAttacks(occ, from) & p.colorBB[opp(side)]
		for bb != 0 {
			to := lsb(bb)
			list[n] = (to << 6) | from; n++
			bb &= bb - 1
		}
		pieces &= pieces - 1
	}

	// ---- King captures ----
	bb := kingAtk[p.kingSq[side]] & p.colorBB[opp(side)]
	for bb != 0 {
		to := lsb(bb)
		list[n] = (to << 6) | p.kingSq[side]; n++
		bb &= bb - 1
	}

	return n
}

// genQuiet fills list with all non-capturing, non-promoting moves for
// the side to move, then returns the count.
func genQuiet(p *Pos, list []int) int {
	n     := 0
	side  := p.side
	occ   := p.occupied()
	empty := p.empty()

	// ---- Castling ----
	// Requirements: castling rights present, intervening squares empty,
	// king and passing square not under attack.
	if side == White {
		// Kingside: E1->G1; F1, G1 must be empty; E1 and F1 not attacked.
		if p.castleRights&1 != 0 && occ&0x0000000000000060 == 0 {
			if !isAttacked(p, E1, Black) && !isAttacked(p, F1, Black) {
				list[n] = (CASTLE << 12) | (G1 << 6) | E1; n++
			}
		}
		// Queenside: E1->C1; B1, C1, D1 must be empty; E1 and D1 not attacked.
		if p.castleRights&2 != 0 && occ&0x000000000000000E == 0 {
			if !isAttacked(p, E1, Black) && !isAttacked(p, D1, Black) {
				list[n] = (CASTLE << 12) | (C1 << 6) | E1; n++
			}
		}
		// Double pawn push: only from rank 2, and rank 3 must be clear too.
		bb := (((p.pieceBB(White, P)&rank2BB)<<8)&empty)<<8 & empty
		for bb != 0 {
			to := lsb(bb)
			list[n] = (EP_SET << 12) | (to << 6) | (to - 16); n++
			bb &= bb - 1
		}
		// Single pawn push (not rank 7; promotions are in genCaptures).
		bb = ((p.pieceBB(White, P) & ^rank7BB) << 8) & empty
		for bb != 0 {
			to := lsb(bb)
			list[n] = (to << 6) | (to - 8); n++
			bb &= bb - 1
		}
	} else {
		// Black kingside: E8->G8.
		if p.castleRights&4 != 0 && occ&0x6000000000000000 == 0 {
			if !isAttacked(p, E8, White) && !isAttacked(p, F8, White) {
				list[n] = (CASTLE << 12) | (G8 << 6) | E8; n++
			}
		}
		// Black queenside: E8->C8.
		if p.castleRights&8 != 0 && occ&0x0E00000000000000 == 0 {
			if !isAttacked(p, E8, White) && !isAttacked(p, D8, White) {
				list[n] = (CASTLE << 12) | (C8 << 6) | E8; n++
			}
		}
		// Double pawn push.
		bb := (((p.pieceBB(Black, P)&rank7BB)>>8)&empty)>>8 & empty
		for bb != 0 {
			to := lsb(bb)
			list[n] = (EP_SET << 12) | (to << 6) | (to + 16); n++
			bb &= bb - 1
		}
		// Single pawn push.
		bb = ((p.pieceBB(Black, P) & ^rank2BB) >> 8) & empty
		for bb != 0 {
			to := lsb(bb)
			list[n] = (to << 6) | (to + 8); n++
			bb &= bb - 1
		}
	}

	// ---- Knight quiet moves ----
	pieces := p.pieceBB(side, N)
	for pieces != 0 {
		from := lsb(pieces)
		bb   := knightAtk[from] & empty
		for bb != 0 {
			to := lsb(bb)
			list[n] = (to << 6) | from; n++
			bb &= bb - 1
		}
		pieces &= pieces - 1
	}

	// ---- Bishop quiet moves ----
	pieces = p.pieceBB(side, B)
	for pieces != 0 {
		from := lsb(pieces)
		bb   := bishopAttacks(occ, from) & empty
		for bb != 0 {
			to := lsb(bb)
			list[n] = (to << 6) | from; n++
			bb &= bb - 1
		}
		pieces &= pieces - 1
	}

	// ---- Rook quiet moves ----
	pieces = p.pieceBB(side, R)
	for pieces != 0 {
		from := lsb(pieces)
		bb   := rookAttacks(occ, from) & empty
		for bb != 0 {
			to := lsb(bb)
			list[n] = (to << 6) | from; n++
			bb &= bb - 1
		}
		pieces &= pieces - 1
	}

	// ---- Queen quiet moves ----
	pieces = p.pieceBB(side, Q)
	for pieces != 0 {
		from := lsb(pieces)
		bb   := queenAttacks(occ, from) & empty
		for bb != 0 {
			to := lsb(bb)
			list[n] = (to << 6) | from; n++
			bb &= bb - 1
		}
		pieces &= pieces - 1
	}

	// ---- King quiet moves ----
	bb := kingAtk[p.kingSq[side]] & empty
	for bb != 0 {
		to := lsb(bb)
		list[n] = (to << 6) | p.kingSq[side]; n++
		bb &= bb - 1
	}

	return n
}

// generates selected checks (non-discovered piece checks)
func genChecks(p *Pos, list []int) int {
	n     := 0
	side  := p.side
	occ   := p.occupied()
	empty := p.empty()
	ksq := p.kingSq[opp(side)]
	nChecks := knightAtk[ksq] & empty
	bChecks := bishopAttacks(occ, ksq) & empty
	rChecks := rookAttacks(occ, ksq) & empty
	qChecks := rChecks | bChecks


	// ---- Knight quiet checks ----
	pieces := p.pieceBB(side, N)
	for pieces != 0 {
		from := lsb(pieces)
		bb   := knightAtk[from] & nChecks
		for bb != 0 {
			to := lsb(bb)
			list[n] = (to << 6) | from; n++
			bb &= bb - 1
		}
		pieces &= pieces - 1
	}

	// ---- Bishop quiet checks ----
	pieces = p.pieceBB(side, B)
	for pieces != 0 {
		from := lsb(pieces)
		bb   := bishopAttacks(occ, from) & bChecks
		for bb != 0 {
			to := lsb(bb)
			list[n] = (to << 6) | from; n++
			bb &= bb - 1
		}
		pieces &= pieces - 1
	}

	// ---- Rook quiet checks ----
	pieces = p.pieceBB(side, R)
	for pieces != 0 {
		from := lsb(pieces)
		bb   := rookAttacks(occ, from) & rChecks
		for bb != 0 {
			to := lsb(bb)
			list[n] = (to << 6) | from; n++
			bb &= bb - 1
		}
		pieces &= pieces - 1
	}

	// ---- Queen quiet checks ----
	pieces = p.pieceBB(side, Q)
	for pieces != 0 {
		from := lsb(pieces)
		bb   := queenAttacks(occ, from) & qChecks
		for bb != 0 {
			to := lsb(bb)
			list[n] = (to << 6) | from; n++
			bb &= bb - 1
		}
		pieces &= pieces - 1
	}

	return n
}

