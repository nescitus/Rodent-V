package main

// ============================================================
// PARTIAL TUNER - we are releasing in the middle of eval overhaul
//
// Tunes ONLY:
//   - material P..Q
//   - bishop pair
//   - rook pair
//   - material imbalances: exchangePlus and twoMinors
//   - main PSTs P..K
//   - mobility lookup tables: nMobMg/Eg, bMobMg/Eg, rMobMg/Eg, qMobMg/Eg
//   - passedBonusMG/EG[blocked][relative rank]
//   - ourPasserProximityMG/EG
//   - theirPasserProximityMG/EG
//   - phalanxMG/EG[64] (ranks 1 and 8 fixed zero; ranks 2..7 tunable)
//   - pawnAdjust / knightAdjust / bishopAdjust center-pattern tables (MG only)
//   - king pawn shield / enemy pawn storm terms (MG only)
//
// Everything else is frozen from the CURRENT eval_internal().
//
// Untuned stuff
//	 - threats
//	 - king safety
//	 - assorted pawn weaknesses
//	 - king's pawn shield and pawn storm
//	 - rook bonuses
//
// Entry point:
//     ctTune("training.epd", 1000, 0.05, 0.10)
//
// No changes to eval.go are required.
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

// Which parameter blocks are we tuning?
// Please note that we try to keep pst tables
// near 0, and as a compensation we change
// material values.
var tuneMaterial bool = false
var tunePST bool = false
var tuneMobility bool = false
var tunePassers bool = false
var tunePhalanx bool = false
var tuneAdjustments bool = true
var tuneShield bool = false
var tunePawnWeaknesses bool = false

type ctPair [2]float64

const ctAdjustSquares = 40 // relative ranks 1..5; ranks 6..8 are fixed at zero
const ctPawnAdjustStart = 8
const ctPawnAdjustSquares = ctAdjustSquares - ctPawnAdjustStart // ranks 2..5
const ctPhalanxStart = 8  // first square of relative rank 2
const ctPhalanxEnd = 56   // first square of relative rank 8
const ctPhalanxSquares = ctPhalanxEnd - ctPhalanxStart // ranks 2..7

type ctCoeff struct {
	index uint16
	value int16
}

type ctEntry struct {
	result     float64 // game result: 1.0 = White win, 0.5 = draw, 0.0 = Black win.
	frozen     float64 // part of current eval that isn't retuned: fullEval - scale * tunablePart(initialParams).
	scale      float64 // eval drawishness scale applied to tunable terms, usually just 1.0
	phase      uint8   // game phase for linear interpolation
	coeffStart uint32  // Start index of this position's sparse coefficient list in ctDataset.coeffs.
	coeffCount uint16  // number of sparse coefficients in this position

	// Packed branch decisions for pawnShieldMG(), one byte per relevant file.
	shieldDesc [2][3]uint8
}

type ctDataset struct {
	entries []ctEntry
	coeffs  []ctCoeff
}

type ctLayout struct {
	pieceVal int // 5 entries: P..Q
	pst      int // 6*64

	nMob int
	bMob int
	rMob int
	qMob int

	bishopPair   int
	rookPair     int
	exchangePlus int
	twoMinors    int

	passed    int // 2*8
	ourProx   int // 8
	theirProx int // 8
	phalanx   int // ctPhalanxSquares (relative ranks 2..7)

	// Center-pattern adjustment tables (MG-only)
	pawnAdjust   int // len(pawnAdjust)*2*ctPawnAdjustSquares
	knightAdjust int // len(knightAdjust)*2*ctAdjustSquares
	bishopAdjust int // len(bishopAdjust)*2*ctAdjustSquares

	// King pawn shield / pawn storm (MG-only).
	shield   int // 11 entries
	pawnWeak int // 8 entries: isolated, isolatedOpen, backward, backwardOpen, doubled[4]

	num int
}

type ctTuneMask struct {
	material     bool
	pst          bool
	mobility     bool
	passers      bool
	phalanx      bool
	adjustments  bool
	shield       bool
	pawnWeakness bool
}

func ctParamEnabled(i int, l ctLayout, mask ctTuneMask) bool {
	switch {
	case i >= l.pieceVal && i < l.pieceVal+5:
		return mask.material
	case i >= l.pst && i < l.pst+6*64:
		return mask.pst
	case i >= l.nMob && i < l.bishopPair:
		return mask.mobility
	case i >= l.bishopPair && i < l.passed:
		return mask.material
	case i >= l.passed && i < l.phalanx:
		return mask.passers
	case i >= l.phalanx && i < l.pawnAdjust:
		return mask.phalanx
	case i >= l.pawnAdjust && i < l.shield:
		return mask.adjustments
	case i >= l.shield && i < l.pawnWeak:
		return mask.shield
	case i >= l.pawnWeak && i < l.num:
		return mask.pawnWeakness
	default:
		return false
	}
}

func ctCountEnabled(l ctLayout, mask ctTuneMask) int {
	n := 0
	for i := 0; i < l.num; i++ {
		if ctParamEnabled(i, l, mask) {
			n++
		}
	}
	return n
}

// ctParamPhaseEnabled is like ctParamEnabled, but adjustment tables and
// shield/storm terms are MG-only in the engine, so their EG half is disabled.
func ctParamPhaseEnabled(i, ph int, l ctLayout, mask ctTuneMask) bool {
	if !ctParamEnabled(i, l, mask) {
		return false
	}
	if i >= l.pawnAdjust && i < l.pawnWeak && ph == 1 {
		return false
	}
	if ph == 1 && (i == l.pawnWeak+ctWeakIsolatedOpen || i == l.pawnWeak+ctWeakBackwardOpen) {
		return false
	}
	return true
}

// Number of independently tunable scalar values, used for L2 normalization.
func ctCountEnabledScalars(l ctLayout, mask ctTuneMask) int {
	n := 0
	for i := 0; i < l.num; i++ {
		for ph := 0; ph < 2; ph++ {
			if ctParamPhaseEnabled(i, ph, l, mask) {
				n++
			}
		}
	}
	return n
}

func ctMakeLayout() ctLayout {
	var l ctLayout

	l.pieceVal = 0
	l.pst = l.pieceVal + 5

	l.nMob = l.pst + 6*64
	l.bMob = l.nMob + len(nMobMg)
	l.rMob = l.bMob + len(bMobMg)
	l.qMob = l.rMob + len(rMobMg)

	l.bishopPair = l.qMob + len(qMobMg)
	l.rookPair = l.bishopPair + 1
	l.exchangePlus = l.rookPair + 1
	l.twoMinors = l.exchangePlus + 1

	l.passed = l.twoMinors + 1
	l.ourProx = l.passed + 16
	l.theirProx = l.ourProx + 8
	l.phalanx = l.theirProx + 8

	l.pawnAdjust = l.phalanx + ctPhalanxSquares
	l.knightAdjust = l.pawnAdjust + len(pawnAdjust)*2*ctPawnAdjustSquares
	l.bishopAdjust = l.knightAdjust + len(knightAdjust)*2*ctAdjustSquares
	l.shield = l.bishopAdjust + len(bishopAdjust)*2*ctAdjustSquares
	l.pawnWeak = l.shield + 11
	l.num = l.pawnWeak + 8

	if l.num > 65535 {
		panic("ct tuner: parameter count exceeds uint16 index range")
	}

	return l
}

func ctWorkers() int {
	n := runtime.NumCPU() / 2
	if n < 1 {
		n = 1
	}
	return n
}

