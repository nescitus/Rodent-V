package main

import "unsafe"

func addSingleScalar(
	a, w *int16,
) {
	acc := unsafe.Slice(a, NNUEHiddenSize)
	ww := unsafe.Slice(w, NNUEHiddenSize)

	for i := 0; i < NNUEHiddenSize; i++ {
		acc[i] += ww[i]
	}
}

func addScalar(
	a0, a1 *int16,
	w0, w1 *int16,
) {
	acc0 := unsafe.Slice(a0, NNUEHiddenSize)
	acc1 := unsafe.Slice(a1, NNUEHiddenSize)
	ww0 := unsafe.Slice(w0, NNUEHiddenSize)
	ww1 := unsafe.Slice(w1, NNUEHiddenSize)

	for i := 0; i < NNUEHiddenSize; i++ {
		acc0[i] += ww0[i]
		acc1[i] += ww1[i]
	}
}

func moveScalar(
	a0, a1 *int16,
	wFrom0, wTo0 *int16,
	wFrom1, wTo1 *int16,
) {
	acc0 := unsafe.Slice(a0, NNUEHiddenSize)
	acc1 := unsafe.Slice(a1, NNUEHiddenSize)

	from0 := unsafe.Slice(wFrom0, NNUEHiddenSize)
	to0 := unsafe.Slice(wTo0, NNUEHiddenSize)

	from1 := unsafe.Slice(wFrom1, NNUEHiddenSize)
	to1 := unsafe.Slice(wTo1, NNUEHiddenSize)

	for i := 0; i < NNUEHiddenSize; i++ {
		acc0[i] += to0[i] - from0[i]
		acc1[i] += to1[i] - from1[i]
	}
}

func captureScalar(
	a0, a1 *int16,
	wTo0, wFrom0, wCap0 *int16,
	wTo1, wFrom1, wCap1 *int16,
) {
	acc0 := unsafe.Slice(a0, NNUEHiddenSize)
	acc1 := unsafe.Slice(a1, NNUEHiddenSize)

	to0 := unsafe.Slice(wTo0, NNUEHiddenSize)
	from0 := unsafe.Slice(wFrom0, NNUEHiddenSize)
	cap0 := unsafe.Slice(wCap0, NNUEHiddenSize)

	to1 := unsafe.Slice(wTo1, NNUEHiddenSize)
	from1 := unsafe.Slice(wFrom1, NNUEHiddenSize)
	cap1 := unsafe.Slice(wCap1, NNUEHiddenSize)

	for i := range acc0 {
		acc0[i] += to0[i] - from0[i] - cap0[i]
		acc1[i] += to1[i] - from1[i] - cap1[i]
	}
}

func castleScalar(
	a0, a1 *int16,
	wKFrom0, wKTo0, wRFrom0, wRTo0 *int16,
	wKFrom1, wKTo1, wRFrom1, wRTo1 *int16,
) {
	acc0 := unsafe.Slice(a0, NNUEHiddenSize)
	acc1 := unsafe.Slice(a1, NNUEHiddenSize)

	kFrom0 := unsafe.Slice(wKFrom0, NNUEHiddenSize)
	kTo0 := unsafe.Slice(wKTo0, NNUEHiddenSize)
	rFrom0 := unsafe.Slice(wRFrom0, NNUEHiddenSize)
	rTo0 := unsafe.Slice(wRTo0, NNUEHiddenSize)

	kFrom1 := unsafe.Slice(wKFrom1, NNUEHiddenSize)
	kTo1 := unsafe.Slice(wKTo1, NNUEHiddenSize)
	rFrom1 := unsafe.Slice(wRFrom1, NNUEHiddenSize)
	rTo1 := unsafe.Slice(wRTo1, NNUEHiddenSize)

	for i := range acc0 {
		acc0[i] += kTo0[i] - kFrom0[i] +
			rTo0[i] - rFrom0[i]

		acc1[i] += kTo1[i] - kFrom1[i] +
			rTo1[i] - rFrom1[i]
	}
}

func evalScalar(
	a0, a1 *int16,
	w0, w1 *int16,
	sum *int32,
) {
	acc0 := unsafe.Slice(a0, NNUEHiddenSize)
	acc1 := unsafe.Slice(a1, NNUEHiddenSize)
	weights0 := unsafe.Slice(w0, NNUEHiddenSize)
	weights1 := unsafe.Slice(w1, NNUEHiddenSize)

	var sum0 int32
	var sum1 int32
	var sum2 int32
	var sum3 int32

	for i := 0; i < NNUEHiddenSize; i += 4 {
		sum0 += screluWeighted(acc0[i+0], weights0[i+0]) +
			screluWeighted(acc1[i+0], weights1[i+0])

		sum1 += screluWeighted(acc0[i+1], weights0[i+1]) +
			screluWeighted(acc1[i+1], weights1[i+1])

		sum2 += screluWeighted(acc0[i+2], weights0[i+2]) +
			screluWeighted(acc1[i+2], weights1[i+2])

		sum3 += screluWeighted(acc0[i+3], weights0[i+3]) +
			screluWeighted(acc1[i+3], weights1[i+3])
	}

	*sum = sum0 + sum1 + sum2 + sum3
}
