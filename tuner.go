package main

// ============================================================
// Design principles:
//   1. eval.go is NEVER modified
//   2. Full engine eval provides the "frozen" component via the
//      additional_score trick:
//        additional_score = engine_score - linear_dot(coeffs, init_params)
//      All non-linear terms (king safety, dev penalty, drawishness,
//      hard-coded constants) live here permanently.
//   3. Every linear eval term is extracted as a sparse coefficient and
//      ALL 550 parameters are tuned simultaneously in one Adam pass.
//
// Parameter layout (each entry is an [mg, eg] pair):
//
//   [  0..  4]  pieceVal[P..Q]                             5
//   [  5..388]  pst[P..K][sq 0..63]                      384  (6x64)
//   [389..392]  mob[N..Q]                                   4
//   [393]       bishopPair                                  1
//   [394]       rookOpenFile                                1
//   [395]       rookSemiOpenFile                            1
//   [396..459]  phalanxPST[sq 0..63]                      64
//   [460]       isolated                                    1
//   [461]       isolatedOpen   (EG always 0)               1
//   [462]       backward                                    1
//   [463]       backwardOpen   (EG always 0)               1
//   [464..467]  doubledPawn[edge-dist 0..3]                4
//   [468..483]  passedBonus[blocked 0..1][relRank 0..7]   16
//   [484..491]  ourPasserProximity[dist 0..7]               8
//   [492..499]  theirPasserProximity[dist 0..7]             8
//   [500..504]  threatByPawn[victim P..Q]                   5
//   [505..514]  threatByKnight[def 0..1][victim P..Q]      10
//   [515..524]  threatByBishop[def 0..1][victim P..Q]      10
//   [525..534]  threatByRook  [def 0..1][victim P..Q]      10
//   [535..544]  threatByQueen [def 0..1][victim P..Q]      10
//   [545..548]  threatByKing  [victim P..R]                 4
//   [549]       pushThreat                                  1
//   Total: 550 parameters
//
// Left in additional_score (non-linear):
//   - King safety danger formula (quadratic in attack weights)
//   - devPenaltyScale (quadratic in undeveloped count)
//   - Drawishness scaling
//   - Slider-behind passer penalty (hard-coded -25/-45)
// ============================================================

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

var pstLabels = [6]string{"P", "N", "B", "R", "Q", "K"}

// parseResultFromLine auto-detects the file format and returns a result in [0,1].
// Book format: line ends with a bracketed float, e.g. "[0.0]", "[0.5]", "[1.0]".
// EPD format:  result is embedded as the string "1-0", "0-1", or "1/2-1/2".
func parseResultFromLine(line string) float64 {
	// Book format detection: look for trailing [score].
	if lb := strings.LastIndex(line, "["); lb != -1 {
		if rb := strings.LastIndex(line, "]"); rb > lb {
			if score, err := strconv.ParseFloat(strings.TrimSpace(line[lb+1:rb]), 64); err == nil {
				return score
			}
		}
	}

	// EPD format detection.
	// Semicolon format: "fen; 0.0"
	if idx := strings.LastIndex(line, ";"); idx != -1 {
		if score, err := strconv.ParseFloat(strings.TrimSpace(line[idx+1:]), 64); err == nil {
			return score
		}
	}
	// EPD format: "1-0" / "0-1" in comment
	if strings.Contains(line, "1-0") {
		return 1.0
	} else if strings.Contains(line, "0-1") {
		return 0.0
	}
	return 0.5
}

// --- Parameter indices ---

const (
	tv2IdxPieceVal     = 0
	tv2IdxPST          = 5
	tv2IdxMob          = 389
	tv2IdxBishopPair   = 393
	tv2IdxRookOpen     = 394
	tv2IdxRookSemi     = 395
	tv2IdxPhalanx      = 396
	tv2IdxIsolated     = 460
	tv2IdxIsolatedOpen = 461
	tv2IdxBackward     = 462
	tv2IdxBackwardOpen = 463
	tv2IdxDoubled      = 464
	tv2IdxPassedBonus  = 468
	tv2IdxOurProx      = 484
	tv2IdxTheirProx    = 492
	tv2IdxThreatPawn   = 500
	tv2IdxThreatKnight = 505
	tv2IdxThreatBishop = 515
	tv2IdxThreatRook   = 525
	tv2IdxThreatQueen  = 535
	tv2IdxThreatKing   = 545
	tv2IdxPushThreat   = 549
	tv2NumParams       = 550
)

type tv2Pair [2]float64

type tv2Coeff struct {
	index int16
	value int16
}

type tv2Entry struct {
	result          float64
	additionalScore float64
	phase           int
	coeffs          []tv2Coeff
}

// --- Parameter initialisation ---