// ctParseResult extracts the game result from an EPD record,
// accepting the result formats used by our different tuning sets.
func ctParseResult(line string) float64 {

	// Bracketed numeric result, e.g. [0.0], [0.5], [1.0].
	if lb := strings.LastIndex(line, "["); lb != -1 {
		if rb := strings.LastIndex(line, "]"); rb > lb {
			if x, err := strconv.ParseFloat(strings.TrimSpace(line[lb+1:rb]), 64); err == nil {
				return x
			}
		}
	}

	// Numeric result after semicolon, e.g. ; 0.5
	if idx := strings.LastIndex(line, ";"); idx != -1 {
		if x, err := strconv.ParseFloat(strings.TrimSpace(line[idx+1:]), 64); err == nil {
			return x
		}
	}

	// Zurichess-style result token.
	if strings.Contains(line, "1-0") {
		return 1.0
	}
	if strings.Contains(line, "0-1") {
		return 0.0
	}
	if strings.Contains(line, "1/2-1/2") {
		return 0.5
	}

	// perhaps should panic instead
	return 0.5
}

func ctInitParams(l ctLayout) []ctPair {
	p := make([]ctPair, l.num)

	for piece := P; piece <= Q; piece++ {
		p[l.pieceVal+piece] = ctPair{
			float64(pieceValMG[piece]),
			float64(pieceValEG[piece]),
		}
	}

	for piece := P; piece <= K; piece++ {
		for sq := 0; sq < 64; sq++ {
			p[l.pst+piece*64+sq] = ctPair{
				float64(pstMG[piece][sq]),
				float64(pstEG[piece][sq]),
			}
		}
	}

	for i := range nMobMg {
		p[l.nMob+i] = ctPair{float64(nMobMg[i]), float64(nMobEg[i])}
	}
	for i := range bMobMg {
		p[l.bMob+i] = ctPair{float64(bMobMg[i]), float64(bMobEg[i])}
	}
	for i := range rMobMg {
		p[l.rMob+i] = ctPair{float64(rMobMg[i]), float64(rMobEg[i])}
	}
	for i := range qMobMg {
		p[l.qMob+i] = ctPair{float64(qMobMg[i]), float64(qMobEg[i])}
	}

	p[l.bishopPair] = ctPair{
		float64(bishopPairMG),
		float64(bishopPairEG),
	}
	p[l.rookPair] = ctPair{
		float64(rookPairMG),
		float64(rookPairEG),
	}
	p[l.exchangePlus] = ctPair{
		float64(exchangePlusMG),
		float64(exchangePlusEG),
	}
	p[l.twoMinors] = ctPair{
		float64(twoMinorsMG),
		float64(twoMinorsEG),
	}

	for blocked := 0; blocked < 2; blocked++ {
		for rank := 0; rank < 8; rank++ {
			p[l.passed+blocked*8+rank] = ctPair{
				float64(passedBonusMG[blocked][rank]),
				float64(passedBonusEG[blocked][rank]),
			}
		}
	}

	for d := 0; d < 8; d++ {
		p[l.ourProx+d] = ctPair{
			float64(ourPasserProximityMG[d]),
			float64(ourPasserProximityEG[d]),
		}
		p[l.theirProx+d] = ctPair{
			float64(theirPasserProximityMG[d]),
			float64(theirPasserProximityEG[d]),
		}
	}

	for pstSq := ctPhalanxStart; pstSq < ctPhalanxEnd; pstSq++ {
		i := l.phalanx + (pstSq - ctPhalanxStart)
		p[i] = ctPair{
			float64(phalanxMG[pstSq]),
			float64(phalanxEG[pstSq]),
		}
	}

	// Center-pattern adjustments are MG-only. Only relative ranks 1..5 are stored;
	// relative ranks 6..8 are fixed at zero. EG stays exactly zero.
	for center := 0; center < len(pawnAdjust); center++ {
		for side := White; side <= Black; side++ {
			for canonicalSq := ctPawnAdjustStart; canonicalSq < ctAdjustSquares; canonicalSq++ {
				engineSq := canonicalSq
				if side == Black {
					engineSq ^= 56
				}
				i := l.pawnAdjust + (center*2+side)*ctPawnAdjustSquares + (canonicalSq-ctPawnAdjustStart)
				p[i] = ctPair{float64(pawnAdjust[center][side][engineSq]), 0}
			}
		}
	}

	for center := 0; center < len(knightAdjust); center++ {
		for side := White; side <= Black; side++ {
			for canonicalSq := 0; canonicalSq < ctAdjustSquares; canonicalSq++ {
				engineSq := canonicalSq
				if side == Black {
					engineSq ^= 56
				}
				i := l.knightAdjust + (center*2+side)*ctAdjustSquares + canonicalSq
				p[i] = ctPair{float64(knightAdjust[center][side][engineSq]), 0}
			}
		}
	}
	for center := 0; center < len(bishopAdjust); center++ {
		for side := White; side <= Black; side++ {
			for canonicalSq := 0; canonicalSq < ctAdjustSquares; canonicalSq++ {
				engineSq := canonicalSq
				if side == Black {
					engineSq ^= 56
				}
				i := l.bishopAdjust + (center*2+side)*ctAdjustSquares + canonicalSq
				p[i] = ctPair{float64(bishopAdjust[center][side][engineSq]), 0}
			}
		}
	}

	// Pawn shield / storm parameters are MG-only.
	p[l.shield+0] = ctPair{float64(shieldRank2), 0}
	p[l.shield+1] = ctPair{float64(shieldRank3), 0}
	p[l.shield+2] = ctPair{float64(shieldRank4), 0}
	p[l.shield+3] = ctPair{float64(shieldRank5), 0}
	p[l.shield+4] = ctPair{float64(shieldRank6), 0}
	p[l.shield+5] = ctPair{float64(shieldRank7), 0}
	p[l.shield+6] = ctPair{float64(shieldNoPawn), 0}
	p[l.shield+7] = ctPair{float64(stormRank3), 0}
	p[l.shield+8] = ctPair{float64(stormRank4), 0}
	p[l.shield+9] = ctPair{float64(stormRank5), 0}
	p[l.shield+10] = ctPair{float64(stormNoPawn), 0}

	p[l.pawnWeak+ctWeakIsolated] = ctPair{float64(isolatedMG), float64(isolatedEG)}
	p[l.pawnWeak+ctWeakIsolatedOpen] = ctPair{float64(isolatedOpenMG), 0}
	p[l.pawnWeak+ctWeakBackward] = ctPair{float64(backwardMG), float64(backwardEG)}
	p[l.pawnWeak+ctWeakBackwardOpen] = ctPair{float64(backwardOpenMG), 0}
	for i := 0; i < 4; i++ {
		p[l.pawnWeak+ctWeakDoubledA+i] = ctPair{float64(doubledPawnMG[i]), float64(doubledPawnEG[i])}
	}

	return p
}

func ctPhase(pos *Pos) int {
	phase :=
		pos.count[White][N] + pos.count[Black][N] +
			pos.count[White][B] + pos.count[Black][B] +
			2*(pos.count[White][R]+pos.count[Black][R]) +
			4*(pos.count[White][Q]+pos.count[Black][Q])

	if phase > 24 {
		phase = 24
	}
	return phase
}

// Center adjustments for knights/bishops tune relative ranks 1..5.
// Pawn adjustments additionally skip relative rank 1, so they cover ranks 2..5.
func ctAdjustIndex(base, center, side, sq int) (int, bool) {
	canonicalSq := sq
	if side == Black {
		canonicalSq ^= 56
	}
	if canonicalSq < 0 || canonicalSq >= ctAdjustSquares {
		return 0, false
	}
	return base + (center*2+side)*ctAdjustSquares + canonicalSq, true
}

