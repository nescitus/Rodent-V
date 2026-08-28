package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// TextLineChunk represents a batch of lines for parallel search rescoring.
type TextLineChunk struct {
	seq   int64
	lines []string
}

// runRescoreText runs parallel search-based rescoring on text datasets in "<fen> | <eval> | <wdl>" format.
func runRescoreText(inputFile, outputFile string, nodesPerPos int, numThreads int, netFile string) {
	if numThreads <= 0 {
		numThreads = runtime.NumCPU()
	}
	if nodesPerPos <= 0 {
		nodesPerPos = 1000
	}
	if outputFile == "" {
		ext := filepath.Ext(inputFile)
		base := strings.TrimSuffix(inputFile, ext)
		outputFile = base + "_rescored" + ext
	}

	if netFile != "" {
		nnueLoad(netFile)
	}

	if !nnue.Loaded {
		fmt.Printf("Error: NNUE network is not loaded!\n")
		return
	}

	inFile, err := os.Open(inputFile)
	if err != nil {
		fmt.Printf("Error opening input file '%s': %v\n", inputFile, err)
		return
	}
	defer inFile.Close()

	fileInfo, err := inFile.Stat()
	if err != nil {
		fmt.Printf("Error stating input file: %v\n", err)
		return
	}
	totalFileSize := fileInfo.Size()

	outFile, err := os.Create(outputFile)
	if err != nil {
		fmt.Printf("Error creating output file '%s': %v\n", outputFile, err)
		return
	}
	defer outFile.Close()

	fmt.Printf("=================================================================\n")
	fmt.Printf("           RODENT-V PARALLEL SEARCH RESCORING TOOL               \n")
	fmt.Printf("=================================================================\n")
	fmt.Printf("Input:       %s (%.2f MB)\n", inputFile, float64(totalFileSize)/(1024*1024))
	fmt.Printf("Output:      %s\n", outputFile)
	fmt.Printf("Net:         %s (Loaded: %v)\n", nnuePath, nnue.Loaded)
	fmt.Printf("Nodes/Pos:   %d nodes\n", nodesPerPos)
	fmt.Printf("Threads:     %d worker threads\n", numThreads)
	fmt.Printf("=================================================================\n\n")

	const linesPerChunk = 512
	jobsChan := make(chan *TextLineChunk, numThreads*4)
	resultsChan := make(chan *TextLineChunk, numThreads*4)

	var totalProcessedBytes int64
	var totalProcessedPositions int64
	var totalSearchNodes int64

	startTime := time.Now()

	// 1. Start Worker Goroutines
	var workerWg sync.WaitGroup
	for i := 0; i < numThreads; i++ {
		workerWg.Add(1)
		go func() {
			defer workerWg.Done()

			ss := new(SearchState)
			ss.tt = new(TTable)
			ss.tt.alloc(2) // 2MB TT per worker thread
			ss.isUsingNNUE = nnue.Loaded && singleOptionValue[NnuePerc] > 0

			var p Pos

			for chunk := range jobsChan {
				chunkNodes := int64(0)
				chunkBytes := int64(0)

				for idx, line := range chunk.lines {
					trimmed := strings.TrimSpace(line)
					chunkBytes += int64(len(line) + 1)
					if trimmed == "" || strings.HasPrefix(trimmed, "#") {
						continue
					}

					parts := strings.Split(trimmed, "|")
					if len(parts) < 3 {
						continue
					}

					fen := strings.TrimSpace(parts[0])
					wdl := strings.TrimSpace(parts[2])

					parseFEN(&p, fen)
					ss.tt.clear()
					ss.finny = [2][2][NNUEInputBuckets]FinnyEntry{}
					refresh(&p, &ss.accStack[0])

					_, score := runDatagenSearch(&p, ss, nodesPerPos)
					chunkNodes += ss.nodes

					chunk.lines[idx] = fmt.Sprintf("%s | %d | %s", fen, score, wdl)
				}

				atomic.AddInt64(&totalProcessedPositions, int64(len(chunk.lines)))
				atomic.AddInt64(&totalProcessedBytes, chunkBytes)
				atomic.AddInt64(&totalSearchNodes, chunkNodes)

				resultsChan <- chunk
			}
		}()
	}

	// 2. Start Ordered Writer Goroutine
	var writerWg sync.WaitGroup
	writerWg.Add(1)
	go func() {
		defer writerWg.Done()
		bufWriter := bufio.NewWriterSize(outFile, 4*1024*1024)
		defer bufWriter.Flush()

		pendingChunks := make(map[int64]*TextLineChunk)
		expectedSeq := int64(0)

		for chunk := range resultsChan {
			pendingChunks[chunk.seq] = chunk
			for {
				if nextChunk, found := pendingChunks[expectedSeq]; found {
					for _, l := range nextChunk.lines {
						bufWriter.WriteString(l)
						bufWriter.WriteByte('\n')
					}
					delete(pendingChunks, expectedSeq)
					expectedSeq++
				} else {
					break
				}
			}
		}
	}()

	// 3. Start Live Progress Monitor
	stopProgress := make(chan struct{})
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		var lastPos int64
		var lastNodes int64
		var lastTime = time.Now()

		for {
			select {
			case <-stopProgress:
				return
			case now := <-ticker.C:
				procBytes := atomic.LoadInt64(&totalProcessedBytes)
				procPos := atomic.LoadInt64(&totalProcessedPositions)
				procNodes := atomic.LoadInt64(&totalSearchNodes)

				pct := float64(procBytes) * 100.0 / float64(totalFileSize)
				elapsed := now.Sub(startTime).Seconds()
				dt := now.Sub(lastTime).Seconds()
				dPos := procPos - lastPos
				dNodes := procNodes - lastNodes

				instPosSpeed := float64(dPos) / dt
				instNps := float64(dNodes) / dt

				etaSec := 0.0
				if pct > 0 && pct < 100 {
					remainingBytes := totalFileSize - procBytes
					bytesPerSec := float64(procBytes) / elapsed
					if bytesPerSec > 0 {
						etaSec = float64(remainingBytes) / bytesPerSec
					}
				}

				fmt.Printf("\r[%.1f%%] Positions: %d | Pos/s: %.0f | NPS: %.2fM | ETA: %.0fs    ",
					pct, procPos, instPosSpeed, instNps/1_000_000, etaSec)

				lastPos = procPos
				lastNodes = procNodes
				lastTime = now
			}
		}
	}()

	// 4. Main Streaming Reader Loop
	scanner := bufio.NewScanner(inFile)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	var currentLines []string
	chunkSeq := int64(0)

	for scanner.Scan() {
		line := scanner.Text()
		currentLines = append(currentLines, line)

		if len(currentLines) >= linesPerChunk {
			chunkLines := make([]string, len(currentLines))
			copy(chunkLines, currentLines)
			jobsChan <- &TextLineChunk{
				seq:   chunkSeq,
				lines: chunkLines,
			}
			chunkSeq++
			currentLines = currentLines[:0]
		}
	}

	if len(currentLines) > 0 {
		chunkLines := make([]string, len(currentLines))
		copy(chunkLines, currentLines)
		jobsChan <- &TextLineChunk{
			seq:   chunkSeq,
			lines: chunkLines,
		}
	}

	close(jobsChan)
	workerWg.Wait()
	close(resultsChan)
	writerWg.Wait()
	close(stopProgress)

	totalElapsed := time.Since(startTime).Seconds()
	finalPos := atomic.LoadInt64(&totalProcessedPositions)
	finalNodes := atomic.LoadInt64(&totalSearchNodes)

	fmt.Printf("\n\n=================================================================\n")
	fmt.Printf("Search Rescoring Complete in %.2f seconds (%.2f minutes)!\n", totalElapsed, totalElapsed/60.0)
	fmt.Printf("Total Positions Rescored: %d\n", finalPos)
	fmt.Printf("Total Search Nodes:       %d\n", finalNodes)
	fmt.Printf("Average Search Speed:     %.2f Million NPS\n", (float64(finalNodes)/totalElapsed)/1_000_000)
	fmt.Printf("Rescoring Throughput:     %.0f positions/second\n", float64(finalPos)/totalElapsed)
	fmt.Printf("Output File:              %s\n", outputFile)
	fmt.Printf("=================================================================\n")
}
