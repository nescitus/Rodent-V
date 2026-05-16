// ================================================================
// S1  CONSTANTS & TYPES
// ================================================================
//
//   COLOR ENCODING
//   --------------
//   White = 0, Black = 1.  Flipping the color is a single XOR:
//
//       opp(White) = White ^ 1 = 1 = Black
//       opp(Black) = Black ^ 1 = 0 = White
//
//   PIECE TYPE ENCODING
//   -------------------
//   Six types, numbered 0-5. NO_TP (6) means "empty square".
//
//   PIECE ENCODING
//   --------------
//   One int per piece: piece = (type << 1) | color.
//   So White Pawn = 0, Black Pawn = 1, White Knight = 2, ...
//
//       colorOf(piece) = piece & 1
//       typeOf(piece)  = piece >> 1
//       makePiece(c,t) = (t << 1) | c
//
//   MOVE ENCODING
//   -------------
//   One int per move, packed into three fields:
//
//       bits  5.. 0  = from square   (0-63)
//       bits 11.. 6  = to   square   (0-63)
//       bits 15..12  = move type     (see below)
//
//   TRANSPOSITION TABLE FLAGS
//   -------------------------
//   UPPER: score is an upper bound (failed low; all moves tried,
//          none improved alpha).
//   LOWER: score is a lower bound (failed high; a move caused a
//          beta cutoff, real score may be higher).
//   EXACT: exact minimax score (a PV node).
//

package main

import "math/bits"

// Colors
const (
	White   = 0
	Black   = 1
	NoColor = 2
)

// Piece types
const (
	P     = 0 // Pawn
	N     = 1 // Knight
	B     = 2 // Bishop
	R     = 3 // Rook
	Q     = 4 // Queen
	K     = 5 // King
	NO_TP = 6 // empty square sentinel
)

// Pieces: makePiece(color, type) = (type << 1) | color
const (
	WP    = 0
	BP    = 1
	WN    = 2
	BN    = 3
	WB    = 4
	BB    = 5
	WR    = 6
	BR    = 7
	WQ    = 8
	BQ    = 9
	WK    = 10
	BK    = 11
	NO_PC = 12 // empty square sentinel
)

// Move types (stored in bits 15..12 of the move integer)
const (
	NORMAL = 0 // standard move or capture
	CASTLE = 1 // king castles kingside or queenside
	EP_CAP = 2 // en passant capture
	EP_SET = 3 // double pawn push (sets en passant square)
	N_PROM = 4 // promotion to knight
	B_PROM = 5 // promotion to bishop
	R_PROM = 6 // promotion to rook
	Q_PROM = 7 // promotion to queen
)

// Transposition table bound flags
const (
	UPPER = 1 // score <= alpha (upper bound)
	LOWER = 2 // score >= beta  (lower bound / cut node)
	EXACT = 3 // exact score    (PV node)
)

// Squares: A1=0, B1=1, ... H8=63.
// Files run A-H (0-7), ranks run 1-8 (0-7).
// square = rank*8 + file.
const (
	A1, B1, C1, D1, E1, F1, G1, H1 = 0, 1, 2, 3, 4, 5, 6, 7
	A2, B2, C2, D2, E2, F2, G2, H2 = 8, 9, 10, 11, 12, 13, 14, 15
	A3, B3, C3, D3, E3, F3, G3, H3 = 16, 17, 18, 19, 20, 21, 22, 23
	A4, B4, C4, D4, E4, F4, G4, H4 = 24, 25, 26, 27, 28, 29, 30, 31
	A5, B5, C5, D5, E5, F5, G5, H5 = 32, 33, 34, 35, 36, 37, 38, 39
	A6, B6, C6, D6, E6, F6, G6, H6 = 40, 41, 42, 43, 44, 45, 46, 47
	A7, B7, C7, D7, E7, F7, G7, H7 = 48, 49, 50, 51, 52, 53, 54, 55
	A8, B8, C8, D8, E8, F8, G8, H8 = 56, 57, 58, 59, 60, 61, 62, 63
	NO_SQ                          = 64 // sentinel for "no square"
)