func ctPawnAdjustIndex(base, center, side, sq int) (int, bool) {
	canonicalSq := sq
	if side == Black {
		canonicalSq ^= 56
	}
	if canonicalSq < ctPawnAdjustStart || canonicalSq >= ctAdjustSquares {
		return 0, false
	}
	return base + (center*2+side)*ctPawnAdjustSquares + (canonicalSq-ctPawnAdjustStart), true
}

func ctPhalanxIndex(pstSq int) (int, bool) {
	if pstSq < ctPhalanxStart || pstSq >= ctPhalanxEnd {
		return 0, false
	}
	return pstSq - ctPhalanxStart, true
}

// Shield/storm offsets inside l.shield.
const (
	ctShieldR2 = iota
	ctShieldR3
	ctShieldR4
	ctShieldR5
	ctShieldR6
	ctShieldR7
	ctShieldNone
	ctStormR3
	ctStormR4
	ctStormR5
	ctStormNone
)

const (
	ctWeakIsolated = iota
	ctWeakIsolatedOpen
	ctWeakBackward
	ctWeakBackwardOpen
	ctWeakDoubledA
	ctWeakDoubledB
	ctWeakDoubledC
	ctWeakDoubledD
)

// ctMakeShieldDesc records the branch choices made by pawnShieldMG().
func ctMakeShieldDesc(pos *Pos, side int) [3]uint8 {
	var out [3]uint8

	kFile := fileOf(pos.kingSq[side])
	ownPawns := pos.pieceBB(side, P)
	enemyPawns := pos.pieceBB(opp(side), P)

	var r2, r3, r4, r5, r6, r7 int
	if side == White {
		r2, r3, r4, r5, r6, r7 = rankOf(A2), rankOf(A3), rankOf(A4), rankOf(A5), rankOf(A6), rankOf(A7)
	} else {
		r2, r3, r4, r5, r6, r7 = rankOf(A7), rankOf(A6), rankOf(A5), rankOf(A4), rankOf(A3), rankOf(A2)
	}

	slot := 0
	for df := -1; df <= 1; df++ {
		f := kFile + df
		if f < 0 || f > 7 {
			continue
		}

		ownCode := uint8(ctShieldNone)
		switch {
		case ownPawns&squareBit(makeSquare(f, r2)) != 0:
			ownCode = ctShieldR2
		case ownPawns&squareBit(makeSquare(f, r3)) != 0:
			ownCode = ctShieldR3
		case ownPawns&squareBit(makeSquare(f, r4)) != 0:
			ownCode = ctShieldR4
		case ownPawns&squareBit(makeSquare(f, r5)) != 0:
			ownCode = ctShieldR5
		case ownPawns&squareBit(makeSquare(f, r6)) != 0:
			ownCode = ctShieldR6
		case ownPawns&squareBit(makeSquare(f, r7)) != 0:
			ownCode = ctShieldR7
		}

		// 7 = no storm term; 3 = stormNoPawn.
		stormCode := uint8(7)
		switch {
		case enemyPawns&squareBit(makeSquare(f, r3)) != 0:
			stormCode = 0
		case enemyPawns&squareBit(makeSquare(f, r4)) != 0:
			stormCode = 1
		case enemyPawns&squareBit(makeSquare(f, r5)) != 0:
			stormCode = 2
		default:
			fileMask := fileABB << uint(f)
			if fileMask&enemyPawns == 0 {
				stormCode = 3
			}
		}

		d := ownCode | (stormCode << 4)
		if f == kFile {
			d |= 0x80
		}
		out[slot] = d
		slot++
	}

	for ; slot < 3; slot++ {
		out[slot] = 0x7f // unused edge slot
	}
	return out
}

// ctShieldScore is the differentiable equivalent of pawnShieldMG().
// It preserves the current shieldRank2 assignment/reset and the cumulative
// king-file scaling, using *1.2 instead of integer *12/10 truncation.
func ctShieldScore(desc [3]uint8, params []ctPair, l ctLayout) float64 {
	penalty := 0.0

	for _, d := range desc {
		if d == 0x7f {
			continue
		}

		ownCode := int(d & 0x0f)
		stormCode := int((d >> 4) & 0x07)

		v := params[l.shield+ownCode][0]
		if ownCode == ctShieldR2 {
			penalty = v
		} else {
			penalty += v
		}

		if d&0x80 != 0 {
			penalty *= 1.2
		}

		switch stormCode {
		case 0:
			penalty += params[l.shield+ctStormR3][0]
		case 1:
			penalty += params[l.shield+ctStormR4][0]
		case 2:
			penalty += params[l.shield+ctStormR5][0]
		case 3:
			penalty += params[l.shield+ctStormNone][0]
		}
	}

	return -penalty
}

// Derivatives of ctShieldScore for one side.
func ctShieldDeriv(desc [3]uint8) [11]float64 {
	var g [11]float64

	for _, d := range desc {
		if d == 0x7f {
			continue
		}

		ownCode := int(d & 0x0f)
		stormCode := int((d >> 4) & 0x07)

		if ownCode == ctShieldR2 {
			for i := range g {
				g[i] = 0
			}
			g[ctShieldR2] = 1
		} else {
			g[ownCode] += 1
		}

		if d&0x80 != 0 {
			for i := range g {
				g[i] *= 1.2
			}
		}

		switch stormCode {
		case 0:
			g[ctStormR3] += 1
		case 1:
			g[ctStormR4] += 1
		case 2:
			g[ctStormR5] += 1
		case 3:
			g[ctStormNone] += 1
		}
	}

	for i := range g {
		g[i] = -g[i]
	}
	return g
}

