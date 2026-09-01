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
//   4. PASSED PAWNS
//      Bonus that grows with rank (closer to promotion). Evaluation takes
//      into account blockade, king proximity and enemy major piece behind
//      the pawn
//
//   5. PAWN STRUCTURE
//      Isolated pawns: penalty when no friendly pawn stands on an
//      adjacent file.
//      Backward pawns: penalty for pawns that cannot be defended by another
//      Doubled pawns: only one blocking another
//      Phalanx: bonus for pawns standing side by side
//
//   6. KING SAFETY
//      Evaluationg attacks on the squares in the king's ring,
//      safe checks, contact queen checks, attacked and undefended
//      squares near the king
//
//   7. THREATS
//      Attacks on pieces, subdivided into defended and undefended

package main

// Stuff not in params.go, because we don't tune it

// minorHomeBB[side]: bitboard of the four squares where knights and bishops
// start the game.  A minor still on one of these squares counts as undeveloped.
var minorHomeBB = [2]uint64{
	White: (1 << B1) | (1 << C1) | (1 << F1) | (1 << G1),
	Black: (1 << B8) | (1 << C8) | (1 << F8) | (1 << G8),
}

// devPenaltyScale: multiplier for the quadratic undevelopment penalty.
// penalty = undeveloped^2 * devPenaltyScale  (MG only)
// 1 undeveloped:  -5 cp   (barely noticeable)
// 2 undeveloped: -20 cp   (a tempo behind)
// 3 undeveloped: -45 cp   (serious lag)
// 4 undeveloped: -80 cp   (opening disaster)
const devPenaltyScale = 5

// --- Bitmasks used in eval only, put into init() to preserve locality ---
var (
	passedMask  [2][64]uint64
	supportMask [2][64]uint64
	adjFileMask [8]uint64
)