func tv2InitParams() [tv2NumParams]tv2Pair {
	var p [tv2NumParams]tv2Pair

	for piece := P; piece <= Q; piece++ {
		p[tv2IdxPieceVal+piece] = tv2Pair{float64(pieceValMG[piece]), float64(pieceValEG[piece])}
	}
	for piece := P; piece <= K; piece++ {
		for sq := 0; sq < 64; sq++ {
			p[tv2IdxPST+piece*64+sq] = tv2Pair{float64(pstMG[piece][sq]), float64(pstEG[piece][sq])}
		}
	}
	for piece := N; piece <= Q; piece++ {
		p[tv2IdxMob+(piece-N)] = tv2Pair{float64(mobMG[piece]), float64(mobEG[piece])}
	}
	p[tv2IdxBishopPair] = tv2Pair{float64(bishopPairMG), float64(bishopPairEG)}
	p[tv2IdxRookOpen] = tv2Pair{float64(rookOpenFileMG), float64(rookOpenFileEG)}
	p[tv2IdxRookSemi] = tv2Pair{float64(rookSemiOpenFileMG), float64(rookSemiOpenFileEG)}

	for sq := 0; sq < 64; sq++ {
		p[tv2IdxPhalanx+sq] = tv2Pair{float64(phalanxMG[sq]), float64(phalanxEG[sq])}
	}

	p[tv2IdxIsolated] = tv2Pair{float64(isolatedMG), float64(isolatedEG)}
	p[tv2IdxIsolatedOpen] = tv2Pair{float64(isolatedOpenMG), 0}
	p[tv2IdxBackward] = tv2Pair{float64(backwardMG), float64(backwardEG)}
	p[tv2IdxBackwardOpen] = tv2Pair{float64(backwardOpenMG), 0}

	for i := 0; i < 4; i++ {
		p[tv2IdxDoubled+i] = tv2Pair{float64(doubledPawnMG[i]), float64(doubledPawnEG[i])}
	}
	for blocked := 0; blocked < 2; blocked++ {
		for rank := 0; rank < 8; rank++ {
			p[tv2IdxPassedBonus+blocked*8+rank] = tv2Pair{
				float64(passedBonusMG[blocked][rank]),
				float64(passedBonusEG[blocked][rank]),
			}
		}
	}
	for d := 0; d < 8; d++ {
		p[tv2IdxOurProx+d] = tv2Pair{float64(ourPasserProximityMG[d]), float64(ourPasserProximityEG[d])}
		p[tv2IdxTheirProx+d] = tv2Pair{float64(theirPasserProximityMG[d]), float64(theirPasserProximityEG[d])}
	}

	for v := P; v <= Q; v++ {
		p[tv2IdxThreatPawn+v] = tv2Pair{float64(threatByPawnMG[v]), float64(threatByPawnEG[v])}
	}
	for def := 0; def <= 1; def++ {
		for v := P; v <= Q; v++ {
			p[tv2IdxThreatKnight+def*5+v] = tv2Pair{float64(threatByKnightMG[def][v]), float64(threatByKnightEG[def][v])}
			p[tv2IdxThreatBishop+def*5+v] = tv2Pair{float64(threatByBishopMG[def][v]), float64(threatByBishopEG[def][v])}
			p[tv2IdxThreatRook+def*5+v] = tv2Pair{float64(threatByRookMG[def][v]), float64(threatByRookEG[def][v])}
			p[tv2IdxThreatQueen+def*5+v] = tv2Pair{float64(threatByQueenMG[def][v]), float64(threatByQueenEG[def][v])}
		}
	}
	for v := P; v <= R; v++ {
		p[tv2IdxThreatKing+v] = tv2Pair{float64(threatByKingMG[v]), float64(threatByKingEG[v])}
	}
	p[tv2IdxPushThreat] = tv2Pair{float64(pushThreatMG), float64(pushThreatEG)}

	return p
}

// --- Feature extraction ---
// Returns dense coefficients where c[i] = White_count_i - Black_count_i.
// Mirrors every linear term in eval.go exactly.