// Sparse builder for one position.
// Dense is cheap here because parameter count is only ~450.
func ctCoefficients(pos *Pos, l ctLayout, dense []int16, out []ctCoeff) []ctCoeff {
	for i := range dense {
		dense[i] = 0
	}

	occ := pos.occupied()

	// Use the engine's own center classifier so the adjustment-table
	// coefficients always match eval_internal().
	var centerEval EvalData
	initCenterType(pos, &centerEval)

	addCoeff := func(idx int, sign int16) {
		dense[idx] += sign
	}

	for side := White; side <= Black; side++ {
		sign := int16(1)
		if side == Black {
			sign = -1
		}
		enemy := opp(side)

		// Match the current eval's slider mobility exactly, including its
		// intentionally retained x-ray/transparency behavior.
		occForBishop := occ ^ (pos.pieceBB(side, B) | pos.pieceBB(side, Q))
		occForRook := occ ^ (pos.pieceBB(side, R) | pos.pieceBB(side, Q))
		occForQueen := occ ^ (pos.pieceBB(side, B) | pos.pieceBB(side, R))

		// --------------------------------------------------------
		// Pawns: material + PST + phalanx + passer terms
		// --------------------------------------------------------
		ownPawns := pos.pieceBB(side, P)

		for bb := ownPawns; bb != 0; bb &= bb - 1 {
			sq := lsb(bb)

			pstSq := sq
			if side == Black {
				pstSq ^= 56
			}

			addCoeff(l.pieceVal+P, sign)
			addCoeff(l.pst+P*64+pstSq, sign)

			if centerEval.center[side] != Undefined {
				center := int(centerEval.center[side])
				if idx, ok := ctPawnAdjustIndex(l.pawnAdjust, center, side, sq); ok {
					addCoeff(idx, sign)
				}
			}

			// Pawn weaknesses: mirror evaluatePawns() exactly.
			frontMask := fillForward(squareBit(sq), side)
			isOpen := frontMask&ownPawns == 0

			if adjFileMask[fileOf(sq)]&ownPawns == 0 {
				addCoeff(l.pawnWeak+ctWeakIsolated, sign)
				if isOpen {
					addCoeff(l.pawnWeak+ctWeakIsolatedOpen, sign)
				}
			} else if supportMask[side][sq]&ownPawns == 0 {
				addCoeff(l.pawnWeak+ctWeakBackward, sign)
				if isOpen {
					addCoeff(l.pawnWeak+ctWeakBackwardOpen, sign)
				}
			}

			pushSqWeak := getPushSq(side, sq)
			if pushSqWeak >= 0 && pushSqWeak < 64 && ownPawns&squareBit(pushSqWeak) != 0 {
				fileIdx := fileOf(sq)
				if fileIdx > 3 {
					fileIdx = 7 - fileIdx
				}
				addCoeff(l.pawnWeak+ctWeakDoubledA+fileIdx, sign)
			}

			// Count each phalanx pair once: the eastern pawn represents the pair.
			if shiftWest(squareBit(sq))&ownPawns != 0 {
				if phIdx, ok := ctPhalanxIndex(pstSq); ok {
					addCoeff(l.phalanx+phIdx, sign)
				}
			}

			if passedMask[side][sq]&pos.pieceBB(enemy, P) != 0 {
				continue
			}

			pushSq := sq + 8
			if side == Black {
				pushSq = sq - 8
			}

			relRank := rankOf(sq)
			if side == Black {
				relRank = 7 - relRank
			}

			blocked := 0
			if pushSq >= 0 && pushSq < 64 && pos.board[pushSq] != NO_PC {
				blocked = 1
			}

			addCoeff(l.passed+blocked*8+relRank, sign)

			if relRank >= 3 && pushSq >= 0 && pushSq < 64 {
				ourDist := chebyshev(pos.kingSq[side], pushSq)
				theirDist := chebyshev(pos.kingSq[enemy], pushSq)

				addCoeff(l.ourProx+ourDist, sign)
				addCoeff(l.theirProx+theirDist, sign)
			}
		}

		// --------------------------------------------------------
		// Knights
		// --------------------------------------------------------
		for bb := pos.pieceBB(side, N); bb != 0; bb &= bb - 1 {
			sq := lsb(bb)

			pstSq := sq
			if side == Black {
				pstSq ^= 56
			}

			addCoeff(l.pieceVal+N, sign)
			addCoeff(l.pst+N*64+pstSq, sign)

			if centerEval.center[side] != Undefined {
				center := int(centerEval.center[side])
				if idx, ok := ctAdjustIndex(l.knightAdjust, center, side, sq); ok {
					addCoeff(idx, sign)
				}
			}

			atks := knightAtk[sq]
			cnt := popCount(atks &^ pos.colorBB[side])

			if cnt < 0 || cnt >= len(nMobMg) {
				panic(fmt.Sprintf("ct tuner: knight mobility %d outside table [0,%d)", cnt, len(nMobMg)))
			}

			addCoeff(l.nMob+cnt, sign)
		}

		// --------------------------------------------------------
		// Bishops
		// --------------------------------------------------------
		for bb := pos.pieceBB(side, B); bb != 0; bb &= bb - 1 {
			sq := lsb(bb)

			pstSq := sq
			if side == Black {
				pstSq ^= 56
			}

			addCoeff(l.pieceVal+B, sign)
			addCoeff(l.pst+B*64+pstSq, sign)

			if centerEval.center[side] != Undefined {
				center := int(centerEval.center[side])
				if idx, ok := ctAdjustIndex(l.bishopAdjust, center, side, sq); ok {
					addCoeff(idx, sign)
				}
			}

			cnt := popCount(bishopAttacks(occForBishop, sq))

			if cnt < 0 || cnt >= len(bMobMg) {
				panic(fmt.Sprintf("ct tuner: bishop mobility %d outside table [0,%d)", cnt, len(bMobMg)))
			}

			addCoeff(l.bMob+cnt, sign)
		}

		if popCount(pos.pieceBB(side, B)) >= 2 {
			addCoeff(l.bishopPair, sign)
		}

		// --------------------------------------------------------
		// Rooks
		// --------------------------------------------------------
		for bb := pos.pieceBB(side, R); bb != 0; bb &= bb - 1 {
			sq := lsb(bb)

			pstSq := sq
			if side == Black {
				pstSq ^= 56
			}

			addCoeff(l.pieceVal+R, sign)
			addCoeff(l.pst+R*64+pstSq, sign)

			cnt := popCount(rookAttacks(occForRook, sq))
			if cnt < 0 || cnt >= len(rMobMg) {
				panic(fmt.Sprintf("ct tuner: rook mobility %d outside table [0,%d)", cnt, len(rMobMg)))
			}

			addCoeff(l.rMob+cnt, sign)
		}

		if popCount(pos.pieceBB(side, R)) >= 2 {
			addCoeff(l.rookPair, sign)
		}

		// --------------------------------------------------------
		// Queens
		// --------------------------------------------------------
		for bb := pos.pieceBB(side, Q); bb != 0; bb &= bb - 1 {
			sq := lsb(bb)

			pstSq := sq
			if side == Black {
				pstSq ^= 56
			}

			addCoeff(l.pieceVal+Q, sign)
			addCoeff(l.pst+Q*64+pstSq, sign)

			cnt := popCount(queenAttacks(occForQueen, sq))
			if cnt < 0 || cnt >= len(qMobMg) {
				panic(fmt.Sprintf("ct tuner: queen mobility %d outside table [0,%d)", cnt, len(qMobMg)))
			}

			addCoeff(l.qMob+cnt, sign)
		}

		// King PST.
		ksq := pos.kingSq[side]
		pstSq := ksq
		if side == Black {
			pstSq ^= 56
		}
		addCoeff(l.pst+K*64+pstSq, sign)
	}

	// Material imbalance corrections. Match eval_internal() exactly.
	wMinors := pos.count[White][N] + pos.count[White][B]
	bMinors := pos.count[Black][N] + pos.count[Black][B]
	wMajors := pos.count[White][R] + 2*pos.count[White][Q]
	bMajors := pos.count[Black][R] + 2*pos.count[Black][Q]

	if wMajors == bMajors+1 && wMinors == bMinors-1 {
		addCoeff(l.exchangePlus, 1)
	}
	if bMajors == wMajors+1 && bMinors == wMinors-1 {
		addCoeff(l.exchangePlus, -1)
	}
	if wMajors == bMajors-1 && wMinors == bMinors+2 {
		addCoeff(l.twoMinors, 1)
	}
	if bMajors == wMajors-1 && bMinors == wMinors+2 {
		addCoeff(l.twoMinors, -1)
	}

	out = out[:0]
	for i, v := range dense {
		if v != 0 {
			out = append(out, ctCoeff{
				index: uint16(i),
				value: v,
			})
		}
	}

	return out
}

func ctLinDotRange(coeffs []ctCoeff, params []ctPair, l ctLayout) (mg, eg float64) {
	for _, c := range coeffs {
		i := int(c.index)
		f := float64(c.value)

		mg += f * params[i][0]
		// Center adjustment tables are MG-only in eval_internal().
		if i < l.pawnAdjust || i >= l.pawnWeak {
			eg += f * params[i][1]
		}
	}
	return
}

func ctTapered(mg, eg float64, phase int) float64 {
	return (mg*float64(phase) + eg*float64(24-phase)) / 24.0
}

func ctSelectedScore(coeffs []ctCoeff, params []ctPair, phase int, l ctLayout) float64 {
	mg, eg := ctLinDotRange(coeffs, params, l)
	return ctTapered(mg, eg, phase)
}

func ctEntryCoeffs(data *ctDataset, e *ctEntry) []ctCoeff {
	start := int(e.coeffStart)
	end := start + int(e.coeffCount)
	return data.coeffs[start:end]
}

