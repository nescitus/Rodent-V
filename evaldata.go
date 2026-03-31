package main

import "fmt"

type EvalComponent int

const (
	EvalMaterial EvalComponent = iota
	EvalPst
	EvalMobility
	EvalSafety
	EvalPawns
	EvalPassers
	EvalOther
	EvalComponentN
)

var evalComponentName = [EvalComponentN]string{
	EvalMaterial: "Material",
	EvalPst:      "PST",
	EvalMobility: "Mobility",
	EvalSafety:   "Safety",
	EvalPawns:    "Pawns",
	EvalPassers:  "Passers",
	EvalOther:    "Other",
}


// A struct serving as a scratchpad for evaluation, filled with data
// gathered in the process.
type EvalData struct {
	phase   int
	mgScore [2][EvalComponentN]int
	egScore [2][EvalComponentN]int
	attacks[2] int
	pawnControl[2] uint64
}

func (e *EvalData) sumMg(side int) int {
	sum := 0
	for c := EvalComponent(0); c < EvalComponentN; c++ {
		sum += e.mgScore[side][c]
	}
	return sum
}

func (e *EvalData) sumEg(side int) int {
	sum := 0
	for c := EvalComponent(0); c < EvalComponentN; c++ {
		sum += e.egScore[side][c]
	}
	return sum
}

func nameEvalComponent(c EvalComponent) string {
		if c < 0 || c >= EvalComponentN {
		return "Unknown"
	}
	return evalComponentName[c]
}

func interpScore(mg, eg, phase int) int {
	if phase > 24 {
		phase = 24
	}
	return (mg*phase + eg*(24-phase)) / 24
}

func (e *EvalData) PrintEvalDetails(p *Pos) {

	phase := e.phase
	if phase > 24 {
		phase = 24
	}

	fmt.Printf("Evaluation breakdown (phase = %d/24)\n", phase)
	fmt.Println("---------------------------------------------------------------")
	fmt.Printf("%-10s %8s %8s %8s %8s %8s %8s %8s\n",
		"Term", "MG W", "MG B", "MG diff", "EG W", "EG B", "EG diff", "Blend")
	fmt.Println("---------------------------------------------------------------")

	totalMG := 0
	totalEG := 0

	for c := EvalComponent(0); c < EvalComponentN; c++ {
		mgW := e.mgScore[White][c]
		mgB := e.mgScore[Black][c]
		egW := e.egScore[White][c]
		egB := e.egScore[Black][c]

		mgDiff := mgW - mgB
		egDiff := egW - egB
		blend := interpScore(mgDiff, egDiff, phase)

		totalMG += mgDiff
		totalEG += egDiff

		fmt.Printf("%-10s %8d %8d %8d %8d %8d %8d %8d\n",
			nameEvalComponent(c),
			mgW, mgB, mgDiff,
			egW, egB, egDiff,
			blend)
	}

	fmt.Println("---------------------------------------------------------------")

	totalBlend := interpScore(totalMG, totalEG, phase)
	stmBlend := totalBlend
	if p.side == Black {
		stmBlend = -stmBlend
	}

	fmt.Printf("%-10s %8s %8s %8d %8s %8s %8d %8d\n",
		"TOTAL", "", "", totalMG, "", "", totalEG, totalBlend)
	fmt.Println("---------------------------------------------------------------")
	fmt.Printf("From White's point of view: %d cp\n", totalBlend)
	fmt.Printf("From side-to-move point of view: %d cp\n", stmBlend)
	fmt.Println()
}