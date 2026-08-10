package main

/*

Rodent supports networks trained by bullet simple.rs.
The net architecture is 768 -> (N)x2 -> 1, allowing
some variance of N.

Code is optimized and tries to run SIMD version if possible;
if not, it defaults to much slower scalar code. State of the
network is kept in the "accumulator", kept on accStack
within the search state. Updates are following copy-make
principle: old accumulator is copied to the child node
and modified there; trying a new move requires copying
the accumulator yet again. Within the parent node, accumulator
update is delayed as much as possible, and not performed
on illegal or pruned moves.

 build command for AVX2 version
 set GOAMD64=v3
 go build

// If you want to build a portable version, avoid setting GOAMD64=v3
   If you miss cpu detection, run: go get golang.org/x/sys/cpu
*/

import (
	_ "embed"
	"os"
	"unsafe"

	"golang.org/x/sys/cpu"
)

//go:embed nets/rodent_hm_512hl_4.bin
var embeddedNet []byte

// NNUE size and scale. AVX2 code supports following net sizes:
// 64, 128, 256, 384, 512
const (
	NNUEInputSize  = 768
	NNUEHiddenSize = 512
	NNUEL0Scale    = 255
	NNUEL1Scale    = 64
)

// Types of NNUE updates
type AccUpdateKind int

const (
	uNORMAL AccUpdateKind = iota
	uCASTLE
	uEP_CAP
	uEP_SET
	uCAPTURE
	uPROMO
	uPROMCAPT
)

// Params for NNUE evaluation
type NNUEParameters struct {
	InputWeights  [NNUEInputSize][NNUEHiddenSize]int16
	InputBiases   [NNUEHiddenSize]int16
	OutputWeights [2][NNUEHiddenSize]int16
	OutputBias    int16
}

var nnueParams = &NNUEParameters{}
var nnue NNUEState

type Accumulator struct {
	values [2][NNUEHiddenSize]int16
}

// Update contains data for, well, updating nnue accumulator.
// Data are stored on a stack, created in makeMove() and used
// to postpone accumulator update *within a single node*.
// In practice it means that we can avoid the update when
// a move is illegal (leaves us in check) or if it is pruned.
type Update struct {

	// Data used by NNUE accumulator.
	dirty      bool
	color      int
	flag       AccUpdateKind
	from       int
	to         int
	capSq      int
	movingType int
	captType   int
	prom       int
	rookFrom   int // for castling
	rookTo     int
}

// The accumulator itself now belongs to searchState.
// This state contains only information shared by the engine.
type NNUEState struct {
	Loaded bool
}

// Interface to cater for assembly code for various net sizes.

type addSingleFunc func(
	a, w *int16,
)

type captureFunc func(
	a0, a1 *int16,
	wTo0, wFrom0, wCap0 *int16,
	wTo1, wFrom1, wCap1 *int16,
)

type moveFunc func(
	a0, a1 *int16,
	wFrom0, wTo0 *int16,
	wFrom1, wTo1 *int16,
)

type castleFunc func(
	a0, a1 *int16,
	wKFrom0, wKTo0, wRFrom0, wRTo0 *int16,
	wKFrom1, wKTo1, wRFrom1, wRTo1 *int16,
)

type evalFunc func(
	a0, a1 *int16,
	w0, w1 *int16,
	sum *int32,
)

var addSingleFunction addSingleFunc
var captureFunction captureFunc
var moveFunction moveFunc
var castleFunction castleFunc
var evalFunction evalFunc

// init picks the correct assembly routines for the configured NNUE size.
func init() {
	// Safe fallback.
	addSingleFunction = addSingleScalar
	moveFunction = moveScalar
	captureFunction = captureScalar
	castleFunction = castleScalar
	evalFunction = evalScalar

	if !cpu.X86.HasAVX2 {
		return
	}

	switch NNUEHiddenSize {
	case 64:
		moveFunction = moveAVX2_64
		captureFunction = captureAVX2_64
		castleFunction = castleAVX2_64
		evalFunction = getEvalAVX2_64

	case 128:
		moveFunction = moveAVX2_128
		captureFunction = captureAVX2_128
		castleFunction = castleAVX2_128
		evalFunction = getEvalAVX2_128

	case 256:
		moveFunction = moveAVX2_256
		captureFunction = captureAVX2_256
		castleFunction = castleAVX2_256
		evalFunction = getEvalAVX2_256

	case 384:
		moveFunction = moveAVX2_384
		captureFunction = captureAVX2_384
		castleFunction = castleAVX2_384
		evalFunction = getEvalAVX2_384

	case 512:
		moveFunction = moveAVX2_512
		captureFunction = captureAVX2_512
		castleFunction = castleAVX2_512
		evalFunction = getEvalAVX2_512
		addSingleFunction = addSingleAVX2_512

	default:
		panic("unsupported NNUE hidden size")
	}
}