func ctScore(data *ctDataset, e *ctEntry, params []ctPair, l ctLayout) float64 {
	selected := ctSelectedScore(ctEntryCoeffs(data, e), params, int(e.phase), l)
	shieldSelected := ctShieldScore(e.shieldDesc[White], params, l) -
		ctShieldScore(e.shieldDesc[Black], params, l)
	selected += shieldSelected * float64(e.phase) / 24.0
	return e.frozen + e.scale*selected
}

// Mirrors the current drawish scaling for phase < 7.
// The branch is frozen from the current engine score's White-POV sign.
func ctLinearScale(pos *Pos, phase int, engineWhiteScore float64) float64 {
	if phase >= 7 || engineWhiteScore == 0 {
		return 1.0
	}

	weight := 100

	if engineWhiteScore > 0 {
		weight = getDrawishness(pos, White, Black)
	} else {
		weight = getDrawishness(pos, Black, White)
	}

	return float64(weight) / 100.0
}

// Stream input file directly into compact dataset.
func ctLoadDataset(filename string, initParams []ctPair, l ctLayout) (*ctDataset, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	data := &ctDataset{
		entries: make([]ctEntry, 0, 1<<20),
		coeffs:  make([]ctCoeff, 0, 16<<20),
	}

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)

	var pos Pos
	dense := make([]int16, l.num)
	tmpCoeffs := make([]ctCoeff, 0, 64)

	var count int
	var paritySum float64
	var parityMax float64

	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}

		parseFEN(&pos, line)
		result := ctParseResult(line)

		// Full engine evaluation.
		engineScore := float64(eval_internal(&pos, false, nil))

		// Convert side-to-move POV -> White POV.
		if pos.side == Black {
			engineScore = -engineScore
		}

		phase := ctPhase(&pos)

		tmpCoeffs = ctCoefficients(&pos, l, dense, tmpCoeffs)

		if len(tmpCoeffs) > 65535 {
			return nil, fmt.Errorf("too many sparse coefficients in one position: %d", len(tmpCoeffs))
		}
		if len(data.coeffs) > math.MaxUint32-len(tmpCoeffs) {
			return nil, fmt.Errorf("coefficient arena exceeded uint32 offset range")
		}

		scale := ctLinearScale(&pos, phase, engineScore)

		selected := ctSelectedScore(tmpCoeffs, initParams, phase, l)
		shieldDesc := [2][3]uint8{
			ctMakeShieldDesc(&pos, White),
			ctMakeShieldDesc(&pos, Black),
		}
		shieldSelected := ctShieldScore(shieldDesc[White], initParams, l) -
			ctShieldScore(shieldDesc[Black], initParams, l)
		selected += shieldSelected * float64(phase) / 24.0
		frozen := engineScore - scale*selected

		start := len(data.coeffs)
		data.coeffs = append(data.coeffs, tmpCoeffs...)

		entry := ctEntry{
			result:     result,
			frozen:     frozen,
			scale:      scale,
			phase:      uint8(phase),
			coeffStart: uint32(start),
			coeffCount: uint16(len(tmpCoeffs)),
			shieldDesc: shieldDesc,
		}

		data.entries = append(data.entries, entry)

		// Reconstruction parity sanity check.
		reconstructed := frozen + scale*selected
		d := math.Abs(reconstructed - engineScore)

		paritySum += d
		if d > parityMax {
			parityMax = d
		}

		count++

		if count%1_000_000 == 0 {
			fmt.Printf(
				"[core-tuner] loaded %d positions, %d coeffs\n",
				count,
				len(data.coeffs),
			)
		}
	}

	if err := sc.Err(); err != nil {
		return nil, err
	}

	if count > 0 {
		fmt.Printf(
			"[core-tuner] reconstruction mean abs %.12g  max %.12g\n",
			paritySum/float64(count),
			parityMax,
		)
	}

	return data, nil
}

func ctSigmoid(score, k float64) float64 {
	return 1.0 / (1.0 + math.Pow(10.0, -k*score/400.0))
}

func ctSigDeriv(score, k float64) float64 {
	p := ctSigmoid(score, k)
	return math.Log(10.0) * k / 400.0 * p * (1.0 - p)
}

func ctMSE(data *ctDataset, params, initial []ctPair, l ctLayout, mask ctTuneMask,
	k, lambda float64) float64 {
	n := len(data.entries)
	if n == 0 {
		return 0
	}

	nw := ctWorkers()
	if nw > n {
		nw = n
	}

	chunk := (n + nw - 1) / nw
	totals := make([]float64, nw)
	var wg sync.WaitGroup

	for w := 0; w < nw; w++ {
		wg.Add(1)
		go func(wid int) {
			defer wg.Done()
			start := wid * chunk
			end := start + chunk
			if end > n {
				end = n
			}
			sum := 0.0
			for i := start; i < end; i++ {
				e := &data.entries[i]
				d := e.result - ctSigmoid(ctScore(data, e, params, l), k)
				sum += d * d
			}
			totals[wid] = sum
		}(w)
	}
	wg.Wait()

	total := 0.0
	for _, x := range totals {
		total += x
	}
	loss := total / float64(n)

	if lambda > 0 {
		enabled := ctCountEnabledScalars(l, mask)
		if enabled > 0 {
			reg := 0.0
			for i := range params {
				for ph := 0; ph < 2; ph++ {
					if !ctParamPhaseEnabled(i, ph, l, mask) {
						continue
					}
					d := params[i][ph] - initial[i][ph]
					reg += d * d
				}
			}
			reg /= float64(enabled)
			loss += lambda * reg / 10000.0
		}
	}
	return loss
}

// Find K coefficient for current tuning session
func ctFitK(data *ctDataset, params, initial []ctPair, l ctLayout, mask ctTuneMask) float64 {
	lo, hi := 0.1, 6.0
	for i := 0; i < 15; i++ {
		m1 := lo + (hi-lo)/3.0
		m2 := hi - (hi-lo)/3.0
		if ctMSE(data, params, initial, l, mask, m1, 0) <
			ctMSE(data, params, initial, l, mask, m2, 0) {
			hi = m2
		} else {
			lo = m1
		}
	}
	return (lo + hi) / 2.0
}

// ctIsotonicIncreasing replaces values with the closest non-decreasing
// sequence in least-squares sense (pool-adjacent-violators algorithm).
func ctIsotonicIncreasing(values []float64) {
	n := len(values)
	if n <= 1 {
		return
	}

	level := make([]float64, n)
	weight := make([]float64, n)
	start := make([]int, n)
	blocks := 0

	for i := 0; i < n; i++ {
		level[blocks] = values[i]
		weight[blocks] = 1.0
		start[blocks] = i
		blocks++

		for blocks >= 2 && level[blocks-2] > level[blocks-1] {
			w1 := weight[blocks-2]
			w2 := weight[blocks-1]
			level[blocks-2] = (level[blocks-2]*w1 + level[blocks-1]*w2) / (w1 + w2)
			weight[blocks-2] = w1 + w2
			blocks--
		}
	}

	for b := 0; b < blocks; b++ {
		from := start[b]
		to := n
		if b+1 < blocks {
			to = start[b+1]
		}
		for i := from; i < to; i++ {
			values[i] = level[b]
		}
	}
}

// ctConstrainPassers enforces a non-decreasing base passer value from
// relative rank 1 through rank 6. It is applied independently to
// free/blocked passers and MG/EG. Indices 0 and 7 are left untouched.
func ctConstrainPassers(params []ctPair, l ctLayout) {
	var v [6]float64

	for blocked := 0; blocked < 2; blocked++ {
		base := l.passed + blocked*8

		for ph := 0; ph < 2; ph++ {
			for rank := 1; rank <= 6; rank++ {
				v[rank-1] = params[base+rank][ph]
			}

			ctIsotonicIncreasing(v[:])

			for rank := 1; rank <= 6; rank++ {
				params[base+rank][ph] = v[rank-1]
			}
		}
	}
}

