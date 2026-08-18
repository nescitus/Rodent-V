package main

var usePawnPairs bool

// --- Eval params ---
var pieceValMG = [7]int{88, 336, 344, 461, 938, 0, 0}
var pieceValEG = [7]int{135, 447, 463, 787, 1534, 0, 0}

// mobility
var mobOffset = [7]int{0, 4, 6, 7, 14, 0, 0}
var mobMG = [7]int{0, 3, 8, 3, 2, 0, 0}
var mobEG = [7]int{0, 0, 8, 5, 6, 0, 0}

// bishopPairMG/EG: bonus for owning both bishops.
// The EG value is higher because open boards in the endgame
// let the bishop pair dominate knight+bishop or two knights.
var bishopPairMG = 26
var bishopPairEG = 59

// Rook on open/semi-open file bonuses.
// Open file (no pawns at all): bigger bonus since the rook has full
// penetration potential.  Semi-open (no own pawn, enemy pawn present):
// smaller bonus; the rook pressures the enemy pawn but is partly blocked.
// EG values are near-zero: open files drive MG tactics, not endgame play.
var rookOpenFileMG = 30
var rookOpenFileEG = 6
var rookSemiOpenFileMG = 18
var rookSemiOpenFileEG = -3

// Pawn weaknesses
var isolatedMG = -17
var isolatedEG = -35
var isolatedOpenMG = -7
var backwardMG = -7
var backwardEG = -17
var backwardOpenMG = -9

// doubledPawnMG / doubledPawnEG: penalty for the rear pawn of a doubled
// pair, indexed by distance-to-edge (0=a/h file, 1=b/g, 2=c/f, 3=d/e).
// The penalty is applied only when the doubled pawn cannot immediately
// capture an enemy pawn (if it can capture, the structure is likely to be
// resolved tactically so the positional penalty is inappropriate).
// Values are mostly EG-heavy: doubled pawns become most dangerous as the
// position simplifies, since they cannot create a passed pawn by themselves.
var doubledPawnMG = [4]int{-18, 1, -10, -13}
var doubledPawnEG = [4]int{-28, -14, -16, -14}

// passedBonusMG / passedBonusEG: bonus for a passed pawn indexed by
// [blocked][relativeRank].  relativeRank is 0 at own back rank and 7
// at the promotion square, so it is the same for White and Black.
//
// The values are tuned automatically, by a variant of Texel tuning
// that uses many small batches and I am deeply sorry how it turned out.
var passedBonusMG = [2][8]int{
	0: {0, -5, -13, 0, 1, -15, 87, 0}, // free: push square empty
	1: {0, -5, -15, -7, 1, 7, 73, 0},  // blocked: push square occupied
}
var passedBonusEG = [2][8]int{
	0: {0, 14, 21, -2, 53, 146, 206, 0}, // free
	1: {0, 3, 19, -17, 24, 57, 59, 0},   // blocked
}

// ourPasserProximityMG/EG: bonus when our king is close to the passer's
// push square, indexed by Chebyshev distance (0 = same square, 7 = far corner).
// A king escorting its passer is a major endgame advantage.
var ourPasserProximityMG = [8]int{122, 0, -9, -37, -12, 4, 20, 3}
var ourPasserProximityEG = [8]int{63, 99, 60, 43, 11, -5, -18, -8}

// theirPasserProximityMG/EG: bonus indexed by Chebyshev distance between
// the enemy king and the passer's push square.  Convention matches Sirius:
// a positive value at large distance means the enemy king is far away (good
// for us); a negative value at distance 0 means the enemy king blocks (bad).
var theirPasserProximityMG = [8]int{-43, 32, 22, 11, -5, -6, -2, -23}
var theirPasserProximityEG = [8]int{-38, -56, -13, 23, 66, 81, 90, 81}

// kingAttackerWeight[pieceType]: how dangerous is each piece type
// when it attacks squares near the enemy king.
// Indexed P=0..Q=4; pawns and kings are handled separately.
var kingAttackerWeight = [6]int{0, 65, 78, 44, -29, 0}

