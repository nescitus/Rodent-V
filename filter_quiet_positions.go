package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
)

const (
	maxEvalQSDifference = 50
	progressInterval    = 50000
)

type filterResult uint8

const (
	filterAccepted filterResult = iota
	filterInCheck
	filterNoisy
)

func filterQuietBulletFile(inputPath string, outputPath string) error {
	in, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("open input: %w", err)
	}
	defer in.Close()

	out, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	defer out.Close()

	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	writer := bufio.NewWriter(out)

	type job struct {
		line string
	}
	type result struct {
		line   string
		status filterResult
	}

	jobs := make(chan job, 10000)
	results := make(chan result, 10000)

	var wg sync.WaitGroup
	numWorkers := 16 // parallelize 16x

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var ss SearchState
			ss.tt = new(TTable)
			ss.tt.alloc(1)
			ss.evalHash = newEvalHashTable(128 * 128)
			ss.pawnHash = newPawnHashTable(64 * 128)
			var p Pos
			for j := range jobs {
				res := result{line: j.line, status: filterNoisy}
				fields := strings.Split(j.line, "|")
				if len(fields) >= 3 {
					fen := strings.TrimSpace(fields[0])
					status, _ := testQuietPositionWithState(fen, &p, &ss)
					res.status = status
				}
				results <- res
			}
		}()
	}

	var totalPositions int64
	var acceptedCount int64
	var inCheckCount int64
	var noisyCount int64

	// read lines
	go func() {
		for scanner.Scan() {
			line := scanner.Text()
			if strings.TrimSpace(line) == "" {
				continue
			}
			jobs <- job{line: line}
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()

	// process results
	for r := range results {
		totalPositions++
		switch r.status {
		case filterAccepted:
			fmt.Fprintln(writer, r.line)
			acceptedCount++
		case filterInCheck:
			inCheckCount++
		case filterNoisy:
			noisyCount++
		}

		if totalPositions%progressInterval == 0 {
			writer.Flush()
			fmt.Printf("checked %d, accepted %d (%.1f%%), noisy %d, in check %d\n",
				totalPositions, acceptedCount, percentage(int(acceptedCount), int(totalPositions)), noisyCount, inCheckCount)
		}
	}

	writer.Flush()
	fmt.Printf("filter complete: checked %d, accepted %d (%.1f%%), noisy %d, in check %d\n",
		totalPositions, acceptedCount, percentage(int(acceptedCount), int(totalPositions)), noisyCount, inCheckCount)

	return nil
}

func testQuietPositionWithState(fen string, p *Pos, ss *SearchState) (filterResult, error) {
	parseFEN(p, fen)

	if p.inCheck() {
		return filterInCheck, nil
	}

	if ss.resetForSearch(p) {
		ss.clearSearchHashes()
	}
	refresh(p, &ss.accStack[0])

	atomic.StoreInt32(&abortFlag, 0)
	hardTimeLimit = -1
	singleOptionValue[NodesLimit] = 0

	staticScore := evaluate(p, &ss.accStack[0], ss)

	var pv [maxPly]int
	qsScore := ss.quiesce(p, 0, -inf, inf, pv[:])

	if absInt(staticScore-qsScore) >= maxEvalQSDifference {
		return filterNoisy, nil
	}

	return filterAccepted, nil
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func percentage(part, total int) float64 {
	if total == 0 {
		return 0
	}
	return 100 * float64(part) / float64(total)
}
