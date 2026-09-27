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
	"math"
	"math/bits"
	"os"
	"unsafe"

	"golang.org/x/sys/cpu"
)

//go:embed nets/rodent_hm_1536hl_8ob.bin
var embeddedNet []byte

// NNUE size and scale. AVX2 code supports following net sizes:
// 64, 128, 256, 384, 512, 768, 1024 and 1536
const (
	NNUEInputBuckets   = 1
	NNUEInputSize      = 768
	TotalInputFeatures = NNUEInputBuckets * NNUEInputSize
	NNUEHiddenSize     = 1536
	OutputBuckets      = 8
	NNUEL0Scale        = 255
	NNUEL1Scale        = 64

	// Minimum parameter byte sizes for 1-bucket, 4-bucket, and 8-bucket network blobs (2 bytes per int16)
	SingleBucketNetSize      = (NNUEInputSize*NNUEHiddenSize + NNUEHiddenSize + 2*NNUEHiddenSize + 1) * 2
	OutputBucketNetSize      = (NNUEInputSize*NNUEHiddenSize + NNUEHiddenSize + OutputBuckets*2*NNUEHiddenSize + OutputBuckets) * 2
	FourBucketSingleNetSize  = (4*NNUEInputSize*NNUEHiddenSize + NNUEHiddenSize + 2*NNUEHiddenSize + 1) * 2
	FourBucketOutputNetSize  = (4*NNUEInputSize*NNUEHiddenSize + NNUEHiddenSize + OutputBuckets*2*NNUEHiddenSize + OutputBuckets) * 2
	EightBucketSingleNetSize = (TotalInputFeatures*NNUEHiddenSize + NNUEHiddenSize + 2*NNUEHiddenSize + 1) * 2
	EightBucketOutputNetSize = (TotalInputFeatures*NNUEHiddenSize + NNUEHiddenSize + OutputBuckets*2*NNUEHiddenSize + OutputBuckets) * 2

	// Multilayer Dense Head constants ((16 -> 32 -> 1)x8)
	NNUEMultilayerL1Size   = 16
	NNUEMultilayerL2Size   = 32
	NNUEMultilayerL2Inputs = 32 // 16 CReLU + 16 SCReLU when Dual Activation is active

	// Base multilayer (pure SCReLU, 16 inputs to L2): 84,512 dense head bytes -> 6,376,992 bytes
	MultilayerDenseHeadBaseSize = (NNUEHiddenSize * OutputBuckets * NNUEMultilayerL1Size * 1) +
		(OutputBuckets * NNUEMultilayerL1Size * 4) +
		(NNUEMultilayerL1Size * OutputBuckets * NNUEMultilayerL2Size * 4) +
		(OutputBuckets * NNUEMultilayerL2Size * 4) +
		(NNUEMultilayerL2Size * OutputBuckets * 4) +
		(OutputBuckets * 4)
	MultilayerNetSize = (TotalInputFeatures*NNUEHiddenSize+NNUEHiddenSize)*2 + MultilayerDenseHeadBaseSize

	// Multilayer with Dual Activation (32 inputs to L2): 100,896 dense head bytes -> 6,393,376 bytes
	MultilayerDenseHeadDualActSize = (NNUEHiddenSize * OutputBuckets * NNUEMultilayerL1Size * 1) +
		(OutputBuckets * NNUEMultilayerL1Size * 4) +
		(2 * NNUEMultilayerL1Size * OutputBuckets * NNUEMultilayerL2Size * 4) +
		(OutputBuckets * NNUEMultilayerL2Size * 4) +
		(NNUEMultilayerL2Size * OutputBuckets * 4) +
		(OutputBuckets * 4)
	MultilayerDualActNetSize = (TotalInputFeatures*NNUEHiddenSize+NNUEHiddenSize)*2 + MultilayerDenseHeadDualActSize
)

// 8 King Input Buckets Layout (Horizontally Mirrored across 64 squares)
// Bucket 0: Corner King on Rank 1 (a1, h1)
// Bucket 1: Castled King on Rank 1 (b1, g1)
// Bucket 2: Queenside Castled / Stepped King on Rank 1 (c1, f1)
// Bucket 3: Central Uncastled King on Rank 1 (d1, e1)
// Bucket 4: Flank / Fianchetto King on Rank 2 (a2, b2, g2, h2)
// Bucket 5: Central King on Rank 2 (c2, d2, e2, f2)
// Bucket 6: Midfield Active King (Ranks 3-4)
// Bucket 7: Enemy Territory (Ranks 5-8)
var kingBucketTable = [64]int{
	0, 1, 2, 3, 3, 2, 1, 0, // Rank 1: a1..h1 (0..7)
	4, 4, 5, 5, 5, 5, 4, 4, // Rank 2: a2..h2 (8..15)
	6, 6, 6, 6, 6, 6, 6, 6, // Rank 3: a3..h3 (16..23)
	6, 6, 6, 6, 6, 6, 6, 6, // Rank 4: a4..h4 (24..31)
	7, 7, 7, 7, 7, 7, 7, 7, // Rank 5: a5..h5 (32..39)
	7, 7, 7, 7, 7, 7, 7, 7, // Rank 6: a6..h6 (40..47)
	7, 7, 7, 7, 7, 7, 7, 7, // Rank 7: a7..h7 (48..55)
	7, 7, 7, 7, 7, 7, 7, 7, // Rank 8: a8..h8 (56..63)
}

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
	InputWeights  [TotalInputFeatures][NNUEHiddenSize]int16
	InputBiases   [NNUEHiddenSize]int16
	OutputWeights [OutputBuckets][2][NNUEHiddenSize]int16
	OutputBiases  [OutputBuckets]int16

	// Multilayer Dense Head
	IsMultilayer     bool
	IsDualActivation bool
	L1WeightsRaw     [OutputBuckets][NNUEHiddenSize][NNUEMultilayerL1Size]int8
	L1WeightsTiled   [OutputBuckets][NNUEHiddenSize / 4][2][32]int8
	L1Weights        [OutputBuckets][NNUEHiddenSize][NNUEMultilayerL1Size]float32
	L1Biases         [OutputBuckets][NNUEMultilayerL1Size]float32
	L2Weights        [OutputBuckets][NNUEMultilayerL2Inputs][NNUEMultilayerL2Size]float32
	L2Biases         [OutputBuckets][NNUEMultilayerL2Size]float32
	L3Weights        [OutputBuckets][NNUEMultilayerL2Size]float32
	L3Biases         [OutputBuckets]float32
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
	Loaded     bool
	generation uint64
}

