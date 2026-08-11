// ================================================================
// BITBOARD HELPERS
// ================================================================
//

package main

import "fmt"

const (
	excludeA uint64 = 0xfefefefefefefefe
	excludeH uint64 = 0x7f7f7f7f7f7f7f7f
)

// Shift helpers. White moves north, i.e. toward higher ranks.

func shiftNorth(b uint64) uint64 { return b << 8 }
func shiftSouth(b uint64) uint64 { return b >> 8 }
func shiftWest(b uint64) uint64  { return (b & excludeA) >> 1 }
func shiftEast(b uint64) uint64  { return (b & excludeH) << 1 }
func shiftNW(b uint64) uint64    { return (b & excludeA) << 7 }
func shiftNE(b uint64) uint64    { return (b & excludeH) << 9 }
func shiftSW(b uint64) uint64    { return (b & excludeA) >> 9 }
func shiftSE(b uint64) uint64    { return (b & excludeH) >> 7 }

// Side-dependent forward shift.
func shiftForward(b uint64, side int) uint64 {
	if side == White {
		return shiftNorth(b)
	}
	return shiftSouth(b)
}

// Shift to both sides.
func shiftSides(b uint64) uint64 {
	return shiftWest(b) | shiftEast(b)
}

// White pawn attacks.
func shiftWPAttacks(b uint64) uint64 {
	return shiftNE(b) | shiftNW(b)
}

// Black pawn attacks.
func shiftBPAttacks(b uint64) uint64 {
	return shiftSE(b) | shiftSW(b)
}

// White pawn double attacks.
func doubleWPAttacks(b uint64) uint64 {
	return shiftNE(b) & shiftNW(b)
}

// Black pawn double attacks.
func doubleBPAttacks(b uint64) uint64 {
	return shiftSE(b) & shiftSW(b)
}

func fillNorth(b uint64) uint64 {
	b |= b << 8
	b |= b << 16
	b |= b << 32
	return b
}

func fillSouth(b uint64) uint64 {
	b |= b >> 8
	b |= b >> 16
	b |= b >> 32
	return b
}

func fillWest(b uint64) uint64 {
	pr0 := excludeH
	pr1 := pr0 & (pr0 >> 1)
	pr2 := pr1 & (pr1 >> 2)
	b |= pr0 & (b >> 1)
	b |= pr1 & (b >> 2)
	b |= pr2 & (b >> 4)
	return b
}

func fillEast(b uint64) uint64 {
	pr0 := excludeA
	pr1 := pr0 & (pr0 << 1)
	pr2 := pr1 & (pr1 << 2)
	b |= pr0 & (b << 1)
	b |= pr1 & (b << 2)
	b |= pr2 & (b << 4)
	return b
}

func fillSideways(b uint64) uint64 {
	return fillEast(shiftEast(b)) | fillWest(shiftWest(b))
}

// color-dependent forward fill
func fillForward(b uint64, color int) uint64 {

	if color == White {
		return fillNorth(shiftNorth(b))
	}
	return fillSouth(shiftSouth(b))
}

// color-dependent backward fill
func fillBackward(b uint64, color int) uint64 {

	if color == Black {
		return fillNorth(shiftNorth(b))
	}
	return fillSouth(shiftSouth(b))
}

func PrintBitboard(bb uint64) {
	fmt.Println("---------------------------------")
	for rank := 7; rank >= 0; rank-- {
		fmt.Print("  ")
		for file := 0; file < 8; file++ {
			sq := rank*8 + file
			if bb&squareBit(sq) != 0 {
				fmt.Print("1 ")
			} else {
				fmt.Print(". ")
			}
		}
		fmt.Printf(" %d\n", rank+1)
	}
	fmt.Println()
	fmt.Println("  a b c d e f g h")
	fmt.Println("---------------------------------")
}
