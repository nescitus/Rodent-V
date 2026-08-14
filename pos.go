// ================================================================
// S2  BOARD REPRESENTATION
// ================================================================
//
//   Sungorus uses a hybrid representation:
//
//   1. BITBOARDS (colorBB, typeBB)
//      A uint64 per color and per piece-type.  Any intersection
//      gives the set of squares occupied by a specific piece:
//
//          pieceBB(White, R)  =  colorBB[White] & typeBB[R]
//
//      Bitboards make bulk operations (generating all rook moves,
//      counting pawns on a file) cheap and branch-free.
//
//   2. PIECE ARRAY (board[64])
//      One int per square encoding the piece (or NO_PC).  Needed
//      whenever we want "what piece is on sq?" without a loop
//      through bitboards.  make / unmake keep both in sync.
//
//   INCREMENTAL SCORES
//   ------------------
//   material[color] and pstScore[color] are updated as pieces move
//   or are captured so that evaluate() can compute the score in O(1)
//   rather than re-scanning all 64 squares each time.
//
//   ZOBRIST HASH KEY
//   ----------------
//   key is the Zobrist fingerprint of the current position.  It is
//   updated incrementally in makeMove / makeNullMove so that the
//   transposition table lookup is a single array index, not a scan.
//
//   REPETITION DETECTION
//   --------------------
//   keyHist stores the Zobrist key at every position since the last
//   pawn move or capture (the 50-move clock resets histLen at those
//   points).  isRepetition() walks backward in steps of 2 (only
//   same-side positions can repeat) and checks for a key match.
//

package main

import (
	"fmt"
	"strconv"
	"strings"
)

// Pos holds the complete, self-consistent state of a chess position.
// Every field can be derived from the board array alone, but the
// redundant fields are maintained incrementally for speed.
type Pos struct {
	colorBB        [2]uint64   // colorBB[c]: bitboard of all pieces of color c
	typeBB         [6]uint64   // typeBB[t]: bitboard of all pieces of type t
	board          [64]int     // board[sq]: piece on that square, or NO_PC
	kingSq         [2]int      // kingSq[c]: square of the king of color c
	count          [2][6]int   // count[c][t] count of pieces of color c and type t
	side           int         // side to move: White or Black
	castleRights   int         // castling availability: bit0=WK, bit1=WQ, bit2=BK, bit3=BQ
	castleMask     [64]int     // dynamic castling mask for the current position
	castlingRookSq [2][2]int   // castlingRookSq[color][0=kingside, 1=queenside]
	epSquare       int         // en-passant target square, or NO_SQ
	clock          int         // half-move clock for the 50-move rule
	histLen        int         // number of keys stored in keyHist
	key            uint64      // Zobrist hash of the current position
	pawnKey        [2]uint64   // Zobrist hash of pawn position (per side)
	nonPawnKey     [2]uint64   // Zobrist hash of non-pawn pieces (per side)
	majorKey       [2]uint64   // Zobrist hash of king and major pieces (per side)
	minorKey       [2]uint64   // Zobrist hash of king and minor pieces (per side)
	keyHist        [256]uint64 // hash keys since last irreversible move
}

// ---- Position query helpers ----

// occupied returns a bitboard of all occupied squares.
func (p *Pos) occupied() uint64 { return p.colorBB[White] | p.colorBB[Black] }

// empty returns a bitboard of all empty squares.
func (p *Pos) empty() uint64 { return ^p.occupied() }

// pieceBB returns the bitboard of pieces of the given color and type.
func (p *Pos) pieceBB(color, pieceType int) uint64 {
	return p.colorBB[color] & p.typeBB[pieceType]
}

// typeAt returns the piece type on sq (NO_TP if the square is empty).
func (p *Pos) typeAt(sq int) int { return typeOf(p.board[sq]) }

// inCheck reports whether the side to move is currently in check.
func (p *Pos) inCheck() bool {
	return isAttacked(p, p.kingSq[p.side], opp(p.side))
}