// Search and engine limits.
const (
	maxPly       = 64    // maximum search depth
	maxMoves     = 256   // maximum legal moves in any position
	inf          = 32767 // larger than any real score; used as +/-inf
	mate         = 32000 // base value of checkmate score
	maxEval      = 29999 // largest returned by evaluate(); mate scores exceed this
	useRFP       = true
	rfpDepth     = 6        // was 8
	rfpMargin    = 80       // centipawns per depth for reverse futility pruning (not improving)
	rfpImpMargin = 60       // centipawns per depth for reverse futility pruning (improving)
	noEval       = -inf - 1 // sentinel: no static eval stored for this ply (in check)

	useFutility = false
	fpMargin    = 120 // centipawns per depth for main-search move-loop futility pruning
	fpMaxDepth  = 4   // only prune quiet moves by futility at shallow depth

	useNULL           = true
	nmpBaseReduction  = 2 // base ply reduction for null-move pruning
	nmpDepthReduction = 6 // depth divisor for depth-scaled NMP reduction

	useIIR      = false
	IIRmaxDepth = 4

	useProbcut       = false
	probcutMargin    = 120 // extra margin above beta for ProbCut verification
	probcutMinDepth  = 6   // only apply ProbCut when enough depth remains
	probcutReduction = 2   // depth reduction used by the reduced verification search

	useSingularExt = false
	useDoubleExt   = false
	seBetaMargin   = 6  // centipawns per ply subtracted from ttScore for singular verification
	seDoubleMargin = 64 // extra centipawns below singular beta needed for a double extension

	useRazoring   = true
	maxRazorDepth = 3
	razorMargin   = 300 // centipawns per depth for razoring

	qsFpPawnMargin  = 300 // qs futility margin when capturing a pawn (passers warrant extra slack)
	qsFpPieceMargin = 200 // qs futility margin when capturing a piece
	qsLmpLimit      = 2   // max captures tried per qs node (outside check) to cap explosion

	useLMP           = true
	LMPnormalStep    = 6 // TODO: test 4, as was previously
	LMPimprovingStep = 6

	useLMR = true
)

// TEST POSITION 6k1/ppp2p1p/6p1/3p4/3n4/4B2P/P1P1rPP1/3n2K1 b - - 1 20 ???

// TEST POSITION r2q1rk1/1b3p1p/p3pPp1/2ppP3/7R/1PN1B1R1/1PP2P1P/4K3 w - - 1 3
//
// pure ab-pvs: info depth 12 seldepth 32 time 292345 nodes 465282130 nps 1591551 hashfull 1000 score mate 8 pv h4h7 g8h7 g3h3 h7g8 e3h6 d8c7 f2f4 c7a5 h6g7 a5a1 e1d2 a1d1 c3d1 f8e8 h3h8
// only null m: info depth 21 seldepth 48 time 2297254 nodes 3418081729 nps 1487898 hashfull 1000 score cp -601 pv e3c5 d5d4 h4d4 d8a5 c5d6 f8d8 g3d3 b7c6 h2h3 h7h5 d3d2 a8c8 d2d1 a5b6 d1d3 b6b7 e1d2 a6a5 d2c1 c6f3 c1b1
// null + lmr : info depth 19 seldepth 39 time 28132 nodes 42555060 nps 1512692 hashfull 1000 score mate 8 pv h4h7 g8h7 g3h3 h7g8 e3h6 d8c7 f2f4 c7a5 h6g7 a5a1 e1d2 a1c1 d2c1 a8c8 h3h8
// changed par: info depth 22 seldepth 46 time 67631 nodes 109972506 nps 1626066 hashfull 1000 score mate 8 pv h4h7 g8h7 g3h3 h7g8 e3h6 d
// lmp :(     : info depth 25 seldepth 55 time 426787 nodes 629666120 nps 1475363 hashfull 1000 score mate 8 pv h4h7 g8h7 g3h3 h7g8 e3h6 d8c7 f2f4 c7a5 h6g7 a5a1 e1e2 a1f1 e2f1 c5c4 h3h8
// sing ext   : info depth 23 seldepth 57 time 245612 nodes 383432323 nps 1561130 hashfull 1000 score mate 8 pv h4h7 g8h7 g3h3 h7g8 e3h6

