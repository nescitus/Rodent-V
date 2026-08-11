// ================================================================
// EVAL FLAIRS
// ================================================================
//
// Previous Rodents had evaluation functions that mixed two aspects
// of eval: things done for strength and things done for aeshetics.
// Rodent V tries to reduce complexity by moving the aestethics part
// to separate small functions.
//
// The intention is to use at most one flair per personality,
// as they may conflict with each other.
//
// flairClosed() - preference for closed positions
// flairTropism() - moving pieces towards enemy king
// flairForward() - moving pieces to opponent's half of the board

package main

// tropism values come from GambitFruit,
// https://github.com/Randl/GambitFruit/blob/master/GambitFruit/eval.cpp#L80
var tropismMG = [6]int{P: 0, N: 3, B: 2, R: 2, Q: 2, K: 0}
var tropismEG = [6]int{P: 0, N: 3, B: 1, R: 1, Q: 4, K: 0}

// Forwardness concept comes from Toga II 3.0
var forward = [6]int{P: 0, N: 1, B: 1, R: 2, Q: 4, K: 0}
var fwdBonus = [16]int{0, 2, 5, 8, 12, 15, 20, 20, 20, 20, 20, 20, 20, 20, 20, 20}
var away = [2]uint64{rank5BB | rank6BB | rank7BB | rank8BB, rank4BB | rank3BB | rank2BB | rank1BB}

// flairClosed gives material incentives for closing position,
// especially when engine already exchanged bishop for knight.
func flairClosed(p *Pos) int {
	var score [2]int // zero-initialized

	for side := White; side <= Black; side++ {

		// Increase knight value with more pawns on the board
		score[side] += 6 * p.count[side][N] * (p.count[side][P] - 4)

		// Upgrade own pawn value, downgrade bishop and rook values
		score[side] += 2 * p.count[side][P] // max +16
		score[side] -= 6 * p.count[side][B] // max -12
		score[side] -= 2 * p.count[side][R] // max -4
		//                   initial position total: 0
	}

	// Return score from the perspective of the side to move.
	return scoreFromPerspective(score[White]-score[Black], p.side)
}

// flairTropism encourages moving pieces towards enemy king.
// It is blind to blocked pawns and stuff, so performs best
// in closed positions.
func flairTropism(p *Pos) int {
	var mg [2]int
	var eg [2]int
	var phase int = 0

	for side := White; side <= Black; side++ {
		for pt := N; pt <= Q; pt++ {
			pieces := p.pieceBB(side, pt)

			for pieces != 0 {
				sq := lsb(pieces)
				pieces &= pieces - 1
				dst := distBonus(sq, p.kingSq[side^1])

				mg[side] += tropismMG[pt] * dst
				eg[side] += tropismEG[pt] * dst
				phase += gamePhase[pt] // gamePhase borrowed from pesto.go
			}
		}
	}

	// Clamp phase.
	phase = minOf(phase, 24)

	// Interpolate between midgame/endgame scores.
	mgScore := mg[White] - mg[Black]
	egScore := eg[White] - eg[Black]
	var result = (mgScore*phase + egScore*(24-phase)) / 24

	// Return score from the perspective of the side to move.
	return scoreFromPerspective(result, p.side)
}

// flairForward encourages moving pieces to enemy half
// of the board.
func flairForward(p *Pos) int {
	var mg [2]int
	var phase int
	var fwdWeight [2]int
	var fwdCount [2]int

	for side := White; side <= Black; side++ {
		for pt := N; pt <= Q; pt++ {
			pieces := p.pieceBB(side, pt)

			for pieces != 0 {
				sq := lsb(pieces)
				pieces &= pieces - 1

				// Award pieces on enemy half of the board.
				if squareBit(sq)&away[side] > 0 {
					fwdCount[side]++
					fwdWeight[side] += forward[pt]
				}

				phase += gamePhase[pt] // gamePhase borrowed from pesto.go
			}
		}
	}

	// Clamp phase.
	phase = minOf(phase, 24)

	// Set forwardness bonuses.
	mg[White] = fwdBonus[fwdCount[White]] * fwdWeight[White]
	mg[Black] = fwdBonus[fwdCount[Black]] * fwdWeight[Black]

	// Interpolate between midgame/endgame scores.
	mgScore := mg[White] - mg[Black]
	egScore := 0
	var result = (mgScore*phase + egScore*(24-phase)) / 24

	// Return score from the perspective of the side to move.
	return scoreFromPerspective(result, p.side)
}

func distBonus(s1, s2 int) int {
	return 7 - chebyshev(s1, s2)
}