// Dispatch to assembly code for various net sizes lives in the nnueX
// wrapper functions below (nnueAddSingle, nnueMove, nnueEval, ...).
// hasAVX2 is resolved once at startup; every nnueX wrapper branches on
// it directly instead of going through a package-level func variable.
//
// The previous design stored the chosen kernel in six func-var globals
// (addSingleFunction, moveFunction, evalFunction, ...) selected once in
// init(). That makes every accumulator update and every evaluation an
// indirect call, which defeats escape analysis: the compiler cannot see
// through a call to an unknown function value to confirm the callee's
// //go:noescape pragma applies, so e.g. `sum` in (*Accumulator).getEval
// was heap-allocated on every single evaluation. Calling concrete
// functions here (through a compile-time constant switch on
// NNUEHiddenSize, so only one case per operation is ever live) lets the
// compiler verify none of the pointer arguments escape, at zero cost to
// the existing multi-size flexibility.
var hasAVX2 bool

func init() {
	hasAVX2 = cpu.X86.HasAVX2
}

func nnueAddSingle(a, w *int16) {
	if !hasAVX2 {
		addSingleScalar(a, w)
		return
	}
	switch NNUEHiddenSize {
	case 64:
		addSingleAVX2_64(a, w)
	case 128:
		addSingleAVX2_128(a, w)
	case 256:
		addSingleAVX2_256(a, w)
	case 384:
		addSingleAVX2_384(a, w)
	case 512:
		addSingleAVX2_512(a, w)
	case 768:
		addSingleAVX2_768(a, w)
	case 1024:
		addSingleAVX2_1024(a, w)
	case 1536:
		addSingleAVX2_1536(a, w)		
	default:
		panic("unsupported NNUE hidden size")
	}
}

func nnueSubSingle(a, w *int16) {
	if !hasAVX2 {
		subSingleScalar(a, w)
		return
	}
	switch NNUEHiddenSize {
	case 64:
		subSingleAVX2_64(a, w)
	case 128:
		subSingleAVX2_128(a, w)
	case 256:
		subSingleAVX2_256(a, w)
	case 384:
		subSingleAVX2_384(a, w)
	case 512:
		subSingleAVX2_512(a, w)
	case 768:
		subSingleAVX2_768(a, w)
	case 1024:
		subSingleAVX2_1024(a, w)
	case 1536:
		subSingleAVX2_1536(a, w)
	default:
		panic("unsupported NNUE hidden size")
	}
}

func nnueMove(a0, a1 *int16, wFrom0, wTo0, wFrom1, wTo1 *int16) {
	if !hasAVX2 {
		moveScalar(a0, a1, wFrom0, wTo0, wFrom1, wTo1)
		return
	}
	switch NNUEHiddenSize {
	case 64:
		moveAVX2_64(a0, a1, wFrom0, wTo0, wFrom1, wTo1)
	case 128:
		moveAVX2_128(a0, a1, wFrom0, wTo0, wFrom1, wTo1)
	case 256:
		moveAVX2_256(a0, a1, wFrom0, wTo0, wFrom1, wTo1)
	case 384:
		moveAVX2_384(a0, a1, wFrom0, wTo0, wFrom1, wTo1)
	case 512:
		moveAVX2_512(a0, a1, wFrom0, wTo0, wFrom1, wTo1)
	case 768:
		moveAVX2_768(a0, a1, wFrom0, wTo0, wFrom1, wTo1)
	case 1024:
		moveAVX2_1024(a0, a1, wFrom0, wTo0, wFrom1, wTo1)
	case 1536:
		moveAVX2_1536(a0, a1, wFrom0, wTo0, wFrom1, wTo1)		
	default:
		panic("unsupported NNUE hidden size")
	}
}