// startFEN is the standard opening position in FEN notation.
// sideKey is XORed into the Zobrist hash whenever Black is to move.
const (
	startFEN = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq -"
	sideKey  = ^uint64(0) // all-ones constant, XORed for side-to-move
)

// ================================================================
// BITBOARD MASKS
// ================================================================
//
//   A bitboard is a uint64 where bit N is set if square N is
//   occupied.  File A is the LSB column; rank 1 is the LSB row.
//
//   These constants cover full ranks, files, and the two long
//   diagonals needed to initialise the sliding-attack tables.
//
const (
	rank1BB = uint64(0x00000000000000FF)
	rank2BB = uint64(0x000000000000FF00)
	rank3BB = uint64(0x0000000000FF0000)
	rank4BB = uint64(0x00000000FF000000)
	rank5BB = uint64(0x000000FF00000000)
	rank6BB = uint64(0x0000FF0000000000)
	rank7BB = uint64(0x00FF000000000000)
	rank8BB = uint64(0xFF00000000000000)

	fileABB = uint64(0x0101010101010101)
	fileBBB = uint64(0x0202020202020202)
	fileCBB = uint64(0x0404040404040404)
	fileDBB = uint64(0x0808080808080808)
	fileEBB = uint64(0x1010101010101010)
	fileFBB = uint64(0x2020202020202020)
	fileGBB = uint64(0x4040404040404040)
	fileHBB = uint64(0x8080808080808080)

	diagA1H8BB = uint64(0x8040201008040201) // A1-H8 main diagonal
	diagA8H1BB = uint64(0x0102040810204080) // A8-H1 anti-diagonal
	diagB8H2BB = uint64(0x0204081020408000) // B8-H2 (used in file-index trick)
)

// ================================================================
// PRECOMPUTED ATTACK TABLES
// ================================================================
//
//   All attack tables are filled once at startup by initTables().
//   After that they are read-only, so no synchronisation is needed.
//
//   lineMasks[dir][sq]: bitboard of all squares on the rank/file/
//                       diagonal through sq. dir: 0=rank, 1=file,
//                       2=diag (A1->H8), 3=anti (A8->H1).
//
//   pawnAtk[color][sq]: squares a pawn of the given color attacks
//                       from sq.
//
//   knightAtk[sq], kingAtk[sq]: pre-computed leap attacks.
//
//   passedMask[color][sq]: bitboard of squares in front of sq (on
//                       the same and adjacent files) that must be
//                       free of enemy pawns for sq's pawn to be
//                       "passed."
//
//   adjFileMask[file]: bitboard of the two files adjacent to file
//                      (used to detect isolated pawns).
//
//   pstTable[type][sq]: piece-square table bonus in centipawns.
//
//   castleMask[sq]: AND-mask applied to castling rights when a
//                   piece on sq moves (or is captured).
//
var (
	lineMasks  [4][64]uint64
	pawnAtk    [2][64]uint64
	knightAtk  [64]uint64
	kingAtk    [64]uint64
	castleMask [64]int
)

// Zobrist keys: random 64-bit values XORed together to form a
// position fingerprint (the "hash key").  Two positions with the
// same pieces, castling rights, and en-passant state are guaranteed
// to share the same key (with overwhelming probability).
//
//   zobPiece[piece][sq]: piece placed on a square
//   zobCastle[flags]: castling-rights nibble (4 bits -> 16 combos)
//   zobEP[file]: en-passant file (0-7)
//
var (
	zobPiece  [12][64]uint64
	zobCastle [16]uint64
	zobEP     [8]uint64
)

// pieceValue[type]: centipawn material value of each piece type.
// King has value 0 because material balance only matters for the
// five capturable piece types.
var pieceValue = [7]int{100, 325, 325, 500, 1000, 0, 0}