func init() {
	// --- Passed pawn masks ---
	// passedMask[White][sq]: squares strictly in front of sq on the
	// same and adjacent files.  A White pawn on sq is "passed" if
	// none of these squares contain a Black pawn.
	for sq := 0; sq < 64; sq++ {
		passedMask[White][sq] = fillNorth(shiftNorth(squareBit(sq)))
		passedMask[White][sq] |= shiftSides(passedMask[White][sq])
		passedMask[Black][sq] = fillSouth(shiftSouth(squareBit(sq)))
		passedMask[Black][sq] |= shiftSides(passedMask[Black][sq])
	}

	// --- Support mask to detect backward pawns ---
	for sq := A1; sq < 64; sq++ {
		base := shiftSides(squareBit(sq))

		supportMask[White][sq] = base | fillSouth(base)
		supportMask[Black][sq] = base | fillNorth(base)
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

	initEvalHash(128 * 128)
	initPawnHash(64 * 128)
}

// evaluate returns the static score for the current position from the
// perspective of the side to move.  Positive = better for the mover.
// Before calculating eval score, it tries to find it in the eval hashtable.
// ss selects which eval/pawn hash table to use (evalHashFor/pawnHashFor);
// pass nil to use the shared global tables.
func evaluate(p *Pos, acc *Accumulator, ss *SearchState) int {

	if score, ok := evalHashFor(ss).probe(p.key); ok {
		return score
	}

	score := 0

	if !nnue.Loaded {
		score = eval_internal(p, false, ss)
	} else {
		nnueScore := 0
		hceScore := 0
		if singleOptionValue[NnuePerc] > 0 {
			nnueScore = evaluateScaledNNUE(p, acc)
		}
		if singleOptionValue[HcePerc] > 0 {
			hceScore = eval_internal(p, false, ss) * singleOptionValue[HcePerc] / 100
		}

		score = hceScore + nnueScore

		// Flair options
		if singleOptionValue[LikesClosed] > 0 {
			score += flairClosed(p)
		}
		if singleOptionValue[KingTropism] > 0 {
			score += flairTropism(p)
		}
		if singleOptionValue[Forwardness] > 0 {
			score += flairForward(p)
		}
	}

	evalHashFor(ss).store(p.key, score)
	return score
}

// evaluateHCE is 100% handcrafted eval, sped up by eval transposition table
func evaluateHCE(p *Pos, ss *SearchState) int {
	if score, ok := evalHashFor(ss).probe(p.key); ok {
		return score
	}
	score := eval_internal(p, false, ss)
	evalHashFor(ss).store(p.key, score)
	return score
}

// evaluateNNUE is weighted NNUE eval, sped up by eval transposition table
func evaluateNNUE(p *Pos, acc *Accumulator, ss *SearchState) int {
	if score, ok := evalHashFor(ss).probe(p.key); ok {
		return score
	}
	score := evaluateScaledNNUE(p, acc)
	evalHashFor(ss).store(p.key, score)
	return score
}

// returns NNUE eval after all the scalings we apply
func evaluateScaledNNUE(p *Pos, acc *Accumulator) int {

	// sum of material for both sides
	material := 100*p.count[White][P] + 100*p.count[Black][P] +
		300*p.count[White][N] + 300*p.count[Black][N] +
		300*p.count[White][B] + 300*p.count[Black][B] +
		500*p.count[White][R] + 500*p.count[Black][R] +
		900*p.count[White][Q] + 900*p.count[Black][Q]

	// percentage scaling
	score := acc.getEval(p, p.side) * singleOptionValue[NnuePerc] / 100

	// decrease score as material disappears
	return score * (25000 + material) / 32768
}

// eval_trace describes engine's hce evaluation
func eval_trace(p *Pos) int {
	return eval_internal(p, true, nil)
}

// eval_internal returns the static score for the current position from the
// perspective of the side to move.  Positive = better for the mover.
// ss selects the pawn hash table (pawnHashFor); pass nil for the shared
// global table (the tuner, which has no SearchState, always does).
func eval_internal(p *Pos, shouldReport bool, ss *SearchState) int {
	var e EvalData // Golang-specific: it will be initialized as all zeroes

	// Tempo. Having the right to move is beneficial. Unfortunately
	// tuning yielded very high values here, when in fact testing with
	// (16, 8) turned out Elo-neutral. So it seems better not to expose
	// tempo bonus to the tuner.
	add(&e, p.side, EvalOther, 8, 4)

	// Init pawn attacks
	e.attackedBy2[White] = doubleWPAttacks(p.pieceBB(White, P))
	e.attackedBy2[Black] = doubleBPAttacks(p.pieceBB(Black, P))
	e.attackedBy[White][P] = shiftWPAttacks(p.pieceBB(White, P))
	e.attackedBy[Black][P] = shiftBPAttacks(p.pieceBB(Black, P))
	e.attacked[White] = e.attackedBy[White][P]
	e.attacked[Black] = e.attackedBy[Black][P]

	// King rings must be set before evaluatePieces so that attack
	// tracking against the enemy king zone is available.
	e.kingRing[White] = kingAtk[p.kingSq[White]]
	e.kingRing[Black] = kingAtk[p.kingSq[Black]]

	evaluatePawnStructure(p, &e, ss)

	evaluatePieces(p, &e, White)
	evaluatePieces(p, &e, Black)
	evaluatePassers(p, &e, White)
	evaluatePassers(p, &e, Black)
	evaluateKing(p, &e, White)
	evaluateKing(p, &e, Black)
	// Threats use the fully-built attack maps from all evaluators above.
	evaluateThreats(p, &e, White)
	evaluateThreats(p, &e, Black)

	// Material imbalance eval
	wMinors := p.count[White][N] + p.count[White][B]
	bMinors := p.count[Black][N] + p.count[Black][B]
	wMajors := p.count[White][R] + 2 * p.count[White][Q]
	bMajors :=  p.count[Black][R] + 2 * p.count[Black][Q]

	if wMajors == bMajors+1 && wMinors == bMinors-1 {
		add(&e, White, EvalMaterial, exchangePlusMG, exchangePlusEG)
	}
	
	if bMajors == wMajors+1 && bMinors == wMinors-1 {
		add(&e, Black, EvalMaterial, exchangePlusMG, exchangePlusEG)
	}

	if wMajors == bMajors-1 && wMinors == bMinors+2 {
		add(&e, White, EvalMaterial, twoMinorsMG, twoMinorsEG)
	}

	if bMajors == wMajors-1 && bMinors == wMinors+2 {
		add(&e, Black, EvalMaterial, twoMinorsMG, twoMinorsEG)
	}

	// Interpolate between game phases
	mg := e.sumMg(White) - e.sumMg(Black)
	eg := e.sumEg(White) - e.sumEg(Black)
	if e.phase > 24 {
		e.phase = 24
	}

	score := (mg*e.phase + eg*(24-e.phase)) / 24

	// Pull score of drawish endgames closer to 0
	if e.phase < 7 { // R+R+B = 5, Q vs R = 6

		score += checkmateHelper(p, &e)

		weight := 100
		if score > 0 {
			weight = getDrawishness(p, White, Black)
		} else if score < 0 {
			weight = getDrawishness(p, Black, White)
		}
		score *= weight
		score /= 100
	}

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

	// Knight eval
	pieces := p.pieceBB(side, N)
	for pieces != 0 {
		sq := lsb(pieces)
		add(e, side, EvalMaterial, pieceValMG[N], pieceValEG[N])
		addPST(e, side, N, sq)

		// Piece/square adjustement for predefined pawn centers
		if e.center[side] != Undefined {
			e.mgScore[side][EvalPst] += knightAdjust[e.center[side]][side][sq]
		}

		// knight board control
		atks := knightAtk[sq]
		e.addAttacks(side, N, atks)

		// knight mobility
		mob := popCount(atks&^p.colorBB[side])
		add(e, side, EvalMobility, nMobMg[mob], nMobEg[mob])

		// knight attacks enemy king
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

	// NOTE: this is buggy, because queen can move along different
	// lines, and we make bishops transparent for a queen moving
	// like a rook, yet I dod not manage to tune it away.

	// Bishop eval
	pieces = p.pieceBB(side, B)
	if popCount(pieces) >= 2 {
		add(e, side, EvalMaterial, bishopPairMG, bishopPairEG)
	}
	for pieces != 0 {
		sq := lsb(pieces)

		// bishop material and pst tables
		add(e, side, EvalMaterial, pieceValMG[B], pieceValEG[B])
		addPST(e, side, B, sq)

		// Piece/square adjustement for predefined pawn centers
		if e.center[side] != Undefined {
			e.mgScore[side][EvalPst] += bishopAdjust[e.center[side]][side][sq]
		}

		// bishop board control
		atks := bishopAttacks(occForBishop, sq)
		e.addAttacks(side, B, bishopAttacks(occ, sq))

		// bishop mobility
		mob := popCount(atks)
		add(e, side, EvalMobility, bMobMg[mob], bMobEg[mob])

		// bishop attacks enemy king
		if ringAtks := atks & enemyRing; ringAtks != 0 {
			e.attackWt[side] += kingAttackerWeight[B]
			e.attackCnt[side] += popCount(ringAtks)
		}

		e.phase += 1
		pieces &= pieces - 1
	}

	// Quadratic undevelopment penalty: each minor still on its home square
	// compounds the punishment.  Two pieces at home is 4× worse than one,
	// four at home is 16× worse — reflecting how a crowded back rank
	// prevents castling and limits all piece coordination.
	minors := p.pieceBB(side, N) | p.pieceBB(side, B)
	undeveloped := popCount(minors & minorHomeBB[side])
	if undeveloped > 0 {
		add(e, side, EvalOther, -(undeveloped * undeveloped * devPenaltyScale), 0)
	}

	// Rook eval
	pieces = p.pieceBB(side, R)
	if popCount(pieces) >= 2 {
		add(e, side, EvalMaterial, rookPairMG, rookPairEG)
	}
	for pieces != 0 {
		sq := lsb(pieces)

		// rook material and pst
		add(e, side, EvalMaterial, pieceValMG[R], pieceValEG[R])
		addPST(e, side, R, sq)

		// rook board control
		atks := rookAttacks(occForRook, sq)
		e.addAttacks(side, R, rookAttacks(occ, sq))

		// rook mobility
		mob := popCount(atks)
		add(e, side, EvalMobility,rMobMg[mob], rMobEg[mob])

		// Rook attacks enemy king.
		if ringAtks := atks & enemyRing; ringAtks != 0 {
			e.attackWt[side] += kingAttackerWeight[R]
			e.attackCnt[side] += popCount(ringAtks)
		}

		// Open / semi-open file bonus.
		fileMask := fileABB << uint(fileOf(sq))
		ownPawnsOnFile := fileMask & p.pieceBB(side, P)
		if ownPawnsOnFile == 0 {
			if fileMask&p.pieceBB(enemy, P) == 0 {
				add(e, side, EvalOther, rookOpenFileMG, rookOpenFileEG)
			} else {
				add(e, side, EvalOther, rookSemiOpenFileMG, rookSemiOpenFileEG)
			}
		}

		e.phase += 2
		pieces &= pieces - 1
	}

	// Queen eval
	pieces = p.pieceBB(side, Q)
	for pieces != 0 {
		sq := lsb(pieces)

		// queen material and pst
		add(e, side, EvalMaterial, pieceValMG[Q], pieceValEG[Q])
		addPST(e, side, Q, sq)

		// queen square control
		atks := queenAttacks(occForQueen, sq)
		e.addAttacks(side, Q, queenAttacks(occ, sq))

		// queen mobility
		mob := popCount(atks)
		add(e, side, EvalMobility, qMobMg[mob], qMobEg[mob])

		// queen attacks enemy king
		if ringAtks := atks & enemyRing; ringAtks != 0 {
			e.attackWt[side] += kingAttackerWeight[Q]
			e.attackCnt[side] += popCount(ringAtks)
		}

		e.phase += 4
		pieces &= pieces - 1
	}
}

// evaluate pawn structure or read the score from the pawn hashtable
func evaluatePawnStructure(p *Pos, e *EvalData, ss *SearchState) {

	var key = getPawnKey(p)
	pawnHash := pawnHashFor(ss)

	if wscoreMG, bscoreMG, wscoreEG, bscoreEG, wCenter, bCenter, ok := pawnHash.probe(key); ok {
		add(e, White, EvalPawns, wscoreMG, wscoreEG)
		add(e, Black, EvalPawns, bscoreMG, bscoreEG)
		e.center[White] = CenterType(wCenter)
		e.center[Black] = CenterType(bCenter)
	} else {

		initCenterType(p, e)
		evaluatePawns(p, e, White)
		evaluatePawns(p, e, Black)
		pawnHash.store(key, e.mgScore[White][EvalPawns], e.mgScore[Black][EvalPawns],
			e.egScore[White][EvalPawns], e.egScore[Black][EvalPawns], int(e.center[White]), int(e.center[Black]))
	}
}

func initCenterType(p *Pos, e *EvalData) {

	// default
	e.center[White] = Undefined
	e.center[Black] = Undefined

	narrow := fileDBB | fileEBB         // narrow center (d-e files)
	wide := fileCBB | fileDBB | fileEBB // wide center (c-d-e files)

	// may be overridden by French
	if isPawnRam(p, D4, D5) {
		setCenterType(e, CLASSIC_d4d5, CLASSIC_d4d5)
	}

	// may be overridden by KID or Sicilian
	if isPawnRam(p, E4, E5) {
		setCenterType(e, CLASSIC_e4e5, CLASSIC_e4e5)
	}

	// detect closed centers (KID / French)
	if popCount(p.pieceBB(White, P)&narrow) == 2 &&
		popCount(p.pieceBB(Black, P)&narrow) == 2 {

		if isPawnRam(p, E4, E5) {
			if isPawnRam(p, D5, D6) {
				setCenterType(e, KID_high, KID_low)
			} else if isPawnRam(p, D3, D4) {
				setCenterType(e, KID_low, KID_high)
			}
		}

		if isPawnRam(p, D4, D5) {
			if isPawnRam(p, E5, E6) {
				setCenterType(e, FRENCH_high, FRENCH_low)
			} else if isPawnRam(p, E3, E4) {
				setCenterType(e, FRENCH_low, FRENCH_high)
			}
		}
	}

	// detect Sicilian center
	if popCount(p.pieceBB(White, P)&wide) == 2 &&
		popCount(p.pieceBB(Black, P)&wide) == 2 {

		if popCount(p.pieceBB(White, P)&fileDBB) == 0 &&
			popCount(p.pieceBB(Black, P)&fileCBB) == 0 &&
			isOnSq(p, White, P, E4) {
			setCenterType(e, SICILIAN_high, SICILIAN_low)
		}

		if popCount(p.pieceBB(Black, P)&fileDBB) == 0 &&
			popCount(p.pieceBB(White, P)&fileCBB) == 0 &&
			isOnSq(p, Black, P, E5) {
			setCenterType(e, SICILIAN_low, SICILIAN_high)
		}
	}
}

func isPawnRam(p *Pos, wsq, bsq int) bool {
	return isOnSq(p, White, P, wsq) && isOnSq(p, Black, P, bsq)
}

func setCenterType(e *EvalData, whiteCenter, blackCenter CenterType) {
	e.center[White] = whiteCenter
	e.center[Black] = blackCenter
}

// evaluatePawns evaluates pawn structure
//
// - king's pawn shield
// - pawn phalanxes
// - isolated pawns
// - backward pawns
// - doubled pawns
func evaluatePawns(p *Pos, e *EvalData, side int) {

	// Pawn shield only matters in the middlegame.
	shieldMG := pawnShieldMG(p, side)
	add(e, side, EvalPawns, shieldMG, 0)

	pieces := p.pieceBB(side, P)

	for pieces != 0 {
		sq := lsb(pieces)
		b := squareBit(sq)

		// Piece/square adjustement for predefined pawn centers
		if e.center[side] != Undefined {
			e.mgScore[side][EvalPawns] += pawnAdjust[e.center[side]][side][sq]
		}

		// Pawn phalanx: two pawns standing side by side.
		if shiftWest(b)&p.pieceBB(side, P) > 0 {
			addPhalanx(e, side, sq)
		}

		frontMask := fillForward(b, side)
		isOpen := frontMask&p.pieceBB(side, P) == 0

		// Isolated pawn: no friendly pawns on adjacent files.
		if adjFileMask[fileOf(sq)]&p.pieceBB(side, P) == 0 {
			add(e, side, EvalPawns, isolatedMG, isolatedEG)
			if isOpen {
				add(e, side, EvalPawns, isolatedOpenMG, 0)
			}
			// Backward pawn: cannot be defended by any other pawn
		} else if supportMask[side][sq]&p.pieceBB(side, P) == 0 {
			add(e, side, EvalPawns, backwardMG, backwardEG)
			if isOpen {
				add(e, side, EvalPawns, backwardOpenMG, 0)
			}
		}

		// Doubled pawn: a friendly pawn stands directly ahead on the same file.
		// We only penalise when the pawn cannot immediately capture an enemy
		// pawn — if it can, the doubled structure is likely resolved tactically.
		// The penalty is indexed by distance-to-edge so central files (where
		// the doubled pawn blocks the most pawn breaks) are hurt the most in MG,
		// while edge files are punished more in EG (they can rarely promote).

		pushSq := getPushSq(side, sq)

		if pushSq >= 0 && pushSq < 64 && p.pieceBB(side, P)&squareBit(pushSq) != 0 {
			fileIdx := fileOf(sq)
			if fileIdx > 3 {
				fileIdx = 7 - fileIdx
			}
			add(e, side, EvalPawns, doubledPawnMG[fileIdx], doubledPawnEG[fileIdx])
		}

		pieces &= pieces - 1
	}
}

// evaluatePassers scores the passed pawns for one side.
//
//	Passed pawn cannot be blocked or captured on the same
//  or adjacent file ahead of it.  The bonus grows with
//	rank; a pawn on the 7th rank is almost a queen.

func evaluatePassers(p *Pos, e *EvalData, side int) {
	enemy := opp(side)
	pieces := p.pieceBB(side, P)

	for pieces != 0 {
		sq := lsb(pieces)
		add(e, side, EvalMaterial, pieceValMG[P], pieceValEG[P])
		addPST(e, side, P, sq)

		// Passed pawn: no enemy pawns in front on same or adjacent files.
		if passedMask[side][sq]&p.pieceBB(enemy, P) == 0 {

			// pushSq: the square directly in front of this pawn.
			// Pawns can't legally sit on the promotion rank, but guard anyway.
			pushSq := getPushSq(side, sq)

			// Relative rank: 0 = own back rank, 7 = promotion square.
			relRank := getRelRank(side, sq)

			// Blocked: any piece standing on the push square.
			// pushSq is valid for all legal pawn squares (rank 1..6 for White,
			// rank 6..1 for Black), but guard against the promotion edge just in case.
			blocked := 0
			if pushSq >= 0 && pushSq < 64 && p.board[pushSq] != NO_PC {
				blocked = 1
			}
			add(e, side, EvalPassers, passedBonusMG[blocked][relRank], passedBonusEG[blocked][relRank])

			// King proximity: meaningful only from rank 3+.
			// Our king wants to escort; enemy king wants to block.
			if relRank >= 3 && pushSq >= 0 && pushSq < 64 {
				ourDist := chebyshev(p.kingSq[side], pushSq)
				theirDist := chebyshev(p.kingSq[enemy], pushSq)
				add(e, side, EvalPassers, ourPasserProximityMG[ourDist], ourPasserProximityEG[ourDist])
				add(e, side, EvalPassers, theirPasserProximityMG[theirDist], theirPasserProximityEG[theirDist])

				// Slider behind: enemy rook or queen behind the passer on
				// the same file controls the promotion path.
				behindMask := fillBackward(squareBit(sq), side)
				enemySliders := p.pieceBB(enemy, R) | p.pieceBB(enemy, Q)
				if behindMask&enemySliders != 0 {
					add(e, side, EvalPassers, -25, -45)
				}
			}
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

	ownPawns := p.pieceBB(side, P)
	enemyPawns := p.pieceBB(opp(side), P)

	// info depth 30 seldepth 43 multipv 1 time 127349 nodes 134613308 nps 1057042 hashfull 1000 score cp 41 pv e2e4 c7c5 g1f3 e
	penalty := 0

	for df := -1; df <= 1; df++ {
		f := kFile + df
		if f < 0 || f > 7 {
			continue
		}

		fileMask := fileABB << uint(f)

		// Ranks immediately in front of the king (r2 closer, r3 further).
		var r2, r3, r4, r5, r6, r7 int
		if side == White {
			r2, r3, r4, r5, r6, r7 = rankOf(A2), rankOf(A3), rankOf(A4), rankOf(A5), rankOf(A6), rankOf(A7)
		} else {
			r2, r3, r4, r5, r6, r7 = rankOf(A7), rankOf(A6), rankOf(A5), rankOf(A4), rankOf(A3), rankOf(A2)
		}

		hasPawnR2 := ownPawns&squareBit(makeSquare(f, r2)) != 0
		hasPawnR3 := ownPawns&squareBit(makeSquare(f, r3)) != 0
		hasPawnR4 := ownPawns&squareBit(makeSquare(f, r4)) != 0
		hasPawnR5 := ownPawns&squareBit(makeSquare(f, r5)) != 0
		hasPawnR6 := ownPawns&squareBit(makeSquare(f, r6)) != 0
		hasPawnR7 := ownPawns&squareBit(makeSquare(f, r7)) != 0

		hasEnemyR3 := enemyPawns&squareBit(makeSquare(f, r3)) != 0
		hasEnemyR4 := enemyPawns&squareBit(makeSquare(f, r4)) != 0
		hasEnemyR5 := enemyPawns&squareBit(makeSquare(f, r5)) != 0

		// pawns protecting the king should not advance,
		// so they are penalized for it
		if hasPawnR2 {
			penalty = shieldRank2
		} else if hasPawnR3 {
			penalty += shieldRank3				
		} else if hasPawnR4 {
			penalty += shieldRank4
		} else if hasPawnR5 {
			penalty += shieldRank5
		} else if hasPawnR6 {
			penalty += shieldRank6
		} else if hasPawnR7 {
			penalty += shieldRank7
		} else {
			penalty += shieldNoPawn
		}

		// king's file penalty is bigger
		if fileMask & p.pieceBB(side, K) > 0 {
			penalty *= 12
			penalty /= 10
		}

		// penalty for enemy pawns storming our king's position
		if hasEnemyR3 {
			penalty += stormRank3				
		} else if hasEnemyR4 {
			penalty += stormRank4
		} else if hasEnemyR5 {
			penalty += stormRank5
		} else {
			if fileMask&enemyPawns == 0 {
				penalty += stormNoPawn
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
	e.addAttacks(side, K, kingAtk[sq])

	// King-attack danger: pressure accumulated by the *enemy* on our
	// king ring.  We only trigger this when at least two distinct pieces
	// are bearing down on the king zone; a lone attacker is rarely fatal.
	enemy := opp(side)
	if e.attackCnt[enemy] >= 2 {
		// Scale danger by weight and count; kept intentionally modest so
		// the engine does not become reckless about piece sacrifices.
		danger := e.attackWt[enemy] * (e.attackCnt[enemy] + 2) / 8

		occ := p.occupied()

		// notOurDefense: squares not protected by our pieces, excluding
		// our king's coverage since the king may be forced to move anyway.
		notOurDefense := ^(e.attacked[side] &^ e.attackedBy[side][K])

		// Weak squares in king zone: squares the enemy attacks that we
		// don't cover — each is a potential entry point for an attacker.
		weakInRing := e.kingRing[side] & e.attacked[enemy] & notOurDefense

		// Safe checks: enemy pieces that can reach a checking square
		// that is not defended by us — more precise than virtual checks
		// since we only count checks the attacker can safely execute.
		safeForEnemy := ^p.colorBB[enemy] & notOurDefense
		safeKnightChecks := popCount(e.attackedBy[enemy][N] & knightAtk[sq] & safeForEnemy)
		safeBishopChecks := popCount(e.attackedBy[enemy][B] & bishopAttacks(occ, sq) & safeForEnemy)
		safeRookChecks := popCount(e.attackedBy[enemy][R] & rookAttacks(occ, sq) & safeForEnemy)
		safeQueenChecks := popCount(e.attackedBy[enemy][Q] & queenAttacks(occ, sq) & safeForEnemy)

		danger += popCount(weakInRing) * weakInRingWeight
		danger += safeKnightChecks*safeCheckWeight[0] + safeBishopChecks*safeCheckWeight[1] + safeRookChecks*safeCheckWeight[2] + safeQueenChecks*safeCheckWeight[3]

		// Queen contact check: enemy queen can land on a square in our king
		// ring that is supported by enemy pieces but not defended by our
		// non-queen pieces.
		enemySupport := e.attackedBy[enemy][P] | e.attackedBy[enemy][N] |
			e.attackedBy[enemy][B] | e.attackedBy[enemy][R]
		ourDefense := e.attackedBy[side][P] | e.attackedBy[side][N] |
			e.attackedBy[side][B] | e.attackedBy[side][R] | e.attackedBy[side][Q]
		if e.kingRing[side]&e.attackedBy[enemy][Q]&enemySupport & ^ourDefense != 0 {
			danger += queenContactBonus
		}

		// No-queen discount: an attacking force without a queen is far
		// less likely to deliver mate; scale danger down sharply.
		if p.pieceBB(enemy, Q) == 0 {
			danger = danger * noQueenMul / noQueenDiv
		}

		// Apply mostly to MG; small residual in EG so the eval is not
		// completely blind to king safety once queens are traded off.
		add(e, side, EvalSafety, -danger, -danger/dangerEgDiv)
	}
}

// --- Threat eval ---

// evaluateThreats scores the positional pressure exerted by (side)'s pieces
// on the enemy.  Must be called after all attack maps are fully built.
func evaluateThreats(p *Pos, e *EvalData, side int) {
	enemy := opp(side)
	enemyPieces := p.colorBB[enemy]

	// defendedBB: squares the enemy double-covers, or covers with a pawn,
	// or covers without us also double-covering.  Hitting a piece on a
	// defended square is less valuable than hitting a hanging piece.
	defendedBB := e.attackedBy2[enemy] |
		e.attackedBy[enemy][P] |
		(e.attacked[enemy] &^ e.attackedBy2[side])

	// Pawn threats: any enemy piece attacked by our pawns.
	pawnThreats := e.attackedBy[side][P] & enemyPieces
	for bb := pawnThreats; bb != 0; {
		sq := lsb(bb)
		bb &= bb - 1
		victim := p.typeAt(sq)
		add(e, side, EvalThreats, threatByPawnMG[victim], threatByPawnEG[victim])
	}

	// Minor/major piece threats with defended flag.
	for _, attacker := range []int{N, B, R, Q} {
		var mgTable, egTable *[2][6]int
		switch attacker {
		case N:
			mgTable, egTable = &threatByKnightMG, &threatByKnightEG
		case B:
			mgTable, egTable = &threatByBishopMG, &threatByBishopEG
		case R:
			mgTable, egTable = &threatByRookMG, &threatByRookEG
		case Q:
			mgTable, egTable = &threatByQueenMG, &threatByQueenEG
		}
		threats := e.attackedBy[side][attacker] & enemyPieces
		if attacker == Q {
			threats &^= p.pieceBB(enemy, K) // queen doesn't threaten king
		}
		for bb := threats; bb != 0; {
			sq := lsb(bb)
			bb &= bb - 1
			victim := p.typeAt(sq)
			defended := 0
			if defendedBB&squareBit(sq) != 0 {
				defended = 1
			}
			add(e, side, EvalThreats, mgTable[defended][victim], egTable[defended][victim])
		}
	}

	// King threats: king attacks undefended enemy pieces.
	kingThreats := e.attackedBy[side][K] & enemyPieces &^ defendedBB
	for bb := kingThreats; bb != 0; {
		sq := lsb(bb)
		bb &= bb - 1
		victim := p.typeAt(sq)
		add(e, side, EvalThreats, threatByKingMG[victim], threatByKingEG[victim])
	}

	// Push threats: safe pawn advances that would attack an enemy non-pawn.
	// Safe = the push square is not controlled by an enemy pawn.
	occ := p.occupied()
	ownPawns := p.pieceBB(side, P)
	nonPawnEnemies := enemyPieces &^ p.pieceBB(enemy, P)
	enemyPawnAtks := e.attackedBy[enemy][P]
	var pushes uint64
	if side == White {
		pushes = (ownPawns << 8) &^ occ
		// Double push from rank 2.
		pushes |= ((pushes & rank3BB) << 8) &^ occ
	} else {
		pushes = (ownPawns >> 8) &^ occ
		// Double push from rank 7 (relative).
		pushes |= ((pushes & rank6BB) >> 8) &^ occ
	}
	// Only safe pushes: not controlled by an enemy pawn.
	safePushes := pushes &^ enemyPawnAtks
	// Count safe pushes that would attack a non-pawn enemy.
	var pushThreatBB uint64
	if side == White {
		pushThreatBB = ((safePushes << 7) &^ fileHBB) | ((safePushes << 9) &^ fileABB)
	} else {
		pushThreatBB = ((safePushes >> 7) &^ fileHBB) | ((safePushes >> 9) &^ fileABB)
	}
	cnt := popCount(pushThreatBB & nonPawnEnemies)
	add(e, side, EvalThreats, cnt*pushThreatMG, cnt*pushThreatEG)
}

// --- Helpers ---

func getRelRank(side, sq int) int {
	if side == White {
		return rankOf(sq)
	} 
	return 7 - rankOf(sq)		
}

func getPushSq(side, sq int) int {
	if side == White {
		return sq + 8
	}
	return sq - 8
}

func addPhalanx(e *EvalData, side, sq int) {
	add(e, side, EvalPawns, phalanxMgByColor[side][sq], phalanxEgByColor[side][sq])
}

// addPST adds the piece-square table score for a piece on sq.
func addPST(e *EvalData, side, piece, sq int) {
	add(e, side, EvalPst, pstMGByColor[side][piece][sq], pstEGByColor[side][piece][sq])
}

// add adds MG/EG scores for one side to EvalData.
func add(e *EvalData, side int, component EvalComponent, mg, eg int) {
	e.mgScore[side][component] += mg
	e.egScore[side][component] += eg
}
