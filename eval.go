// ================================================================
// S7  STATIC EVALUATION
// ================================================================
//
//   evaluate(p) returns a score (in centipawns) from the perspective
//   of the side to move: positive = better for the mover, negative =
//   worse. The score is always in the range [-maxEval, +maxEval].
//
//   COMPONENTS (applied symmetrically for both sides, then differenced)
//   ------------------------------------------------------------------
//   1. MATERIAL BALANCE
//      The most important term. Different values for midgame/endgame,
//      interpolated according to game phase.
//
//   2. MOBILITY
//      Bigger values for weaker pieces, major pieces gain in the endgame.
//
//   3. PIECE-SQUARE TABLES
//      Bonuses for occupying good squares, m*luses for occupying bad ones;
//      skeleton for eval functions. Different values for midgame/endgame,
//      interpolated according to game phase. Right now we use PeSTo tables,
//      modified to cater for existence of passed pawn eval.
//
//   4. PAWN STRUCTURE
//      Passed pawns: bonus that grows with rank (closer to promotion).
//      Isolated pawns: penalty when no friendly pawn stands on an
//      adjacent file.
//
//   5. KING SAFETY
//      To be ported from Rodent
//

package main

// --- Bitmasks used in eval only, put into init() to preserve locality ---
var (
	passedMask  [2][64]uint64
	adjFileMask [8]uint64

	fileMask = [8]uint64{
	fileABB,
	fileBBB,
	fileCBB,
	fileDBB,
	fileEBB,
	fileFBB,
	fileGBB,
	fileHBB,
	}
)

func init() {
	// --- Passed pawn masks ---
	// passedMask[White][sq]: squares strictly in front of sq on the
	// same and adjacent files.  A White pawn on sq is "passed" if
	// none of these squares contain a Black pawn.
	for sq := 0; sq < 64; sq++ {
		passedMask[White][sq] = 0
		for f := fileOf(sq) - 1; f <= fileOf(sq)+1; f++ {
			if f < 0 || f > 7 {
				continue
			}
			for r := rankOf(sq) + 1; r <= 7; r++ {
				passedMask[White][sq] |= squareBit(makeSquare(f, r))
			}
		}
	}
	for sq := 0; sq < 64; sq++ {
		passedMask[Black][sq] = 0
		for f := fileOf(sq) - 1; f <= fileOf(sq)+1; f++ {
			if f < 0 || f > 7 {
				continue
			}
			for r := rankOf(sq) - 1; r >= 0; r-- {
				passedMask[Black][sq] |= squareBit(makeSquare(f, r))
			}
		}
	}

	// --- Adjacent file masks ---
	// adjFileMask[f]: bitboard of the two files neighboring file f.
	// A pawn is isolated if adjFileMask[file] & ownPawns == 0.
	for f := 0; f < 8; f++ {
		adjFileMask[f] = 0
		if f > 0 {
			adjFileMask[f] |= fileABB << uint(f-1)
		}
		if f < 7 {
			adjFileMask[f] |= fileABB << uint(f+1)
		}
	}
}

// --- Eval params ---
var pieceValMG = [7]int{82, 337, 365, 477, 1025, 0, 0}
var pieceValEG = [7]int{94, 281, 297, 513, 937, 0, 0}