// selfInCheck reports whether the side that just moved has left its
// own king in check (i.e. the move was illegal). Called after
// makeMove() before committing to the position.
func (p *Pos) selfInCheck() bool {
	return isAttacked(p, p.kingSq[opp(p.side)], p.side)
}

// canNullMove reports whether a null move (passing the turn) is safe
// to try.  We skip null moves when only pawns and kings remain on the
// moving side, because the zugzwang risk would make the pruning
// unsound in practice.
func (p *Pos) canNullMove() bool {
	return p.colorBB[p.side]&^(p.typeBB[P]|p.typeBB[K]) != 0
}

// isInsufficientMaterial returns true when neither side has enough
// material to force checkmate by any sequence of legal moves.
// Covered cases:
//
//	K vs K
//	K+B vs K  (either side has the bishop)
//	K+N vs K  (either side has the knight)
//
// Two-minor configurations (K+B vs K+B, K+N vs K+N, etc.) are intentionally
// excluded — with one minor per side there are corner-checkmate edge cases
// where a draw cannot be safely claimed.
func (p *Pos) isInsufficientMaterial() bool {
	if p.typeBB[P]|p.typeBB[R]|p.typeBB[Q] != 0 {
		return false
	}
	minors := p.count[White][N] + p.count[White][B] +
		p.count[Black][N] + p.count[Black][B]
	return minors <= 1
}

// ================================================================
// FEN / EPD PARSER
// ================================================================
//
//   parseFEN reads a position string in EPD (or FEN) format:
//
//       "<piece-placement> <side> <castling> <ep-square>"
//
//   Piece placement: ranks 8 down to 1, "/" between ranks, digits
//   for empty runs, letters for pieces (uppercase = White).
//
//   En-passant square: only set if an enemy pawn can actually
//   capture there (avoids phantom EP entries in the hash).
//

