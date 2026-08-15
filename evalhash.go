package main

type EvalHashEntry struct {
	key   uint64
	score int
	used  bool
}

type PawnHashEntry struct {
	key     uint64
	scoreMG [2]int
	scoreEG [2]int
	center  [2]int
	used    bool
}

// evalHashTable and pawnHashTable are separated out from bare package
// globals so datagen can give each of its threads its own private table
// instead of having every thread hammer one shared table with plain,
// non-atomic reads/writes. A torn {key,score} write there isn't just a
// missed cache entry: the wrong score can be returned for a key and, in
// datagen, get written straight into training data.
//
// The process-wide UCI/search path is unaffected: SearchState.evalHash
// and .pawnHash default to nil, and evalHashFor/pawnHashFor resolve a
// nil table to the single global instance below — byte-identical to the
// pre-refactor behavior.
type evalHashTable struct {
	entries []EvalHashEntry
	mask    uint64
}

type pawnHashTable struct {
	entries []PawnHashEntry
	mask    uint64
}

func newEvalHashTable(size int) *evalHashTable {
	if size <= 0 {
		return &evalHashTable{}
	}
	return &evalHashTable{entries: make([]EvalHashEntry, size), mask: uint64(size - 1)}
}

func newPawnHashTable(size int) *pawnHashTable {
	if size <= 0 {
		return &pawnHashTable{}
	}
	return &pawnHashTable{entries: make([]PawnHashEntry, size), mask: uint64(size - 1)}
}

func (t *evalHashTable) probe(key uint64) (int, bool) {
	if len(t.entries) == 0 {
		return 0, false
	}
	e := t.entries[key&t.mask]
	if e.used && e.key == key {
		return e.score, true
	}
	return 0, false
}

func (t *evalHashTable) store(key uint64, score int) {
	if len(t.entries) == 0 {
		return
	}
	t.entries[key&t.mask] = EvalHashEntry{key: key, score: score, used: true}
}

func (t *evalHashTable) clear() {
	for i := range t.entries {
		t.entries[i] = EvalHashEntry{}
	}
}

func (t *pawnHashTable) probe(key uint64) (int, int, int, int, int, int, bool) {
	if len(t.entries) == 0 {
		return 0, 0, 0, 0, int(Undefined), int(Undefined), false
	}
	e := t.entries[key&t.mask]
	if e.used && e.key == key {
		return e.scoreMG[White], e.scoreMG[Black], e.scoreEG[White], e.scoreEG[Black], e.center[White], e.center[Black], true
	}
	return 0, 0, 0, 0, int(Undefined), int(Undefined), false
}

func (t *pawnHashTable) store(key uint64, wscoreMG, bscoreMG, wscoreEG, bscoreEG, wCenter, bCenter int) {
	if len(t.entries) == 0 {
		return
	}
	addr := key & t.mask
	t.entries[addr] = PawnHashEntry{
		key:     key,
		scoreMG: [2]int{wscoreMG, bscoreMG},
		scoreEG: [2]int{wscoreEG, bscoreEG},
		center:  [2]int{wCenter, bCenter},
		used:    true,
	}
}

func (t *pawnHashTable) clear() {
	for i := range t.entries {
		t.entries[i] = PawnHashEntry{}
	}
}

// globalEvalHash and globalPawnHash are the tables used by the UCI engine,
// bench, the tuner, and every caller that doesn't set up its own private
// table via SearchState.evalHash / .pawnHash.
var globalEvalHash evalHashTable
var globalPawnHash pawnHashTable

// evalHashFor and pawnHashFor resolve which table a call should use: the
// SearchState's private table if it has one (batch workers), otherwise the
// shared global table (everything else, unchanged from before this type
// existed). ss may itself be nil (the tuner evaluates positions with no
// SearchState at all), which also resolves to the global table.
func evalHashFor(ss *SearchState) *evalHashTable {
	if ss != nil && ss.evalHash != nil {
		return ss.evalHash
	}
	return &globalEvalHash
}

func pawnHashFor(ss *SearchState) *pawnHashTable {
	if ss != nil && ss.pawnHash != nil {
		return ss.pawnHash
	}
	return &globalPawnHash
}

func initEvalHash(size int) {
	if size <= 0 {
		globalEvalHash = evalHashTable{}
		return
	}
	globalEvalHash = evalHashTable{entries: make([]EvalHashEntry, size), mask: uint64(size - 1)}
}

func clearEvalHash() {
	globalEvalHash.clear()
}

func initPawnHash(size int) {
	if size <= 0 {
		globalPawnHash = pawnHashTable{}
		return
	}
	globalPawnHash = pawnHashTable{entries: make([]PawnHashEntry, size), mask: uint64(size - 1)}
}

func clearPawnHash() {
	globalPawnHash.clear()
}