var pstMG = [6][64]int{
	P: {
		0, 0, 0, 0, 0, 0, 0, 0, -35, -6, -25, -22, -15, 18, 25, -26, /* pawn   r1-r2 */
		-26, -11, -4, -8, 5, 5, 22, -12, -29, -5, -4, 14, 17, 6, 8, -27, /*        r3-r4 */
		-16, 12, 8, 23, 25, 14, 18, -25, -6, 8, 19, 23, 38, 59, 26, -19, /*        r5-r6 */
		80, 76, 49, 54, 50, 57, 26, -3, 0, 0, 0, 0, 0, 0, 0, 0,
	},
	N: {
		-106, -19, -56, -31, -15, -26, -20, -22, -27, -51, -10, -1, 1, 20, -12, -17, /* knight r1-r2 */
		-25, -7, 10, 12, 21, 19, 27, -18, -12, 6, 16, 12, 30, 20, 23, -7, /*        r3-r4 */
		-7, 18, 18, 55, 35, 70, 17, 23, -47, 61, 37, 63, 85, 128, 74, 42, /*        r5-r6 */
		-71, -40, 74, 37, 23, 64, 5, -15, -165, -87, -32, -47, 63, -96, -15, -105,
	},
	B: {
		-32, -1, -12, -19, -11, -14, -39, -21, 5, 19, 17, 0, 9, 23, 36, 2, /* bishop r1-r2 */
		-2, 17, 15, 13, 12, 28, 19, 8, -4, 15, 11, 27, 33, 10, 9, 5, /*        r3-r4 */
		-3, 3, 19, 52, 35, 35, 5, -3, -18, 38, 43, 38, 35, 52, 37, -4, /*        r5-r6 */
		-26, 17, -17, -12, 32, 60, 20, -47, -27, 5, -82, -36, -23, -40, 8, -7,
	},
	R: {
		-17, -11, 2, 15, 14, 9, -39, -25, -43, -15, -18, -10, -1, 13, -4, -72, /* rook   r1-r2 */
		-44, -23, -16, -17, 1, 2, -3, -34, -38, -24, -13, -3, 9, -6, 6, -25, /*        r3-r4 */
		-22, -10, 5, 25, 22, 35, -8, -20, -6, 19, 24, 34, 15, 46, 61, 16, /*        r5-r6 */
		25, 30, 56, 60, 78, 65, 24, 42, 33, 42, 31, 49, 62, 11, 33, 45,
	},
	Q: {
		-2, -17, -7, 12, -13, -23, -29, -49, -34, -9, 11, 4, 10, 17, -1, 3, /* queen  r1-r2 */
		-16, 0, -13, -4, -7, 0, 13, 5, -11, -28, -11, -12, -4, -6, 1, -5, /*        r3-r4 */
		-29, -29, -18, -18, -3, 15, -3, -1, -11, -19, 5, 6, 29, 58, 47, 57, /*        r5-r6 */
		-23, -41, -5, 3, -17, 59, 29, 56, -26, 1, 31, 13, 61, 46, 45, 47,
	},
	K: {
		-17, 36, 14, -56, 6, -26, 26, 12, 1, 8, -6, -66, -45, -14, 11, 7, /* king   r1-r2 */
		-13, -12, -20, -48, -46, -28, -13, -25, -48, 1, -25, -41, -48, -42, -32, -53, /*        r3-r4 */
		-16, -18, -10, -29, -31, -25, -13, -35, -7, 26, 4, -17, -22, 8, 24, -24, /*        r5-r6 */
		30, 1, -18, -5, -10, -2, -36, -28, -66, 24, 18, -14, -58, -32, 3, 13,
	},
}

var pstEG = [6][64]int{
	P: {
		0, 0, 0, 0, 0, 0, 0, 0, 14, 6, 8, 8, 12, -2, 0, -9, /* pawn   r1-r2 */
		2, 5, -8, 0, 0, -5, -3, -10, 11, 7, -5, -9, -9, -10, 1, -3, /*        r3-r4 */
		30, 22, 11, 3, -4, 2, 15, 15, 72, 69, 46, 25, 24, 31, 52, 62, /*        r5-r6 */
		95, 92, 86, 62, 65, 88, 93, 124, 0, 0, 0, 0, 0, 0, 0, 0,
	},
	N: {
		-27, -49, -21, -13, -20, -16, -48, -62, -40, -18, -8, -3, 0, -18, -21, -42, /* knight r1-r2 */
		-21, -2, -1, 16, 12, -2, -18, -20, -16, -4, 18, 27, 17, 17, 6, -16, /*        r3-r4 */
		-15, 5, 24, 24, 24, 12, 10, -20, -22, -18, 9, 10, -3, -11, -19, -42, /*        r5-r6 */
		-23, -6, -24, 0, -9, -27, -26, -50, -56, -36, -11, -26, -30, -25, -61, -98,
	},
	B: {
		-21, -7, -21, -3, -7, -14, -3, -15, -12, -16, -5, 1, 5, -7, -13, -26, /* bishop r1-r2 */
		-10, -1, 10, 10, 15, 2, -5, -13, -4, 4, 14, 20, 7, 9, -2, -7, /*        r3-r4 */
		-1, 11, 13, 9, 14, 8, 4, 4, 4, -7, 0, 0, 0, 6, 2, 6, /*        r5-r6 */
		-6, -2, 9, -10, -2, -11, -4, -12, -12, -19, -9, -6, -5, -7, -15, -22,
	},
	R: {
		-7, 4, 5, -1, -3, -11, 6, -18, -4, -4, 2, 4, -7, -7, -9, -1, /* rook   r1-r2 */
		-2, 2, -3, 1, -5, -10, -6, -14, 5, 7, 10, 3, -4, -4, -6, -9, /*        r3-r4 */
		6, 5, 15, 0, 0, 3, 0, 4, 9, 9, 7, 4, 4, -1, -3, -1, /*        r5-r6 */
		9, 11, 11, 9, -5, 1, 6, 1, 15, 11, 20, 13, 12, 14, 10, 7,
	},
	Q: {
		-33, -27, -21, -41, -3, -31, -18, -40, -20, -21, -28, -15, -15, -21, -34, -31, /* queen  r1-r2 */
		-14, -26, 15, 7, 10, 18, 12, 7, -17, 30, 19, 48, 30, 36, 39, 25, /*        r3-r4 */
		4, 23, 23, 46, 59, 40, 59, 38, -19, 6, 9, 50, 49, 37, 19, 11, /*        r5-r6 */
		-16, 22, 33, 43, 60, 27, 32, 1, -7, 24, 23, 29, 29, 21, 12, 22,
	},
	K: {
		-55, -36, -19, -12, -30, -12, -26, -45, -28, -9, 6, 11, 12, 6, -3, -19, /* king   r1-r2 */
		-21, -1, 13, 19, 21, 18, 9, -10, -19, -3, 23, 22, 25, 25, 11, -12, /*        r3-r4 */
		-10, 24, 26, 25, 24, 35, 28, 1, 10, 18, 24, 13, 18, 46, 45, 11, /*        r5-r6 */
		-12, 19, 16, 16, 15, 40, 25, 12, -74, -34, -18, -20, -13, 17, 5, -19,
	},
}

	var rookHalfMG = 6
	var rookHalfEG = 6
	var rookOpenMG = 12
	var rookOpenEG = 12

