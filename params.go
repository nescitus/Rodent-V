package main

// --- Eval params ---

var pieceValMG = [7]int{88, 336, 344, 461, 938, 0, 0}
var pieceValEG = [7]int{135, 447, 463, 787, 1534, 0, 0}

// bishopPairMG/EG: bonus for owning both bishops.
// The EG value is higher because open boards in the endgame
// let the bishop pair dominate knight+bishop or two knights.
var bishopPairMG = 26
var bishopPairEG = 59

// mobility
var nMobMg = [9]int {-12,  -9,  -6,  -3,   0,   3,   6,   9,  12}
var nMobEg = [9]int {-17, -13,  -9,  -5,  -4,  -3,  -2,  -1,   0}
var bMobMg = [14]int{-48, -40, -32, -24, -16,  -8,   0,   8,  16,  24,  32,  40,  48,  56}
var bMobEg = [14]int{-48, -40, -32, -24, -16,  -8,   0,   8,  16,  24,  32,  40,  48,  56}
var rMobMg = [15]int{-21, -18, -15, -12,  -9,  -6,  -3,   0,   3,   6,   9,  12,  15,  18,  21}
var rMobEg = [15]int{-35, -30, -25, -20, -15, -10,  -5,   0,   5,  10,  15,  20,  25,  30,  35}
var qMobMg = [28]int{-28, -26, -24, -22, -20, -18, -16, -14, -12, -10,  -8,  -6,  -4,  -2,  0,
					   2,   4,   6,   8,  10,  12,  14,  16,  18,  20,  22,  24,  26}