func tv2GetCoefficients(pos *Pos) [tv2NumParams]int16 {
	var c [tv2NumParams]int16

	// Build attack maps (mirrors eval_internal setup).
	var e EvalData
	e.attackedBy2[White] = doubleWPAttacks(pos.pieceBB(White, P))
	e.attackedBy2[Black] = doubleBPAttacks(pos.pieceBB(Black, P))
	e.attackedBy[White][P] = shiftWPAttacks(pos.pieceBB(White, P))
	e.attackedBy[Black][P] = shiftBPAttacks(pos.pieceBB(Black, P))
	e.attacked[White] = e.attackedBy[White][P]
	e.attacked[Black] = e.attackedBy[Black][P]
	e.kingRing[White] = kingAtk[pos.kingSq[White]]
	e.kingRing[Black] = kingAtk[pos.kingSq[Black]]
	evaluatePieces(pos, &e, White)
	evaluatePieces(pos, &e, Black)
	e.addAttacks(White, K, kingAtk[pos.kingSq[White]])
	e.addAttacks(Black, K, kingAtk[pos.kingSq[Black]])

	occ := pos.occupied()

	for side := White; side <= Black; side++ {
		sign := int16(1)
		if side == Black {
			sign = -1
		}
		enemy := opp(side)

		// X-ray occupancies — same as evaluatePieces.
		occForBishop := occ ^ (pos.pieceBB(side, B) | pos.pieceBB(side, Q))
		occForRook := occ ^ (pos.pieceBB(side, R) | pos.pieceBB(side, Q))
		occForQueen := occ ^ (pos.pieceBB(side, B) | pos.pieceBB(side, R))

		// Pawns: material + PST (mirrors evaluatePassers).
		for bb := pos.pieceBB(side, P); bb != 0; bb &= bb - 1 {
			sq := lsb(bb)
			pstSq := sq
			if side == Black {
				pstSq = sq ^ 56
			}
			c[tv2IdxPieceVal+P] += sign
			c[tv2IdxPST+P*64+pstSq] += sign
		}

		// Pieces N..Q: material + PST + mobility + rook file bonuses.
		for piece := N; piece <= Q; piece++ {
			for bb := pos.pieceBB(side, piece); bb != 0; bb &= bb - 1 {
				sq := lsb(bb)
				pstSq := sq
				if side == Black {
					pstSq = sq ^ 56
				}
				c[tv2IdxPieceVal+piece] += sign
				c[tv2IdxPST+piece*64+pstSq] += sign

				var mob int16
				switch piece {
				case N:
					mob = int16(popCount(knightAtk[sq]&^pos.colorBB[side])) - int16(mobOffset[N])
					c[tv2IdxMob+0] += sign * mob
				case B:
					mob = int16(popCount(bishopAttacks(occForBishop, sq))) - int16(mobOffset[B])
					c[tv2IdxMob+1] += sign * mob
				case R:
					mob = int16(popCount(rookAttacks(occForRook, sq))) - int16(mobOffset[R])
					c[tv2IdxMob+2] += sign * mob
					fileMask := fileABB << uint(fileOf(sq))
					if fileMask&pos.pieceBB(side, P) == 0 {
						if fileMask&pos.pieceBB(enemy, P) == 0 {
							c[tv2IdxRookOpen] += sign
						} else {
							c[tv2IdxRookSemi] += sign
						}
					}
				case Q:
					mob = int16(popCount(queenAttacks(occForQueen, sq))) - int16(mobOffset[Q])
					c[tv2IdxMob+3] += sign * mob
				}
			}
		}

		// King PST.
		{
			sq := pos.kingSq[side]
			pstSq := sq
			if side == Black {
				pstSq = sq ^ 56
			}
			c[tv2IdxPST+K*64+pstSq] += sign
		}

		// Bishop pair.
		if popCount(pos.pieceBB(side, B)) >= 2 {
			c[tv2IdxBishopPair] += sign
		}

		// Phalanx PST.
		for bb := pos.pieceBB(side, P); bb != 0; bb &= bb - 1 {
			sq := lsb(bb)
			if shiftSides(squareBit(sq))&pos.pieceBB(side, P) != 0 {
				pstSq := sq
				if side == Black {
					pstSq = sq ^ 56
				}
				c[tv2IdxPhalanx+pstSq] += sign
			}
		}

		// Pawn structure: isolated, backward, doubled.
		ownPawns := pos.pieceBB(side, P)
		var pushSq int
		for bb := ownPawns; bb != 0; bb &= bb - 1 {
			sq := lsb(bb)
			frontMask := fillForward(squareBit(sq), side)
			isOpen := frontMask&ownPawns == 0

			if adjFileMask[fileOf(sq)]&ownPawns == 0 {
				c[tv2IdxIsolated] += sign
				if isOpen {
					c[tv2IdxIsolatedOpen] += sign
				}
			} else if supportMask[side][sq]&ownPawns == 0 {
				c[tv2IdxBackward] += sign
				if isOpen {
					c[tv2IdxBackwardOpen] += sign
				}
			}

			if side == White {
				pushSq = sq + 8
			} else {
				pushSq = sq - 8
			}
			if pushSq >= 0 && pushSq < 64 && ownPawns&squareBit(pushSq) != 0 {
				if pawnAtk[side][sq]&pos.pieceBB(enemy, P) == 0 {
					fileIdx := fileOf(sq)
					if fileIdx > 3 {
						fileIdx = 7 - fileIdx
					}
					c[tv2IdxDoubled+fileIdx] += sign
				}
			}
		}

		// Passed pawns + king proximity.
		for bb := ownPawns; bb != 0; bb &= bb - 1 {
			sq := lsb(bb)
			if passedMask[side][sq]&pos.pieceBB(enemy, P) != 0 {
				continue
			}
			if side == White {
				pushSq = sq + 8
			} else {
				pushSq = sq - 8
			}
			var relRank int
			if side == White {
				relRank = rankOf(sq)
			} else {
				relRank = 7 - rankOf(sq)
			}
			blocked := 0
			if pushSq >= 0 && pushSq < 64 && pos.board[pushSq] != NO_PC {
				blocked = 1
			}
			c[tv2IdxPassedBonus+blocked*8+relRank] += sign

			if relRank >= 3 && pushSq >= 0 && pushSq < 64 {
				ourDist := chebyshev(pos.kingSq[side], pushSq)
				theirDist := chebyshev(pos.kingSq[enemy], pushSq)
				c[tv2IdxOurProx+ourDist] += sign
				c[tv2IdxTheirProx+theirDist] += sign
			}
		}

		// Threats.
		enemyPieces := pos.colorBB[enemy]
		defendedBB := e.attackedBy2[enemy] |
			e.attackedBy[enemy][P] |
			(e.attacked[enemy] &^ e.attackedBy2[side])

		for bb := e.attackedBy[side][P] & enemyPieces; bb != 0; bb &= bb - 1 {
			victim := pos.typeAt(lsb(bb))
			if victim <= Q {
				c[tv2IdxThreatPawn+victim] += sign
			}
		}

		for _, att := range [4]struct{ piece, base int }{
			{N, tv2IdxThreatKnight},
			{B, tv2IdxThreatBishop},
			{R, tv2IdxThreatRook},
			{Q, tv2IdxThreatQueen},
		} {
			threats := e.attackedBy[side][att.piece] & enemyPieces
			if att.piece == Q {
				threats &^= pos.pieceBB(enemy, K)
			}
			for bb := threats; bb != 0; bb &= bb - 1 {
				sq := lsb(bb)
				victim := pos.typeAt(sq)
				if victim <= Q {
					def := 0
					if defendedBB&squareBit(sq) != 0 {
						def = 1
					}
					c[att.base+def*5+victim] += sign
				}
			}
		}

		for bb := e.attackedBy[side][K] & enemyPieces &^ defendedBB; bb != 0; bb &= bb - 1 {
			victim := pos.typeAt(lsb(bb))
			if victim <= R {
				c[tv2IdxThreatKing+victim] += sign
			}
		}

		// Push threats.
		var pushes uint64
		nonPawnEnemies := enemyPieces &^ pos.pieceBB(enemy, P)
		if side == White {
			pushes = (ownPawns << 8) &^ occ
			pushes |= ((pushes & rank3BB) << 8) &^ occ
		} else {
			pushes = (ownPawns >> 8) &^ occ
			pushes |= ((pushes & rank6BB) >> 8) &^ occ
		}
		safePushes := pushes &^ e.attackedBy[enemy][P]
		var pushThreatBB uint64
		if side == White {
			pushThreatBB = ((safePushes << 7) &^ fileHBB) | ((safePushes << 9) &^ fileABB)
		} else {
			pushThreatBB = ((safePushes >> 7) &^ fileHBB) | ((safePushes >> 9) &^ fileABB)
		}
		c[tv2IdxPushThreat] += sign * int16(popCount(pushThreatBB&nonPawnEnemies))
	}

	return c
}