func nnueCapture(a0, a1 *int16, wTo0, wFrom0, wCap0, wTo1, wFrom1, wCap1 *int16) {
	if !hasAVX2 {
		captureScalar(a0, a1, wTo0, wFrom0, wCap0, wTo1, wFrom1, wCap1)
		return
	}
	switch NNUEHiddenSize {
	case 64:
		captureAVX2_64(a0, a1, wTo0, wFrom0, wCap0, wTo1, wFrom1, wCap1)
	case 128:
		captureAVX2_128(a0, a1, wTo0, wFrom0, wCap0, wTo1, wFrom1, wCap1)
	case 256:
		captureAVX2_256(a0, a1, wTo0, wFrom0, wCap0, wTo1, wFrom1, wCap1)
	case 384:
		captureAVX2_384(a0, a1, wTo0, wFrom0, wCap0, wTo1, wFrom1, wCap1)
	case 512:
		captureAVX2_512(a0, a1, wTo0, wFrom0, wCap0, wTo1, wFrom1, wCap1)
	case 768:
		captureAVX2_768(a0, a1, wTo0, wFrom0, wCap0, wTo1, wFrom1, wCap1)
	case 1024:
		captureAVX2_1024(a0, a1, wTo0, wFrom0, wCap0, wTo1, wFrom1, wCap1)
	case 1536:
		captureAVX2_1536(a0, a1, wTo0, wFrom0, wCap0, wTo1, wFrom1, wCap1)		
	default:
		panic("unsupported NNUE hidden size")
	}
}

func nnueCastle(a0, a1 *int16, wKFrom0, wKTo0, wRFrom0, wRTo0, wKFrom1, wKTo1, wRFrom1, wRTo1 *int16) {
	if !hasAVX2 {
		castleScalar(a0, a1, wKFrom0, wKTo0, wRFrom0, wRTo0, wKFrom1, wKTo1, wRFrom1, wRTo1)
		return
	}
	switch NNUEHiddenSize {
	case 64:
		castleAVX2_64(a0, a1, wKFrom0, wKTo0, wRFrom0, wRTo0, wKFrom1, wKTo1, wRFrom1, wRTo1)
	case 128:
		castleAVX2_128(a0, a1, wKFrom0, wKTo0, wRFrom0, wRTo0, wKFrom1, wKTo1, wRFrom1, wRTo1)
	case 256:
		castleAVX2_256(a0, a1, wKFrom0, wKTo0, wRFrom0, wRTo0, wKFrom1, wKTo1, wRFrom1, wRTo1)
	case 384:
		castleAVX2_384(a0, a1, wKFrom0, wKTo0, wRFrom0, wRTo0, wKFrom1, wKTo1, wRFrom1, wRTo1)
	case 512:
		castleAVX2_512(a0, a1, wKFrom0, wKTo0, wRFrom0, wRTo0, wKFrom1, wKTo1, wRFrom1, wRTo1)
	case 768:
		castleAVX2_768(a0, a1, wKFrom0, wKTo0, wRFrom0, wRTo0, wKFrom1, wKTo1, wRFrom1, wRTo1)
	case 1024:
		castleAVX2_1024(a0, a1, wKFrom0, wKTo0, wRFrom0, wRTo0, wKFrom1, wKTo1, wRFrom1, wRTo1)
	case 1536:
		castleAVX2_1536(a0, a1, wKFrom0, wKTo0, wRFrom0, wRTo0, wKFrom1, wKTo1, wRFrom1, wRTo1)		
	default:
		panic("unsupported NNUE hidden size")
	}
}

func nnueEval(a0, a1, w0, w1 *int16, sum *int32) {
	if !hasAVX2 {
		evalScalar(a0, a1, w0, w1, sum)
		return
	}
	switch NNUEHiddenSize {
	case 64:
		getEvalAVX2_64(a0, a1, w0, w1, sum)
	case 128:
		getEvalAVX2_128(a0, a1, w0, w1, sum)
	case 256:
		getEvalAVX2_256(a0, a1, w0, w1, sum)
	case 384:
		getEvalAVX2_384(a0, a1, w0, w1, sum)
	case 512:
		getEvalAVX2_512(a0, a1, w0, w1, sum)
	case 768:
		getEvalAVX2_768(a0, a1, w0, w1, sum)
	case 1024:
		getEvalAVX2_1024(a0, a1, w0, w1, sum)
	case 1536:
		getEvalAVX2_1536(a0, a1, w0, w1, sum)
	default:
		panic("unsupported NNUE hidden size")
	}
}

// nnueMove3/nnueCapture3/nnueCastle3: dst = src + delta in one pass,
// replacing the old "copyFrom(src) then dst += delta" sequence.
//
// The fused AVX2 kernel only exists for size 512 (nnue_avx2_amd64.s), the
// size actually shipped per the NNUEHiddenSize constant. Non-AVX2 builds
// use the scalar three-operand versions for every size (plain Go, so
// fusing them costs nothing extra to verify). The other five AVX2 sizes
// -- unreachable today, kept only so NNUEHiddenSize can still be changed
// to one of them -- fall back to copy + the original two-operand kernel,
// which is exactly their pre-fusion behavior; writing *_3op assembly for
// sizes nothing currently runs isn't worth the risk of an unverifiable
// dead path.
func nnueMove3(dst0, src0, dst1, src1, wFrom0, wTo0, wFrom1, wTo1 *int16) {
	if !hasAVX2 {
		moveScalar3(dst0, src0, dst1, src1, wFrom0, wTo0, wFrom1, wTo1)
		return
	}
	if NNUEHiddenSize == 512 {
		moveAVX2_512_3op(dst0, src0, dst1, src1, wFrom0, wTo0, wFrom1, wTo1)
		return
	}
	copyAccValues(dst0, src0)
	copyAccValues(dst1, src1)
	nnueMove(dst0, dst1, wFrom0, wTo0, wFrom1, wTo1)
}

