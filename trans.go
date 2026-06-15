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
//     bits  0-15  move  (int16)
//     bits 16-31  score (int16, mate-adjusted)
//     bits 32-39  depth (uint8)
//     bits 40-47  bound (uint8)   UPPER=1 LOWER=2 EXACT=3
//     bits 48-55  date  (uint8)   search generation 0-255
//     bits 56-63  (reserved, zero)
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

import "sync/atomic"

// Entry is one slot in the transposition table.
// Exactly 16 bytes (two aligned uint64s).
type Entry struct {
	data uint64 // packed payload (see bit layout above)
	hash uint64 // position key XOR data (self-verifying)
}

// Global TT state.
var (
	tt     []Entry // the flat entry array
	ttSize int     // total number of entries (power of 2)
	ttMask int     // index mask = ttSize - 4 (aligned buckets)
	ttDate int     // current search generation (0-255)
)

// ---- Bit packing helpers ----

func packTTData(move, score, depth, bound, date int) uint64 {
	return uint64(uint16(int16(move))) |
		uint64(uint16(int16(score)))<<16 |
		uint64(uint8(depth))<<32 |
		uint64(uint8(bound))<<40 |
		uint64(uint8(date))<<48
}

func unpackTTData(d uint64) (move, score, depth, bound, date int) {
	move = int(int16(d))
	score = int(int16(d >> 16))
	depth = int(uint8(d >> 32))
	bound = int(uint8(d >> 40))
	date = int(uint8(d >> 48))
	return
}

// ---- Atomic entry access ----

func loadEntry(e *Entry) (data, hash uint64) {
	data = atomic.LoadUint64(&e.data)
	hash = atomic.LoadUint64(&e.hash)
	return
}

func storeEntry(e *Entry, key, data uint64) {
	atomic.StoreUint64(&e.data, data)     // data first
	atomic.StoreUint64(&e.hash, key^data) // then self-verifying key
}

// ---- TT management ----

// allocTT allocates a transposition table of approximately mbSize
// megabytes.  The size is rounded down to the nearest power of 2
// so that the index mask trick works.  Always clears the table.
func allocTT(mbSize int) {
	size := 2
	for size <= mbSize {
		size *= 2
	}
	// Each entry is 16 bytes; allocate (size/2) MiB worth.
	ttSize = ((size / 2) << 20) / 16
	ttMask = ttSize - 4
	tt = make([]Entry, ttSize)
	clearTT()
}

// clearTT zeroes the table and resets the date counter.
// Called at the start of a new game (ucinewgame).
func clearTT() {
	ttDate = 0
	for i := range tt {
		tt[i] = Entry{}
	}
}

// ttHashfull returns TT utilization in UCI hashfull units (permille).
func ttHashfull() int {
	if ttSize <= 0 {
		return 0
	}
	sampleSize := min(ttSize, 1000)
	active := 0
	for i := 0; i < sampleSize; i++ {
		d := atomic.LoadUint64(&tt[i].data)
		if d == 0 {
			continue
		}
		_, _, _, _, date := unpackTTData(d)
		age := (ttDate - date) & 255
		if age <= 1 {
			active++
		}
	}
	return (active * 1000) / sampleSize
}

// ---- TT probe and store ----

// probeTT looks up a position in the transposition table.
//
// If a matching entry is found, *move is set to the stored best move
// and *score is set to the stored score.  Returns true only when the
// score can be used directly as a cutoff (depth sufficient + bound
// matches window).  The move hint is always returned on a key match
// so the search can try it first regardless of depth.
func probeTT(key uint64, move *int, score *int, flag *int, ttDepth *int, alpha, beta, depth, ply int) bool {
	base := int(key) & ttMask
	bucket := tt[base : base+4]
	for i := range bucket {
		e := &bucket[i]
		data, hash := loadEntry(e)
		if hash^data != key {
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

// storeTT writes a search result to the transposition table.
// If the position's key already occupies a slot it is reused
// (preserving the move hint when the new search has none).
// Otherwise the oldest/shallowest entry is evicted.
func storeTT(key uint64, move, score, bound, depth, ply int) {
	// Adjust mate scores to be position-relative.
	if score < -maxEval {
		score -= ply
	} else if score > maxEval {
		score += ply
	}

	base := int(key) & ttMask
	bucket := tt[base : base+4]
	var replace *Entry
	oldest := -1

	for i := range bucket {
		e := &bucket[i]
		data, hash := loadEntry(e)
		if hash^data == key {
			// Reuse existing slot; preserve move if we have none.
			if move == 0 {
				mv, _, _, _, _ := unpackTTData(data)
				move = mv
			}
			replace = e
			break
		}
		_, _, dp, _, dt := unpackTTData(data)
		age := ((ttDate-dt)&255)*256 + (255 - dp)
		if age > oldest {
			oldest = age
			replace = e
		}
	}
	if replace == nil {
		replace = &bucket[0]
	}

	d := packTTData(move, score, depth, bound, ttDate)
	storeEntry(replace, key, d)
}