func tv2ToSparse(dense [tv2NumParams]int16) []tv2Coeff {
	sparse := make([]tv2Coeff, 0, 48)
	for i, v := range dense {
		if v != 0 {
			sparse = append(sparse, tv2Coeff{int16(i), v})
		}
	}
	return sparse
}

// --- Linear evaluation ---

func tv2LinDot(coeffs []tv2Coeff, params *[tv2NumParams]tv2Pair) (mg, eg float64) {
	for _, c := range coeffs {
		f := float64(c.value)
		mg += f * params[c.index][0]
		eg += f * params[c.index][1]
	}
	return
}

func tv2Tapered(mg, eg float64, phase int) float64 {
	return (mg*float64(phase) + eg*float64(24-phase)) / 24.0
}

func tv2Score(e *tv2Entry, params *[tv2NumParams]tv2Pair) float64 {
	mg, eg := tv2LinDot(e.coeffs, params)
	return e.additionalScore + tv2Tapered(mg, eg, e.phase)
}

// tv2Phase counts material phase from the FEN board section (N/B=1, R=2, Q=4, cap 24).
func tv2Phase(fen string) int {
	phase := 0
	for i := 0; i < len(fen) && fen[i] != ' '; i++ {
		switch fen[i] {
		case 'n', 'N', 'b', 'B':
			phase++
		case 'r', 'R':
			phase += 2
		case 'q', 'Q':
			phase += 4
		}
	}
	if phase > 24 {
		phase = 24
	}
	return phase
}