func nnueCapture3(dst0, src0, dst1, src1, wTo0, wFrom0, wCap0, wTo1, wFrom1, wCap1 *int16) {
	if !hasAVX2 {
		captureScalar3(dst0, src0, dst1, src1, wTo0, wFrom0, wCap0, wTo1, wFrom1, wCap1)
		return
	}
	if NNUEHiddenSize == 512 {
		captureAVX2_512_3op(dst0, src0, dst1, src1, wTo0, wFrom0, wCap0, wTo1, wFrom1, wCap1)
		return
	}
	copyAccValues(dst0, src0)
	copyAccValues(dst1, src1)
	nnueCapture(dst0, dst1, wTo0, wFrom0, wCap0, wTo1, wFrom1, wCap1)
}

func nnueCastle3(dst0, src0, dst1, src1, wKFrom0, wKTo0, wRFrom0, wRTo0, wKFrom1, wKTo1, wRFrom1, wRTo1 *int16) {
	if !hasAVX2 {
		castleScalar3(dst0, src0, dst1, src1, wKFrom0, wKTo0, wRFrom0, wRTo0, wKFrom1, wKTo1, wRFrom1, wRTo1)
		return
	}
	if NNUEHiddenSize == 512 {
		castleAVX2_512_3op(dst0, src0, dst1, src1, wKFrom0, wKTo0, wRFrom0, wRTo0, wKFrom1, wKTo1, wRFrom1, wRTo1)
		return
	}
	copyAccValues(dst0, src0)
	copyAccValues(dst1, src1)
	nnueCastle(dst0, dst1, wKFrom0, wKTo0, wRFrom0, wRTo0, wKFrom1, wKTo1, wRFrom1, wRTo1)
}

// copyAccValues copies one perspective's NNUEHiddenSize int16 values from
// src to dst. A no-op when dst == src (the in-place update callers use).
func copyAccValues(dst, src *int16) {
	if dst == src {
		return
	}
	copy(unsafe.Slice(dst, NNUEHiddenSize), unsafe.Slice(src, NNUEHiddenSize))
}

var zeroWeights [NNUEHiddenSize]int16