// ================================================================
// BIT MANIPULATION HELPERS
// ================================================================
//
//   squareBit(sq): the uint64 with only bit sq set, equivalent
//                  to (1 << sq). Used to test/toggle single
//                  squares in bitboards.
//
//   colorOf(piece): 0 (White) or 1 (Black); the low bit of the
//                   piece encoding.
//   typeOf(piece): 0-5 (P..K); piece >> 1.
//   makePiece(c,t): (t<<1)|c; inverse of the above two.
//
//   fileOf(sq): 0-7, the file of a square.
//   rankOf(sq): 0-7, the rank of a square.
//   makeSquare(f,r): (r<<3)|f; file+rank -> square index.
//
//   opp(color): flips White/Black with XOR.
//
//   lsb(bb): index of the lowest set bit (trailing zeros).
//   popCount(bb): number of set bits (piece count).
//

func squareBit(sq int) uint64 { return uint64(1) << uint(sq) }

func colorOf(piece int) int  { return piece & 1 }
func typeOf(piece int) int   { return piece >> 1 }
func makePiece(c, t int) int { return (t << 1) | c }

func fileOf(sq int) int       { return sq & 7 }
func rankOf(sq int) int       { return sq >> 3 }
func makeSquare(f, r int) int { return (r << 3) | f }

// chebyshev returns the Chebyshev (king-move) distance between two squares.
func chebyshev(a, b int) int {
	df := fileOf(a) - fileOf(b)
	dr := rankOf(a) - rankOf(b)
	if df < 0 {
		df = -df
	}
	if dr < 0 {
		dr = -dr
	}
	if df > dr {
		return df
	}
	return dr
}

func opp(color int) int { return color ^ 1 }