var qMobEg = [28]int{-84, -78, -72, -66, -60, -54, -48, -42, -36, -30, -24, -18, -12, -6, 0,
					   6,  12,  18,  24,  30,  36,  42,  48,  54,  60,  66,  72,  78}

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
                   8,    8,    8,    8,    8,    8,    8,    8,
                 -16,  -26,  -20,  -18,  -21,    6,    5,  -13,
                 -17,  -22,   -6,   -6,    5,   -4,    4,   -8,
                 -19,  -21,   -7,   -1,    2,   -6,  -21,  -23,
                 -15,   -4,   -2,    0,   16,    8,   -3,  -20,
                   1,    2,   27,   24,   27,   65,   34,   -1,
                  30,  -26,   13,   31,   31,   -8,  -33,  -61,
                   8,    8,    8,    8,    8,    8,    8,    8,
        },
        N: {
                 -82,  -13,  -26,  -13,   -9,    6,  -11,  -56,
                 -37,  -30,  -14,    7,    6,    5,   -4,   -9,
                 -28,   -1,   14,   22,   34,   24,   16,   -6,
                  -6,   22,   34,   37,   45,   42,   48,   13,
                   9,   32,   50,   61,   49,   77,   51,   32,
                 -16,   16,   32,   52,   76,  104,   40,   18,
                 -54,  -30,  -11,   21,   -5,   55,   -8,  -44,
                -178,  -81,  -52,  -67,   -2,  -53,  -86, -116,
        },
        B: {
                  13,   24,   13,   -1,    9,    8,   22,   22,
                  17,   16,    8,    0,    0,   15,   36,   22,
                   4,   12,    4,   -4,    4,    9,   18,   20,
                  -2,   -3,    4,   13,   14,    8,   10,   17,
                  -6,    7,    5,   12,   19,    5,   17,   -9,
                  10,    4,   -1,    9,    4,   47,   16,   29,
                 -26,  -27,  -27,  -18,  -25,  -12,  -22,    5,
                 -38,  -58,  -12,  -69,  -69,  -15,  -41,  -65,
        },
        R: {
                 -20,  -20,  -18,  -11,   -9,   -5,    5,   -9,
                 -28,  -29,  -21,  -18,  -12,   -3,    5,  -12,
                 -30,  -26,  -25,  -21,  -10,   -5,   17,    8,
                 -25,  -20,  -18,  -11,   -6,   -8,   20,    2,
                  -5,   -4,    6,    3,    8,   24,   22,    5,
                 -10,   14,    1,    5,   32,   49,   72,   32,
                  -8,  -14,   -3,   16,    6,   20,   28,   50,
                   5,  -22,   -1,   -9,  -18,   14,   10,   36,
        },
        Q: {
                   0,  -13,  -13,   -1,   -4,  -12,    9,   12,
                  -6,    0,    4,    2,    3,   10,   24,   39,
                 -13,    6,   -4,   -9,   -1,    2,   24,   28,
                  -2,   -6,   -3,   -4,    6,    9,   25,   28,
                  -6,    8,  -15,  -14,  -11,    8,   15,   17,
                   7,   -1,  -15,   -5,   -4,   12,   15,   37,
                 -15,  -41,  -45,  -28,  -45,  -14,  -45,   32,
                 -10,  -21,    8,   12,   -8,   28,   10,   -6,
        },
        K: {
                  34,   82,   67,   -8,   49,    6,   68,   45,
                  52,   50,   40,   11,    9,   27,   58,   48,
                 -31,   17,   -1,   -9,   -3,    5,   23,  -24,
                 -98,  -31,    5,  -47,  -33,  -38,  -51, -140,
                -129,   -2,  -72, -118,  -77,  -71,  -83, -126,
                -141,   63,   23,  -29,  -45,   21,   18,  -77,
                 -49,  165,  111,   40,    1,   54,   55, -125,
                 -24,  225,  143,  -10,   30, -107,  124,   33,
        },
}
var pstEG = [6][64]int{
        P: {
                 -28,  -28,  -28,  -28,  -28,  -28,  -28,  -28,
                   8,   -4,    3,   -7,    5,    9,   -9,   -6,
                   4,   -7,   -7,   -5,   -1,    0,  -13,   -5,
                   8,    4,   -9,   -4,   -7,   -6,   -1,    0,
                  23,   12,    0,   -9,   -7,   -5,    8,   12,
                  38,   35,    5,  -20,  -16,    2,   23,   32,
                  62,   75,   47,   19,   15,   38,   56,   56,
                 -28,  -28,  -28,  -28,  -28,  -28,  -28,  -28,
        },
        N: {
                 -14,  -34,   -4,   -4,   -5,  -17,  -29,  -38,
                 -14,   -7,   11,    8,    7,    0,   -3,    1,
                  -7,   16,   15,   35,   26,   11,    6,  -10,
                   0,   21,   34,   42,   43,   29,   16,    0,
                   9,   23,   31,   39,   38,   30,   21,  -10,
                  -5,   15,   32,   28,    9,    4,    5,  -19,
                 -28,    4,   19,    8,    7,  -10,   -9,  -38,
                 -87,  -21,   -3,    6,   -9,  -43,  -43, -137,
        },
        B: {
                 -11,  -14,  -18,    0,    1,    2,  -18,  -29,
                  -2,  -14,  -14,   -5,   -5,  -16,  -11,  -31,
                  -4,    7,    1,    3,    4,   -5,   -7,    1,
                  14,   10,    6,    0,   -7,   -2,    6,   -4,
                  24,   19,   -2,    3,   -6,    1,    9,   17,
                  15,   12,    0,   -8,   -6,   -4,    8,    5,
                  -3,    6,    7,    8,    0,   -4,   -6,  -20,
                  12,   29,    9,   18,   20,    1,   -3,    5,
        },
        R: {
                  -1,   -3,    1,   -9,  -10,   -1,  -14,  -11,
                  -1,    0,   -3,   -8,  -11,  -18,  -21,  -11,
                   5,   -1,    2,   -2,  -11,  -15,  -30,  -22,
                  11,    5,    7,    5,    0,    3,  -20,  -11,
                  12,   10,    7,    4,   -7,  -12,  -10,   -5,
                  10,    1,    8,   -2,  -16,  -16,  -25,  -20,
                  14,   24,   25,   17,   16,    9,    4,   -9,
                  17,   26,   24,   25,   20,   21,   14,    9,
        },
        Q: {
                 -31,  -23,   -8,  -24,  -23,  -27,  -37,  -43,
                  -8,  -19,  -20,  -11,  -17,  -39,  -66,  -83,
                  10,  -10,    7,    1,   -5,    4,  -18,  -34,
                  15,   31,    6,   19,    5,   13,   16,    8,
                  18,    2,   20,   21,   25,    8,   43,   13,
                 -23,  -22,   15,    1,   28,   27,   38,  -10,
                 -20,   17,   47,   14,   43,   40,   56,    4,
                 -15,   -7,   -3,  -12,   16,   12,   17,   -5,
        },
        K: {
                 -83,  -63,  -36,  -14,  -45,  -21,  -51, -103,
                 -45,  -16,   -3,    3,    6,   -2,  -23,  -46,
                 -30,   -5,   12,   23,   22,   12,   -4,  -21,
                 -11,   16,   27,   42,   38,   33,   26,   -2,
                  -7,   19,   47,   60,   56,   51,   40,    4,
                  -4,   30,   45,   59,   56,   47,   36,   -5,
                 -16,   11,   41,   41,   64,   52,   51,    5,
                -181,  -87,  -36,   12,   31,   34,  -58, -137,
        },
}

// Phalanx pawns are pawns standing side by side.
// This is generally a good trait, increasing board
// control.

var phalanxMG = [64]int{
    0,    0,    0,    0,    0,    0,    0,    0,
    0,  -13,  -14,   -4,   -1,   -8,  -17,  -11,
    0,   -6,  -12,   -1,    7,    4,   -8,  -15,
    0,    2,   14,   16,   18,   20,   29,   13,
    0,   14,   44,   70,   65,   55,   33,   11,
    0,   41,   32,   32,   40,   25,   25,   32,
    0,   51,   53,   53,   53,   53,   53,   52,
    0,    0,    0,    0,    0,    0,    0,    0,
}
var phalanxEG = [64]int{
    0,    0,    0,    0,    0,    0,    0,    0,
    0,  -31,  -20,   10,   14,  -14,  -30,  -35,
    0,  -28,  -14,    0,   -7,  -12,  -16,  -24,
    0,  -17,   -4,    9,   10,    7,   -7,  -16,
    0,   20,   23,   49,   53,   48,   30,   25,
    0,   47,   49,   58,   61,   48,   51,   46,
    0,   93,   93,   92,   93,   92,   91,   97,
    0,    0,    0,    0,    0,    0,    0,    0,
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