const (
	ctBeta1   = 0.9
	ctBeta2   = 0.999
	ctAdamEps = 1e-8
	ctLRMin   = 1e-4
)

func ctEpoch(
	data *ctDataset,
	params, initial, mom, vel []ctPair,
	l ctLayout,
	mask ctTuneMask,
	lr, k, lambda float64,
) float64 {
	n := len(data.entries)
	if n == 0 {
		return 0
	}

	nw := ctWorkers()
	if nw > n {
		nw = n
	}
	chunk := (n + nw - 1) / nw

	type workerResult struct {
		grad []ctPair
		loss float64
	}
	results := make([]workerResult, nw)
	for i := range results {
		results[i].grad = make([]ctPair, len(params))
	}

	var wg sync.WaitGroup
	for w := 0; w < nw; w++ {
		wg.Add(1)
		go func(wid int) {
			defer wg.Done()
			start := wid * chunk
			end := start + chunk
			if end > n {
				end = n
			}
			r := &results[wid]

			for i := start; i < end; i++ {
				e := &data.entries[i]
				score := ctScore(data, e, params, l)
				pred := ctSigmoid(score, k)
				diff := pred - e.result
				r.loss += diff * diff

				dLoss := 2.0 * diff * ctSigDeriv(score, k)
				phase := float64(e.phase)
				mgF := e.scale * phase / 24.0
				egF := e.scale * (24.0 - phase) / 24.0

				for _, c := range ctEntryCoeffs(data, e) {
					idx := int(c.index)
					f := float64(c.value)
					r.grad[idx][0] += dLoss * f * mgF
					if idx < l.pawnAdjust || idx >= l.pawnWeak {
						r.grad[idx][1] += dLoss * f * egF
					}
				}

				if mask.shield {
					wg := ctShieldDeriv(e.shieldDesc[White])
					bg := ctShieldDeriv(e.shieldDesc[Black])
					for j := 0; j < 11; j++ {
						f := wg[j] - bg[j]
						r.grad[l.shield+j][0] += dLoss * f * mgF
					}
				}
			}
		}(w)
	}
	wg.Wait()

	totalGrad := make([]ctPair, len(params))
	totalLoss := 0.0
	for _, r := range results {
		totalLoss += r.loss
		for i := range totalGrad {
			totalGrad[i][0] += r.grad[i][0]
			totalGrad[i][1] += r.grad[i][1]
		}
	}

	invN := 1.0 / float64(n)
	enabled := ctCountEnabledScalars(l, mask)

	for i := range params {
		for ph := 0; ph < 2; ph++ {
			if !ctParamPhaseEnabled(i, ph, l, mask) {
				continue
			}
			g := totalGrad[i][ph] * invN
			if lambda > 0 && enabled > 0 {
				delta := params[i][ph] - initial[i][ph]
				g += lambda * 2.0 * delta / (10000.0 * float64(enabled))
			}

			mom[i][ph] = ctBeta1*mom[i][ph] + (1.0-ctBeta1)*g
			vel[i][ph] = ctBeta2*vel[i][ph] + (1.0-ctBeta2)*g*g
			params[i][ph] -= lr * mom[i][ph] /
				(math.Sqrt(vel[i][ph]) + ctAdamEps)
		}
	}

	if mask.passers {
		ctConstrainPassers(params, l)
	}

	loss := totalLoss * invN
	if lambda > 0 && enabled > 0 {
		reg := 0.0
		for i := range params {
			for ph := 0; ph < 2; ph++ {
				if !ctParamPhaseEnabled(i, ph, l, mask) {
					continue
				}
				d := params[i][ph] - initial[i][ph]
				reg += d * d
			}
		}
		reg /= float64(enabled)
		loss += lambda * reg / 10000.0
	}
	return loss
}

// Round float and transform it to int
func ctRound(x float64) int {
	if x >= 0 {
		return int(x + 0.5)
	}
	return int(x - 0.5)
}

// PST normalization. We want piece/square tables centered around zero,
// which gives theoretical possibility of changing them without touching
// anything else. If table's mean score grows too high or too low, we
// offload the difference into material values. No action needs to be
// taken for the king (there are no legal positions with king imbalance)
func ctRecenterPST(params []ctPair, l ctLayout) []ctPair {
	n := append([]ctPair(nil), params...)

	for piece := P; piece <= K; piece++ {
		for ph := 0; ph < 2; ph++ {
			sum := 0.0

			for sq := 0; sq < 64; sq++ {
				sum += n[l.pst+piece*64+sq][ph]
			}

			mean := sum / 64.0

			for sq := 0; sq < 64; sq++ {
				n[l.pst+piece*64+sq][ph] -= mean
			}

			if piece <= Q {
				n[l.pieceVal+piece][ph] += mean
			}
		}
	}

	return n
}

//
// Printing results
//

func ctPrintParams(params []ctPair, l ctLayout) {
	n := ctRecenterPST(params, l)
	fmt.Println()
	fmt.Println("// ================= CURRENT CORE TUNER OUTPUT =================")

	if tuneMaterial || tunePST {
		ctPrintMaterial(n, l) // piece values, bishop pair
	}
	if tunePST {
		ctPrintPST(n, l) // main piece/square tables
	}
	if tuneMobility {
		ctPrintMobilityTables(n, l) // mobility tables
	}
	if tunePassers {
		ctPrintPassers(n, l) // passer base values and proximity bonuses
	}
	if tunePhalanx {
		ctPrintPhalanx(n, l) // pawn phalanx table
	}
	if tuneAdjustments {
		ctPrintAdjustments(n, l) // center-pattern adjustment tables
	}
	if tuneShield {
		ctPrintShield(n, l) // pawn shield / pawn storm
	}
	if tunePawnWeaknesses {
		ctPrintPawnWeaknesses(n, l) // isolated/backward/doubled pawn terms
	}

	fmt.Println("// ==============================================================")
	fmt.Println()
}

func ctPrintMaterial(n []ctPair, l ctLayout) {
	fmt.Printf(
		"var pieceValMG = [7]int{%d, %d, %d, %d, %d, 0, 0}\n",
		ctRound(n[l.pieceVal+P][0]),
		ctRound(n[l.pieceVal+N][0]),
		ctRound(n[l.pieceVal+B][0]),
		ctRound(n[l.pieceVal+R][0]),
		ctRound(n[l.pieceVal+Q][0]),
	)

	fmt.Printf(
		"var pieceValEG = [7]int{%d, %d, %d, %d, %d, 0, 0}\n",
		ctRound(n[l.pieceVal+P][1]),
		ctRound(n[l.pieceVal+N][1]),
		ctRound(n[l.pieceVal+B][1]),
		ctRound(n[l.pieceVal+R][1]),
		ctRound(n[l.pieceVal+Q][1]),
	)

	fmt.Println()
	fmt.Println("// bishopPairMG/EG: bonus for owning both bishops.")
	fmt.Println("// The EG value is higher because open boards in the endgame")
	fmt.Println("// let the bishop pair dominate knight+bishop or two knights.")

	fmt.Printf("const bishopPairMG = %d\n", ctRound(n[l.bishopPair][0]))
	fmt.Printf("const bishopPairEG = %d\n", ctRound(n[l.bishopPair][1]))

	fmt.Println()
	fmt.Printf("const rookPairMG = %d\n", ctRound(n[l.rookPair][0]))
	fmt.Printf("const rookPairEG = %d\n", ctRound(n[l.rookPair][1]))
	fmt.Printf("const exchangePlusMG = %d\n", ctRound(n[l.exchangePlus][0]))
	fmt.Printf("const exchangePlusEG = %d\n", ctRound(n[l.exchangePlus][1]))
	fmt.Printf("const twoMinorsMG = %d\n", ctRound(n[l.twoMinors][0]))
	fmt.Printf("const twoMinorsEG = %d\n", ctRound(n[l.twoMinors][1]))
}