// --- Dataset construction ---
// additionalScore = engine_score - linear_dot(coeffs, initParams) freezes all
// non-linear terms for the entire tuning run.

func tv2BuildDataset(lines []string, initParams *[tv2NumParams]tv2Pair) []tv2Entry {
	var pos Pos
	entries := make([]tv2Entry, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		parseFEN(&pos, line)
		result := parseResultFromLine(line)

		engineScore := float64(eval_internal(&pos, false, nil))
		if pos.side == Black {
			engineScore = -engineScore
		}

		phase := tv2Phase(line)
		dense := tv2GetCoefficients(&pos)
		sparse := tv2ToSparse(dense)

		mg, eg := tv2LinDot(sparse, initParams)
		additionalScore := engineScore - tv2Tapered(mg, eg, phase)

		entries = append(entries, tv2Entry{
			result:          result,
			additionalScore: additionalScore,
			phase:           phase,
			coeffs:          sparse,
		})
	}
	return entries
}

// --- Sigmoid + MSE ---

func tv2Sigmoid(score, k float64) float64 {
	return 1.0 / (1.0 + math.Pow(10.0, -k*score/400.0))
}

func tv2SigDeriv(score, k float64) float64 {
	p := tv2Sigmoid(score, k)
	return math.Log(10) * k / 400.0 * p * (1.0 - p)
}

func tv2MSE(data []tv2Entry, params *[tv2NumParams]tv2Pair, k float64) float64 {
	nw := runtime.NumCPU()
	if nw > len(data) {
		nw = len(data)
	}
	chunk := (len(data) + nw - 1) / nw
	totals := make([]float64, nw)
	var wg sync.WaitGroup
	for w := 0; w < nw; w++ {
		wg.Add(1)
		go func(wid int) {
			defer wg.Done()
			start := wid * chunk
			end := start + chunk
			if end > len(data) {
				end = len(data)
			}
			sum := 0.0
			for i := start; i < end; i++ {
				d := data[i].result - tv2Sigmoid(tv2Score(&data[i], params), k)
				sum += d * d
			}
			totals[wid] = sum
		}(w)
	}
	wg.Wait()
	total := 0.0
	for _, t := range totals {
		total += t
	}
	return total / float64(len(data))
}

// tv2FitK finds optimal Texel K via ternary search over [0.1, 6.0].
func tv2FitK(data []tv2Entry, params *[tv2NumParams]tv2Pair) float64 {
	lo, hi := 0.1, 6.0
	for i := 0; i < 50; i++ {
		m1 := lo + (hi-lo)/3.0
		m2 := hi - (hi-lo)/3.0
		if tv2MSE(data, params, m1) < tv2MSE(data, params, m2) {
			hi = m2
		} else {
			lo = m1
		}
	}
	return (lo + hi) / 2.0
}

// --- Adam optimiser ---

const (
	tv2Beta1   = 0.9
	tv2Beta2   = 0.999
	tv2AdamEps = 1e-8
	tv2LRMin   = 1e-4
)

func tv2Epoch(
	data []tv2Entry,
	params *[tv2NumParams]tv2Pair,
	mom, vel *[tv2NumParams]tv2Pair,
	lr, k float64,
) float64 {
	nw := runtime.NumCPU()
	if nw > len(data) {
		nw = len(data)
	}

	type wResult struct {
		grad [tv2NumParams]tv2Pair
		loss float64
	}
	results := make([]wResult, nw)
	chunk := (len(data) + nw - 1) / nw

	var wg sync.WaitGroup
	for w := 0; w < nw; w++ {
		wg.Add(1)
		go func(wid int) {
			defer wg.Done()
			start := wid * chunk
			end := start + chunk
			if end > len(data) {
				end = len(data)
			}
			var r wResult
			for i := start; i < end; i++ {
				e := &data[i]
				score := tv2Score(e, params)
				pred := tv2Sigmoid(score, k)
				diff := pred - e.result
				r.loss += diff * diff

				dLoss := 2.0 * diff * tv2SigDeriv(score, k)
				mgF := float64(e.phase) / 24.0
				egF := float64(24-e.phase) / 24.0

				for _, coeff := range e.coeffs {
					f := float64(coeff.value)
					r.grad[coeff.index][0] += dLoss * f * mgF
					r.grad[coeff.index][1] += dLoss * f * egF
				}
			}
			results[wid] = r
		}(w)
	}
	wg.Wait()

	var totalGrad [tv2NumParams]tv2Pair
	totalLoss := 0.0
	for _, r := range results {
		totalLoss += r.loss
		for i := range r.grad {
			totalGrad[i][0] += r.grad[i][0]
			totalGrad[i][1] += r.grad[i][1]
		}
	}

	scale := 1.0 / float64(len(data))
	for i := range params {
		for ph := 0; ph < 2; ph++ {
			g := totalGrad[i][ph] * scale
			mom[i][ph] = tv2Beta1*mom[i][ph] + (1-tv2Beta1)*g
			vel[i][ph] = tv2Beta2*vel[i][ph] + (1-tv2Beta2)*g*g
			params[i][ph] -= lr * mom[i][ph] / (math.Sqrt(vel[i][ph]) + tv2AdamEps)
		}
	}

	return totalLoss * scale
}

