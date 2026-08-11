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

// measureScale measures scale of a net against a large epd set
func measureScale(path string) {
	file, err := os.Open(path)
	if err != nil {
		fmt.Println("info string Error opening file:", err)
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var totalAbs int64
	var count int64
	var acc Accumulator

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var p Pos
		parseFEN(&p, line)

		refresh(&p, &acc)
		eval := acc.getEval(&p, p.side)

		absEval := eval
		if absEval < 0 {
			absEval = -absEval
		}

		totalAbs += int64(absEval)
		count++

		if count%100000 == 0 {
			fmt.Printf("info string Processed %d positions...\n", count)
		}
	}

	if count > 0 {
		absMean := float64(totalAbs) / float64(count)
		fmt.Printf("info string Positions: %d\n", count)
		fmt.Printf("info string Average Absolute Eval: %.2f\n", absMean)
		fmt.Printf("info string Current NnueScale: %d\n", singleOptionValue[NnueScale])
	} else {
		fmt.Println("info string No valid positions found.")
	}
}

// Old function to adjust material and pst values. It assumes
// we want piece/square tables zero-centered and reminder should
// become a part of piece values.
func printPSToffsets(pst *[6][64]int) {
	fmt.Printf("offsets to move into material:\n")

	for piece := 0; piece < 6; piece++ {
		sum := 0
		for sq := 0; sq < 64; sq++ {
			sum += pst[piece][sq]
		}

		avg := float64(sum) / 64.0
		fmt.Printf("%s: sum = %d, avg = %.3f, material correction ~= %+d\n",
			pstLabels[piece], sum, avg, roundFloat(avg))
	}
}