// ================================================================
// FITNESS
// ================================================================

package main

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"strings"
)

// texelFitFile calculates Texel mean squared error for all positions
// in an EPD file and returns the fit. Lower is better. Can be used
// for trying out a few values of a new HCE parameter, without using
// entire tuning infrastructure.
//
// eval must return a side-to-move-relative score.
func texelFitFile(path string, eval func(*Pos) int) (float64, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	const k = 1.335

	var p Pos
	var sum float64
	count := 0

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var result float64
		switch {
		case strings.Contains(line, "1/2-1/2"):
			result = 0.5
		case strings.Contains(line, "1-0"):
			result = 1.0
		case strings.Contains(line, "0-1"):
			result = 0.0
		default:
			continue // no usable game result
		}

		parseFEN(&p, line)

		score := eval(&p)

		// Texel result is White-relative, while evaluation is
		// side-to-move-relative.
		if p.side == Black {
			score = -score
		}

		expected := texelSigmoid(score, k)
		diff := result - expected
		sum += diff * diff
		count++
	}

	if err := scanner.Err(); err != nil {
		return 0, err
	}

	if count == 0 {
		return 0, fmt.Errorf("no scored positions in %q", path)
	}

	return 1000.0 * sum / float64(count), nil
}

func texelSigmoid(score int, k float64) float64 {
	exponent := -(k * float64(score) / 400.0)
	return 1.0 / (1.0 + math.Pow(10.0, exponent))
}