func parseFEN(p *Pos, epd string) {
	const pieceChars = "PpNnBbRrQqKk"

	// Clear all incremental scores.
	for c := 0; c < 2; c++ {
		p.colorBB[c] = 0
	}
	for t := 0; t < 6; t++ {
		p.typeBB[t] = 0
		p.count[White][t] = 0
		p.count[Black][t] = 0
	}
	p.castleRights = 0
	p.clock = 0
	p.histLen = 0

	idx := 0 // cursor into epd string

	// --- Piece placement (rank 8 down to rank 1) ---
	for rank := 56; rank >= 0; rank -= 8 {
		file := 0
		for file < 8 {
			ch := epd[idx]
			if ch >= '1' && ch <= '8' {
				// Empty run: fill with NO_PC.
				cnt := int(ch - '0')
				for n := 0; n < cnt; n++ {
					p.board[rank+file] = NO_PC
					file++
				}
			} else {
				// Find the piece in the lookup string.
				pc := 0
				for pc < 12 && pieceChars[pc] != ch {
					pc++
				}
				sq := rank + file
				p.board[sq] = pc
				p.colorBB[colorOf(pc)] ^= squareBit(sq)
				p.typeBB[typeOf(pc)] ^= squareBit(sq)
				if typeOf(pc) == K {
					p.kingSq[colorOf(pc)] = sq
				}
				p.count[colorOf(pc)][typeOf(pc)]++
				file++
			}
			idx++
		}
		idx++ // skip '/' or trailing space
	}

	// --- Side to move ---
	if epd[idx] == 'w' {
		p.side = White
	} else {
		p.side = Black
	}
	idx += 2 // advance past side char and space

	// --- Castling rights ---
	p.castlingRookSq[White][0] = NO_SQ
	p.castlingRookSq[White][1] = NO_SQ
	p.castlingRookSq[Black][0] = NO_SQ
	p.castlingRookSq[Black][1] = NO_SQ

	if epd[idx] == '-' {
		idx++
	} else {
		for idx < len(epd) && epd[idx] != ' ' {
			ch := epd[idx]
			idx++
			switch {
			case ch == 'K':
				p.castleRights |= 1
				for f := fileOf(p.kingSq[White]) + 1; f < 8; f++ {
					sq := makeSquare(f, 0)
					if p.typeAt(sq) == R && colorOf(p.board[sq]) == White {
						p.castlingRookSq[White][0] = sq
						break
					}
				}
			case ch == 'Q':
				p.castleRights |= 2
				for f := fileOf(p.kingSq[White]) - 1; f >= 0; f-- {
					sq := makeSquare(f, 0)
					if p.typeAt(sq) == R && colorOf(p.board[sq]) == White {
						p.castlingRookSq[White][1] = sq
						break
					}
				}
			case ch == 'k':
				p.castleRights |= 4
				for f := fileOf(p.kingSq[Black]) + 1; f < 8; f++ {
					sq := makeSquare(f, 7)
					if p.typeAt(sq) == R && colorOf(p.board[sq]) == Black {
						p.castlingRookSq[Black][0] = sq
						break
					}
				}
			case ch == 'q':
				p.castleRights |= 8
				for f := fileOf(p.kingSq[Black]) - 1; f >= 0; f-- {
					sq := makeSquare(f, 7)
					if p.typeAt(sq) == R && colorOf(p.board[sq]) == Black {
						p.castlingRookSq[Black][1] = sq
						break
					}
				}
			case ch >= 'A' && ch <= 'H':
				f := int(ch - 'A')
				if f > fileOf(p.kingSq[White]) {
					p.castleRights |= 1
					p.castlingRookSq[White][0] = makeSquare(f, 0)
				} else {
					p.castleRights |= 2
					p.castlingRookSq[White][1] = makeSquare(f, 0)
				}
			case ch >= 'a' && ch <= 'h':
				f := int(ch - 'a')
				if f > fileOf(p.kingSq[Black]) {
					p.castleRights |= 4
					p.castlingRookSq[Black][0] = makeSquare(f, 7)
				} else {
					p.castleRights |= 8
					p.castlingRookSq[Black][1] = makeSquare(f, 7)
				}
			}
		}
	}
	if idx < len(epd) && epd[idx] == ' ' {
		idx++ // skip space
	}

	// --- Initialize dynamic castleMask ---
	for i := 0; i < 64; i++ {
		p.castleMask[i] = 15
	}
	p.castleMask[p.kingSq[White]] &^= 3
	p.castleMask[p.kingSq[Black]] &^= 12
	if sq := p.castlingRookSq[White][0]; sq != NO_SQ {
		p.castleMask[sq] &^= 1
	}
	if sq := p.castlingRookSq[White][1]; sq != NO_SQ {
		p.castleMask[sq] &^= 2
	}
	if sq := p.castlingRookSq[Black][0]; sq != NO_SQ {
		p.castleMask[sq] &^= 4
	}
	if sq := p.castlingRookSq[Black][1]; sq != NO_SQ {
		p.castleMask[sq] &^= 8
	}

	// --- En passant square ---
	// Only record it if a pawn of the side to move can actually
	// execute the capture, otherwise the hash entry would be wrong.
	if idx >= len(epd) || epd[idx] == '-' {
		p.epSquare = NO_SQ
	} else {
		epFile := int(epd[idx] - 'a')
		epRank := int(epd[idx+1] - '1')
		sq := makeSquare(epFile, epRank)
		if pawnAtk[opp(p.side)][sq]&p.pieceBB(p.side, P) != 0 {
			p.epSquare = sq
		} else {
			p.epSquare = NO_SQ
		}
	}

	// --- Halfmove clock (field 5 of full FEN, optional) ---
	// Skip to the next space-separated token if present.
	for idx < len(epd) && epd[idx] != ' ' {
		idx++
	}
	if idx < len(epd) {
		idx++ // skip space
		clock := 0
		for idx < len(epd) && epd[idx] >= '0' && epd[idx] <= '9' {
			clock = clock*10 + int(epd[idx]-'0')
			idx++
		}
		p.clock = clock
	}

	p.key = computeZobrist(p)
	p.pawnKey[White] = computePawnKey(p, White)
	p.pawnKey[Black] = computePawnKey(p, Black)
	p.nonPawnKey[White] = computeNonPawnKey(p, White)
	p.nonPawnKey[Black] = computeNonPawnKey(p, Black)
	p.minorKey[White] = computeMinorKey(p, White)
	p.minorKey[Black] = computeMinorKey(p, Black)
	p.majorKey[White] = computeMajorKey(p, White)
	p.majorKey[Black] = computeMajorKey(p, Black)
}