// King-safety weights used in evaluateKing.
// These are package-level vars so the tuner can read/write them.
var (
	safeCheckWeight   = [4]int{143, 14, 56, 38} // N, B, R, Q safe check weights
	weakInRingWeight  = -19
	queenContactBonus = 87
	noQueenMul        = 3
	noQueenDiv        = 8
	dangerEgDiv       = 4
)

// Pawn shield penalties (MG only).
var (
	shieldMissing  = 11
	shieldAdvanced = 7
	shieldOpenFile = 39
	shieldSemiOpen = 19
)

// Threat scores reward the side whose pieces attack undefended or
// poorly-defended enemy pieces.  The bonus depends on:
//   - what piece type is doing the attacking
//   - what piece type is being attacked (victim)
//   - whether the victim is defended (index 0=hanging, 1=defended)
//
// Pawn and king threats do not use the defended flag: a pawn threat is
// always serious because capturing is free; a king threat is only
// rewarded when the victim is undefended (handled in the code).
//
// Push threats: a pawn one step away from attacking an enemy non-pawn.
// Only counted when the push square is safe (not controlled by an enemy pawn).

// threatByPawnMG/EG[victimType] — P..Q (K is never threatened by a pawn).
var threatByPawnMG = [6]int{-7, 73, 65, 72, 56, 0}
var threatByPawnEG = [6]int{-19, 41, 72, 50, 24, 0}

// threatByKnightMG/EG[defended][victimType] — 0=hanging, 1=defended.
var threatByKnightMG = [2][6]int{
	{5, 12, 50, 86, 41, 0},
	{-8, 9, 38, 71, 50, 0},
}

// threatByKnightMG/EG[defended][victimType].
var threatByKnightEG = [2][6]int{
	{37, 85, 33, 13, 8, 0},
	{11, 79, 29, 45, 46, 0},
}

// threatByBishopMG/EG[defended][victimType].
var threatByBishopMG = [2][6]int{
	{3, 36, 12, 58, 61, 0},
	{-5, 20, 4, 56, 63, 0},
}
var threatByBishopEG = [2][6]int{
	{34, 44, 102, 35, 53, 0},
	{4, 21, 76, 60, 74, 0},
}

// threatByRookMG/EG[defended][victimType].
var threatByRookMG = [2][6]int{
	{-3, 35, 45, -12, 67, 0},
	{-10, 8, 19, 1, 54, 0},
}
var threatByRookEG = [2][6]int{
	{50, 52, 49, 50, -10, 0},
	{10, 15, 4, 22, 85, 0},
}

// threatByQueenMG/EG[defended][victimType].
var threatByQueenMG = [2][6]int{
	{8, 25, 18, 16, -2, 0},
	{-5, 2, -9, -7, -19, 0},
}
var threatByQueenEG = [2][6]int{
	{21, 30, 65, 12, -17, 0},
	{16, 8, 37, 7, 1, 0},
}

// threatByKingMG/EG[victimType] — king only attacks undefended squares.
var threatByKingMG = [6]int{39, 33, 99, 83, 0, 0}
var threatByKingEG = [6]int{18, 38, 33, 8, 0, 0}

// pushThreatMG/EG: per non-pawn enemy piece attacked by a safe pawn push.
var pushThreatMG = 13
var pushThreatEG = 17

// Piece/square tables are roughly centered around zero, which means that
// the sum of their values is close to zero. It has a few advantages:
// changing pst percentage value should not disturb engine's perception
// of material advantage, and changing pst to another zero-centered set
// should not require adjustement of material values.