// passedBonus[color][rank]: centipawn bonus awarded to a passed
// pawn based on how far it has advanced.  White advances up the
// board (rank 1 -> 8); Black advances down (rank 8 -> 1).
var passedBonus = [2][8]int{
	{0, 25, 30, 35, 40, 45, 50, 0}, // White: rank 1..8
	{0, 50, 45, 40, 35, 30, 25, 0}, // Black: rank 8..1 (mirrored)
}

// kingAttackerWeight[pieceType]: how dangerous is each piece type
// when it attacks squares near the enemy king.
// Indexed P=0..Q=4; pawns and kings are handled separately.
var kingAttackerWeight = [6]int{0, 2, 2, 3, 5, 0}

// evaluate returns the static score for the current position from the
// perspective of the side to move.  Positive = better for the mover.
func evaluate(p *Pos) int {
	return eval_internal(p, false)
}

// eval_trace describes engine's evaluation
func eval_trace(p *Pos) int {
	return eval_internal(p, true)
}

// eval_internal returns the static score for the current position from the
// perspective of the side to move.  Positive = better for the mover.
func eval_internal(p *Pos, shouldReport bool) int {
	var e EvalData // Golang-specific: it will be initialized as all zeroes

	// King rings must be set before evaluatePieces so that attack
	// tracking against the enemy king zone is available.
	e.kingRing[White] = kingAtk[p.kingSq[White]]
	e.kingRing[Black] = kingAtk[p.kingSq[Black]]

	evaluatePieces(p, &e, White)
	evaluatePieces(p, &e, Black)
	evaluatePawns(p, &e, White)
	evaluatePawns(p, &e, Black)
	evaluateKing(p, &e, White)
	evaluateKing(p, &e, Black)

	// Interpolate between game phases
    mg := e.sumMg(White) - e.sumMg(Black)
    eg := e.sumEg(White) - e.sumEg(Black)
	if e.phase > 24 {
		e.phase = 24
	}

	score := (mg*e.phase + eg*(24-e.phase)) / 24

	// Clamp to the range that the transposition table can distinguish
	// from a forced mate score.
	if score < -maxEval {
		score = -maxEval
	} else if score > maxEval {
		score = maxEval
	}

	if shouldReport {
		e.PrintEvalDetails(p)
	}

	// Return score from the perspective of the side to move.
	if p.side == White {
		return score
	}
	return -score
}