// ================================================================
// ZOBRIST KEY FROM SCRATCH
// ================================================================
//
//   computeZobrist rebuilds the position's Zobrist hash by scanning
//   all 64 squares.  This is called only once per parseFEN(); after
//   that the key is kept up to date incrementally in makeMove().
//

func computeZobrist(p *Pos) uint64 {
	var key uint64
	for sq := 0; sq < 64; sq++ {
		if p.board[sq] != NO_PC {
			key ^= zobPiece[p.board[sq]][sq]
		}
	}
	key ^= zobCastle[p.castleRights]
	if p.epSquare != NO_SQ {
		key ^= zobEP[fileOf(p.epSquare)]
	}
	if p.side == Black {
		key ^= sideKey
	}
	return key
}

// calculates pawn hash key for one side
func computePawnKey(p *Pos, side int) uint64 {
	var key uint64
	for sq := 0; sq < 64; sq++ {
		if p.board[sq] == makePiece(side, P) {
			key ^= zobPiece[p.board[sq]][sq]
		}
	}

	return key
}

// computeNonPawnKey builds the non-pawn Zobrist hash for one color by scanning
// all non-pawn pieces (including kings) of that color.  Called once from parseFEN();
// updated incrementally in makeMove/unmakeMove afterwards.
func computeNonPawnKey(p *Pos, side int) uint64 {
	var key uint64
	for sq := 0; sq < 64; sq++ {
		pc := p.board[sq]
		if pc != NO_PC && colorOf(pc) == side && typeOf(pc) != P {
			key ^= zobPiece[pc][sq]
		}
	}
	return key
}

func computeMinorKey(p *Pos, side int) uint64 {
	var key uint64
	for sq := 0; sq < 64; sq++ {
		pc := p.board[sq]
		if pc != NO_PC && colorOf(pc) == side &&
			(typeOf(pc) == N || typeOf(pc) == B || typeOf(pc) == K) {
			key ^= zobPiece[pc][sq]
		}
	}
	return key
}

func computeMajorKey(p *Pos, side int) uint64 {
	var key uint64
	for sq := 0; sq < 64; sq++ {
		pc := p.board[sq]
		if pc != NO_PC && colorOf(pc) == side &&
			(typeOf(pc) == R || typeOf(pc) == Q || typeOf(pc) == K) {
			key ^= zobPiece[pc][sq]
		}
	}
	return key
}

// returns full pawn-king hash key
func getPawnKey(p *Pos) uint64 {
	return p.pawnKey[White] ^ p.pawnKey[Black] ^
		zobPiece[makePiece(White, K)][p.kingSq[White]] ^
		zobPiece[makePiece(Black, K)][p.kingSq[Black]]
}

// is certain piece on this square?
func isOnSq(p *Pos, side, piece, sq int) bool {
	return (squareBit(sq) & (p.colorBB[side] & p.typeBB[piece])) != 0
}

// --- Printing the board ---

const (
	ansiReset       = "\x1b[0m"
	ansiWhitePieces = "\x1b[97m"
	ansiBlackPieces = "\x1b[96m" // cyan
)

var pieceColored = []string{
	ansiWhitePieces + "  ♙ " + ansiReset, // white pawn
	ansiBlackPieces + "  ♙ " + ansiReset, // black pawn, same glyph recolored

	ansiWhitePieces + "  ♘ " + ansiReset,
	ansiBlackPieces + "  ♘ " + ansiReset,

	ansiWhitePieces + "  ♗ " + ansiReset,
	ansiBlackPieces + "  ♗ " + ansiReset,

	ansiWhitePieces + "  ♖ " + ansiReset,
	ansiBlackPieces + "  ♖ " + ansiReset,

	ansiWhitePieces + "  ♕ " + ansiReset,
	ansiBlackPieces + "  ♕ " + ansiReset,

	ansiWhitePieces + "  ♔ " + ansiReset,
	ansiBlackPieces + "  ♔ " + ansiReset,

	"  . ",
}