// --- Output ---

func tv2Round(x float64) int {
	if x >= 0 {
		return int(x + 0.5)
	}
	return int(x - 0.5)
}

// tv2PrintParams prints all tuned parameters as ready-to-paste Go source.
// PST normalisation: each PST mean is removed from the squares and added
// to pieceVal, preserving the zero-centred PST convention from params.go.
func tv2PrintParams(params *[tv2NumParams]tv2Pair) {
	norm := *params
	for piece := P; piece <= K; piece++ {
		for ph := 0; ph < 2; ph++ {
			sum := 0.0
			for sq := 0; sq < 64; sq++ {
				sum += params[tv2IdxPST+piece*64+sq][ph]
			}
			mean := sum / 64.0
			for sq := 0; sq < 64; sq++ {
				norm[tv2IdxPST+piece*64+sq][ph] -= mean
			}
			if piece <= Q {
				norm[tv2IdxPieceVal+piece][ph] += mean
			}
		}
	}

	fmt.Printf("var pieceValMG = [7]int{%d, %d, %d, %d, %d, 0, 0}\n",
		tv2Round(norm[tv2IdxPieceVal+P][0]), tv2Round(norm[tv2IdxPieceVal+N][0]),
		tv2Round(norm[tv2IdxPieceVal+B][0]), tv2Round(norm[tv2IdxPieceVal+R][0]),
		tv2Round(norm[tv2IdxPieceVal+Q][0]))
	fmt.Printf("var pieceValEG = [7]int{%d, %d, %d, %d, %d, 0, 0}\n",
		tv2Round(norm[tv2IdxPieceVal+P][1]), tv2Round(norm[tv2IdxPieceVal+N][1]),
		tv2Round(norm[tv2IdxPieceVal+B][1]), tv2Round(norm[tv2IdxPieceVal+R][1]),
		tv2Round(norm[tv2IdxPieceVal+Q][1]))

	fmt.Printf("var mobOffset = [7]int{0, 4, 6, 7, 14, 0, 0}\n") // not tuned

	fmt.Printf("var mobMG = [7]int{0, %d, %d, %d, %d, 0, 0}\n",
		tv2Round(norm[tv2IdxMob+0][0]), tv2Round(norm[tv2IdxMob+1][0]),
		tv2Round(norm[tv2IdxMob+2][0]), tv2Round(norm[tv2IdxMob+3][0]))
	fmt.Printf("var mobEG = [7]int{0, %d, %d, %d, %d, 0, 0}\n",
		tv2Round(norm[tv2IdxMob+0][1]), tv2Round(norm[tv2IdxMob+1][1]),
		tv2Round(norm[tv2IdxMob+2][1]), tv2Round(norm[tv2IdxMob+3][1]))

	fmt.Printf("const bishopPairMG = %d\n", tv2Round(norm[tv2IdxBishopPair][0]))
	fmt.Printf("const bishopPairEG = %d\n", tv2Round(norm[tv2IdxBishopPair][1]))
	fmt.Printf("const rookOpenFileMG = %d\n", tv2Round(norm[tv2IdxRookOpen][0]))
	fmt.Printf("const rookOpenFileEG = %d\n", tv2Round(norm[tv2IdxRookOpen][1]))
	fmt.Printf("const rookSemiOpenFileMG = %d\n", tv2Round(norm[tv2IdxRookSemi][0]))
	fmt.Printf("const rookSemiOpenFileEG = %d\n", tv2Round(norm[tv2IdxRookSemi][1]))

	fmt.Printf("const isolatedMG = %d\n", tv2Round(norm[tv2IdxIsolated][0]))
	fmt.Printf("const isolatedEG = %d\n", tv2Round(norm[tv2IdxIsolated][1]))
	fmt.Printf("const isolatedOpenMG = %d\n", tv2Round(norm[tv2IdxIsolatedOpen][0]))
	fmt.Printf("const backwardMG = %d\n", tv2Round(norm[tv2IdxBackward][0]))
	fmt.Printf("const backwardEG = %d\n", tv2Round(norm[tv2IdxBackward][1]))
	fmt.Printf("const backwardOpenMG = %d\n", tv2Round(norm[tv2IdxBackwardOpen][0]))
	fmt.Printf("var doubledPawnMG = [4]int{%d, %d, %d, %d}\n",
		tv2Round(norm[tv2IdxDoubled+0][0]), tv2Round(norm[tv2IdxDoubled+1][0]),
		tv2Round(norm[tv2IdxDoubled+2][0]), tv2Round(norm[tv2IdxDoubled+3][0]))
	fmt.Printf("var doubledPawnEG = [4]int{%d, %d, %d, %d}\n",
		tv2Round(norm[tv2IdxDoubled+0][1]), tv2Round(norm[tv2IdxDoubled+1][1]),
		tv2Round(norm[tv2IdxDoubled+2][1]), tv2Round(norm[tv2IdxDoubled+3][1]))

	// passedBonus: printed as valid Go array literal.
	fmt.Println("var passedBonusMG = [2][8]int{")
	for blocked := 0; blocked < 2; blocked++ {
		fmt.Printf("\t%d: {", blocked)
		for rank := 0; rank < 8; rank++ {
			if rank > 0 {
				fmt.Print(", ")
			}
			fmt.Print(tv2Round(norm[tv2IdxPassedBonus+blocked*8+rank][0]))
		}
		fmt.Println("},")
	}
	fmt.Println("}")
	fmt.Println("var passedBonusEG = [2][8]int{")
	for blocked := 0; blocked < 2; blocked++ {
		fmt.Printf("\t%d: {", blocked)
		for rank := 0; rank < 8; rank++ {
			if rank > 0 {
				fmt.Print(", ")
			}
			fmt.Print(tv2Round(norm[tv2IdxPassedBonus+blocked*8+rank][1]))
		}
		fmt.Println("},")
	}
	fmt.Println("}")

	fmt.Printf("var ourPasserProximityMG = [8]int{")
	for d := 0; d < 8; d++ {
		if d > 0 {
			fmt.Print(", ")
		}
		fmt.Print(tv2Round(norm[tv2IdxOurProx+d][0]))
	}
	fmt.Println("}")
	fmt.Printf("var ourPasserProximityEG = [8]int{")
	for d := 0; d < 8; d++ {
		if d > 0 {
			fmt.Print(", ")
		}
		fmt.Print(tv2Round(norm[tv2IdxOurProx+d][1]))
	}
	fmt.Println("}")
	fmt.Printf("var theirPasserProximityMG = [8]int{")
	for d := 0; d < 8; d++ {
		if d > 0 {
			fmt.Print(", ")
		}
		fmt.Print(tv2Round(norm[tv2IdxTheirProx+d][0]))
	}
	fmt.Println("}")
	fmt.Printf("var theirPasserProximityEG = [8]int{")
	for d := 0; d < 8; d++ {
		if d > 0 {
			fmt.Print(", ")
		}
		fmt.Print(tv2Round(norm[tv2IdxTheirProx+d][1]))
	}
	fmt.Println("}")

	tv2PrintThreat1("threatByPawnMG", &norm, tv2IdxThreatPawn, 0)
	tv2PrintThreat1("threatByPawnEG", &norm, tv2IdxThreatPawn, 1)
	tv2PrintThreat2("threatByKnightMG", &norm, tv2IdxThreatKnight, 0)
	tv2PrintThreat2("threatByKnightEG", &norm, tv2IdxThreatKnight, 1)
	tv2PrintThreat2("threatByBishopMG", &norm, tv2IdxThreatBishop, 0)
	tv2PrintThreat2("threatByBishopEG", &norm, tv2IdxThreatBishop, 1)
	tv2PrintThreat2("threatByRookMG", &norm, tv2IdxThreatRook, 0)
	tv2PrintThreat2("threatByRookEG", &norm, tv2IdxThreatRook, 1)
	tv2PrintThreat2("threatByQueenMG", &norm, tv2IdxThreatQueen, 0)
	tv2PrintThreat2("threatByQueenEG", &norm, tv2IdxThreatQueen, 1)
	fmt.Printf("var threatByKingMG = [6]int{%d, %d, %d, %d, 0, 0}\n",
		tv2Round(norm[tv2IdxThreatKing+P][0]), tv2Round(norm[tv2IdxThreatKing+N][0]),
		tv2Round(norm[tv2IdxThreatKing+B][0]), tv2Round(norm[tv2IdxThreatKing+R][0]))
	fmt.Printf("var threatByKingEG = [6]int{%d, %d, %d, %d, 0, 0}\n",
		tv2Round(norm[tv2IdxThreatKing+P][1]), tv2Round(norm[tv2IdxThreatKing+N][1]),
		tv2Round(norm[tv2IdxThreatKing+B][1]), tv2Round(norm[tv2IdxThreatKing+R][1]))
	fmt.Printf("const pushThreatMG = %d\n", tv2Round(norm[tv2IdxPushThreat][0]))
	fmt.Printf("const pushThreatEG = %d\n", tv2Round(norm[tv2IdxPushThreat][1]))

	for _, t := range []struct {
		ph   int
		name string
	}{{0, "phalanxMG"}, {1, "phalanxEG"}} {
		fmt.Printf("var %s = [64]int{\n", t.name)
		for rank := 0; rank < 8; rank++ {
			fmt.Print("\t")
			for file := 0; file < 8; file++ {
				sq := rank*8 + file
				if file > 0 {
					fmt.Print(", ")
				}
				fmt.Printf("%4d", tv2Round(norm[tv2IdxPhalanx+sq][t.ph]))
			}
			fmt.Println(",")
		}
		fmt.Println("}")
	}

	for _, t := range []struct {
		ph   int
		name string
	}{{0, "pstMG"}, {1, "pstEG"}} {
		fmt.Printf("var %s = [6][64]int{\n", t.name)
		for piece := P; piece <= K; piece++ {
			fmt.Printf("\t%s: {\n", pstLabels[piece])
			for rank := 0; rank < 8; rank++ {
				fmt.Print("\t\t")
				for file := 0; file < 8; file++ {
					sq := rank*8 + file
					if file > 0 {
						fmt.Print(", ")
					}
					fmt.Printf("%4d", tv2Round(norm[tv2IdxPST+piece*64+sq][t.ph]))
				}
				fmt.Println(",")
			}
			fmt.Println("\t},")
		}
		fmt.Println("}")
	}
}

