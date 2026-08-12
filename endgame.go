package main

var squaresOfColor = [2]uint64{0x55AA55AA55AA55AA, 0xAA55AA55AA55AA55}

var BN_lightCorner = [64]int{
	0, 0, 15, 30, 45, 60, 85, 100,
	0, 15, 30, 45, 60, 85, 100, 85,
	15, 30, 45, 60, 85, 100, 85, 60,
	30, 45, 60, 85, 100, 85, 60, 45,
	45, 60, 85, 100, 85, 60, 45, 30,
	60, 85, 100, 85, 60, 45, 30, 15,
	85, 100, 85, 60, 45, 30, 15, 0,
	100, 85, 60, 45, 30, 15, 0, 0,
}

var BN_darkCorner = [64]int{
	100, 85, 60, 45, 30, 15, 0, 0,
	85, 100, 85, 60, 45, 30, 15, 0,
	60, 85, 100, 85, 60, 45, 30, 15,
	45, 60, 85, 100, 85, 60, 45, 30,
	30, 45, 60, 85, 100, 85, 60, 45,
	15, 30, 45, 60, 85, 100, 85, 60,
	0, 15, 30, 45, 60, 85, 100, 85,
	0, 0, 15, 30, 45, 60, 85, 100,
}

var centralize = [64]int{
	0, 10, 20, 30, 30, 20, 10, 0,
	10, 20, 30, 40, 40, 30, 20, 10,
	20, 30, 40, 50, 50, 40, 30, 20,
	30, 40, 50, 60, 60, 50, 40, 30,
	30, 40, 50, 60, 60, 50, 40, 30,
	20, 30, 40, 50, 50, 40, 30, 20,
	10, 20, 30, 40, 40, 30, 20, 10,
	0, 10, 20, 30, 30, 20, 10, 0,
}

// getDrawishness returns what percentage of normal evaluation will be
// returned. Note that "strong" refers to te side that is known to be
// ahead, so if for example KB va Kpp is already better for the side
// with pawns, the value won't be brought down.
func getDrawishness(p *Pos, strong, weak int) int {

	if p.count[strong][P] == 0 {

		// K vs K, KB vs K, KN v K, KB vs KP(P), KN vs KP(P)
		if atMostMinor(p, strong) {
			return 0
		}

		// KR vs KB(P), KR vs KN(P)
		if justRook(p, strong) && justMinor(p, weak) {
			return 25
		}

		// KRB vs KR(P), KRN vs KR(P)
		if justRookMinor(p, strong) && justRook(p, weak) {
			return 25
		}

		// KNN vs K: theoretical draw (KNN vs KP can be a win, so require bare king)
		if p.count[strong][N] == 2 && p.count[strong][B]+p.count[strong][R]+p.count[strong][Q] == 0 &&
			justKing(p, weak) && p.count[weak][P] == 0 {
			return 0
		}

	} // no pawns for the stronger side

	// Bishops of opposite colors
	if bishopEndgame(p) && differentBishops(p) {
		if p.count[strong][P]-p.count[weak][P] < 3 {
			return 50
		}
	}

	return 100
}

// --- Facilitating checkmate ---

func checkmateHelper(p *Pos, e *EvalData) int {

	// TODO: make color-agnostic
	result := 0

	// KQ vs Kx: drive enemy king towards the edge using own king

	if p.count[White][Q] > 0 && p.count[White][P] == 0 && justNotQueen(p, Black) {
		return kingDistanceScore(p, Black)
	}

	if p.count[Black][Q] > 0 && p.count[Black][P] == 0 && justNotQueen(p, White) {
		return -kingDistanceScore(p, White)
	}

	// Weaker side has bare king (KQK, KRK, KBBK + bigger advantage

	if p.count[Black][P] == 0 && justKing(p, Black) && easyCheckmate(p, White) {
		result += kingDistanceScore(p, Black)
	}

	if p.count[White][P] == 0 && justKing(p, White) && easyCheckmate(p, Black) {
		result -= kingDistanceScore(p, White)
	}

	// KBN vs K specialized code

	if p.count[White][P] == 0 && p.count[Black][P] == 0 && e.phase == 2 {

		if justBN(p, White) && justKing(p, Black) { // mate with white bishop and knight
			if p.pieceBB(White, B)&squaresOfColor[White] != 0 {
				result -= 2 * BN_darkCorner[p.kingSq[Black]]
			}
			if p.pieceBB(White, B)&squaresOfColor[Black] != 0 {
				result -= 2 * BN_lightCorner[p.kingSq[Black]]
			}
		}

		if justBN(p, Black) && justKing(p, White) { // mate with black bishop and knight
			if p.pieceBB(Black, B)&squaresOfColor[White] != 0 {
				result += 2 * BN_darkCorner[p.kingSq[White]]
			}
			if p.pieceBB(Black, B)&squaresOfColor[Black] != 0 {
				result += 2 * BN_lightCorner[p.kingSq[White]]
			}
		}
	}

	return result
}

