// ================================================================
// CORRHIST
// ================================================================
//
// Corrhist is a shorthand for *Static Evaluation Correction History*
// - an algorithm that adjusts static eval based on how it differs
// from search result. Average difference is accumulated in the tables
// indexed by hash keys related to certain aspects of position (pawn
// placement, non-pawn placement etc.) and added to static eval.
// The big idea is that such correction captures things missed by
// evaluation function.
//
// Good external descriptions of the algorithm can be found at
// https://int0x80.ca/posts/chess-engines/19-corrhist or
// https://www.chessprogramming.org/Static_Evaluation_Correction_History
//
// Rodent uses:
// - pawn corection history
// - non-pawn correction history
// - majors history (including king position)
// - minors history (including king position)

package main

// Correction history constants.
const (
	corrHistSize        = 16384              // number of entries per side
	corrHistGrain       = 512                // scaling factor for diff
	corrHistWeightScale = 256                // weight divisor
	corrHistMax         = corrHistGrain * 32 // absolute clamp
)

// Corrhist tables are per-thread fields inside SearchState (thread.go).

// getCorrection returns the total correction value for the position,
// combining pawn and non-pawn correction history.
func (ss *SearchState) getCorrection(p *Pos) int {
	side := p.side

	// pawn corrhist
	pawnIdx := int((p.pawnKey[White] ^ p.pawnKey[Black]) % corrHistSize)
	corr := int(ss.pawnCorrHist[side][pawnIdx] / corrHistGrain)

	// non pawn corrhist
	corr += int(ss.nonPawnCorrHist[White][side][int(p.nonPawnKey[White]%corrHistSize)]) / corrHistGrain
	corr += int(ss.nonPawnCorrHist[Black][side][int(p.nonPawnKey[Black]%corrHistSize)]) / corrHistGrain

	// minor corrhist (halved)
	corr += (int(ss.minorCorrHist[White][side][int(p.minorKey[White]%corrHistSize)]) / corrHistGrain) / 2
	corr += (int(ss.minorCorrHist[Black][side][int(p.minorKey[Black]%corrHistSize)]) / corrHistGrain) / 2

	// major corrhist (halved)
	corr += (int(ss.majorCorrHist[White][side][int(p.majorKey[White]%corrHistSize)]) / corrHistGrain) / 2
	corr += (int(ss.majorCorrHist[Black][side][int(p.majorKey[Black]%corrHistSize)]) / corrHistGrain) / 2

	return corr
}

// updateCorrEntry applies a weighted update to a single correction history entry.
func updateCorrEntry(entry *int16, newWeight, scaledDiff int) {
	old := int(*entry)
	update := old*(corrHistWeightScale-newWeight) + scaledDiff*newWeight
	update /= corrHistWeightScale
	update = max(-corrHistMax, min(corrHistMax, update))
	*entry = int16(update)
}

// addCorrection updates all correction history entries for the position.
func (ss *SearchState) addCorrection(p *Pos, depth, diff int) {
	side := p.side
	newWeight := min(16, 1+depth)
	scaledDiff := diff * corrHistGrain

	// pawn corrhist
	pawnIdx := int((p.pawnKey[White] ^ p.pawnKey[Black]) % corrHistSize)
	updateCorrEntry(&ss.pawnCorrHist[side][pawnIdx], newWeight, scaledDiff)

	// non pawn corrhist
	updateCorrEntry(&ss.nonPawnCorrHist[White][side][int(p.nonPawnKey[White]%corrHistSize)], newWeight, scaledDiff)
	updateCorrEntry(&ss.nonPawnCorrHist[Black][side][int(p.nonPawnKey[Black]%corrHistSize)], newWeight, scaledDiff)

	// minor corrhist
	updateCorrEntry(&ss.minorCorrHist[White][side][int(p.minorKey[White]%corrHistSize)], newWeight, scaledDiff)
	updateCorrEntry(&ss.minorCorrHist[Black][side][int(p.minorKey[Black]%corrHistSize)], newWeight, scaledDiff)

	// major corrhist
	updateCorrEntry(&ss.majorCorrHist[White][side][int(p.majorKey[White]%corrHistSize)], newWeight, scaledDiff)
	updateCorrEntry(&ss.majorCorrHist[Black][side][int(p.majorKey[Black]%corrHistSize)], newWeight, scaledDiff)
}