// evaluatePieces evaluates pieces (except pawns and king),
// sets game phase, and accumulates king-safety attack data.
func evaluatePieces(p *Pos, e *EvalData, side int) {
	occ := p.occupied()
	enemy := opp(side)
	enemyRing := e.kingRing[enemy]

	pieces := p.pieceBB(side, N)
	for pieces != 0 {
		sq := lsb(pieces)
		add(e, side, EvalMaterial, pieceValMG[N], pieceValEG[N])
		addPST(e, side, N, sq)
		atks := knightAtk[sq]
		mob := popCount(atks&^p.colorBB[side]) - 4
		add(e, side, EvalMobility, 3*mob, 3*mob)
		if ringAtks := atks & enemyRing; ringAtks != 0 {
			e.attackWt[side] += kingAttackerWeight[N]
			e.attackCnt[side] += popCount(ringAtks)
		}
		e.phase += 1
		pieces &= pieces - 1
	}

	// X-ray occupancies: remove friendly pieces that a slider can see
	// through, so battery partners (doubled rooks, rook+queen,
	// bishop+queen) are treated as a single coordinated attack.
	occForBishop := occ ^ (p.pieceBB(side, B) | p.pieceBB(side, Q))
	occForRook := occ ^ (p.pieceBB(side, R) | p.pieceBB(side, Q))
	occForQueen := occ ^ (p.pieceBB(side, B) | p.pieceBB(side, R))

	pieces = p.pieceBB(side, B)
	for pieces != 0 {
		sq := lsb(pieces)
		add(e, side, EvalMaterial, pieceValMG[B], pieceValEG[B])
		addPST(e, side, B, sq)
		atks := bishopAttacks(occForBishop, sq)
		mob := popCount(atks) - 6
		add(e, side, EvalMobility, 5*mob, 4*mob)
		if ringAtks := atks & enemyRing; ringAtks != 0 {
			e.attackWt[side] += kingAttackerWeight[B]
			e.attackCnt[side] += popCount(ringAtks)
		}
		e.phase += 1
		pieces &= pieces - 1
	}

	pieces = p.pieceBB(side, R)
	for pieces != 0 {
		sq := lsb(pieces)
		add(e, side, EvalMaterial, pieceValMG[R], pieceValEG[R])
		addPST(e, side, R, sq)
		atks := rookAttacks(occForRook, sq)
		mob := popCount(atks) - 7
		add(e, side, EvalMobility, 3*mob, 2*mob)
		if ringAtks := atks & enemyRing; ringAtks != 0 {
			e.attackWt[side] += kingAttackerWeight[R]
			e.attackCnt[side] += popCount(ringAtks)
		}

		// Rook on open/half open file
		file := fileMask[fileOf(sq)]

		if (file & p.pieceBB(side, P)) == 0 {
			if (file & p.pieceBB(opp(side), P)) != 0 {
				add(e, side, EvalOther, rookHalfMG, rookHalfEG)
			} else {
				add(e, side, EvalOther, rookOpenMG, rookOpenEG)
			}
		}

		e.phase += 2
		pieces &= pieces - 1
	}

	pieces = p.pieceBB(side, Q)
	for pieces != 0 {
		sq := lsb(pieces)
		add(e, side, EvalMaterial, pieceValMG[Q], pieceValEG[Q])
		addPST(e, side, Q, sq)
		atks := queenAttacks(occForQueen, sq)
		mob := popCount(atks) - 14
		add(e, side, EvalMobility, 2*mob, 2*mob)
		if ringAtks := atks & enemyRing; ringAtks != 0 {
			e.attackWt[side] += kingAttackerWeight[Q]
			e.attackCnt[side] += popCount(ringAtks)
		}
		e.phase += 4
		pieces &= pieces - 1
	}
}

// evaluatePawns scores the pawn structure for one side.
//
//	Passed pawn (+25..+50 cp): no enemy pawn can block or capture it
//	on the same or adjacent file ahead of it.  The bonus grows with
//	rank; a pawn on the 7th rank is almost a queen.
//
//	Isolated pawn (-20 cp): no friendly pawn on an adjacent file.
//	Isolated pawns cannot be defended by other pawns and are easy
//	targets for the opponent's rooks.
func evaluatePawns(p *Pos, e *EvalData, side int) {

	pieces := p.pieceBB(side, P)

	for pieces != 0 {
		sq := lsb(pieces)
		add(e, side, EvalMaterial, pieceValMG[P], pieceValEG[P])
		addPST(e, side, P, sq)

		// Passed pawn: no enemy pawns in front on same or adjacent files.
		if passedMask[side][sq]&p.pieceBB(opp(side), P) == 0 {
			add(e, side, EvalPassers, passedBonus[side][rankOf(sq)], passedBonus[side][rankOf(sq)])
		}
		// Isolated pawn: no friendly pawns on adjacent files.
		if adjFileMask[fileOf(sq)]&p.pieceBB(side, P) == 0 {
			add(e, side, EvalPawns, -20, -20)
		}
		pieces &= pieces - 1
	}
}