var pstMG = [6][64]int{
	P: {
		9, 9, 9, 9, 9, 9, 9, 9,
		-21, -21, -13, -18, -18, 5, 14, -19,
		-21, -20, -7, -8, 2, -10, 1, -15,
		-24, -23, -10, 5, 3, -11, -23, -25,
		-19, -4, -3, -5, 15, 2, 1, -18,
		3, 1, 26, 21, 25, 65, 36, 0,
		30, -26, 13, 31, 32, -7, -32, -60,
		9, 9, 9, 9, 9, 9, 9, 9,
	},
	N: {
		-82, -18, -25, -13, -11, 7, -14, -56,
		-37, -32, -14, 9, 7, 6, -3, -8,
		-32, -4, 16, 20, 33, 25, 19, -6,
		-7, 23, 37, 36, 46, 45, 45, 10,
		10, 32, 53, 67, 56, 81, 49, 30,
		-16, 18, 32, 54, 76, 103, 39, 17,
		-56, -30, -11, 20, -8, 55, -8, -45,
		-179, -82, -52, -68, -3, -54, -87, -117,
	},
	B: {
		12, 24, 21, 3, 14, 6, 23, 22,
		17, 13, 9, -2, 6, 18, 37, 24,
		4, 13, 7, -3, 4, 13, 18, 19,
		-1, -6, 2, 12, 10, 10, 8, 17,
		-5, 1, 5, 7, 18, 3, 11, -12,
		11, 4, -3, 8, 1, 47, 15, 31,
		-26, -26, -28, -18, -25, -13, -22, 5,
		-38, -58, -12, -69, -69, -15, -41, -65,
	},
	R: {
		-15, -17, -18, -10, -5, -2, 7, -6,
		-23, -28, -17, -14, -7, -1, 7, -10,
		-27, -26, -23, -20, -8, -3, 17, 10,
		-24, -20, -18, -11, -7, -10, 19, 2,
		-5, -6, 4, -3, 3, 21, 21, 3,
		-12, 12, -1, 0, 29, 47, 71, 31,
		-11, -14, -3, 15, 4, 19, 28, 49,
		3, -24, -2, -10, -19, 13, 9, 35,
	},
	Q: {
		1, -12, -11, 2, -2, -10, 10, 12,
		-4, 1, 6, 9, 9, 13, 26, 40,
		-12, 6, -4, -7, 2, 4, 25, 28,
		0, -7, -2, -3, 6, 5, 22, 26,
		-6, 10, -14, -14, -13, 4, 8, 10,
		8, 0, -14, -6, -6, 10, 11, 35,
		-18, -38, -46, -28, -47, -16, -46, 30,
		-11, -23, 7, 11, -9, 27, 9, -8,
	},
	K: {
		35, 85, 74, -6, 48, 12, 68, 49,
		52, 50, 40, 13, 8, 30, 59, 41,
		-31, 17, -2, -10, -5, 2, 19, -25,
		-98, -31, 5, -47, -34, -40, -54, -141,
		-129, -2, -72, -118, -77, -71, -84, -126,
		-141, 63, 23, -29, -45, 21, 18, -77,
		-49, 165, 111, 40, 1, 54, 55, -125,
		-24, 225, 143, -10, 30, -107, 124, 33,
	},
}

