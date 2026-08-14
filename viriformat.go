package main

import (
	"encoding/binary"
	"math/bits"
)

// ViriBuffer holds the viriformat data for a single game.
type ViriBuffer struct {
	buf []byte
}

func (vb *ViriBuffer) Reset() {
	if cap(vb.buf) < 32+512*4+4 {
		vb.buf = make([]byte, 0, 32+512*4+4)
	} else {
		vb.buf = vb.buf[:0]
	}
}

// WriteBoard packs the starting board state into 32 bytes and appends it to the buffer.
// The WDL byte (index 30) is left as 0 and must be patched at the end of the game using PatchWDL.
func (vb *ViriBuffer) WriteBoard(p *Pos, halfmoveClock int, fullmoveCounter int) {
	var pb [32]byte

	occ := p.colorBB[White] | p.colorBB[Black]
	binary.LittleEndian.PutUint64(pb[0:8], occ)

	idx := 0
	for occ != 0 {
		sq := bits.TrailingZeros64(occ)
		occ &= occ - 1

		pc := p.board[sq]
		color := colorOf(pc)
		typ := typeOf(pc)

		// Check castling rights for unmoved rooks
		if typ == R {
			if sq == p.castlingRookSq[White][0] && (p.castleRights&1) != 0 {
				typ = 6
			} else if sq == p.castlingRookSq[White][1] && (p.castleRights&2) != 0 {
				typ = 6
			} else if sq == p.castlingRookSq[Black][0] && (p.castleRights&4) != 0 {
				typ = 6
			} else if sq == p.castlingRookSq[Black][1] && (p.castleRights&8) != 0 {
				typ = 6
			}
		}

		nibble := byte(typ)
		if color == Black {
			nibble |= 8
		}

		byteIdx := 8 + (idx / 2)
		if idx%2 == 1 {
			pb[byteIdx] |= (nibble << 4)
		} else {
			pb[byteIdx] = nibble
		}
		idx++
	}

	ep := p.epSquare
	if ep == NO_SQ {
		ep = 64
	}
	pb[24] = byte(ep & 0x7F)
	if p.side == Black {
		pb[24] |= 0x80
	}

	pb[25] = byte(halfmoveClock)
	binary.LittleEndian.PutUint16(pb[26:28], uint16(fullmoveCounter))
	// 28-29: score (0)
	// 30: wdl (patched later)
	// 31: extra (0)

	vb.buf = append(vb.buf, pb[:]...)
}

// WriteMoveEval appends a 4-byte record (2-byte move, 2-byte eval).
func (vb *ViriBuffer) WriteMoveEval(move int, eval int) {
	var b [4]byte

	fr := moveFrom(move)
	to := moveTo(move)
	mType := moveType(move)

	var vType uint16
	var vPromo uint16

	if mType == EP_CAP {
		vType = 1
	} else if mType == CASTLE {
		vType = 2
		if fr < 32 { // White
			if to > fr {
				to = H1
			} else {
				to = A1
			}
		} else { // Black
			if to > fr {
				to = H8
			} else {
				to = A8
			}
		}
	} else if mType >= N_PROM {
		vType = 3
		vPromo = uint16(mType - N_PROM)
	}

	vMove := uint16(fr) | (uint16(to) << 6) | (vPromo << 12) | (vType << 14)

	binary.LittleEndian.PutUint16(b[0:2], vMove)
	binary.LittleEndian.PutUint16(b[2:4], uint16(int16(eval)))

	vb.buf = append(vb.buf, b[:]...)
}

// PatchWDL writes the WDL result into the 32-byte header and appends the 4-byte zero terminator.
// wdl: 0 = Black win, 1 = Draw, 2 = White win
func (vb *ViriBuffer) PatchWDL(wdl int) {
	if len(vb.buf) >= 32 {
		vb.buf[30] = byte(wdl)
	}
	// Append terminator
	vb.buf = append(vb.buf, 0, 0, 0, 0)
}