func ctPrintShield(n []ctPair, l ctLayout) {
	fmt.Println()
	fmt.Println("// King pawn shield / enemy pawn storm (MG only).")
	fmt.Printf("const shieldRank2 = %d\n", ctRound(n[l.shield+ctShieldR2][0]))
	fmt.Printf("const shieldRank3 = %d\n", ctRound(n[l.shield+ctShieldR3][0]))
	fmt.Printf("const shieldRank4 = %d\n", ctRound(n[l.shield+ctShieldR4][0]))
	fmt.Printf("const shieldRank5 = %d\n", ctRound(n[l.shield+ctShieldR5][0]))
	fmt.Printf("const shieldRank6 = %d\n", ctRound(n[l.shield+ctShieldR6][0]))
	fmt.Printf("const shieldRank7 = %d\n", ctRound(n[l.shield+ctShieldR7][0]))
	fmt.Printf("const shieldNoPawn = %d\n", ctRound(n[l.shield+ctShieldNone][0]))
	fmt.Printf("const stormRank3 = %d\n", ctRound(n[l.shield+ctStormR3][0]))
	fmt.Printf("const stormRank4 = %d\n", ctRound(n[l.shield+ctStormR4][0]))
	fmt.Printf("const stormRank5 = %d\n", ctRound(n[l.shield+ctStormR5][0]))
	fmt.Printf("const stormNoPawn = %d\n", ctRound(n[l.shield+ctStormNone][0]))
}

func ctPrintPawnWeaknesses(n []ctPair, l ctLayout) {
	fmt.Println()
	fmt.Println("// Pawn weaknesses.")
	fmt.Printf("const isolatedMG = %d\n", ctRound(n[l.pawnWeak+ctWeakIsolated][0]))
	fmt.Printf("const isolatedEG = %d\n", ctRound(n[l.pawnWeak+ctWeakIsolated][1]))
	fmt.Printf("const isolatedOpenMG = %d\n", ctRound(n[l.pawnWeak+ctWeakIsolatedOpen][0]))
	fmt.Printf("const backwardMG = %d\n", ctRound(n[l.pawnWeak+ctWeakBackward][0]))
	fmt.Printf("const backwardEG = %d\n", ctRound(n[l.pawnWeak+ctWeakBackward][1]))
	fmt.Printf("const backwardOpenMG = %d\n", ctRound(n[l.pawnWeak+ctWeakBackwardOpen][0]))
	fmt.Print("var doubledPawnMG = [4]int{")
	for i := 0; i < 4; i++ {
		if i != 0 {
			fmt.Print(", ")
		}
		fmt.Print(ctRound(n[l.pawnWeak+ctWeakDoubledA+i][0]))
	}
	fmt.Println("}")
	fmt.Print("var doubledPawnEG = [4]int{")
	for i := 0; i < 4; i++ {
		if i != 0 {
			fmt.Print(", ")
		}
		fmt.Print(ctRound(n[l.pawnWeak+ctWeakDoubledA+i][1]))
	}
	fmt.Println("}")
}

func ctPrintMobilityTables(n []ctPair, l ctLayout) {
	ctPrintMob("nMobMg", n, l.nMob, len(nMobMg), 0)
	ctPrintMob("nMobEg", n, l.nMob, len(nMobEg), 1)

	ctPrintMob("bMobMg", n, l.bMob, len(bMobMg), 0)
	ctPrintMob("bMobEg", n, l.bMob, len(bMobEg), 1)

	ctPrintMob("rMobMg", n, l.rMob, len(rMobMg), 0)
	ctPrintMob("rMobEg", n, l.rMob, len(rMobEg), 1)

	ctPrintMob("qMobMg", n, l.qMob, len(qMobMg), 0)
	ctPrintMob("qMobEg", n, l.qMob, len(qMobEg), 1)
}

func ctPrintPassers(n []ctPair, l ctLayout) {
	fmt.Println("var passedBonusMG = [2][8]int{")
	for blocked := 0; blocked < 2; blocked++ {
		fmt.Print("\t{")
		for rank := 0; rank < 8; rank++ {
			if rank != 0 {
				fmt.Print(", ")
			}
			fmt.Print(ctRound(n[l.passed+blocked*8+rank][0]))
		}
		fmt.Println("},")
	}
	fmt.Println("}")

	fmt.Println("var passedBonusEG = [2][8]int{")
	for blocked := 0; blocked < 2; blocked++ {
		fmt.Print("\t{")
		for rank := 0; rank < 8; rank++ {
			if rank != 0 {
				fmt.Print(", ")
			}
			fmt.Print(ctRound(n[l.passed+blocked*8+rank][1]))
		}
		fmt.Println("},")
	}
	fmt.Println("}")

	fmt.Print("var ourPasserProximityMG = [8]int{")
	for d := 0; d < 8; d++ {
		if d != 0 {
			fmt.Print(", ")
		}
		fmt.Print(ctRound(n[l.ourProx+d][0]))
	}
	fmt.Println("}")

	fmt.Print("var ourPasserProximityEG = [8]int{")
	for d := 0; d < 8; d++ {
		if d != 0 {
			fmt.Print(", ")
		}
		fmt.Print(ctRound(n[l.ourProx+d][1]))
	}
	fmt.Println("}")

	fmt.Print("var theirPasserProximityMG = [8]int{")
	for d := 0; d < 8; d++ {
		if d != 0 {
			fmt.Print(", ")
		}
		fmt.Print(ctRound(n[l.theirProx+d][0]))
	}
	fmt.Println("}")

	fmt.Print("var theirPasserProximityEG = [8]int{")
	for d := 0; d < 8; d++ {
		if d != 0 {
			fmt.Print(", ")
		}
		fmt.Print(ctRound(n[l.theirProx+d][1]))
	}
	fmt.Println("}")
}

func ctPrintPhalanx(n []ctPair, l ctLayout) {
	fmt.Println()
	for _, t := range []struct {
		ph   int
		name string
	}{
		{0, "phalanxMG"},
		{1, "phalanxEG"},
	} {
		fmt.Printf("var %s = [64]int{\n", t.name)
		for rank := 0; rank < 8; rank++ {
			fmt.Print("\t")
			for file := 0; file < 8; file++ {
				if file != 0 {
					fmt.Print(", ")
				}
				pstSq := rank*8 + file
				v := 0
				if phIdx, ok := ctPhalanxIndex(pstSq); ok {
					v = ctRound(n[l.phalanx+phIdx][t.ph])
				}
				fmt.Printf("%4d", v)
			}
			fmt.Println(",")
		}
		fmt.Println("}")
	}
}

func ctPrintNamedAdjustTable(name string, n []ctPair, start, center, firstSq, squareCount int) {
	fmt.Println()
	fmt.Printf("var %s = [64]int{\n", name)

	for rank := 0; rank < 8; rank++ {
		fmt.Print("\t")
		for file := 0; file < 8; file++ {
			if file != 0 {
				fmt.Print(", ")
			}

			sq := rank*8 + file
			v := 0.0

			if sq >= firstSq && sq < firstSq+squareCount {
				rel := sq - firstSq
				whiteIdx := start + (center*2+White)*squareCount + rel
				blackIdx := start + (center*2+Black)*squareCount + rel
				v = (n[whiteIdx][0] + n[blackIdx][0]) / 2.0
			}

			fmt.Printf("%4d", ctRound(v))
		}
		fmt.Println(",")
	}

	fmt.Println("}")
}

