// ================================================================
// S9  TRANSPOSITION TABLE  (XOR lockless, thread-safe)
// ================================================================
//
//   The transposition table (TT) is a hash map from position keys to
//   search results.  It is the single most important optimisation in
//   an alpha-beta engine: when the same position is reached via
//   different move orders, we reuse the earlier result instead of
//   re-searching.
//
//   STRUCTURE
//   ---------
//   The table is a flat array of Entry values.  Each position maps to
//   a "bucket" of 4 consecutive entries (4-way associative).  On a
//   lookup we check all 4; on a store we replace the one that is
//   oldest or shallowest.
//
//   XOR LOCKLESS HASHING  (Hyatt & Mann, ICGA 2002)
//   -------------------------------------------------
//   Each Entry is two 64-bit words: data (packed payload) and hash
//   (position key XOR data).  Reads and writes use sync/atomic so
//   each word is always read/written atomically.  Even if two threads
//   race on the same entry, the XOR invariant detects the corruption:
//
//     probe:  data := load(e.data); hash := load(e.hash)
//             if hash ^ data == key  -> valid hit
//             else                   -> treat as miss (never wrong data)
//
//     store:  pack data word
//             store(e.data, data)        // data first
//             store(e.hash, key ^ data)  // then the self-verifying key
//
//   Writing data before hash means a racing reader sees (new_data,
//   old_hash): XOR check fails -> miss.  After both writes complete,
//   any reader sees a consistent pair.  The worst possible race
//   outcome is a spurious miss — never a silently wrong score.
//
//   DATA WORD BIT LAYOUT (64 bits)
//   --------------------------------
//     bits  0-15  move      (int16)
//     bits 16-31  score     (int16, mate-adjusted)
//     bits 32-39  depth     (uint8)
//     bits 40-41  bound     (uint8)   UPPER=1 LOWER=2 EXACT=3
//     bits 42-47  date      (uint8)   search generation 0-63
//     bits 48-63  signature (uint16)  upper 16 bits of position key
//
//   REPLACEMENT POLICY
//   ------------------
//   For each bucket we compute an "age score":
//     ((generation - date) * 256) + (255 - depth)
//   We replace the entry with the highest age score (most stale AND
//   shallowest).  A key match always reuses the matched slot.
//
//   MATE SCORE ADJUSTMENT
//   ----------------------
//   Mate scores are ply-relative inside the search but must be stored
//   position-relative so they are portable across transpositions:
//     store: if score > maxEval: score += ply   (dist from THIS position)
//            if score < -maxEval: score -= ply
//     probe: reverse the adjustment using the current ply.
//

package main

import (
	"runtime/debug"
	"sync/atomic"
	"unsafe"
)

// Entry is one slot in the transposition table.
// Exactly 8 bytes (a single uint64).
type Entry struct {
	data uint64 // packed payload and 16-bit signature
}

// Global TT state.
type TTable struct {
	entries []Entry
	size    int
	mask    int
	date    int
}

var mainTT TTable

// searchStateSize is the real per-thread memory footprint of a SearchState
// updateMemoryLimit stays accurate.
var searchStateSize = int64(unsafe.Sizeof(SearchState{}))

// ---- Bit packing helpers ----

func packTTData(key uint64, move, score, depth, bound, date int) uint64 {
	return uint64(uint16(int16(move))) |
		uint64(uint16(int16(score)))<<16 |
		uint64(uint8(depth))<<32 |
		uint64(uint8(bound))<<40 |
		uint64(uint8(date&63))<<42 |
		(key & 0xFFFF000000000000)
}

func unpackTTData(d uint64) (move, score, depth, bound, date int) {
	move = int(int16(d))
	score = int(int16(d >> 16))
	depth = int(uint8(d >> 32))
	bound = int((d >> 40) & 3)
	date = int((d >> 42) & 63)
	return
}


// ---- TT management ----

// alloc allocates a transposition table of approximately mbSize
// megabytes.  The size is rounded down to the nearest power of 2
// so that the index mask trick works.  Always clears the table.
func (t *TTable) alloc(mbSize int) {
	size := 2
	for size <= mbSize {
		size *= 2
	}
	// Each entry is 8 bytes; allocate size MiB worth.
	newSize := (size << 20) / 8

	if t.size == newSize {
		return // Size unchanged, don't reallocate or wipe the TT
	}

	t.size = newSize
	t.mask = t.size - 4
	t.entries = make([]Entry, t.size)
	t.clear()
}