var zeroWeights [NNUEHiddenSize]int16

func featureIndex(color, pt, sq, kingSq, perspective int) int {
	idxSq := sq
	if perspective == 1 {
		idxSq ^= 56
	}
	if singleOptionValue[HorizontalMirroring] == 1 {
		if kingSq%8 > 3 {
			idxSq ^= 7
		}
	}
	return (color^perspective)*384 + pt*64 + idxSq
}

// Clear = empty-board state = biases only.
func (acc *Accumulator) clear() {
	a0 := &acc.values[0]
	a1 := &acc.values[1]
	biases := &nnueParams.InputBiases

	for i := 0; i < NNUEHiddenSize; i++ {
		a0[i] = biases[i]
		a1[i] = biases[i]
	}
}

// Copy the accumulator
func (acc *Accumulator) copyFrom(src *Accumulator) {
	*acc = *src
}

// Add one feature: piece(color,type) on sq.
func (acc *Accumulator) addPiece(color, pt, sq, k0, k1 int) {
	idx0 := featureIndex(color, pt, sq, k0, 0)
	idx1 := featureIndex(color, pt, sq, k1, 1)

	a0 := &acc.values[0]
	a1 := &acc.values[1]

	w0 := &nnueParams.InputWeights[idx0]
	w1 := &nnueParams.InputWeights[idx1]

	for i := 0; i < NNUEHiddenSize; i++ {
		a0[i] += w0[i]
		a1[i] += w1[i]
	}
}

// Move one piece without a capture. Loops are expensive,
// so we use one instead of separate addition/deletion loops.
func (acc *Accumulator) move(color, pt, from, to, k0, k1 int, refresh0, refresh1 bool) {
	var pFrom0, pTo0, pFrom1, pTo1 *int16

	if refresh0 {
		pFrom0 = &zeroWeights[0]
		pTo0 = &zeroWeights[0]
	} else {
		pFrom0 = &nnueParams.InputWeights[featureIndex(color, pt, from, k0, 0)][0]
		pTo0 = &nnueParams.InputWeights[featureIndex(color, pt, to, k0, 0)][0]
	}

	if refresh1 {
		pFrom1 = &zeroWeights[0]
		pTo1 = &zeroWeights[0]
	} else {
		pFrom1 = &nnueParams.InputWeights[featureIndex(color, pt, from, k1, 1)][0]
		pTo1 = &nnueParams.InputWeights[featureIndex(color, pt, to, k1, 1)][0]
	}

	moveFunction(
		&acc.values[0][0],
		&acc.values[1][0],

		pFrom0,
		pTo0,

		pFrom1,
		pTo1,
	)

}

func (acc *Accumulator) capture(
	moverColor, moverPT, from, to int,
	capturedColor, capturedPT, capturedSq int,
	k0, k1 int, refresh0, refresh1 bool,
) {
	var pTo0, pFrom0, pCap0, pTo1, pFrom1, pCap1 *int16

	if refresh0 {
		pTo0 = &zeroWeights[0]
		pFrom0 = &zeroWeights[0]
		pCap0 = &zeroWeights[0]
	} else {
		pTo0 = &nnueParams.InputWeights[featureIndex(moverColor, moverPT, to, k0, 0)][0]
		pFrom0 = &nnueParams.InputWeights[featureIndex(moverColor, moverPT, from, k0, 0)][0]
		pCap0 = &nnueParams.InputWeights[featureIndex(capturedColor, capturedPT, capturedSq, k0, 0)][0]
	}

	if refresh1 {
		pTo1 = &zeroWeights[0]
		pFrom1 = &zeroWeights[0]
		pCap1 = &zeroWeights[0]
	} else {
		pTo1 = &nnueParams.InputWeights[featureIndex(moverColor, moverPT, to, k1, 1)][0]
		pFrom1 = &nnueParams.InputWeights[featureIndex(moverColor, moverPT, from, k1, 1)][0]
		pCap1 = &nnueParams.InputWeights[featureIndex(capturedColor, capturedPT, capturedSq, k1, 1)][0]
	}

	captureFunction(
		&acc.values[0][0],
		&acc.values[1][0],

		pTo0,
		pFrom0,
		pCap0,

		pTo1,
		pFrom1,
		pCap1,
	)

}

