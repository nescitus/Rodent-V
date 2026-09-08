package main

// --- Eval params ---

// Basic piece values.
var pieceValMG = [7]int{95, 337, 348, 462, 938, 0, 0}
var pieceValEG = [7]int{142, 445, 466, 788, 1531, 0, 0}

// bishopPairMG/EG: bonus for owning both bishops.
// The EG value is higher because open boards in the endgame
// let the bishop pair dominate knight+bishop or two knights.
var bishopPairMG = 28
var bishopPairEG = 60

// rookPairMG/EG: an incentive to exchange pair of rooks
// in positions with material imbalance.
var rookPairMG = -4
var rookPairEG = -15

// exchangePlusMG/EG: discourages exchange sacrifices.
var exchangePlusMG = 14
var exchangePlusEG = 10

// twoMinorsMG/EG: two minors for a rook adjustement.
var twoMinorsMG = 21
var twoMinorsEG = -19

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
var isolatedMG = -16
var isolatedEG = -35
var isolatedOpenMG = -8
var backwardMG = -7
var backwardEG = -17
var backwardOpenMG = -10

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
	0: {0, -5, -12, 0, 1,  9, 87, 0}, // free: push square empty
	1: {0, -5, -14, -7, 1, 7, 73, 0},  // blocked: push square occupied
}
var passedBonusEG = [2][8]int{
	0: {0, 14, 21, -2, 53, 147, 206, 0}, // free
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
var kingAttackerWeight = [6]int{0, 65, 78, 44, -26, 0}

// King-safety weights used in evaluateKing.
// These are package-level vars so the tuner can read/write them.
// King-safety weights used in evaluateKing.
// These are package-level vars so the tuner can read/write them.
var (
	safeCheckWeight   = [4]int{143, 14, 56, 36} // N, B, R, Q safe check weights
	weakInRingWeight  = -19
	queenContactBonus = 87
	noQueenMul        = 3
	noQueenDiv        = 8
	dangerEgDiv       = 4
)

// Pawn shield penalties (MG only).
var (
 shieldRank2 = 1
 shieldRank3 = 4
 shieldRank4 = 6
 shieldRank5 = 12
 shieldRank6 = 9
 shieldRank7 = 9
 shieldNoPawn = 29
 stormRank3 = 10
 stormRank4 = 7
 stormRank5 = 2
 stormNoPawn = 6
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
	{3, 37, 12, 58, 61, 0},
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

// King-relative piece/square tables.
// MG uses same-wing and opposite-wing buckets; EG is shared.
var mgPSQTSame = [6][64]int{
        P: {
                   4,    4,    4,    4,    4,    4,    4,    4,
                 -14,  -22,  -17,   -9,  -17,   23,   17,  -21,
                 -14,  -17,   -4,   -2,   10,    7,   11,  -10,
                 -17,  -15,   -3,    3,    7,    6,  -12,  -25,
                 -11,    1,    2,    5,   14,   20,    4,  -23,
                   3,   12,   36,   33,   33,   74,   35,   -7,
                  53,  -27,    9,   31,    1,  -15,  -60,  -97,
                   4,    4,    4,    4,    4,    4,    4,    4,
        },
        N: {
                 -72,   -5,  -27,   -6,   -1,   15,   -1,  -43,
                 -33,  -21,   -8,   13,   15,    9,    9,    0,
                 -21,    6,   21,   26,   39,   33,   25,    0,
                  -1,   27,   38,   41,   52,   50,   53,   19,
                  11,   35,   47,   63,   52,   80,   56,   44,
                 -14,   10,   37,   50,   63,  149,   41,   50,
                 -60,  -35,  -18,   14,    5,   85,    7,  -20,
                -121,  -80,  -41,  -64,   -8,  -36,  -92, -182,
        },
        B: {
                  17,   26,   18,    1,   10,   14,   36,   24,
                  22,   20,   11,    3,    5,   19,   44,   24,
                   8,   16,    8,    0,    9,   15,   23,   26,
                   0,    2,    9,   16,   24,   12,   14,   20,
                  -9,   11,    4,   21,   20,    8,   22,   -2,
                   8,    0,    4,    7,   15,   69,   18,   32,
                 -27,  -31,  -22,  -11,  -40,   -8,   -9,   18,
                 -28,  -43,   -6,  -47,  -68,   39,  -38,  -58,
        },
        R: {
                  -6,   -7,   -3,    6,    6,    8,    5,   -3,
                 -20,  -15,   -6,   -4,    1,    7,   21,    4,
                 -16,  -14,  -13,   -6,    3,    5,   30,   15,
                  -9,  -11,   -6,   -1,   11,    2,   30,    2,
                   3,    4,   19,   22,   28,   34,   37,   13,
                   5,   30,   18,   24,   55,   71,   78,   37,
                   9,    3,   14,   35,   19,   46,   33,   61,
                   2,    5,   10,   13,   10,    7,   28,   24,
        },
        Q: {
                  -4,  -20,  -17,   -4,   -9,  -22,   10,   -1,
                  -5,   -5,    0,   -2,   -1,   10,   22,   30,
                 -16,    2,   -9,  -11,   -6,   -1,   22,   24,
                 -10,   -7,   -9,  -10,    4,   10,   25,   26,
                 -16,   -2,  -12,  -17,   -6,    7,   23,   24,
                  -1,   -5,  -11,   -1,    2,   38,   23,   41,
                 -26,  -43,  -37,  -17,  -47,   10,  -17,   79,
                 -31,  -46,    3,  -10,   -5,   42,   41,   32,
        },
        K: {
                   0,    0,    0,    0,   26,    6,   64,   54,
                   0,    0,    0,    0,    3,   25,   59,   59,
                   0,    0,    0,    0,   18,    6,   31,  -12,
                   0,    0,    0,    0,  -35,  -35,  -73, -150,
                   0,    0,    0,    0,  -84,  -89,  -91, -141,
                   0,    0,    0,    0,  -49,   33,   19,  -87,
                   0,    0,    0,    0,   11,   52,   56, -132,
                   0,    0,    0,    0,   31, -106,  124,   23,
        },
}
var mgPSQTOpposite = [6][64]int{
        P: {
                   4,    4,    4,    4,    4,    4,    4,    4,
                 -42,  -43,  -18,  -35,  -35,   -1,   26,   10,
                 -40,  -41,  -13,   -6,   -4,   -2,   21,   10,
                 -42,  -32,   -9,    0,    2,   -6,   -5,    4,
                 -41,  -14,    7,   12,   -2,    6,   21,   16,
                 -25,   -8,   64,   59,   30,   45,   66,   50,
                 -51,  -28,    1,   19,   24,   17,    5,   -6,
                   4,    4,    4,    4,    4,    4,    4,    4,
        },
        N: {
                 -93,  -32,  -33,  -35,  -20,    3,  -32,  -57,
                 -37,  -28,  -10,    2,    1,    3,  -21,  -29,
                 -29,    1,    3,   18,   18,   23,   -2,  -28,
                  -1,   30,   33,   48,   33,   36,   33,    1,
                  34,   42,   66,   53,   73,   73,   39,   16,
                 -26,   16,   58,   53,   60,   29,   14,    3,
                 -45,  -39,   -3,   -4,    1,    3,   14,  -56,
                -180,  -86,  -69,  -52,   -8,  -56,  -77,  -90,
        },
        B: {
                  -9,   -9,   -4,  -11,   40,    2,   17,   30,
                 -10,   -3,   -3,   -9,    6,   32,   54,   18,
                 -12,  -10,   -9,   -3,    2,   28,   31,   11,
                  -1,  -20,   -7,    9,   29,   16,    5,   20,
                 -29,    7,   13,   23,   16,   -3,    9,   -2,
                  15,    0,   39,   10,   -4,  -23,    2,    5,
                 -35,   -6,  -17,  -54,  -38,  -21,  -48,  -25,
                 -49,  -71,  -17,  -75,  -66,  -21,  -55,  -55,
        },
        R: {
                 -35,  -18,  -28,  -28,  -22,  -18,  -29,  -49,
                 -55,  -37,  -40,  -34,  -22,  -24,   -2,  -23,
                 -48,  -33,  -37,  -31,  -16,  -14,    4,   -9,
                 -32,  -42,  -43,  -22,   -9,  -45,   -5,  -17,
                 -23,  -18,  -22,  -23,    2,    8,   -2,    7,
                  -3,    7,  -11,   -2,    7,   28,   41,   19,
                  -4,  -26,  -46,   11,   -9,    4,   38,   36,
                  18,   -4,  -16,  -51,   16,   27,   20,   23,
        },
        Q: {
                   2,  -24,  -33,  -13,  -28,  -21,  -33,  -20,
                   9,    0,    8,   -8,    1,   14,    0,   -6,
                   4,   14,  -11,   -6,  -11,    2,   -1,   -1,
                  19,   15,   -2,    2,   10,   -1,   -3,    1,
                  11,   23,   11,   -5,  -17,   -3,   -7,    7,
                  45,   35,   36,   26,    1,  -24,    1,  -12,
                  34,  -11,  -33,  -35,  -37,  -22,  -38,   -4,
                  27,    6,   18,   38,    8,   36,   12,  -37,
        },
        K: {
                   0,    0,    0,    0,   14,   29,   41,   25,
                   0,    0,    0,    0,    9,   23,   33,   31,
                   0,    0,    0,    0,   -7,   -8,    1,  -44,
                   0,    0,    0,    0,  -23,  -15,   -7, -107,
                   0,    0,    0,    0,  -78,  -53,  -52, -110,
                   0,    0,    0,    0,  -21,   11,   23,  -82,
                   0,    0,    0,    0,   10,   63,   68, -114,
                   0,    0,    0,    0,   33,  -99,  119,   21,
        },
}
var egPSQT = [6][64]int{
        P: {
                 -23,  -23,  -23,  -23,  -23,  -23,  -23,  -23,
                   8,  -12,   -2,  -15,    2,   -1,  -25,  -17,
                  -1,  -16,  -13,   -9,   -4,   -4,  -20,   -7,
                   7,   -6,  -12,  -11,  -11,  -13,   -8,   -3,
                  20,    3,   -6,  -14,  -10,  -10,    0,   12,
                  37,   25,   -4,  -21,  -15,    6,   25,   38,
                  61,   63,   48,   24,   41,   63,   91,   81,
                 -23,  -23,  -23,  -23,  -23,  -23,  -23,  -23,
        },
        N: {
                 -21,  -36,   -1,   -3,   -5,  -16,  -21,  -38,
                 -18,  -11,    6,    7,    7,   -1,    1,    8,
                 -10,    7,   13,   31,   27,    8,    9,   -8,
                  -5,   16,   36,   41,   47,   33,   26,    6,
                   4,   21,   32,   44,   42,   30,   26,   -3,
                  -9,   17,   27,   22,   15,   12,   11,  -21,
                 -22,    8,   20,    9,   17,  -16,  -23,  -46,
                 -79,  -33,   -4,    3,  -16,  -33,  -51, -139,
        },
        B: {
                 -16,  -18,  -21,   -8,   -8,   -4,  -18,  -35,
                  -9,  -17,  -18,   -9,   -8,  -19,  -19,  -34,
                  -6,    3,    0,    3,    1,   -8,   -8,   -1,
                  10,   11,   11,    1,   -4,    2,   11,   -6,
                  23,   19,    0,    5,    2,    9,   17,   18,
                  10,   12,    0,   -5,   -1,    8,   15,   13,
                  -2,    2,    4,    5,    9,    2,    4,  -22,
                   8,   13,    1,   23,   21,   15,   18,   -6,
        },
        R: {
                  -6,  -10,   -5,  -14,  -12,   -2,   -3,  -15,
                  -6,   -7,  -11,  -14,  -18,  -20,  -26,  -18,
                  -2,   -3,   -3,   -6,  -13,  -15,  -27,  -23,
                   4,    6,   10,    5,    0,    9,   -7,   -5,
                  11,    9,    6,    5,   -3,   -2,   -4,    3,
                   6,    0,    6,   -1,  -12,  -11,  -20,   -9,
                  13,   19,   22,   14,   21,    7,    0,   -8,
                  24,   25,   27,   24,   23,   26,   15,   18,
        },
        Q: {
                 -32,  -22,   -9,  -22,  -17,  -21,  -47,  -62,
                 -27,  -12,  -23,  -13,  -17,  -55,  -65,  -54,
                   1,  -11,    9,    0,   -4,    6,  -13,  -21,
                   7,   22,    4,   16,    7,   21,   30,   23,
                  18,   -1,    2,   15,   25,   27,   59,   19,
                 -28,  -26,   -1,   -3,   22,   21,   38,    7,
                 -11,   -1,   31,    1,   49,   27,   31,   -8,
                   1,   18,    3,    7,   27,   19,   13,    1,
        },
        K: {
                   0,    0,    0,    0,  -41,  -18,  -40,  -97,
                   0,    0,    0,    0,    9,    5,  -14,  -38,
                   0,    0,    0,    0,   27,   19,    2,  -17,
                   0,    0,    0,    0,   46,   38,   34,    5,
                   0,    0,    0,    0,   59,   55,   45,   12,
                   0,    0,    0,    0,   60,   54,   45,    2,
                   0,    0,    0,    0,   66,   57,   45,    6,
                   0,    0,    0,    0,   17,   21,  -55,  -99,
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
           0,    0,    0,    0,    0,    0,    0,    0,
           4,   21,   -7,    0,    0,   -6,   -7,   -8,
           9,   22,   36,    0,    0,  -11,   -1,   -9,
          10,   17,   -6,    2,    0,   15,   18,   -2,
           8,   12,   10,    0,    2,   -2,  -11,   10,
           0,    0,    0,    0,    0,    0,    0,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
}

var frenchLowP = [64]int{
           0,    0,    0,    0,    0,    0,    0,    0,
          -7,    2,    0,    0,    0,   -5,   15,   14,
          -9,  -12,  -14,    0,   -2,   -1,   -1,   15,
           3,  -14,   18,   -2,    0,    4,   15,    7,
          -4,  -11,   -9,    0,    0,    6,    5,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
}

var KidHighP = [64]int{
           0,    0,    0,    0,    0,    0,    0,    0,
           1,    1,    1,    0,    0,  -13,    4,   10,
          -2,    3,  -11,    0,    0,   13,   15,    3,
           6,   12,    1,    0,   -3,   -7,   10,   12,
          11,    7,    3,   -3,    0,   11,    2,    1,
           0,    0,    0,    0,    0,    0,    0,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
}

var KidLowP = [64]int{
           0,    0,    0,    0,    0,    0,    0,    0,
           3,    8,    6,    0,    0,   -7,   -2,    0,
           3,    3,    1,    3,    0,   -8,    3,   -2,
           6,   10,   -8,    0,   -5,   12,    9,    7,
           8,    7,   -4,    0,    0,   18,   12,   -2,
           0,    0,    0,    0,    0,    0,    0,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
}

var SicHighP = [64]int{
           0,    0,    0,    0,    0,    0,    0,    0,
           7,    3,   -5,    0,   -4,   -7,   -2,   -4,
           0,    7,    1,    0,    2,    8,   -3,   -5,
           6,    3,    2,    0,    3,   -9,    3,    8,
          12,    0,    2,    0,    2,    1,    2,   -3,
           0,    0,    0,    0,    0,    0,    0,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
}

var SicLowP = [64]int{
           0,    0,    0,    0,    0,    0,    0,    0,
          -1,    0,    0,   -2,    4,   -3,    6,    3,
          11,   -6,    0,  -13,   -9,    0,   10,    2,
           3,    7,    0,   -1,   -1,   -3,    6,    0,
           4,    6,    0,   16,    0,    2,    0,   -3,
           0,    0,    0,    0,    0,    0,    0,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
}

var e4e5P = [64]int{
           0,    0,    0,    0,    0,    0,    0,    0,
          11,    5,   13,  -14,  -10,  -14,    5,    3,
           8,    5,   16,    7,   -2,   -2,   12,    2,
           9,    8,   10,  -13,    0,  -18,    7,   10,
          11,   10,   14,   18,    0,    8,   -5,    6,
           0,    0,    0,    0,    0,    0,    0,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
}

var d4d5P = [64]int{
           0,    0,    0,    0,    0,    0,    0,    0,
           2,   12,  -10,   12,   -9,   10,    8,    7,
           4,   11,    2,    3,   -5,   14,    6,    6,
           7,    5,   -9,    0,  -21,    5,   13,    6,
           7,    7,    5,    0,   28,    7,    2,    7,
           0,    0,    0,    0,    0,    0,    0,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
}

var frenchHighN = [64]int{
           0,  -16,    1,   -1,   -1,    0,    3,    1,
          -4,    2,    5,  -16,   -4,   -6,    4,    5,
           2,    0,  -14,    3,    1,    1,   -5,    5,
          -2,   -2,    0,    0,    0,    9,   16,   12,
           0,   -4,    1,    0,    0,    0,   19,    6,
           0,    0,    0,    0,    0,    0,    0,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
}

var frenchLowN = [64]int{
           0,   13,    1,   -1,   -5,  -10,    2,    0,
           6,    0,    2,  -10,    8,   -1,   -3,    0,
           0,    6,   10,    0,    0,    8,   -3,    5,
           6,    4,    1,    0,    0,   -4,   -2,   -8,
           1,   -2,    3,    0,    2,   -4,   -4,    2,
           0,    0,    0,    0,    0,    0,    0,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
}

var KidHighN = [64]int{
           0,   -1,    2,    3,   -1,    3,   -1,    1,
           1,    1,    1,    0,   -6,    5,    2,   -3,
          10,   -3,    1,    5,   -3,  -10,    8,   -5,
           3,    3,    9,   -1,    0,    0,   -1,    0,
           3,    5,    0,    0,    0,   -1,    2,    1,
           0,    0,    0,    0,    0,    0,    0,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
}

var KidLowN = [64]int{
           0,   -5,   -1,   -4,   -6,   -2,    1,    1,
          -1,    0,   -2,    8,  -12,    2,   -1,   -1,
           1,   -7,    7,    0,    1,    3,   -3,    2,
          -2,    0,    3,    0,    0,    0,    0,    4,
          -5,   -3,   -2,    1,    0,   -7,   -4,    4,
           0,    0,    0,    0,    0,    0,    0,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
}

var SicHighN = [64]int{
           0,    4,    1,    1,    1,    2,   -1,    0,
           0,    3,    3,   -5,   -5,    0,    0,    4,
           4,    8,   -1,   -2,    4,   -7,    0,    1,
           3,    2,    7,   -7,    0,    0,    0,   -1,
          -2,    7,    0,    8,   -2,   -5,   -2,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
}

var SicLowN = [64]int{
           0,    5,    0,    2,    5,   -1,   -1,    0,
          -1,   -2,   -2,    8,   -8,   -1,   -2,    1,
           2,    3,    2,    2,    1,    4,    2,   -6,
           5,    0,    3,   -1,    9,    0,    2,   -1,
           0,   -4,   -5,   -6,    0,   -4,    4,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
}

var e4e5N = [64]int{
          -1,    8,   -1,    5,    6,    6,   -2,   -1,
          -5,    2,   -1,    7,  -14,    8,   -3,    2,
          14,   -3,    4,   -6,   10,   -7,    1,   -8,
           4,   -1,    4,   -5,    0,    2,    7,    1,
           0,    6,   -7,   -6,    0,  -12,   -7,   -2,
           0,    0,    0,    0,    0,    0,    0,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
}

var d4d5N = [64]int{
          -2,    6,    1,   -1,   13,    3,   11,   -4,
           0,    4,    4,   -3,    4,    1,    0,    5,
          -1,    6,   -3,   15,   -7,    6,   -5,    6,
           1,    2,   11,    0,   -8,   -1,   12,    2,
          -4,   -7,    1,    0,    0,   -1,    0,    3,
           0,    0,    0,    0,    0,    0,    0,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
}

var frenchHighB = [64]int{
          -3,    3,   -5,   -3,   -5,   -3,   -3,   -2,
          -2,  -19,   14,    3,    2,    6,  -13,  -10,
          -1,    1,   -3,   14,    3,   -2,    1,    4,
           7,    2,    0,    0,    1,    3,    1,    3,
           0,   -2,    2,    0,    0,    0,    7,    6,
           0,    0,    0,    0,    0,    0,    0,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
}

var frenchLowB = [64]int{
          -3,   -2,   -8,   -1,   -3,   12,    0,   -2,
          -3,  -25,   -1,  -11,   -4,   -2,  -19,    5,
         -14,    0,  -18,    4,    0,   -1,    6,   -3,
           4,   -8,    0,    0,    0,   10,    0,    2,
          -8,    2,    2,    0,    1,    1,    5,   -4,
           0,    0,    0,    0,    0,    0,    0,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
}

var KidHighB = [64]int{
          -2,   -1,    3,   -2,   -1,    4,    1,    0,
          -1,    4,   -6,    5,   -4,    4,  -10,   -3,
           9,    5,    4,    3,    5,   -7,    0,   -5,
           0,   10,    1,    0,    0,    0,    0,    2,
           1,   -1,   -1,    0,    0,    1,    6,    1,
           0,    0,    0,    0,    0,    0,    0,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
}

var KidLowB = [64]int{
          -4,   -2,    1,   -2,   -3,  -10,   -1,   -2,
           1,  -18,   -4,    2,   -3,   -3,  -18,   -3,
           0,   -1,    0,    0,    4,  -14,    1,    0,
           1,    4,    3,    0,    0,    0,   -6,    1,
           3,    1,   -1,    2,    0,    4,    7,    4,
           0,    0,    0,    0,    0,    0,    0,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
}

var SicHighB = [64]int{
          -1,   -3,   -1,    0,    0,    4,    0,    0,
           2,    3,   -1,    3,   -9,   -3,   -8,    1,
           4,    2,    1,    8,   -1,   -9,    2,    0,
           0,    5,   -5,  -10,    0,    0,    0,    0,
           1,    2,    1,   13,    1,    0,    3,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
}

var SicLowB = [64]int{
           0,    1,   -4,   -1,   -2,   -6,   -1,   -1,
          -1,    1,   -4,   -1,    8,    0,   -9,   -4,
           1,    2,    3,    0,    1,   -4,   -1,   -3,
           0,    1,   -2,    0,    5,    1,   -5,    0,
           0,    4,    0,   -1,    0,    2,    0,    5,
           0,    0,    0,    0,    0,    0,    0,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
}

var e4e5B = [64]int{
          -1,  -13,    4,   -7,   -5,   -5,    3,   -3,
          -2,    5,   -7,    2,   -4,   -8,  -16,  -10,
           7,   -5,    3,   -4,   -2,  -16,   -5,  -11,
         -12,   12,   -4,   -3,    0,    8,   -4,    5,
           6,    0,   -2,   20,    0,   12,    3,    4,
           0,    0,    0,    0,    0,    0,    0,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
}

var d4d5B = [64]int{
         -17,    3,  -10,   -2,  -15,   -2,   -5,   -4,
          -8,  -16,    2,   -2,   -3,  -12,   -4,   -3,
          -6,   -6,  -22,    1,   -9,    3,   -6,    5,
          10,  -10,    3,    0,   -3,  -12,   10,    0,
         -11,    4,   11,    0,    3,   -2,    1,    8,
           0,    0,    0,    0,    0,    0,    0,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
           0,    0,    0,    0,    0,    0,    0,    0,
}

// per-color tables used to speed up the evaluation
var phalanxMgByColor [2][64]int
var phalanxEgByColor [2][64]int

var pawnAdjust [Undefined][2][64]int
var knightAdjust [Undefined][2][64]int
var bishopAdjust [Undefined][2][64]int

var pstOppositeByColor[2][6][64]int
var pstSameByColor[2][6][64]int
var pstByColor[2][6][64]int
var pstEGByColor[2][6][64]int

// Init

func init() {
    for piece := 0; piece < 6; piece++ {
		for sq := 0; sq < 64; sq++ {
			pstSameByColor[White][piece][sq] = mgPSQTSame[piece][sq]
			pstSameByColor[Black][piece][sq^56] = mgPSQTSame[piece][sq]

      pstOppositeByColor[White][piece][sq] = mgPSQTOpposite[piece][sq]
			pstOppositeByColor[Black][piece][sq^56] = mgPSQTOpposite[piece][sq]

			pstEGByColor[White][piece][sq] = egPSQT[piece][sq]
			pstEGByColor[Black][piece][sq^56] = egPSQT[piece][sq]
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