func tv2PrintThreat1(name string, p *[tv2NumParams]tv2Pair, base, ph int) {
	fmt.Printf("var %s = [6]int{", name)
	for v := P; v <= Q; v++ {
		if v > P {
			fmt.Print(", ")
		}
		fmt.Print(tv2Round(p[base+v][ph]))
	}
	fmt.Println(", 0}")
}

func tv2PrintThreat2(name string, p *[tv2NumParams]tv2Pair, base, ph int) {
	fmt.Printf("var %s = [2][6]int{\n", name)
	for def := 0; def <= 1; def++ {
		fmt.Print("\t{")
		for v := P; v <= Q; v++ {
			if v > P {
				fmt.Print(", ")
			}
			fmt.Print(tv2Round(p[base+def*5+v][ph]))
		}
		fmt.Println(", 0},")
	}
	fmt.Println("}")
}

// --- Entry point ---

func tv2Tune(filename string, epochs int, lr float64) {
	fmt.Printf("[tuner-v2] loading %q...\n", filename)
	f, err := os.Open(filename)
	if err != nil {
		fmt.Println("[tuner-v2] error:", err)
		return
	}
	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		if line := sc.Text(); line != "" {
			lines = append(lines, line)
		}
	}
	f.Close()
	fmt.Printf("[tuner-v2] %d positions read\n", len(lines))

	params := tv2InitParams()

	fmt.Printf("[tuner-v2] %d params x %d positions (%d goroutines)...\n",
		tv2NumParams, len(lines), runtime.NumCPU())
	data := tv2BuildDataset(lines, &params)
	fmt.Printf("[tuner-v2] dataset built: %d entries\n", len(data))

	k := tv2FitK(data, &params)
	fmt.Printf("[tuner-v2] K = %.6f\n", k)

	var mom, vel [tv2NumParams]tv2Pair

	lrInit := lr
	prevLoss := tv2MSE(data, &params, k)
	fmt.Printf("[tuner-v2] initial MSE = %.10f  lr = %.6f\n", prevLoss, lr)

	stall := 0
	for epoch := 1; epoch <= epochs; epoch++ {
		// Cosine annealing: lr_init -> tv2LRMin over the full run.
		lr = tv2LRMin + 0.5*(lrInit-tv2LRMin)*(1+math.Cos(math.Pi*float64(epoch-1)/float64(epochs)))
		loss := tv2Epoch(data, &params, &mom, &vel, lr, k)
		fmt.Printf("epoch %4d  loss = %.10f  lr = %.6f\n", epoch, loss, lr)

		if epoch%100 == 0 {
			fmt.Println("--- snapshot ---")
			tv2PrintParams(&params)
			fmt.Println("--- end snapshot ---")
		}

		if lr <= tv2LRMin*1.01 && math.Abs(prevLoss-loss) < 1e-10 {
			stall++
			if stall >= 20 {
				fmt.Println("[tuner-v2] converged")
				break
			}
		} else {
			stall = 0
		}
		prevLoss = loss
	}

	fmt.Println("--- FINAL PARAMETERS ---")
	tv2PrintParams(&params)
}