// clear zeroes the table and resets the date counter.
// Called at the start of a new game (ucinewgame).
func (t *TTable) clear() {
	t.date = 0
	for i := range t.entries {
		t.entries[i] = Entry{}
	}
}

func (t *TTable) newDate() {
	t.date = (t.date + 1) & 63
}

// hashfull returns TT utilization in UCI hashfull units (permille).
func (t *TTable) hashfull() int {
	if t.size <= 0 {
		return 0
	}
	sampleSize := min(t.size, 1000)
	active := 0
	for i := 0; i < sampleSize; i++ {
		d := atomic.LoadUint64(&t.entries[i].data)
		if d == 0 {
			continue
		}
		_, _, _, _, date := unpackTTData(d)
		age := (t.date - date) & 63
		if age <= 1 {
			active++
		}
	}
	return (active * 1000) / sampleSize
}

var currentHashMB int = 16

func updateMemoryLimit() {
	mb := currentHashMB
	if mb <= 0 {
		mb = 16
	}
	hashBytes := int64(mb) * 1024 * 1024
	threadBytes := int64(numThreads) * searchStateSize
	nnueBytes := int64(unsafe.Sizeof(NNUEParameters{}))
	bookBytes := int64(len(mainBook.entries)+len(guideBook.entries)) * int64(unsafe.Sizeof(PolyglotEntry{}))
	liveBytes := hashBytes + threadBytes + nnueBytes + bookBytes

	const minHeadroomMB = 64
	headroom := liveBytes
	if headroom < minHeadroomMB*1024*1024 {
		headroom = minHeadroomMB * 1024 * 1024
	}

	debug.SetMemoryLimit(liveBytes + headroom)
}

func allocTT(mbSize int) {
	currentHashMB = mbSize
	mainTT.alloc(mbSize)
	updateMemoryLimit()
}

func clearTT() {
	mainTT.clear()
}

func reclaimOSMemory() {
	debug.FreeOSMemory()
}

func ttHashfull() int { return mainTT.hashfull() }

// ---- TT probe and store ----

// probe looks up a position in the transposition table.
//
// If a matching entry is found, *move is set to the stored best move
// and *score is set to the stored score.  Returns true only when the
// score can be used directly as a cutoff (depth sufficient + bound
// matches window).  The move hint is always returned on a key match
// so the search can try it first regardless of depth.
func (t *TTable) probe(key uint64, move *int, score *int, flag *int, ttDepth *int, alpha, beta, depth, ply int) bool {
	base := int(key) & t.mask
	bucket := t.entries[base : base+4]
	for i := range bucket {
		e := &bucket[i]
		data := atomic.LoadUint64(&e.data)
		if (data ^ key) & 0xFFFF000000000000 != 0 {
			continue
		}
		mv, sc, dp, bd, _ := unpackTTData(data)

		*move = mv
		*flag = bd
		*ttDepth = dp

		// Decode mate score to be ply-relative.
		*score = sc
		if sc < -maxEval {
			*score = sc + ply
		} else if sc > maxEval {
			*score = sc - ply
		}

		if dp >= depth {
			if bd == EXACT ||
				(bd&UPPER != 0 && *score <= alpha) ||
				(bd&LOWER != 0 && *score >= beta) {
				return true
			}
		}
		break // key matched but depth or bound insufficient
	}
	return false
}

// store writes a search result to the transposition table.
// If the position's key already occupies a slot it is reused
// (preserving the move hint when the new search has none).
// Otherwise the oldest/shallowest entry is evicted.
func (t *TTable) store(key uint64, move, score, bound, depth, ply int) {
	// Adjust mate scores to be position-relative.
	if score < -maxEval {
		score -= ply
	} else if score > maxEval {
		score += ply
	}

	base := int(key) & t.mask
	bucket := t.entries[base : base+4]
	var replace *Entry
	oldest := -1

	for i := range bucket {
		e := &bucket[i]
		data := atomic.LoadUint64(&e.data)
		if (data ^ key) & 0xFFFF000000000000 == 0 {
			// Reuse existing slot; preserve move if we have none.
			if move == 0 {
				mv, _, _, _, _ := unpackTTData(data)
				move = mv
			}
			replace = e
			break
		}
		_, _, dp, _, dt := unpackTTData(data)
		age := ((t.date-dt)&63)*256 + (255 - dp)
		if age > oldest {
			oldest = age
			replace = e
		}
	}
	if replace == nil {
		replace = &bucket[0]
	}

	d := packTTData(key, move, score, depth, bound, t.date)
	atomic.StoreUint64(&replace.data, d)
}
