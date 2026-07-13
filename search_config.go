package main

// Search config.
// Cost of disabling is measured by 1000 game matches at 16 + 0.16
const (
	useChecksInQs        = false
	adjustEvalByCorrhist = true
	adjustEvalByTT       = false

	useRFP       = true
	rfpMaxDepth  = 7        //
	rfpMargin    = 80       // centipawns per depth for reverse futility pruning (not improving)
	rfpImpMargin = 60       // centipawns per depth for reverse futility pruning (improving)
	noEval       = -inf - 1 // sentinel: no static eval stored for this ply (in check)

	useRazoring   = true // cost of disabling: 4 Elo
	maxRazorDepth = 3
	razorMargin   = 300 // centipawns per depth for razoring

	useNULL           = true
	nmpMinDepth       = 2
	nmpBaseReduction  = 3 // base ply reduction for null-move pruning
	nmpDepthReduction = 4 // depth divisor for depth-scaled NMP reduction
	useVerification   = false
	minVerDepth       = 6
	verReduction      = 4

	useProbcut       = true // cost of disabling: 0 Elo
	probcutMargin    = 120  // extra margin above beta for ProbCut verification
	probcutMinDepth  = 6    // only apply ProbCut when enough depth remains
	probcutReduction = 2    // depth reduction used by the reduced verification search

	useIIR      = true
	IIRmaxDepth = 4

	useSingularExt = true
	useDoubleExt   = true
	seBetaMargin   = 6  // centipawns per ply subtracted from ttScore for singular verification
	seDoubleMargin = 64 // extra centipawns below singular beta needed for a double extension

	useLMP           = true
	LMPdepth         = 9
	useLmpTable      = true
	LMPnormalStep    = 4
	LMPimprovingStep = 6

	useFutility = true // cost of disabling: 20 Elo
	fpMargin    = 120  // centipawns per depth for main-search move-loop futility pruning
	fpMaxDepth  = 4    // only prune quiet moves by futility at shallow depth

	useLMR          = true
	LMRnonImproving = true
	lmrDivisor      = 2.0
	lmrLinear       = 1.0
	lmrMax          = 10
	minLmrDepth     = 3
	minLmrMove      = 4

	qsFpPawnMargin  = 300 // qs futility margin when capturing a pawn (passers warrant extra slack)
	qsFpPieceMargin = 200 // qs futility margin when capturing a piece
	qsLmpLimit      = 2   // max captures tried per qs node (outside check) to cap explosion

)

var lmp = [2][11]int{
	{0, 2, 3, 5, 9, 13, 18, 25, 34, 45, 55},
	{0, 5, 6, 9, 14, 21, 30, 41, 55, 69, 84},
}

// TEST POSITION 6k1/ppp2p1p/6p1/3p4/3n4/4B2P/P1P1rPP1/3n2K1 b - - 1 20 ???

// m8
// TEST POSITION r2q1rk1/1b3p1p/p3pPp1/2ppP3/7R/1PN1B1R1/1PP2P1P/4K3 w - - 1 3
// info depth 26 seldepth 45 time 28206 nodes 18186988 nps 644791 hashfull 1000 score mate 8 pv h4h7 g8h7 g3h3 h7g8 e3h6 d8c7 f2f4 c7a5 h6g7 a5a1 e1d2 a1d1 c3d1 d5d4