var pstEG = [6][64]int{
	P: {
		-26, -26, -26, -26, -26, -26, -26, -26,
		4, 0, 0, -8, 4, 5, -5, -3,
		0, -1, -9, -7, -3, -5, -9, -8,
		1, 2, -11, -8, -8, -8, 1, -7,
		22, 12, -2, -9, -10, -5, 11, 10,
		36, 36, 2, -25, -19, 3, 27, 34,
		64, 76, 47, 18, 16, 40, 58, 58,
		-26, -26, -26, -26, -26, -26, -26, -26,
	},
	N: {
		-14, -32, -3, -4, -5, -16, -29, -38,
		-14, -8, 13, 10, 7, 2, -2, 2,
		-5, 18, 17, 37, 27, 15, 7, -8,
		1, 21, 34, 42, 42, 27, 14, -1,
		10, 23, 32, 36, 33, 30, 18, -12,
		-5, 16, 32, 29, 8, 2, 4, -20,
		-29, 5, 20, 6, 4, -10, -9, -39,
		-87, -21, -4, 5, -10, -44, -44, -137,
	},
	B: {
		-10, -14, -15, 4, 5, 7, -18, -29,
		-1, -12, -14, -3, -2, -14, -9, -29,
		-4, 9, 3, 2, 5, -3, -7, 1,
		15, 10, 5, -2, -11, -5, 5, -4,
		25, 18, -3, 0, -11, -1, 5, 16,
		16, 12, -2, -11, -9, -6, 7, 5,
		-4, 5, 6, 8, -1, -5, -7, -21,
		12, 29, 9, 17, 20, 1, -3, 5,
	},
	R: {
		7, 0, 5, -5, -8, -2, -13, -6,
		4, 3, 1, -3, -6, -15, -20, -10,
		9, -1, 4, 1, -9, -14, -31, -21,
		13, 4, 6, 5, -1, 2, -22, -11,
		12, 10, 6, 0, -12, -15, -12, -8,
		9, 1, 7, -5, -19, -19, -26, -24,
		11, 25, 27, 15, 13, 9, 4, -11,
		14, 22, 22, 24, 17, 19, 11, 6,
	},
	Q: {
		-30, -22, -6, -23, -21, -26, -36, -43,
		-7, -18, -16, -8, -14, -36, -65, -83,
		11, -10, 8, 2, -4, 5, -18, -34,
		16, 32, 7, 19, 4, 11, 15, 7,
		19, 3, 21, 21, 23, 5, 40, 10,
		-23, -22, 16, 1, 27, 26, 36, -12,
		-22, 17, 47, 13, 42, 39, 55, 3,
		-16, -9, -4, -13, 15, 11, 16, -6,
	},
	K: {
		-83, -65, -37, -11, -38, -21, -61, -97,
		-45, -15, -2, 5, 8, -5, -26, -47,
		-30, -5, 13, 22, 21, 8, -6, -20,
		-11, 16, 28, 42, 37, 30, 22, -4,
		-8, 19, 48, 62, 58, 51, 39, 3,
		-4, 31, 46, 61, 58, 47, 36, -5,
		-16, 11, 42, 41, 65, 53, 51, 5,
		-181, -87, -36, 12, 31, 34, -58, -137,
	},
}

// Phalanx pawns are pawns standing side by side.
// This is generally a good trait, increasing board
// control.

var phalanxMG = [64]int{
	0, 0, 0, 0, 0, 0, 0, 0,
	0, -15, -8, -2, 10, -15, -13, -12,
	-11, -5, -8, 7, 2, -3, -2, -13,
	-9, 3, 11, -2, 14, 12, 15, 6,
	-22, 30, -12, 58, -3, 76, -15, 27,
	-40, 120, 30, -28, 138, 38, 100, 86,
	62, 72, 65, 81, 64, 67, 15, -12,
	0, 0, 0, 0, 0, 0, 0, 0,
}

var phalanxEG = [64]int{
	0, 0, 0, 0, 0, 0, 0, 0,
	-12, -8, -9, 4, 19, -26, 5, -29,
	-13, -6, 4, -3, 2, -4, -1, -12,
	-7, 2, 2, 13, 9, 0, 2, -12,
	44, -3, 47, 14, 47, -6, 22, 5,
	21, 147, 98, 153, 41, 18, 137, -98,
	257, 296, 172, 193, 257, 284, 280, 192,
	0, 0, 0, 0, 0, 0, 0, 0,
}

var frenchHighP = [64]int{
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	1, 0, 10, 0, 0, 0, 1, 0,
	0, 3, 0, 0, 0, 6, 1, 0,
	0, 0, 0, 0, 0, 1, 0, 1,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
}

var frenchLowP = [64]int{
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, -2, 0, 0, 0, 0, 0,
	0, 0, -2, 0, 0, 0, -2, -2,
	0, 0, 3, 0, 0, -2, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
}

var KidHighP = [64]int{
	0, 0, 0, 0, 0, 0, 0, 0,
	-2, 0, 0, 0, 0, 0, 0, 0,
	-2, 1, -1, 0, 0, 12, 0, 0,
	-2, 6, 2, 0, 0, 0, 2, 0,
	0, 0, 2, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
}

var KidLowP = [64]int{
	0, 0, 0, 0, 0, 0, 0, 0,
	-1, 0, -1, 0, 0, -4, -3, -2,
	0, -1, 0, 0, 0, -3, -3, 0,
	0, 1, 1, 0, -8, 8, 8, 4,
	0, 0, 0, 0, 0, 10, 10, -2,
	0, 0, 0, 0, 0, 15, 15, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
}

