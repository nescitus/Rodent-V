package main

// nnueMove3/nnueCapture3/nnueCastle3 (nnue.go) and their AVX2 assembly
// (nnue_avx2_amd64.s) replace a copy-then-update sequence with a single
// fused dst = src + delta pass. bench/perft can only prove the search
// tree is unchanged, not that the accumulator arithmetic itself is still
// correct -- a wrong register or operand order in the assembly could
// easily leave both untouched. These tests check the fused path against
// the original copy + two-operand path directly, including int16
// wraparound at the value range's edges and the dst==src aliasing that
// datagen and uci.go's applyMoves rely on.

import (
	"math/rand"
	"testing"
)

// randVec fills v with random int16 values across the full range,
// including near over/underflow boundaries where wraparound behavior
// must match exactly between the old and new code paths.
func randVec(rng *rand.Rand, v []int16) {
	for i := range v {
		v[i] = int16(rng.Intn(65536) - 32768)
	}
}

func TestFusion3opMoveMatchesCopyPlusTwoOp(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for trial := 0; trial < 2000; trial++ {
		var src0, src1 [NNUEHiddenSize]int16
		var wFrom0, wTo0, wFrom1, wTo1 [NNUEHiddenSize]int16
		randVec(rng, src0[:])
		randVec(rng, src1[:])
		randVec(rng, wFrom0[:])
		randVec(rng, wTo0[:])
		randVec(rng, wFrom1[:])
		randVec(rng, wTo1[:])

		// OLD: copy then in-place two-operand update.
		var oldDst0, oldDst1 [NNUEHiddenSize]int16
		copy(oldDst0[:], src0[:])
		copy(oldDst1[:], src1[:])
		nnueMove(&oldDst0[0], &oldDst1[0], &wFrom0[0], &wTo0[0], &wFrom1[0], &wTo1[0])

		// NEW: fused three-operand update.
		var newDst0, newDst1 [NNUEHiddenSize]int16
		nnueMove3(&newDst0[0], &src0[0], &newDst1[0], &src1[0], &wFrom0[0], &wTo0[0], &wFrom1[0], &wTo1[0])

		if oldDst0 != newDst0 || oldDst1 != newDst1 {
			t.Fatalf("trial %d: move mismatch\nold0=%v\nnew0=%v\nold1=%v\nnew1=%v", trial, oldDst0, newDst0, oldDst1, newDst1)
		}
	}
}

func TestFusion3opCaptureMatchesCopyPlusTwoOp(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	for trial := 0; trial < 2000; trial++ {
		var src0, src1 [NNUEHiddenSize]int16
		var wTo0, wFrom0, wCap0, wTo1, wFrom1, wCap1 [NNUEHiddenSize]int16
		randVec(rng, src0[:])
		randVec(rng, src1[:])
		randVec(rng, wTo0[:])
		randVec(rng, wFrom0[:])
		randVec(rng, wCap0[:])
		randVec(rng, wTo1[:])
		randVec(rng, wFrom1[:])
		randVec(rng, wCap1[:])

		var oldDst0, oldDst1 [NNUEHiddenSize]int16
		copy(oldDst0[:], src0[:])
		copy(oldDst1[:], src1[:])
		nnueCapture(&oldDst0[0], &oldDst1[0], &wTo0[0], &wFrom0[0], &wCap0[0], &wTo1[0], &wFrom1[0], &wCap1[0])

		var newDst0, newDst1 [NNUEHiddenSize]int16
		nnueCapture3(&newDst0[0], &src0[0], &newDst1[0], &src1[0], &wTo0[0], &wFrom0[0], &wCap0[0], &wTo1[0], &wFrom1[0], &wCap1[0])

		if oldDst0 != newDst0 || oldDst1 != newDst1 {
			t.Fatalf("trial %d: capture mismatch\nold0=%v\nnew0=%v\nold1=%v\nnew1=%v", trial, oldDst0, newDst0, oldDst1, newDst1)
		}
	}
}

func TestFusion3opCastleMatchesCopyPlusTwoOp(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	for trial := 0; trial < 2000; trial++ {
		var src0, src1 [NNUEHiddenSize]int16
		var wKFrom0, wKTo0, wRFrom0, wRTo0, wKFrom1, wKTo1, wRFrom1, wRTo1 [NNUEHiddenSize]int16
		randVec(rng, src0[:])
		randVec(rng, src1[:])
		randVec(rng, wKFrom0[:])
		randVec(rng, wKTo0[:])
		randVec(rng, wRFrom0[:])
		randVec(rng, wRTo0[:])
		randVec(rng, wKFrom1[:])
		randVec(rng, wKTo1[:])
		randVec(rng, wRFrom1[:])
		randVec(rng, wRTo1[:])

		var oldDst0, oldDst1 [NNUEHiddenSize]int16
		copy(oldDst0[:], src0[:])
		copy(oldDst1[:], src1[:])
		nnueCastle(&oldDst0[0], &oldDst1[0], &wKFrom0[0], &wKTo0[0], &wRFrom0[0], &wRTo0[0], &wKFrom1[0], &wKTo1[0], &wRFrom1[0], &wRTo1[0])

		var newDst0, newDst1 [NNUEHiddenSize]int16
		nnueCastle3(&newDst0[0], &src0[0], &newDst1[0], &src1[0], &wKFrom0[0], &wKTo0[0], &wRFrom0[0], &wRTo0[0], &wKFrom1[0], &wKTo1[0], &wRFrom1[0], &wRTo1[0])

		if oldDst0 != newDst0 || oldDst1 != newDst1 {
			t.Fatalf("trial %d: castle mismatch\nold0=%v\nnew0=%v\nold1=%v\nnew1=%v", trial, oldDst0, newDst0, oldDst1, newDst1)
		}
	}
}