func (acc *Accumulator) castle(
	color, kingFrom, kingTo, rookFrom, rookTo int,
	k0, k1 int, refresh0, refresh1 bool,
) {
	var pKTo0, pKFrom0, pRTo0, pRFrom0, pKTo1, pKFrom1, pRTo1, pRFrom1 *int16

	if refresh0 {
		pKTo0 = &zeroWeights[0]
		pKFrom0 = &zeroWeights[0]
		pRTo0 = &zeroWeights[0]
		pRFrom0 = &zeroWeights[0]
	} else {
		pKTo0 = &nnueParams.InputWeights[featureIndex(color, K, kingTo, k0, 0)][0]
		pKFrom0 = &nnueParams.InputWeights[featureIndex(color, K, kingFrom, k0, 0)][0]
		pRTo0 = &nnueParams.InputWeights[featureIndex(color, R, rookTo, k0, 0)][0]
		pRFrom0 = &nnueParams.InputWeights[featureIndex(color, R, rookFrom, k0, 0)][0]
	}

	if refresh1 {
		pKTo1 = &zeroWeights[0]
		pKFrom1 = &zeroWeights[0]
		pRTo1 = &zeroWeights[0]
		pRFrom1 = &zeroWeights[0]
	} else {
		pKTo1 = &nnueParams.InputWeights[featureIndex(color, K, kingTo, k1, 1)][0]
		pKFrom1 = &nnueParams.InputWeights[featureIndex(color, K, kingFrom, k1, 1)][0]
		pRTo1 = &nnueParams.InputWeights[featureIndex(color, R, rookTo, k1, 1)][0]
		pRFrom1 = &nnueParams.InputWeights[featureIndex(color, R, rookFrom, k1, 1)][0]
	}

	castleFunction(
		&acc.values[0][0],
		&acc.values[1][0],

		pKFrom0,
		pKTo0,
		pRFrom0,
		pRTo0,

		pKFrom1,
		pKTo1,
		pRFrom1,
		pRTo1,
	)
}

func (acc *Accumulator) promotion(color, from, to, prom int, k0, k1 int, refresh0, refresh1 bool) {
	var pFrom0, pTo0, pFrom1, pTo1 *int16

	if refresh0 {
		pFrom0 = &zeroWeights[0]
		pTo0 = &zeroWeights[0]
	} else {
		pFrom0 = &nnueParams.InputWeights[featureIndex(color, P, from, k0, 0)][0]
		pTo0 = &nnueParams.InputWeights[featureIndex(color, prom, to, k0, 0)][0]
	}

	if refresh1 {
		pFrom1 = &zeroWeights[0]
		pTo1 = &zeroWeights[0]
	} else {
		pFrom1 = &nnueParams.InputWeights[featureIndex(color, P, from, k1, 1)][0]
		pTo1 = &nnueParams.InputWeights[featureIndex(color, prom, to, k1, 1)][0]
	}

	moveFunction(
		&acc.values[0][0],
		&acc.values[1][0],

		pFrom0,
		pTo0,

		pFrom1,
		pTo1,
	)
}

func (acc *Accumulator) promotionCapture(color, from, to, prom, captType int, k0, k1 int, refresh0, refresh1 bool) {
	enemy := color ^ 1
	var pTo0, pFrom0, pCap0, pTo1, pFrom1, pCap1 *int16

	if refresh0 {
		pTo0 = &zeroWeights[0]
		pFrom0 = &zeroWeights[0]
		pCap0 = &zeroWeights[0]
	} else {
		pTo0 = &nnueParams.InputWeights[featureIndex(color, prom, to, k0, 0)][0]
		pFrom0 = &nnueParams.InputWeights[featureIndex(color, P, from, k0, 0)][0]
		pCap0 = &nnueParams.InputWeights[featureIndex(enemy, captType, to, k0, 0)][0]
	}

	if refresh1 {
		pTo1 = &zeroWeights[0]
		pFrom1 = &zeroWeights[0]
		pCap1 = &zeroWeights[0]
	} else {
		pTo1 = &nnueParams.InputWeights[featureIndex(color, prom, to, k1, 1)][0]
		pFrom1 = &nnueParams.InputWeights[featureIndex(color, P, from, k1, 1)][0]
		pCap1 = &nnueParams.InputWeights[featureIndex(enemy, captType, to, k1, 1)][0]
	}

	captureFunction(
		&acc.values[0][0],
		&acc.values[1][0],

		pTo0,
		pFrom0,
		pCap0,

		pTo1,
		pFrom1,
		pCap1,
	)
}