var SicHighP = [64]int{
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, -2, 0, 0,
	0, -2, -1, 0, 0, 0, 0, 0,
	2, 0, 1, 0, 1, 2, 0, 1,
	4, 0, 0, 0, 0, 1, 1, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
}

var SicLowP = [64]int{
	0, 0, 0, 0, 0, 0, 0, 0,
	-1, -1, 0, 0, 0, 0, 0, 0,
	8, 0, 0, 0, 1, 0, 0, 0,
	0, 3, 0, 0, 1, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
}

var e4e5P = [64]int{
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, -1, 0, 0, 0, 0, 0,
	0, 0, 0, 1, 0, -2, 0, 0,
	0, 0, 0, 0, 0, 1, 0, 0,
	0, 0, 0, 3, 0, 1, 1, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
}

var d4d5P = [64]int{
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 1, -3, 0, 0, 0, 0, 0,
	0, 0, 3, 0, -1, 1, 0, 0,
	0, 0, 2, 0, 1, -1, 0, 0,
	0, 0, 0, 0, 6, 0, 0, 0, // e5 = "and no black pawn on e6"
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
}

var frenchHighN = [64]int{
	0, 2, 0, 0, 0, 0, 1, 0,
	0, 0, 0, 0, 1, 0, 0, 0,
	0, 0, 1, 1, 0, 1, 0, 0,
	0, 0, 0, 0, 0, 1, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
}

var frenchLowN = [64]int{
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	2, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
}

var KidHighN = [64]int{
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 3, 0, -3, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
}

var KidLowN = [64]int{
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 1, 0, 0,
	2, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 2, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 2, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
}

var SicHighN = [64]int{
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 7, 0, -1, 1, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 3, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
}

var SicLowN = [64]int{
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 1, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 1, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
}

var e4e5N = [64]int{
	0, 0, 0, 0, 0, 4, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 2,
	0, -1, -1, 0, 0, -4, 4, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 5, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
}

var d4d5N = [64]int{
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, -1, -1, 1, 0, 0, 0,
	-1, -1, -1, 0, 0, 3, -1, 0,
	0, 0, 0, 0, 0, 1, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
}

var frenchHighB = [64]int{
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 2, 0, 0, 0,
	3, 0, 0, 3, 0, 0, 0, 1,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, -1, 0, 0, 0, 0, 1, 0,
	0, 0, 0, 0, 0, 0, 0, 1,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
}

var frenchLowB = [64]int{
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, -20, 2,
	0, 0, 0, 0, 0, 0, 2, 0,
	1, 0, 0, 0, 0, 2, 0, 2,
	1, 0, 0, 0, 0, 0, 3, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
}

var KidHighB = [64]int{
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, -1, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 1, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
}

var KidLowB = [64]int{
	-1, -1, 2, 0, 0, 0, 0, 0,
	0, -10, 0, 2, 0, 0, 0, 0,
	-2, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
}

var SicHighB = [64]int{
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 1, 0, 0, 0, 5, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
}

var SicLowB = [64]int{
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 1, 0, 0, 4, 0, 0, 0,
	0, 0, 0, 0, 2, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
}

var e4e5B = [64]int{
	0, 0, 0, 0, 0, 0, 0, 0,
	2, -4, 2, 0, 0, 0, -5, 0,
	0, 3, 0, 0, 0, 0, 0, 0,
	1, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
}

var d4d5B = [64]int{
	0, 2, 0, 0, 0, 0, 0, 0,
	0, -3, 2, 0, 0, 0, -1, 1,
	0, -1, 0, 3, 0, 0, 3, 0,
	-1, 0, 0, 0, 0, 3, 0, 5,
	0, 1, 0, 0, 0, 0, 3, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
}

// per-color tables used to speed up the evaluation

var pstMGByColor [2][6][64]int
var pstEGByColor [2][6][64]int
var phalanxMgByColor [2][64]int
var phalanxEgByColor [2][64]int

