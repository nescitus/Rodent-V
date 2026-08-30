package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// augmentEPDWings writes every original EPD record.
// If castling rights are "-", it also writes a left-right mirrored copy.
func augmentEPDWings(inputFile, outputFile string) error {
	in, err := os.Open(inputFile)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(outputFile)
	if err != nil {
		return err
	}
	defer out.Close()

	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 1<<20), 1<<20)

	w := bufio.NewWriter(out)
	defer w.Flush()

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}

		// Always save original.
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}

		mirrored, ok := mirrorEPDWings(line)
		if ok {
			if _, err := fmt.Fprintln(w, mirrored); err != nil {
				return err
			}
		}
	}

	return sc.Err()
}

func mirrorEPDWings(line string) (string, bool) {
	fields := strings.Fields(line)
	if len(fields) < 5 {
		return "", false
	}

	// Only augment positions without castling rights.
	if fields[2] != "-" {
		return "", false
	}

	ranks := strings.Split(fields[0], "/")
	if len(ranks) != 8 {
		return "", false
	}

	for r := 0; r < 8; r++ {
		expanded, ok := expandEPDRank(ranks[r])
		if !ok {
			return "", false
		}

		// Mirror a <-> h, b <-> g, ...
		for i, j := 0, 7; i < j; i, j = i+1, j-1 {
			expanded[i], expanded[j] = expanded[j], expanded[i]
		}

		ranks[r] = compressEPDRank(expanded)
	}

	fields[0] = strings.Join(ranks, "/")

	// Mirror EP square file, if present.
	if fields[3] != "-" {
		ep := fields[3]
		if len(ep) != 2 ||
			ep[0] < 'a' || ep[0] > 'h' ||
			ep[1] < '1' || ep[1] > '8' {
			return "", false
		}

		file := byte('h' - (ep[0] - 'a'))
		fields[3] = string([]byte{file, ep[1]})
	}

	return strings.Join(fields, " "), true
}

// Expands e.g. "2n2rk1" into 8 board characters,
// with '.' representing empty squares.
func expandEPDRank(s string) ([]byte, bool) {
	row := make([]byte, 0, 8)

	for i := 0; i < len(s); i++ {
		c := s[i]

		if c >= '1' && c <= '8' {
			n := int(c - '0')
			for j := 0; j < n; j++ {
				row = append(row, '.')
			}
		} else {
			row = append(row, c)
		}
	}

	if len(row) != 8 {
		return nil, false
	}

	return row, true
}

func compressEPDRank(row []byte) string {
	var b strings.Builder
	empty := 0

	flushEmpty := func() {
		if empty > 0 {
			b.WriteByte(byte('0' + empty))
			empty = 0
		}
	}

	for _, c := range row {
		if c == '.' {
			empty++
		} else {
			flushEmpty()
			b.WriteByte(c)
		}
	}

	flushEmpty()
	return b.String()
}