// --- Helper functions ---

func kingDistanceScore(p *Pos, weak int) int {
	return 200 + 10*proximityBonus(p.kingSq[White], p.kingSq[Black]) - 2*centralize[p.kingSq[weak]]
}

// proximityBonus awards bonus for squares that are near
func proximityBonus(s1, s2 int) int {
	r_delta := abs(rankOf(s1) - rankOf(s2))
	f_delta := abs(fileOf(s1) - fileOf(s2))
	return 14 - r_delta - f_delta
}

// stronger side has queen or rook or two bishops
func easyCheckmate(p *Pos, side int) bool {
	return p.count[side][Q]+p.count[side][R] > 0 || p.count[side][B] > 1
}

func justNotQueen(p *Pos, side int) bool {
	return (p.count[side][Q] == 0) &&
		(p.count[side][P] == 0 && p.count[side][R]+p.count[side][B]+p.count[side][N] <= 1)
}

func differentBishops(p *Pos) bool {

	if (squaresOfColor[White]&p.pieceBB(White, B) != 0) &&
		(squaresOfColor[Black]&p.pieceBB(Black, B) != 0) {
		return true
	}

	if (squaresOfColor[Black]&p.pieceBB(White, B) != 0) &&
		(squaresOfColor[White]&p.pieceBB(Black, B) != 0) {
		return true
	}

	return false
}

// bishopEndgame detects bishop endings, with or without pawns
func bishopEndgame(p *Pos) bool {
	return justBishop(p, White) && justBishop(p, Black)
}

// rookEndgame detects rook endings, with or withour pawns
func rookEndgame(p *Pos) bool {
	return justRook(p, White) && justRook(p, Black)
}

// a couple of functions detecting piece configurations (pawns excluded)
func justKing(p *Pos, side int) bool {
	return p.count[side][N]+p.count[side][B]+p.count[side][R]+p.count[side][Q] == 0
}

func justKnight(p *Pos, side int) bool {
	return p.count[side][N] == 1 &&
		p.count[side][B]+p.count[side][R]+p.count[side][Q] == 0
}

func justBishop(p *Pos, side int) bool {
	return p.count[side][B] == 1 &&
		p.count[side][N]+p.count[side][R]+p.count[side][Q] == 0
}

func justBN(p *Pos, side int) bool {
	return p.count[side][B] == 1 &&
		p.count[side][N] == 1 &&
		p.count[side][R]+p.count[side][Q] == 0
}

func justTwoBishops(p *Pos, side int) bool {
	return p.count[side][B] == 2 &&
		p.count[side][N]+p.count[side][R]+p.count[side][Q] == 0
}

func justMinor(p *Pos, side int) bool {
	return p.count[side][N]+p.count[side][B] == 1 &&
		p.count[side][R]+p.count[side][Q] == 0
}

func justRook(p *Pos, side int) bool {
	return p.count[side][R] == 1 &&
		p.count[side][N]+p.count[side][B]+p.count[side][Q] == 0
}

func justRookMinor(p *Pos, side int) bool {
	return p.count[side][R] == 1 &&
		p.count[side][N]+p.count[side][B] == 1 &&
		p.count[side][Q] == 0
}

func justQueenMinor(p *Pos, side int) bool {
	return p.count[side][Q] == 1 &&
		p.count[side][N]+p.count[side][B] == 1 &&
		p.count[side][R] == 0
}

func justQueen(p *Pos, side int) bool {
	return p.count[side][Q] == 1 &&
		p.count[side][N]+p.count[side][B]+p.count[side][R] == 0
}

func atMostMinor(p *Pos, side int) bool {
	return p.count[side][N]+p.count[side][B] < 2 &&
		p.count[side][R]+p.count[side][Q] == 0
}

func atMostRook(p *Pos, side int) bool {
	return p.count[side][N]+p.count[side][B]+p.count[side][R] < 2 && p.count[side][Q] == 0
}

func isKBNEndgame(p *Pos) bool {
	return p.count[White][P] == 0 && p.count[Black][P] == 0 &&
		((justBN(p, White) && justKing(p, Black)) ||
			(justBN(p, Black) && justKing(p, White)))
}
