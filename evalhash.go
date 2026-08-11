package main

import (
	"runtime"
	"sync/atomic"
)

const evalHashUsed = uint64(1) << 32

// EvalHashEntry reuses the main TT's Entry layout
type EvalHashEntry = Entry

// PawnHashEntry uses a sequence counter to publish its multi-word payload.
type PawnHashEntry struct {
	version uint64
	key     uint64
	scoreMG uint64
	scoreEG uint64
	center  uint64
}

var evalTT []EvalHashEntry
var pawnTT []PawnHashEntry

func initEvalHash(size int) {
	if size <= 0 {
		evalTT = nil
		return
	}
	evalTT = make([]EvalHashEntry, size)
}

func clearEvalHash() {
	for i := range evalTT {
		storeEntry(&evalTT[i], 0, 0)
	}
}

func probeEvalHash(key uint64) (int, bool) {
	if len(evalTT) == 0 {
		return 0, false
	}

	e := &evalTT[key%uint64(len(evalTT))]
	data, hash := loadEntry(e)
	if data&evalHashUsed != 0 && hash^data == key {
		return int(int32(data)), true
	}
	return 0, false
}

func storeEvalHash(key uint64, score int) {
	if len(evalTT) == 0 {
		return
	}

	e := &evalTT[key%uint64(len(evalTT))]
	data := uint64(uint32(int32(score))) | evalHashUsed
	storeEntry(e, key, data)
}

// --- Pawn hash ---

func initPawnHash(size int) {
	if size <= 0 {
		pawnTT = nil
		return
	}
	pawnTT = make([]PawnHashEntry, size)
}

func clearPawnHash() {
	for i := range pawnTT {
		e := &pawnTT[i]
		for {
			version := atomic.LoadUint64(&e.version)
			if version&1 != 0 {
				runtime.Gosched()
				continue
			}
			if atomic.CompareAndSwapUint64(&e.version, version, 0) {
				break
			}
		}
	}
}

func packInt32Pair(first, second int) uint64 {
	return uint64(uint32(int32(first))) | uint64(uint32(int32(second)))<<32
}

func unpackInt32Pair(data uint64) (int, int) {
	return int(int32(data)), int(int32(data >> 32))
}

func probePawnHash(key uint64, data *EvalData) bool {
	if len(pawnTT) == 0 {
		return false
	}

	e := &pawnTT[key%uint64(len(pawnTT))]
	version := atomic.LoadUint64(&e.version)
	if version == 0 || version&1 != 0 {
		return false
	}

	storedKey := atomic.LoadUint64(&e.key)
	mg := atomic.LoadUint64(&e.scoreMG)
	eg := atomic.LoadUint64(&e.scoreEG)
	center := atomic.LoadUint64(&e.center)
	if atomic.LoadUint64(&e.version) != version || storedKey != key {
		return false
	}

	wscoreMG, bscoreMG := unpackInt32Pair(mg)
	wscoreEG, bscoreEG := unpackInt32Pair(eg)
	wCenter, bCenter := unpackInt32Pair(center)
	add(data, White, EvalPawns, wscoreMG, wscoreEG)
	add(data, Black, EvalPawns, bscoreMG, bscoreEG)
	data.center[White] = CenterType(wCenter)
	data.center[Black] = CenterType(bCenter)
	return true
}

func storePawnHash(key uint64, data *EvalData) {
	if len(pawnTT) == 0 {
		return
	}

	e := &pawnTT[key%uint64(len(pawnTT))]
	for {
		version := atomic.LoadUint64(&e.version)
		if version&1 != 0 {
			runtime.Gosched()
			continue
		}
		if !atomic.CompareAndSwapUint64(&e.version, version, version+1) {
			continue
		}

		atomic.StoreUint64(&e.key, key)
		atomic.StoreUint64(&e.scoreMG, packInt32Pair(data.mgScore[White][EvalPawns], data.mgScore[Black][EvalPawns]))
		atomic.StoreUint64(&e.scoreEG, packInt32Pair(data.egScore[White][EvalPawns], data.egScore[Black][EvalPawns]))
		atomic.StoreUint64(&e.center, packInt32Pair(int(data.center[White]), int(data.center[Black])))
		atomic.StoreUint64(&e.version, version+2)
		return
	}
}