func PrintBoard(p *Pos) {
	fmt.Println("--------------------------------------------")
	fmt.Print("  ")

	for sq := 0; sq < 64; sq++ {
		mappedSq := sq ^ 56
		fmt.Print(pieceColored[p.board[mappedSq]])

		if (sq+1)%8 == 0 {
			fmt.Printf("   %d\n", 8-sq/8)
			fmt.Println()
			if sq != 63 {
				fmt.Print("  ")
			}
		}
	}

	fmt.Println()
	fmt.Println("    a   b   c   d   e   f   g   h")
	fmt.Println()

	fmt.Println("--------------------------------------------")
}

func PrintBoardAscii(p *Pos) {
	pieceName := []string{
		"P ", "p ",
		"N ", "n ",
		"B ", "b ",
		"R ", "r ",
		"Q ", "q ",
		"K ", "k ",
		". ",
	}

	fmt.Println("--------------------------------------------")
	fmt.Print("  ")

	for sq := 0; sq < 64; sq++ {
		mappedSq := sq ^ 56
		fmt.Print(pieceName[p.board[mappedSq]])

		if (sq+1)%8 == 0 {
			fmt.Printf("  %d\n", 8-sq/8)
			if sq != 63 {
				fmt.Print("  ")
			}
		}
	}

	fmt.Println()
	fmt.Println("  a b c d e f g h")
	fmt.Println()

	fmt.Printf("White counts: P=%d N=%d B=%d R=%d Q=%d K=%d\n",
		p.count[White][P],
		p.count[White][N],
		p.count[White][B],
		p.count[White][R],
		p.count[White][Q],
		p.count[White][K],
	)

	fmt.Printf("Black counts: P=%d N=%d B=%d R=%d Q=%d K=%d\n",
		p.count[Black][P],
		p.count[Black][N],
		p.count[Black][B],
		p.count[Black][R],
		p.count[Black][Q],
		p.count[Black][K],
	)

	fmt.Println("--------------------------------------------")
}

// generateFen converts the position into a standard FEN string.
func (p *Pos) generateFen() string {
	var sb strings.Builder
	for rank := 7; rank >= 0; rank-- {
		empty := 0
		for file := 0; file < 8; file++ {
			sq := rank*8 + file
			pc := p.board[sq]
			if pc == NO_PC {
				empty++
			} else {
				if empty > 0 {
					sb.WriteByte(byte('0' + empty))
					empty = 0
				}
				const pieceChars = "PpNnBbRrQqKk"
				sb.WriteByte(pieceChars[pc])
			}
		}
		if empty > 0 {
			sb.WriteByte(byte('0' + empty))
		}
		if rank > 0 {
			sb.WriteByte('/')
		}
	}

	sb.WriteByte(' ')
	if p.side == White {
		sb.WriteByte('w')
	} else {
		sb.WriteByte('b')
	}

	sb.WriteByte(' ')
	if p.castleRights == 0 {
		sb.WriteByte('-')
	} else {
		if (p.castleRights & 1) != 0 {
			sb.WriteByte('K')
		}
		if (p.castleRights & 2) != 0 {
			sb.WriteByte('Q')
		}
		if (p.castleRights & 4) != 0 {
			sb.WriteByte('k')
		}
		if (p.castleRights & 8) != 0 {
			sb.WriteByte('q')
		}
	}

	sb.WriteByte(' ')
	if p.epSquare == NO_SQ {
		sb.WriteByte('-')
	} else {
		f := fileOf(p.epSquare)
		r := rankOf(p.epSquare)
		sb.WriteByte(byte('a' + f))
		sb.WriteByte(byte('1' + r))
	}

	sb.WriteByte(' ')
	sb.WriteString(strconv.Itoa(p.clock))
	sb.WriteByte(' ')
	sb.WriteString("1") // fullmove clock

	return sb.String()
}