// pawnShieldMG computes the middlegame pawn-shield penalty for a king.
// We inspect the two ranks directly in front of the king on its file
// and the two adjacent files.  Missing pawns and open/semi-open files
// near the king are penalised.
func pawnShieldMG(p *Pos, side int) int {
	kSq := p.kingSq[side]
	kFile := fileOf(kSq)
	kRank := rankOf(kSq)

	ownPawns := p.pieceBB(side, P)
	enemyPawns := p.pieceBB(opp(side), P)

	penalty := 0

	for df := -1; df <= 1; df++ {
		f := kFile + df
		if f < 0 || f > 7 {
			continue
		}
		fileMask := fileABB << uint(f)

		// Ranks immediately in front of the king (r1 closer, r2 further).
		var r1, r2 int
		if side == White {
			r1, r2 = kRank+1, kRank+2
		} else {
			r1, r2 = kRank-1, kRank-2
		}

		hasPawnR1 := r1 >= 0 && r1 <= 7 && ownPawns&squareBit(makeSquare(f, r1)) != 0
		hasPawnR2 := r2 >= 0 && r2 <= 7 && ownPawns&squareBit(makeSquare(f, r2)) != 0

		if !hasPawnR1 && !hasPawnR2 {
			penalty += 28 // no shield pawn at all
		} else if !hasPawnR1 {
			penalty += 10 // pawn advanced one step
		}

		// Additional penalty for open / semi-open files through the king zone.
		if fileMask&ownPawns == 0 {
			if fileMask&enemyPawns == 0 {
				penalty += 15 // open file
			} else {
				penalty += 7 // semi-open file
			}
		}
	}

	return -penalty
}

// evaluateKing scores the king for one side: PST + pawn shield (MG) +
// king-attack danger based on what the *enemy* accumulated in
// evaluatePieces.
func evaluateKing(p *Pos, e *EvalData, side int) {
	sq := p.kingSq[side]
	addPST(e, side, K, sq)

	// Pawn shield only matters in the middlegame.
	shieldMG := pawnShieldMG(p, side)
	add(e, side, EvalSafety, shieldMG, 0)

	// King-attack danger: pressure accumulated by the *enemy* on our
	// king ring.  We only trigger this when at least two distinct pieces
	// are bearing down on the king zone; a lone attacker is rarely fatal.
	enemy := opp(side)
	if e.attackCnt[enemy] >= 2 {
		// Scale danger by weight and count; kept intentionally modest so
		// the engine does not become reckless about piece sacrifices.
		danger := e.attackWt[enemy] * (e.attackCnt[enemy] + 2) / 8

		// Virtual checks: enemy pieces that can reach a checking square
		// right now (ignoring whether that square is defended).
		occ := p.occupied()
		knightChecks := popCount(p.pieceBB(enemy, N) & knightAtk[sq])
		bishopChecks := popCount(p.pieceBB(enemy, B) & bishopAttacks(occ, sq))
		rookChecks := popCount(p.pieceBB(enemy, R) & rookAttacks(occ, sq))
		queenChecks := popCount(p.pieceBB(enemy, Q) & queenAttacks(occ, sq))

		danger += knightChecks*3 + bishopChecks*3 + rookChecks*4 + queenChecks*6

		// Apply mostly to MG; small residual in EG so the eval is not
		// completely blind to king safety once queens are traded off.
		add(e, side, EvalSafety, -danger, -danger/4)
	}
}

// TODO: initialize pst tables in a separate file and
// do not handle White and Black separately in addPST

// addPST adds the piece-square table score for a piece on sq.
func addPST(e *EvalData, side, piece, sq int) {
	if side == White {
		add(e, side, EvalPst, pstMG[piece][sq], pstEG[piece][sq])
		return
	}

	msq := sq ^ 56
	add(e, side, EvalPst, pstMG[piece][msq], pstEG[piece][msq])
}

// add adds MG/EG scores for one side to EvalData.
func add(e *EvalData, side int, component EvalComponent, mg, eg int) {
	e.mgScore[side] [component] += mg
	e.egScore[side] [component] += eg
}