var pawnAdjust [Undefined][2][64]int
var knightAdjust [Undefined][2][64]int
var bishopAdjust [Undefined][2][64]int
var pawnPairMg[2][64][64] int
var pawnPairEg[2][64][64] int

// Init

func init() {
	for piece := 0; piece < 6; piece++ {
		for sq := 0; sq < 64; sq++ {
			pstMGByColor[White][piece][sq] = pstMG[piece][sq]
			pstMGByColor[Black][piece][sq^56] = pstMG[piece][sq]

			pstEGByColor[White][piece][sq] = pstEG[piece][sq]
			pstEGByColor[Black][piece][sq^56] = pstEG[piece][sq]
		}
	}
	usePawnPairs = false //

	// 56.378785953357436
	// 56.321991399224174
	
	// touching doubled pawns
	addPawnRelation(A2, A3, -18, -28)
	addPawnRelation(A3, A4, -18, -28)
	addPawnRelation(A4, A5, -18, -28)
	addPawnRelation(A5, A6, -18, -28)
	addPawnRelation(A6, A7, -18, -28)

	addPawnRelation(H2, H3, -18, -28)
	addPawnRelation(H3, H4, -18, -28)
	addPawnRelation(H4, H5, -18, -28)
	addPawnRelation(H5, H6, -18, -28)
	addPawnRelation(H6, H7, -18, -28)

	addPawnRelation(B2, B3,  1, -14)
	addPawnRelation(B3, B4,  1, -14)
	addPawnRelation(B4, B5,  1, -14)
	addPawnRelation(B5, B6,  1, -14)
	addPawnRelation(B6, B7,  1, -14)

	addPawnRelation(G2, G3,  1, -14)
	addPawnRelation(G3, G4,  1, -14)
	addPawnRelation(G4, G5,  1, -14)
	addPawnRelation(G5, G6,  1, -14)
	addPawnRelation(G6, G7,  1, -14)

	addPawnRelation(C2, C3,  -10, -16)
	addPawnRelation(C3, C4,  -11, -16)
	addPawnRelation(C4, C5,  -10, -16)
	addPawnRelation(C5, C6,  -10, -16)
	addPawnRelation(C6, C7,  -10, -16)

	addPawnRelation(F2, F3,  -10, -16)
	addPawnRelation(F3, F4,  -10, -16)
	addPawnRelation(F4, F5,  -10, -16)
	addPawnRelation(F5, F6,  -10, -16)
	addPawnRelation(F6, F7,  -10, -16)

	addPawnRelation(D2, D3,  -13, -14)
	addPawnRelation(D3, D4,  -13, -14)
	addPawnRelation(D4, D5,  -13, -14)
	addPawnRelation(D5, D6,  -13, -14)
	addPawnRelation(D6, D7,  -13, -14)

	addPawnRelation(E2, E3,  -14, -14)
	addPawnRelation(E3, E4,  -14, -14)
	addPawnRelation(E4, E5,  -13, -14)
	addPawnRelation(E5, E6,  -13, -14)
	addPawnRelation(E6, E7,  -13, -14)

	// non touching doubled pawns
	addPawnRelation(A2, A4, 0, -1)
	addPawnRelation(D2, D4,-1, -1)
	addPawnRelation(E2, E4,-1, -1)
    addPawnRelation(H2, H4,-1, -1)
	addPawnRelation(C3, C5, 1, 0)

	// defended pawns on 3rd line
	addPawnRelation(A2, B3, 1, 0)
	addPawnRelation(B2, A3, 1, 0)
	addPawnRelation(B2, C3, 4, 0)
	addPawnRelation(C2, B3, 0, 0)
	addPawnRelation(C2, D3,-2, 0)
	addPawnRelation(D2, E3, 0, 0)
	addPawnRelation(F2, E3, 3, 0)
	addPawnRelation(F2, G3, 6, 0)

	// pawn triangles, apex on 4th line
	// assumption: defended pawn good,
	// doubly defended leaves color weakness
	addPawnRelation(A3, B4, 2, 0)
	addPawnRelation(C3, B4, 1, 0)
	addPawnRelation(A3, C3,-1, 0)

	addPawnRelation(B3, C4, 1, 0)
	addPawnRelation(D3, C4, 1, 0)
	addPawnRelation(D3, B3,-2, 0)

	addPawnRelation(C3, D4, 5, 0)
	addPawnRelation(E3, D4, 6, 0)
	addPawnRelation(C3, E3, -5, 0)

	addPawnRelation(D3, E4, 5, 0)
	addPawnRelation(F3, E4, 5, 0)
	addPawnRelation(D3, F3, -12, 0)

	addPawnRelation(E3, F4, 1, 0)
	addPawnRelation(G3, F4, 1, 0)
	addPawnRelation(E3, G3, -7, 0)

	addPawnRelation(F3, G4, 1, 0)
	addPawnRelation(H3, G4, 1, 0)
	addPawnRelation(F3, H3, -12, 0)

	addPawnRelation(B3, A4,-1, 0)
	addPawnRelation(G3, H4,-1, 0)

	// defended pawns on the 5th line
	addPawnRelation(E4, F5, 2, 0)
	addPawnRelation(G4, H5, -1, 0)
	addPawnRelation(D4, E5, 3, 0)
	addPawnRelation(E4, D5, 3, 0)

	// 2-4 rank pattern
	addPawnRelation(B2, A4,  0, 0)
	addPawnRelation(A2, B4, -5, 0)
	addPawnRelation(H2, G4, -5, 0)
	addPawnRelation(C2, D4, -2, 0)
    addPawnRelation(C2, B4, -1, 0)
	addPawnRelation(F2, E4,  1, 0)
	addPawnRelation(F2, G4, -3, 0)
	addPawnRelation(G2, H4, -1, 0)

	// 3-5 rank neighbouring pattern
	addPawnRelation(H3, G5,-1, 0)
	addPawnRelation(C3, D5, 0, 0)
	addPawnRelation(F3, E5,-2, 0)

	// binds: assuming good in the center, boring on the wings
	// (not very interesting term)
	addPawnRelation(F4, H4,-3, 0)
	addPawnRelation(C4, E4, 1, 0)
	addPawnRelation(D4, F4, 1, 0)

    // may or may not be a part of pawn chain
	addPawnRelation(C3, E5, 1, 0)
	addPawnRelation(F3, D5,  1, 0)

	// fianchetto complementary
	addPawnRelation(G3, D3, 3, 0)
	addPawnRelation(B3, E3, 6, 0)
	addPawnRelation(G3, C4, 1, 0)
	addPawnRelation(G3, B5, 1, 0)

	// others
	addPawnRelation(C3, E4, 3, 0)
	addPawnRelation(F4, H3, -3, 0)

	for sq := 0; sq < 64; sq++ {
		phalanxMgByColor[White][sq] = phalanxMG[sq]
		phalanxEgByColor[White][sq] = phalanxEG[sq]
		phalanxMgByColor[Black][sq^56] = phalanxMG[sq]
		phalanxEgByColor[Black][sq^56] = phalanxEG[sq]

		pawnAdjust[FRENCH_high][White][sq] = frenchHighP[sq]
		pawnAdjust[FRENCH_high][Black][sq^56] = frenchHighP[sq]
		knightAdjust[FRENCH_high][White][sq] = frenchHighN[sq]
		knightAdjust[FRENCH_high][Black][sq^56] = frenchHighN[sq]
		bishopAdjust[FRENCH_high][White][sq] = frenchHighB[sq]
		bishopAdjust[FRENCH_high][Black][sq^56] = frenchHighB[sq]

		pawnAdjust[FRENCH_low][White][sq] = frenchLowP[sq]
		pawnAdjust[FRENCH_low][Black][sq^56] = frenchLowP[sq]
		knightAdjust[FRENCH_low][White][sq] = frenchLowN[sq]
		knightAdjust[FRENCH_low][Black][sq^56] = frenchLowN[sq]
		bishopAdjust[FRENCH_low][White][sq] = frenchLowB[sq]
		bishopAdjust[FRENCH_low][Black][sq^56] = frenchLowB[sq]

		pawnAdjust[KID_high][White][sq] = KidHighP[sq]
		pawnAdjust[KID_high][Black][sq^56] = KidHighP[sq]
		knightAdjust[KID_high][White][sq] = KidHighN[sq]
		knightAdjust[KID_high][Black][sq^56] = KidHighN[sq]
		bishopAdjust[KID_high][White][sq] = KidHighB[sq]
		bishopAdjust[KID_high][Black][sq^56] = KidHighB[sq]

		pawnAdjust[KID_low][White][sq] = KidLowP[sq]
		pawnAdjust[KID_low][Black][sq^56] = KidLowP[sq]
		knightAdjust[KID_low][White][sq] = KidLowN[sq]
		knightAdjust[KID_low][Black][sq^56] = KidLowN[sq]
		bishopAdjust[KID_low][White][sq] = KidLowB[sq]
		bishopAdjust[KID_low][Black][sq^56] = KidLowB[sq]

		pawnAdjust[SICILIAN_high][White][sq] = SicHighP[sq]
		pawnAdjust[SICILIAN_high][Black][sq^56] = SicHighP[sq]
		knightAdjust[SICILIAN_high][White][sq] = SicHighN[sq]
		knightAdjust[SICILIAN_high][Black][sq^56] = SicHighN[sq]
		bishopAdjust[SICILIAN_high][White][sq] = SicHighB[sq]
		bishopAdjust[SICILIAN_high][Black][sq^56] = SicHighB[sq]

		pawnAdjust[SICILIAN_low][White][sq] = SicLowP[sq]
		pawnAdjust[SICILIAN_low][Black][sq^56] = SicLowP[sq]
		knightAdjust[SICILIAN_low][White][sq] = SicLowN[sq]
		knightAdjust[SICILIAN_low][Black][sq^56] = SicLowN[sq]
		bishopAdjust[SICILIAN_low][White][sq] = SicLowB[sq]
		bishopAdjust[SICILIAN_low][Black][sq^56] = SicLowB[sq]

		pawnAdjust[CLASSIC_e4e5][White][sq] = e4e5P[sq]
		pawnAdjust[CLASSIC_e4e5][Black][sq^56] = e4e5P[sq]
		knightAdjust[CLASSIC_e4e5][White][sq] = e4e5N[sq]
		knightAdjust[CLASSIC_e4e5][Black][sq^56] = e4e5N[sq]
		bishopAdjust[CLASSIC_e4e5][White][sq] = e4e5B[sq]
		bishopAdjust[CLASSIC_e4e5][Black][sq^56] = e4e5B[sq]

		pawnAdjust[CLASSIC_d4d5][White][sq] = d4d5P[sq]
		pawnAdjust[CLASSIC_d4d5][Black][sq^56] = d4d5P[sq]
		knightAdjust[CLASSIC_d4d5][White][sq] = d4d5N[sq]
		knightAdjust[CLASSIC_d4d5][Black][sq^56] = d4d5N[sq]
		bishopAdjust[CLASSIC_d4d5][White][sq] = d4d5B[sq]
		bishopAdjust[CLASSIC_d4d5][Black][sq^56] = d4d5B[sq]
	}
}

func addPawnRelation(s1, s2, mgVal, egVal int) {
    pawnPairMg[White][RelSq(s1, White)][RelSq(s2, White)] += mgVal;
    pawnPairMg[Black][RelSq(s1, Black)][RelSq(s2, Black)] += mgVal;
    pawnPairMg[White][RelSq(s2, White)][RelSq(s1, White)] += mgVal;
    pawnPairMg[Black][RelSq(s2, Black)][RelSq(s1, Black)] += mgVal;
    pawnPairEg[White][RelSq(s1, White)][RelSq(s2, White)] += egVal;
    pawnPairEg[Black][RelSq(s1, Black)][RelSq(s2, Black)] += egVal;
    pawnPairEg[White][RelSq(s2, White)][RelSq(s1, White)] += egVal;
    pawnPairEg[Black][RelSq(s2, Black)][RelSq(s1, Black)] += egVal;
}

func RelSq(sq, cl int) int {
	return ( sq ^ (cl * 56) )
}