// apply full nnue accumulator update
func (acc *Accumulator) applyPendingChanges(p *Pos, u *Update) {

	// already applied
	if !u.dirty {
		return
	}

	k0 := p.kingSq[White]
	k1 := p.kingSq[Black]
	refresh0 := false
	refresh1 := false

	if singleOptionValue[HorizontalMirroring] == 1 && u.movingType == K {
		if u.color == White {
			refresh0 = (u.from%8 > 3) != (u.to%8 > 3)
		} else {
			refresh1 = (u.from%8 > 3) != (u.to%8 > 3)
		}
	}

	switch u.flag {
	case uNORMAL, uEP_SET:
		acc.move(u.color, u.movingType, u.from, u.to, k0, k1, refresh0, refresh1)

	case uCAPTURE:
		acc.capture(u.color, u.movingType, u.from, u.to, u.color^1, u.captType, u.capSq, k0, k1, refresh0, refresh1)

	case uEP_CAP:
		acc.capture(u.color, P, u.from, u.to, u.color^1, P, u.capSq, k0, k1, refresh0, refresh1)

	case uCASTLE:
		acc.castle(u.color, u.from, u.to, u.rookFrom, u.rookTo, k0, k1, refresh0, refresh1)

	case uPROMO:
		acc.promotion(u.color, u.from, u.to, u.prom, k0, k1, refresh0, refresh1)

	case uPROMCAPT:
		acc.promotionCapture(u.color, u.from, u.to, u.prom, u.captType, k0, k1, refresh0, refresh1)

	}

	if refresh0 {
		refreshPerspective(p, acc, 0)
	}
	if refresh1 {
		refreshPerspective(p, acc, 1)
	}

	u.dirty = false
}

func refreshPerspective(p *Pos, acc *Accumulator, perspective int) {
	kSq := p.kingSq[perspective]

	a := &acc.values[perspective][0]

	// Initialize accumulator from biases.
	copy(
		acc.values[perspective][:NNUEHiddenSize],
		nnueParams.InputBiases[:NNUEHiddenSize],
	)

	for sq := 0; sq < 64; sq++ {
		piece := p.board[sq]
		if piece == NO_PC {
			continue
		}

		color := colorOf(piece)
		pt := typeOf(piece)

		idx := featureIndex(color, pt, sq, kSq, perspective)
		w := &nnueParams.InputWeights[idx][0]

		addSingleFunction(a, w)
	}
}

// Rebuild the accumulator from the current board.
func refresh(p *Pos, acc *Accumulator) {
	acc.clear()

	k0 := p.kingSq[White]
	k1 := p.kingSq[Black]

	for sq := 0; sq < 64; sq++ {
		piece := p.board[sq]
		if piece == NO_PC {
			continue
		}

		color := colorOf(piece)
		pt := typeOf(piece)
		acc.addPiece(color, pt, sq, k0, k1)
	}
}

// Squared clipped ReLu
func screluWeighted(x, w int16) int32 {
	v := int32(x)

	if v < 0 {
		v = 0
	} else if v > NNUEL0Scale {
		v = NNUEL0Scale
	}

	return v * v * int32(w)
}

func (acc *Accumulator) getEval(stm int) int {
	var sum int32

	evalFunction(
		&acc.values[stm][0],
		&acc.values[stm^1][0],
		&nnueParams.OutputWeights[0][0],
		&nnueParams.OutputWeights[1][0],
		&sum,
	)

	sum = sum/NNUEL0Scale + int32(nnueParams.OutputBias)

	return int(sum * int32(singleOptionValue[NnueScale]) /
		(NNUEL0Scale * NNUEL1Scale))
}

func nnueInitEmbedded() bool {
	need := int(unsafe.Sizeof(NNUEParameters{}))
	if len(embeddedNet) < need {
		return false
	}

	base := unsafe.Pointer(&embeddedNet[0])
	if uintptr(base)%unsafe.Alignof(NNUEParameters{}) != 0 {
		panic("embedded net is misaligned")
	}

	nnueParams = (*NNUEParameters)(base)
	nnue.Loaded = true
	return true
}

// Load a raw Bullet-compatible parameter blob.
func nnueLoad(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		nnue.Loaded = false
		return false
	}

	offset := 0

	nnueParams = new(NNUEParameters)

	readI16 := func() int16 {
		value := int16(
			uint16(data[offset]) |
				uint16(data[offset+1])<<8,
		)

		offset += 2
		return value
	}

	for input := 0; input < NNUEInputSize; input++ {
		for neuron := 0; neuron < NNUEHiddenSize; neuron++ {
			nnueParams.InputWeights[input][neuron] = readI16()
		}
	}

	for neuron := 0; neuron < NNUEHiddenSize; neuron++ {
		nnueParams.InputBiases[neuron] = readI16()
	}

	for neuron := 0; neuron < NNUEHiddenSize; neuron++ {
		nnueParams.OutputWeights[0][neuron] = readI16()
	}

	for neuron := 0; neuron < NNUEHiddenSize; neuron++ {
		nnueParams.OutputWeights[1][neuron] = readI16()
	}

	nnueParams.OutputBias = readI16()
	nnue.Loaded = true

	return true
}