func lsb(bb uint64) int      { return bits.TrailingZeros64(bb) }
func popCount(bb uint64) int { return bits.OnesCount64(bb) }

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func maxOf(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ================================================================
// MOVE DECODING HELPERS
// ================================================================
//
//   A move is a single int with three packed fields:
//
//       [ moveType (4) | toSq (6) | fromSq (6) ]
//        bits 15..12     bits 11..6   bits 5..0
//
//   moveFrom / moveTo extract the square fields.
//   moveType extracts bits 15..12.
//   isProm is true for any of the four promotion types.
//   promType maps the move-type back to the promoted piece type
//   (N_PROM=4 -> N=1, B_PROM=5 -> B=2, R_PROM=6 -> R=3, Q_PROM=7 -> Q=4).
//

func moveFrom(m int) int { return m & 63 }
func moveTo(m int) int   { return (m >> 6) & 63 }
func moveType(m int) int { return m >> 12 }
func isProm(m int) bool  { return m&0x4000 != 0 }
func promType(m int) int { return (m >> 12) - 3 }

// ================================================================
// 0x88 BOARD MAPPING
// ================================================================
//
//   The classical "0x88" trick represents an 8x8 board as a 16x8
//   array.  A square is off-board if and only if (index & 0x88) != 0.
//   This lets direction arithmetic detect board edges without
//   per-direction bounds checks.
//
//   to0x88(sq): map a 0-63 index to the 0x88 space (0-127).
//   from0x88(sq): inverse mapping.
//   offBoard(sq): true when sq falls outside the 8x8 grid.
//

func to0x88(sq int) int   { return (sq & 7) | ((sq & ^7) << 1) }
func from0x88(sq int) int { return (sq & 7) | ((sq & ^7) >> 1) }
func offBoard(sq int) int { return int(uint(sq) & 0x88) }

// Sliding piece attack functions (rookAttacks, bishopAttacks, queenAttacks)
// are now in magics.go, implemented via magic bitboard lookups.

// ================================================================
// ZOBRIST RANDOM NUMBER GENERATOR
// ================================================================
//
//   Sungorus uses a simple linear-congruential PRNG seeded at 1 to
//   produce the Zobrist keys.  The seed matches the original C engine
//   so that the same keys are generated, preserving hash table
//   compatibility if you ever compare outputs.
//

var randState uint64 = 1

func nextRand() uint64 {
	randState = randState*1103515245 + 12345
	return randState
}

// ================================================================
// initTables: PRECOMPUTE ALL LOOKUP TABLES
// ================================================================
//
//   Called once at engine startup (before any search or FEN parsing).
//   Order matters: lineMasks must be ready before slidingAtk is built.
//
//   SLIDING ATTACK TABLES
//   ---------------------
//   For each direction d in {rank, file, diag, anti}, each square sq,
//   and each 6-bit occupancy index k, we walk the ray in both
//   directions and stop after the first occupied square.
//
//   The "blocker" position within the line is stored in k: for ranks
//   and diagonals it's the file; for files it's the rank.
//
//   CASTLE RIGHTS MASK
//   ------------------
//   castleMask[sq] starts at 15 (all four rights intact).  The four
//   corner and king squares have bits cleared so that any move from
//   or to those squares automatically strips the relevant rights via
//   a bitwise AND
//

func initTables() {
	pMoves := [2][2]int{{15, 17}, {-17, -15}} // pawn attack offsets (0x88)
	nMoves := [8]int{-33, -31, -18, -14, 14, 18, 31, 33}
	kMoves := [8]int{-17, -16, -15, -1, 1, 15, 16, 17}

	// --- Line masks ---
	for i := 0; i < 64; i++ {
		lineMasks[0][i] = rank1BB << uint(i&56)
		lineMasks[1][i] = fileABB << uint(i&7)
		j := fileOf(i) - rankOf(i)
		if j > 0 {
			lineMasks[2][i] = diagA1H8BB >> uint(j*8)
		} else {
			lineMasks[2][i] = diagA1H8BB << uint(-j*8)
		}
		j = fileOf(i) - (7 - rankOf(i))
		if j > 0 {
			lineMasks[3][i] = diagA8H1BB << uint(j*8)
		} else {
			lineMasks[3][i] = diagA8H1BB >> uint(-j*8)
		}
	}

	// --- Magic bitboard tables ---
	initMagics()

	// --- Pawn attacks ---
	for c := 0; c < 2; c++ {
		for sq := 0; sq < 64; sq++ {
			pawnAtk[c][sq] = 0
			for k := 0; k < 2; k++ {
				x := to0x88(sq) + pMoves[c][k]
				if offBoard(x) == 0 {
					pawnAtk[c][sq] |= squareBit(from0x88(x))
				}
			}
		}
	}

	// --- Knight attacks ---
	for sq := 0; sq < 64; sq++ {
		knightAtk[sq] = 0
		for _, delta := range nMoves {
			x := to0x88(sq) + delta
			if offBoard(x) == 0 {
				knightAtk[sq] |= squareBit(from0x88(x))
			}
		}
	}

	// --- King attacks ---
	for sq := 0; sq < 64; sq++ {
		kingAtk[sq] = 0
		for _, delta := range kMoves {
			x := to0x88(sq) + delta
			if offBoard(x) == 0 {
				kingAtk[sq] |= squareBit(from0x88(x))
			}
		}
	}

	// --- Castling-rights strip mask ---
	// Moving from or to any of the four rook/king home squares clears
	// the corresponding castling-right bit.  All other squares leave
	// all four rights intact (mask = 15 = 0b1111).
	for sq := 0; sq < 64; sq++ {
		castleMask[sq] = 15
	}
	castleMask[A1] = 13 // clears White queenside (bit 1)
	castleMask[E1] = 12 // clears both White rights (bits 0 and 1)
	castleMask[H1] = 14 // clears White kingside (bit 0)
	castleMask[A8] = 7  // clears Black queenside (bit 3)
	castleMask[E8] = 3  // clears both Black rights (bits 2 and 3)
	castleMask[H8] = 11 // clears Black kingside (bit 2)

	// --- Zobrist keys ---
	for i := 0; i < 12; i++ {
		for j := 0; j < 64; j++ {
			zobPiece[i][j] = nextRand()
		}
	}
	for i := 0; i < 16; i++ {
		zobCastle[i] = nextRand()
	}
	for i := 0; i < 8; i++ {
		zobEP[i] = nextRand()
	}
}
