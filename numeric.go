// ================================================================
// NUMERIC
// ================================================================

package main

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func maxOf(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minOf(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func limitValue(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func scoreFromPerspective(score, side int) int {
	if side == White {
		return score
	}
	return -score
}

func roundFloat(x float64) int {
	if x >= 0 {
		return int(x + 0.5)
	}
	return int(x - 0.5)
}