func ctPrintAdjustments(n []ctPair, l ctLayout) {
	// CenterType order:
	// 0 KID_low, 1 KID_high,
	// 2 FRENCH_low, 3 FRENCH_high,
	// 4 SICILIAN_low, 5 SICILIAN_high,
	// 6 CLASSIC_e4e5, 7 CLASSIC_d4d5,
	// 8 Undefined (unused, therefore not printed).

	for _, t := range []struct {
		center int
		name   string
	}{
		{int(FRENCH_high), "frenchHighP"},
		{int(FRENCH_low), "frenchLowP"},
		{int(KID_high), "KidHighP"},
		{int(KID_low), "KidLowP"},
		{int(SICILIAN_high), "SicHighP"},
		{int(SICILIAN_low), "SicLowP"},
		{int(CLASSIC_e4e5), "e4e5P"},
		{int(CLASSIC_d4d5), "d4d5P"},
	} {
		ctPrintNamedAdjustTable(t.name, n, l.pawnAdjust, t.center, ctPawnAdjustStart, ctPawnAdjustSquares)
	}

	for _, t := range []struct {
		center int
		name   string
	}{
		{int(FRENCH_high), "frenchHighN"},
		{int(FRENCH_low), "frenchLowN"},
		{int(KID_high), "KidHighN"},
		{int(KID_low), "KidLowN"},
		{int(SICILIAN_high), "SicHighN"},
		{int(SICILIAN_low), "SicLowN"},
		{int(CLASSIC_e4e5), "e4e5N"},
		{int(CLASSIC_d4d5), "d4d5N"},
	} {
		ctPrintNamedAdjustTable(t.name, n, l.knightAdjust, t.center, 0, ctAdjustSquares)
	}

	for _, t := range []struct {
		center int
		name   string
	}{
		{int(FRENCH_high), "frenchHighB"},
		{int(FRENCH_low), "frenchLowB"},
		{int(KID_high), "KidHighB"},
		{int(KID_low), "KidLowB"},
		{int(SICILIAN_high), "SicHighB"},
		{int(SICILIAN_low), "SicLowB"},
		{int(CLASSIC_e4e5), "e4e5B"},
		{int(CLASSIC_d4d5), "d4d5B"},
	} {
		ctPrintNamedAdjustTable(t.name, n, l.bishopAdjust, t.center, 0, ctAdjustSquares)
	}
}

var ctPSTLabels = [6]string{"P", "N", "B", "R", "Q", "K"}

func ctPrintPST(n []ctPair, l ctLayout) {
	// preamble
	fmt.Println()
	fmt.Println("// Piece/square tables are roughly centered around zero, which means that")
	fmt.Println("// the sum of their values is close to zero. It has a few advantages:")
	fmt.Println("// changing pst percentage value should not disturb engine's perception")
	fmt.Println("// of material advantage, and changing pst to another zero-centered set")
	fmt.Println("// should not require adjustement of material values.")

	for _, t := range []struct {
		ph   int
		name string
	}{
		{0, "pstMG"},
		{1, "pstEG"},
	} {
		fmt.Printf("var %s = [6][64]int{\n", t.name)

		for piece := P; piece <= K; piece++ {
			fmt.Printf("\t%s: {\n", ctPSTLabels[piece])

			for rank := 0; rank < 8; rank++ {
				fmt.Print("\t\t")

				for file := 0; file < 8; file++ {
					if file != 0 {
						fmt.Print(", ")
					}

					sq := rank*8 + file

					fmt.Printf("%4d", ctRound(n[l.pst+piece*64+sq][t.ph]))
				}

				fmt.Println(",")
			}

			fmt.Println("\t},")
		}

		fmt.Println("}")
	}
}

func ctPrintMob(name string, params []ctPair, start, count, ph int) {
	fmt.Printf("var %s = [%d]int{", name, count)

	for i := 0; i < count; i++ {
		if i != 0 {
			fmt.Print(", ")
		}
		fmt.Print(ctRound(params[start+i][ph]))
	}

	fmt.Println("}")
}

// Entry point.
//
// Example:
//
//	ctTune("training.epd", 1000, 0.05)
func ctTune(filename string, epochs int, lr float64, lambda float64) {
	fmt.Printf("[core-tuner] loading %q\n", filename)

	l := ctMakeLayout()
	params := ctInitParams(l)
	initial := append([]ctPair(nil), params...)

	mask := ctTuneMask{
		material:     tuneMaterial,
		pst:          tunePST,
		mobility:     tuneMobility,
		passers:      tunePassers,
		phalanx:      tunePhalanx,
		adjustments:  tuneAdjustments,
		shield:       tuneShield,
		pawnWeakness: tunePawnWeaknesses,
	}

	fmt.Printf(
		"[core-tuner] %d parameter pairs, %d enabled, %d workers\n",
		l.num,
		ctCountEnabled(l, mask),
		ctWorkers(),
	)
	fmt.Printf(
		"[core-tuner] material=%v pst=%v mobility=%v passers=%v phalanx=%v adjustments=%v shield=%v pawnWeakness=%v lambda=%.6g\n",
		mask.material, mask.pst, mask.mobility, mask.passers, mask.phalanx, mask.adjustments, mask.shield, mask.pawnWeakness, lambda,
	)

	data, err := ctLoadDataset(filename, params, l)
	if err != nil {
		fmt.Println("[core-tuner] load error:", err)
		return
	}
	if len(data.entries) == 0 {
		fmt.Println("[core-tuner] no positions loaded")
		return
	}

	entryBytes := float64(len(data.entries)) * 40.0
	coeffBytes := float64(len(data.coeffs)) * 4.0
	fmt.Printf(
		"[core-tuner] loaded %d positions, %d sparse coeffs\n",
		len(data.entries), len(data.coeffs),
	)
	fmt.Printf(
		"[core-tuner] approximate core dataset storage %.1f MB\n",
		(entryBytes+coeffBytes)/(1024.0*1024.0),
	)

	fmt.Println("[core-tuner] fitting K...")
	k := ctFitK(data, params, initial, l, mask)
	fmt.Printf("[core-tuner] K = %.6f\n", k)

	mom := make([]ctPair, len(params))
	vel := make([]ctPair, len(params))

	lrInit := lr
	prevLoss := ctMSE(data, params, initial, l, mask, k, lambda)
	fmt.Printf(
		"[core-tuner] initial objective = %.10f  lr = %.6f\n",
		prevLoss, lr,
	)

	stall := 0
	for epoch := 1; epoch <= epochs; epoch++ {
		lr = ctLRMin +
			0.5*(lrInit-ctLRMin)*
				(1.0+math.Cos(math.Pi*float64(epoch-1)/float64(epochs)))

		loss := ctEpoch(
			data,
			params,
			initial,
			mom,
			vel,
			l,
			mask,
			lr,
			k,
			lambda,
		)

		fmt.Printf(
			"epoch %4d  objective = %.10f  lr = %.6f\n",
			epoch, loss, lr,
		)

		if epoch%100 == 0 {
			fmt.Println("--- snapshot ---")
			ctPrintParams(params, l)
			fmt.Println("--- end snapshot ---")
		}

		if lr <= ctLRMin*1.01 &&
			math.Abs(prevLoss-loss) < 1e-10 {
			stall++
			if stall >= 20 {
				fmt.Println("[core-tuner] converged")
				break
			}
		} else {
			stall = 0
		}
		prevLoss = loss
	}

	fmt.Println("--- FINAL PARAMETERS ---")
	ctPrintParams(params, l)
}