// TestFusion3opAliasedDstSrc verifies the in-place (dst==src) usage that
// datagen and applyMoves rely on -- the pointer identity must not matter.
func TestFusion3opAliasedDstSrc(t *testing.T) {
	rng := rand.New(rand.NewSource(4))
	for trial := 0; trial < 500; trial++ {
		var v0, v1 [NNUEHiddenSize]int16
		var wFrom0, wTo0, wFrom1, wTo1 [NNUEHiddenSize]int16
		randVec(rng, v0[:])
		randVec(rng, v1[:])
		randVec(rng, wFrom0[:])
		randVec(rng, wTo0[:])
		randVec(rng, wFrom1[:])
		randVec(rng, wTo1[:])

		want0, want1 := v0, v1
		nnueMove(&want0[0], &want1[0], &wFrom0[0], &wTo0[0], &wFrom1[0], &wTo1[0])

		got0, got1 := v0, v1
		nnueMove3(&got0[0], &got0[0], &got1[0], &got1[0], &wFrom0[0], &wTo0[0], &wFrom1[0], &wTo1[0])

		if want0 != got0 || want1 != got1 {
			t.Fatalf("trial %d: aliased dst==src mismatch\nwant0=%v\ngot0=%v", trial, want0, got0)
		}
	}
}

func syntheticNNUEWithBias(bias int16) []byte {
	data := make([]byte, SingleBucketNetSize)
	biasOffset := NNUEInputSize * NNUEHiddenSize * 2
	raw := uint16(bias)
	for neuron := 0; neuron < NNUEHiddenSize; neuron++ {
		offset := biasOffset + neuron*2
		data[offset] = byte(raw)
		data[offset+1] = byte(raw >> 8)
	}
	return data
}

func TestFinnyInvalidatesAfterNNUEReload(t *testing.T) {
	oldParams := nnueParams
	oldNNUE := nnue
	oldMirroring := singleOptionValue[HorizontalMirroring]
	oldHCE := singleOptionValue[HcePerc]
	oldNNUEPercent := singleOptionValue[NnuePerc]
	oldPestoEval := pestoEval
	t.Cleanup(func() {
		nnueParams = oldParams
		nnue = oldNNUE
		singleOptionValue[HorizontalMirroring] = oldMirroring
		singleOptionValue[HcePerc] = oldHCE
		singleOptionValue[NnuePerc] = oldNNUEPercent
		pestoEval = oldPestoEval
	})

	singleOptionValue[HorizontalMirroring] = 0
	singleOptionValue[HcePerc] = 0
	singleOptionValue[NnuePerc] = 100
	pestoEval = false
	if !nnueLoadFromBytes(syntheticNNUEWithBias(3)) {
		t.Fatal("failed to load first synthetic NNUE")
	}

	var p Pos
	parseFEN(&p, startFEN)
	var ss SearchState
	if !ss.resetForSearch(&p) {
		t.Fatal("first search did not invalidate the unset evaluation regime")
	}
	if ss.resetForSearch(&p) {
		t.Fatal("unchanged evaluation regime requested another invalidation")
	}
	var first Accumulator
	ss.refreshPerspectiveWithFinny(&p, &first, White)
	if first.values[White][0] != 3 {
		t.Fatalf("first network bias: got %d, want 3", first.values[White][0])
	}

	if !nnueLoadFromBytes(syntheticNNUEWithBias(11)) {
		t.Fatal("failed to load second synthetic NNUE")
	}
	if !ss.resetForSearch(&p) {
		t.Fatal("NNUE reload did not invalidate cached search scores")
	}
	loadedParams := nnueParams
	loadedGeneration := nnue.generation
	if nnueLoadFromBytes([]byte{0}) {
		t.Fatal("accepted truncated NNUE data")
	}
	if !nnue.Loaded || nnueParams != loadedParams || nnue.generation != loadedGeneration {
		t.Fatal("failed NNUE reload replaced the last valid network")
	}
	if ss.resetForSearch(&p) {
		t.Fatal("failed NNUE reload changed the evaluation regime")
	}

	var got Accumulator
	ss.refreshPerspectiveWithFinny(&p, &got, White)
	var want Accumulator
	refreshPerspective(&p, &want, White)

	if got.values[White] != want.values[White] {
		t.Fatalf("Finny retained the previous network: got first neuron %d, want %d", got.values[White][0], want.values[White][0])
	}

	var kbn Pos
	parseFEN(&kbn, "k7/8/2K5/1NB5/8/8/8/8 w - - 0 1")
	if !ss.resetForSearch(&kbn) {
		t.Fatal("entering KBN mode did not invalidate cached search scores")
	}
	if !ss.resetForSearch(&p) {
		t.Fatal("leaving KBN mode did not invalidate cached search scores")
	}
}