func featureIndex(color, pt, sq, kingSq, perspective int) int {
	idxSq := sq
	kSq := kingSq

	if perspective == 1 {
		idxSq ^= 56
		kSq ^= 56
	}

	if singleOptionValue[HorizontalMirroring] == 1 {
		if kingSq%8 > 3 {
			idxSq ^= 7
		}
	}

	bucket := 0
	if NNUEInputBuckets > 1 {
		bucket = kingBucketTable[kSq]
	}

	return bucket*NNUEInputSize + (color^perspective)*384 + pt*64 + idxSq
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
// move computes dst = src + (feature deltas for a normal move). src may
// alias dst (see nnueMove3).
func (dst *Accumulator) move(src *Accumulator, color, pt, from, to, k0, k1 int, refresh0, refresh1 bool) {
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

	nnueMove3(
		&dst.values[0][0], &src.values[0][0],
		&dst.values[1][0], &src.values[1][0],

		pFrom0,
		pTo0,

		pFrom1,
		pTo1,
	)

}

func (dst *Accumulator) capture(
	src *Accumulator,
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

	nnueCapture3(
		&dst.values[0][0], &src.values[0][0],
		&dst.values[1][0], &src.values[1][0],

		pTo0,
		pFrom0,
		pCap0,

		pTo1,
		pFrom1,
		pCap1,
	)

}

func (dst *Accumulator) castle(
	src *Accumulator,
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

	nnueCastle3(
		&dst.values[0][0], &src.values[0][0],
		&dst.values[1][0], &src.values[1][0],

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

func (dst *Accumulator) promotion(src *Accumulator, color, from, to, prom int, k0, k1 int, refresh0, refresh1 bool) {
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

	nnueMove3(
		&dst.values[0][0], &src.values[0][0],
		&dst.values[1][0], &src.values[1][0],

		pFrom0,
		pTo0,

		pFrom1,
		pTo1,
	)
}

func (dst *Accumulator) promotionCapture(src *Accumulator, color, from, to, prom, captType int, k0, k1 int, refresh0, refresh1 bool) {
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

	nnueCapture3(
		&dst.values[0][0], &src.values[0][0],
		&dst.values[1][0], &src.values[1][0],

		pTo0,
		pFrom0,
		pCap0,

		pTo1,
		pFrom1,
		pCap1,
	)
}

// apply full nnue accumulator update: acc = src + (feature deltas for
// u's move). src may be acc itself (in-place update, e.g. datagen
// advancing the game accumulator, or applyMoves in uci.go) or a distinct
// parent accumulator (search descending to a new ply) -- see
// nnueMove3/nnueCapture3/nnueCastle3.
func (acc *Accumulator) applyPendingChanges(src *Accumulator, p *Pos, u *Update, ss *SearchState) {

	// Already applied!
	if !u.dirty {
		return
	}

	// Init king-related stuff.
	k0 := p.kingSq[White]
	k1 := p.kingSq[Black]
	refresh0 := false
	refresh1 := false

	// King moving to a different king bucket requires accumulator refreshing.
	// When horizontal mirroring is enabled, moving across the kingside/queenside
	// center boundary also requires refreshing.
	if u.movingType == K {
		if u.color == White {
			fromBucket := kingBucketTable[u.from]
			toBucket := kingBucketTable[u.to]
			fromMirror := (u.from%8 > 3)
			toMirror := (u.to%8 > 3)
			if fromBucket != toBucket || (singleOptionValue[HorizontalMirroring] == 1 && fromMirror != toMirror) {
				refresh0 = true
			}
		} else {
			fromBucket := kingBucketTable[u.from^56]
			toBucket := kingBucketTable[u.to^56]
			fromMirror := (u.from%8 > 3)
			toMirror := (u.to%8 > 3)
			if fromBucket != toBucket || (singleOptionValue[HorizontalMirroring] == 1 && fromMirror != toMirror) {
				refresh1 = true
			}
		}
	}

	// Apply move.
	switch u.flag {
	case uNORMAL, uEP_SET:
		acc.move(src, u.color, u.movingType, u.from, u.to, k0, k1, refresh0, refresh1)

	case uCAPTURE:
		acc.capture(src, u.color, u.movingType, u.from, u.to, u.color^1, u.captType, u.capSq, k0, k1, refresh0, refresh1)

	case uEP_CAP:
		acc.capture(src, u.color, P, u.from, u.to, u.color^1, P, u.capSq, k0, k1, refresh0, refresh1)

	case uCASTLE:
		acc.castle(src, u.color, u.from, u.to, u.rookFrom, u.rookTo, k0, k1, refresh0, refresh1)

	case uPROMO:
		acc.promotion(src, u.color, u.from, u.to, u.prom, k0, k1, refresh0, refresh1)

	case uPROMCAPT:
		acc.promotionCapture(src, u.color, u.from, u.to, u.prom, u.captType, k0, k1, refresh0, refresh1)

	}

	// Refresh if king move demands so.
	if refresh0 {
		ss.refreshPerspective(p, acc, 0)
	}
	if refresh1 {
		ss.refreshPerspective(p, acc, 1)
	}

	// Done!
	u.dirty = false
}

func (ss *SearchState) refreshPerspective(p *Pos, acc *Accumulator, perspective int) {
	if ss != nil && NNUEHiddenSize < 1536 { // THERE IS A FINNY PROBLEM
		ss.refreshPerspectiveWithFinny(p, acc, perspective)
	} else {
		refreshPerspectivePlain(p, acc, perspective)
	}
}

func (ss *SearchState) refreshPerspectiveWithFinny(p *Pos, acc *Accumulator, perspective int) {
	kSq := p.kingSq[perspective]
	mirror := 0
	if singleOptionValue[HorizontalMirroring] == 1 && kSq%8 > 3 {
		mirror = 1
	}
	kOri := kSq
	if perspective == 1 {
		kOri ^= 56
	}
	bucket := kingBucketTable[kOri]

	entry := &ss.finny[perspective][mirror][bucket]

	if !entry.valid || entry.generation != nnue.generation {
		// Initialize accumulator from biases
		copy(entry.acc[:NNUEHiddenSize], nnueParams.InputBiases[:NNUEHiddenSize])
		entryAccPtr := &entry.acc[0]

		for pc := 0; pc < 12; pc++ {
			c := colorOf(pc)
			pt := typeOf(pc)
			bb := p.colorBB[c] & p.typeBB[pt]
			entry.pieces[pc] = bb

			for bb != 0 {
				sq := bits.TrailingZeros64(bb)
				bb &= bb - 1
				idx := featureIndex(c, pt, sq, kSq, perspective)
				w := &nnueParams.InputWeights[idx][0]
				nnueAddSingle(entryAccPtr, w)
			}
		}
		entry.generation = nnue.generation
		entry.valid = true
	} else {
		entryAccPtr := &entry.acc[0]
		for pc := 0; pc < 12; pc++ {
			c := colorOf(pc)
			pt := typeOf(pc)
			currBB := p.colorBB[c] & p.typeBB[pt]
			prevBB := entry.pieces[pc]
			diff := currBB ^ prevBB
			if diff == 0 {
				continue
			}

			// Added pieces
			added := diff & currBB
			for added != 0 {
				sq := bits.TrailingZeros64(added)
				added &= added - 1
				idx := featureIndex(c, pt, sq, kSq, perspective)
				w := &nnueParams.InputWeights[idx][0]
				nnueAddSingle(entryAccPtr, w)
			}

			// Removed pieces
			removed := diff & prevBB
			for removed != 0 {
				sq := bits.TrailingZeros64(removed)
				removed &= removed - 1
				idx := featureIndex(c, pt, sq, kSq, perspective)
				w := &nnueParams.InputWeights[idx][0]
				nnueSubSingle(entryAccPtr, w)
			}

			entry.pieces[pc] = currBB
		}
	}

	copy(acc.values[perspective][:NNUEHiddenSize], entry.acc[:NNUEHiddenSize])
}

func refreshPerspectivePlain(p *Pos, acc *Accumulator, perspective int) {
	kSq := p.kingSq[perspective]

	a := &acc.values[perspective][0]

	// Initialize accumulator from biases.
	copy(
		acc.values[perspective][:NNUEHiddenSize],
		nnueParams.InputBiases[:NNUEHiddenSize],
	)

	// Scan board and add all the pieces
	for sq := 0; sq < 64; sq++ {
		piece := p.board[sq]
		if piece == NO_PC {
			continue
		}

		color := colorOf(piece)
		pt := typeOf(piece)
		idx := featureIndex(color, pt, sq, kSq, perspective)
		w := &nnueParams.InputWeights[idx][0]

		nnueAddSingle(a, w)
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

func outputBucket(p *Pos) int {
	occupied := p.colorBB[White] | p.colorBB[Black]
	pieceCount := bits.OnesCount64(occupied)
	bucket := (pieceCount - 2) >> 2
	if bucket < 0 {
		return 0
	}
	if bucket >= OutputBuckets {
		return OutputBuckets - 1
	}
	return bucket
}

func (acc *Accumulator) getEval(p *Pos, stm int) int {
	bucket := outputBucket(p)
	if nnueParams.IsMultilayer {
		if hasAVX2 && nnueParams.IsDualActivation {
			var sum int32
			getEvalMultilayerAVX2(
				&acc.values[stm][0],
				&acc.values[stm^1][0],
				&nnueParams.L1WeightsTiled[bucket][0][0][0],
				&nnueParams.L1Biases[bucket][0],
				&nnueParams.L2Weights[bucket][0][0],
				&nnueParams.L2Biases[bucket][0],
				&nnueParams.L3Weights[bucket][0],
				nnueParams.L3Biases[bucket],
				float32(singleOptionValue[NnueScale]),
				&sum,
			)
			return int(sum)
		}
		return acc.getEvalMultilayer(bucket, stm)
	}

	var sum int32

	nnueEval(
		&acc.values[stm][0],
		&acc.values[stm^1][0],
		&nnueParams.OutputWeights[bucket][0][0],
		&nnueParams.OutputWeights[bucket][1][0],
		&sum,
	)

	sum = sum/NNUEL0Scale + int32(nnueParams.OutputBiases[bucket])

	return int(sum * int32(singleOptionValue[NnueScale]) /
		(NNUEL0Scale * NNUEL1Scale))
}

func (acc *Accumulator) getEvalMultilayer(bucket, stm int) int {
	if hasAVX2 && nnueParams.IsDualActivation {
		var sum int32
		getEvalMultilayerAVX2(
			&acc.values[stm][0],
			&acc.values[stm^1][0],
			&nnueParams.L1WeightsTiled[bucket][0][0][0],
			&nnueParams.L1Biases[bucket][0],
			&nnueParams.L2Weights[bucket][0][0],
			&nnueParams.L2Biases[bucket][0],
			&nnueParams.L3Weights[bucket][0],
			nnueParams.L3Biases[bucket],
			float32(singleOptionValue[NnueScale]),
			&sum,
		)
		return int(sum)
	}

	// 1. Pairwise multiplication on accumulator halves into uint8 [0..255]:
	// stm half 0 * stm half 1 (256 values)
	// ntm half 0 * ntm half 1 (256 values)
	var pw [NNUEHiddenSize]uint8
	stmAcc := &acc.values[stm]
	ntmAcc := &acc.values[stm^1]

	const halfSize = NNUEHiddenSize / 2
	for i := 0; i < halfSize; i++ {
		s0 := stmAcc[i]
		if s0 < 0 {
			s0 = 0
		} else if s0 > 255 {
			s0 = 255
		}
		s1 := stmAcc[i+halfSize]
		if s1 < 0 {
			s1 = 0
		} else if s1 > 255 {
			s1 = 255
		}
		pw[i] = uint8((int32(s0) * int32(s1)) >> 8)

		n0 := ntmAcc[i]
		if n0 < 0 {
			n0 = 0
		} else if n0 > 255 {
			n0 = 255
		}
		n1 := ntmAcc[i+halfSize]
		if n1 < 0 {
			n1 = 0
		} else if n1 > 255 {
			n1 = 255
		}
		pw[halfSize+i] = uint8((int32(n0) * int32(n1)) >> 8)
	}

	// 2. Layer 1 forward (512 -> 16) with integer accumulation:
	var intSums [NNUEMultilayerL1Size]int32
	l1wRaw := &nnueParams.L1WeightsRaw[bucket]
	for i := 0; i < NNUEHiddenSize; i++ {
		pwi := int32(pw[i])
		if pwi == 0 {
			continue
		}
		for j := 0; j < NNUEMultilayerL1Size; j++ {
			intSums[j] += pwi * int32(l1wRaw[i][j])
		}
	}

	// 3. Convert to float32 and add L1 bias:
	// Scale factor: pw_float = pw_u8 * 256 / (255 * 255)
	// w_float = w_i8 / 64
	// Total factor = 256 / (255 * 255 * 64) = 4 / 65025
	const l1Factor = float32(4.0 / 65025.0)
	var sums [NNUEMultilayerL1Size]float32
	l1b := &nnueParams.L1Biases[bucket]
	for j := 0; j < NNUEMultilayerL1Size; j++ {
		sums[j] = l1b[j] + float32(intSums[j])*l1Factor
	}

	var l1Out [NNUEMultilayerL2Inputs]float32
	for j := 0; j < NNUEMultilayerL1Size; j++ {
		crelu := sums[j]
		if crelu < 0.0 {
			crelu = 0.0
		} else if crelu > 1.0 {
			crelu = 1.0
		}
		screlu := crelu * crelu

		if nnueParams.IsDualActivation {
			l1Out[j] = crelu
			l1Out[j+NNUEMultilayerL1Size] = screlu
		} else {
			l1Out[j] = screlu
		}
	}

	// 3. Layer 2 forward (32 -> 32 if dual activation, 16 -> 32 if single)
	var l2Out [NNUEMultilayerL2Size]float32
	l2Inputs := NNUEMultilayerL1Size
	if nnueParams.IsDualActivation {
		l2Inputs = 2 * NNUEMultilayerL1Size
	}
	l2w := &nnueParams.L2Weights[bucket]
	l2b := &nnueParams.L2Biases[bucket]

	copy(l2Out[:], l2b[:])
	for j := 0; j < l2Inputs; j++ {
		act := l1Out[j]
		if act == 0.0 {
			continue
		}
		wRow := &l2w[j]
		for k := 0; k < NNUEMultilayerL2Size; k++ {
			l2Out[k] += act * wRow[k]
		}
	}
	for k := 0; k < NNUEMultilayerL2Size; k++ {
		sum := l2Out[k]
		if sum < 0.0 {
			sum = 0.0
		} else if sum > 1.0 {
			sum = 1.0
		}
		l2Out[k] = sum
	}

	// 4. Layer 3 forward (32 -> 1)
	l3w := &nnueParams.L3Weights[bucket]
	score := nnueParams.L3Biases[bucket]
	for k := 0; k < NNUEMultilayerL2Size; k++ {
		score += l2Out[k] * l3w[k]
	}

	// 5. Scale to centipawns
	return int(score * float32(singleOptionValue[NnueScale]))
}

func nnueLoadFromBytes(data []byte) bool {
	if len(data) < SingleBucketNetSize {
		return false
	}

	offset := 0
	nextParams := new(NNUEParameters)

	readI16 := func() int16 {
		value := int16(
			uint16(data[offset]) |
				uint16(data[offset+1])<<8,
		)
		offset += 2
		return value
	}

	readI8 := func() int8 {
		value := int8(data[offset])
		offset++
		return value
	}

	readF32 := func() float32 {
		b := uint32(data[offset]) |
			uint32(data[offset+1])<<8 |
			uint32(data[offset+2])<<16 |
			uint32(data[offset+3])<<24
		offset += 4
		return math.Float32frombits(b)
	}

	isDualAct := len(data) >= MultilayerDualActNetSize && len(data) < MultilayerDualActNetSize+64
	isSingleAct := len(data) >= MultilayerNetSize && len(data) < MultilayerNetSize+64
	isMultilayer := isDualAct || isSingleAct

	if isMultilayer {
		// Multilayer 8-bucket architecture
		for input := 0; input < TotalInputFeatures; input++ {
			for neuron := 0; neuron < NNUEHiddenSize; neuron++ {
				nextParams.InputWeights[input][neuron] = readI16()
			}
		}
		for neuron := 0; neuron < NNUEHiddenSize; neuron++ {
			nextParams.InputBiases[neuron] = readI16()
		}

		// L1 weights: transposed (128 rows x 512 cols in row-major file view), i8 quantized by 64
		for b := 0; b < OutputBuckets; b++ {
			for j := 0; j < NNUEMultilayerL1Size; j++ {
				for i := 0; i < NNUEHiddenSize; i++ {
					w := readI8()
					nextParams.L1WeightsRaw[b][i][j] = w
					nextParams.L1Weights[b][i][j] = float32(w) / float32(NNUEL1Scale)
				}
			}
		}

		// Pre-tile L1 weights for SIMD (vpmaddubsw / vpdpbusd):
		// 128 chunks of 4 inputs x 16 neurons
		for b := 0; b < OutputBuckets; b++ {
			for k := 0; k < NNUEHiddenSize/4; k++ {
				// Neurons 0..7 (first 256-bit register)
				for m := 0; m < 8; m++ {
					for c := 0; c < 4; c++ {
						nextParams.L1WeightsTiled[b][k][0][m*4+c] = nextParams.L1WeightsRaw[b][4*k+c][m]
					}
				}
				// Neurons 8..15 (second 256-bit register)
				for m := 0; m < 8; m++ {
					for c := 0; c < 4; c++ {
						nextParams.L1WeightsTiled[b][k][1][m*4+c] = nextParams.L1WeightsRaw[b][4*k+c][8+m]
					}
				}
			}
		}

		// L1 biases: 128 f32 (8 buckets x 16 neurons)
		for b := 0; b < OutputBuckets; b++ {
			for j := 0; j < NNUEMultilayerL1Size; j++ {
				nextParams.L1Biases[b][j] = readF32()
			}
		}

		// L2 weights: transposed (256 rows x l2InCount cols in row-major file view), f32
		l2InCount := NNUEMultilayerL1Size
		if isDualAct {
			l2InCount = 2 * NNUEMultilayerL1Size
		}
		for b := 0; b < OutputBuckets; b++ {
			for k := 0; k < NNUEMultilayerL2Size; k++ {
				for j := 0; j < l2InCount; j++ {
					nextParams.L2Weights[b][j][k] = readF32()
				}
			}
		}

		// L2 biases: 256 f32 (8 buckets x 32 neurons)
		for b := 0; b < OutputBuckets; b++ {
			for k := 0; k < NNUEMultilayerL2Size; k++ {
				nextParams.L2Biases[b][k] = readF32()
			}
		}

		// L3 weights: transposed (8 rows x 32 cols in row-major file view), f32
		for b := 0; b < OutputBuckets; b++ {
			for k := 0; k < NNUEMultilayerL2Size; k++ {
				nextParams.L3Weights[b][k] = readF32()
			}
		}

		// L3 biases: 8 f32 (8 buckets x 1 score)
		for b := 0; b < OutputBuckets; b++ {
			nextParams.L3Biases[b] = readF32()
		}

		nextParams.IsMultilayer = true
		nextParams.IsDualActivation = isDualAct

		nnueParams = nextParams
		nnue.generation++
		nnue.Loaded = true
		return true
	}

	is8Bucket := len(data) >= EightBucketSingleNetSize
	is4Bucket := !is8Bucket && len(data) >= FourBucketSingleNetSize

	if is8Bucket {
		for input := 0; input < TotalInputFeatures; input++ {
			for neuron := 0; neuron < NNUEHiddenSize; neuron++ {
				nextParams.InputWeights[input][neuron] = readI16()
			}
		}
	} else if is4Bucket {
		// Read 4 buckets (4 * 768) and map onto the 8-bucket layout
		var fourBuckets [4 * NNUEInputSize][NNUEHiddenSize]int16
		for input := 0; input < 4*NNUEInputSize; input++ {
			for neuron := 0; neuron < NNUEHiddenSize; neuron++ {
				fourBuckets[input][neuron] = readI16()
			}
		}
		// 4KB -> 8KB mapping:
		// 8KB 0, 1 (Rank 1 corner/castled)  <- 4KB Bucket 1 (Rank 1 flank)
		// 8KB 2, 3 (Rank 1 queenside/center) <- 4KB Bucket 0 (Rank 1 center)
		// 8KB 4, 5 (Rank 2 flank/center)     <- 4KB Bucket 2 (Rank 2)
		// 8KB 6, 7 (Ranks 3-8 midfield/enemy) <- 4KB Bucket 3 (Ranks 3-8)
		bucketMap := [8]int{1, 1, 0, 0, 2, 2, 3, 3}
		for b := 0; b < 8; b++ {
			srcB := bucketMap[b]
			for input := 0; input < NNUEInputSize; input++ {
				nextParams.InputWeights[b*NNUEInputSize+input] = fourBuckets[srcB*NNUEInputSize+input]
			}
		}
	} else {
		// 1 Input Bucket - read first bucket and duplicate across all 8 buckets
		for input := 0; input < NNUEInputSize; input++ {
			for neuron := 0; neuron < NNUEHiddenSize; neuron++ {
				nextParams.InputWeights[input][neuron] = readI16()
			}
		}
		for b := 1; b < NNUEInputBuckets; b++ {
			for input := 0; input < NNUEInputSize; input++ {
				nextParams.InputWeights[b*NNUEInputSize+input] = nextParams.InputWeights[input]
			}
		}
	}

	for neuron := 0; neuron < NNUEHiddenSize; neuron++ {
		nextParams.InputBiases[neuron] = readI16()
	}

	// 8 Output Buckets vs Single Output Bucket
	isOutputBuckets := (is8Bucket && len(data) >= EightBucketOutputNetSize) ||
		(is4Bucket && len(data) >= FourBucketOutputNetSize) ||
		(!is8Bucket && !is4Bucket && len(data) >= OutputBucketNetSize)

	if isOutputBuckets {
		for b := 0; b < OutputBuckets; b++ {
			for neuron := 0; neuron < NNUEHiddenSize; neuron++ {
				nextParams.OutputWeights[b][0][neuron] = readI16()
			}
			for neuron := 0; neuron < NNUEHiddenSize; neuron++ {
				nextParams.OutputWeights[b][1][neuron] = readI16()
			}
		}

		for b := 0; b < OutputBuckets; b++ {
			nextParams.OutputBiases[b] = readI16()
		}
	} else {
		// Single Output Bucket fallback - broadcast weights to all 8 buckets
		var stmWeights [NNUEHiddenSize]int16
		var ntmWeights [NNUEHiddenSize]int16

		for neuron := 0; neuron < NNUEHiddenSize; neuron++ {
			stmWeights[neuron] = readI16()
		}
		for neuron := 0; neuron < NNUEHiddenSize; neuron++ {
			ntmWeights[neuron] = readI16()
		}
		bias := readI16()

		for b := 0; b < OutputBuckets; b++ {
			nextParams.OutputWeights[b][0] = stmWeights
			nextParams.OutputWeights[b][1] = ntmWeights
			nextParams.OutputBiases[b] = bias
		}
	}

	nextParams.IsMultilayer = false

	nnueParams = nextParams
	nnue.generation++
	nnue.Loaded = true
	return true
}

// Load embedded NNUE parameters
func nnueInitEmbedded() bool {
	return nnueLoadFromBytes(embeddedNet)
}

// Load a raw Bullet-compatible parameter blob.
func nnueLoad(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return nnueLoadFromBytes(data)
}
