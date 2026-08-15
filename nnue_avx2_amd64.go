//go:build amd64

package main

// ADD / SUB SINGLE
//go:noescape
func addSingleAVX2_64(a, w *int16)

//go:noescape
func subSingleAVX2_64(a, w *int16)

//go:noescape
func addSingleAVX2_128(a, w *int16)

//go:noescape
func subSingleAVX2_128(a, w *int16)

//go:noescape
func addSingleAVX2_256(a, w *int16)

//go:noescape
func subSingleAVX2_256(a, w *int16)

//go:noescape
func addSingleAVX2_384(a, w *int16)

//go:noescape
func subSingleAVX2_384(a, w *int16)

//go:noescape
func addSingleAVX2_512(a, w *int16)

//go:noescape
func subSingleAVX2_512(a, w *int16)

//go:noescape
func addSingleAVX2_768(a, w *int16)

//go:noescape
func subSingleAVX2_768(a, w *int16)

// CAPTURE

//go:noescape
func captureAVX2_64(
	a0, a1 *int16,
	wTo0, wFrom0, wCap0 *int16,
	wTo1, wFrom1, wCap1 *int16,
)

//go:noescape
func captureAVX2_128(
	a0, a1 *int16,
	wTo0, wFrom0, wCap0 *int16,
	wTo1, wFrom1, wCap1 *int16,
)

//go:noescape
func captureAVX2_256(
	a0, a1 *int16,
	wTo0, wFrom0, wCap0 *int16,
	wTo1, wFrom1, wCap1 *int16,
)

//go:noescape
func captureAVX2_384(
	a0, a1 *int16,
	wTo0, wFrom0, wCap0 *int16,
	wTo1, wFrom1, wCap1 *int16,
)

//go:noescape
func captureAVX2_512(
	a0, a1 *int16,
	wTo0, wFrom0, wCap0 *int16,
	wTo1, wFrom1, wCap1 *int16,
)

//go:noescape
func captureAVX2_768(
	a0, a1 *int16,
	wTo0, wFrom0, wCap0 *int16,
	wTo1, wFrom1, wCap1 *int16,
)

// MOVE

//go:noescape
func moveAVX2_64(
	a0, a1 *int16,
	wFrom0, wTo0 *int16,
	wFrom1, wTo1 *int16,
)

//go:noescape
func moveAVX2_128(
	a0, a1 *int16,
	wFrom0, wTo0 *int16,
	wFrom1, wTo1 *int16,
)

//go:noescape
func moveAVX2_256(
	a0, a1 *int16,
	wFrom0, wTo0 *int16,
	wFrom1, wTo1 *int16,
)

//go:noescape
func moveAVX2_384(
	a0, a1 *int16,
	wFrom0, wTo0 *int16,
	wFrom1, wTo1 *int16,
)

//go:noescape
func moveAVX2_512(
	a0, a1 *int16,
	wFrom0, wTo0 *int16,
	wFrom1, wTo1 *int16,
)

//go:noescape
func moveAVX2_768(
	a0, a1 *int16,
	wFrom0, wTo0 *int16,
	wFrom1, wTo1 *int16,
)

// CASTLE

//go:noescape
func castleAVX2_64(
	a0, a1 *int16,
	wKFrom0, wKTo0, wRFrom0, wRTo0 *int16,
	wKFrom1, wKTo1, wRFrom1, wRTo1 *int16,
)

//go:noescape
func castleAVX2_128(
	a0, a1 *int16,
	wKFrom0, wKTo0, wRFrom0, wRTo0 *int16,
	wKFrom1, wKTo1, wRFrom1, wRTo1 *int16,
)

//go:noescape
func castleAVX2_256(
	a0, a1 *int16,
	wKFrom0, wKTo0, wRFrom0, wRTo0 *int16,
	wKFrom1, wKTo1, wRFrom1, wRTo1 *int16,
)

//go:noescape
func castleAVX2_384(
	a0, a1 *int16,
	wKFrom0, wKTo0, wRFrom0, wRTo0 *int16,
	wKFrom1, wKTo1, wRFrom1, wRTo1 *int16,
)

//go:noescape
func castleAVX2_512(
	a0, a1 *int16,
	wKFrom0, wKTo0, wRFrom0, wRTo0 *int16,
	wKFrom1, wKTo1, wRFrom1, wRTo1 *int16,
)

//go:noescape
func castleAVX2_768(
	a0, a1 *int16,
	wKFrom0, wKTo0, wRFrom0, wRTo0 *int16,
	wKFrom1, wKTo1, wRFrom1, wRTo1 *int16,
)

// THREE-OPERAND (dst = src + delta) VARIANTS, 512 HIDDEN NEURONS ONLY.
// NNUEHiddenSize is fixed at 512 (nnue.go), so this is the size that
// actually runs; other sizes fall back to copy + the two-operand kernels
// above. See the comment above moveAVX2_512_3op in nnue_avx2_amd64.s.

//go:noescape
func moveAVX2_512_3op(
	dst0, src0, dst1, src1 *int16,
	wFrom0, wTo0, wFrom1, wTo1 *int16,
)

//go:noescape
func captureAVX2_512_3op(
	dst0, src0, dst1, src1 *int16,
	wTo0, wFrom0, wCap0 *int16,
	wTo1, wFrom1, wCap1 *int16,
)

//go:noescape
func castleAVX2_512_3op(
	dst0, src0, dst1, src1 *int16,
	wKFrom0, wKTo0, wRFrom0, wRTo0 *int16,
	wKFrom1, wKTo1, wRFrom1, wRTo1 *int16,
)

// EVAL

//go:noescape
func getEvalAVX2_64(
	a0, a1 *int16,
	w0, w1 *int16,
	sum *int32,
)

//go:noescape
func getEvalAVX2_128(
	a0, a1 *int16,
	w0, w1 *int16,
	sum *int32,
)

//go:noescape
func getEvalAVX2_256(
	a0, a1 *int16,
	w0, w1 *int16,
	sum *int32,
)

//go:noescape
func getEvalAVX2_384(
	a0, a1 *int16,
	w0, w1 *int16,
	sum *int32,
)

//go:noescape
func getEvalAVX2_512(
	a0, a1 *int16,
	w0, w1 *int16,
	sum *int32,
)

//go:noescape
func getEvalAVX2_768(
	a0, a1 *int16,
	w0, w1 *int16,
	sum *int32,
)
