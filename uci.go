// ================================================================
// S12 UCI PROTOCOL
// ================================================================
//
//   This file implements the Universal Chess Interface (UCI) protocol,
//   which lets any UCI-compatible GUI (Arena, Cutechess, SCID,
//   lichess-bot, etc.) drive the engine.
//
//   UCI BASICS
//   ----------
//   Communication is line-based over stdin/stdout.
//   The GUI sends commands; the engine responds asynchronously.
//   The most important commands:
//
//     uci           -> engine identifies itself and lists options
//     isready       -> engine responds "readyok" when idle
//     ucinewgame    -> reset state for a fresh game
//     position ...  -> set up the board from startpos or FEN
//     go ...        -> start searching (may include time controls)
//     stop          -> abort the current search
//     quit          -> exit the process
//
//   CONCURRENCY MODEL
//   -----------------
//   Searching runs in a separate goroutine so that the UCI loop
//   remains responsive to "stop" and "quit" commands during a search.
//   abortFlag (an atomic int32) is the signal: the search goroutine
//   checks it periodically and unwinds when set.  searchDone is a
//   channel that is closed when the goroutine exits; the UCI loop
//   waits on it before starting a new search.
//
//   TIME MANAGEMENT
//   ---------------
//   parseGoParams converts UCI time parameters into a single
//   timeLimit in milliseconds.  The formula is:
//
//       alloc = (myTime + myInc * (movestogo - 1)) / movestogo
//
//   A 10 ms safety margin is subtracted to account for I/O latency.
//   When "movetime" is specified directly, that value is used instead.
//

package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/sys/cpu"
)

// ---- UCI main loop ----

func uciLoop() {
	var p Pos
	parseFEN(&p, startFEN)
	allocTT(16)

	// Allocate one SearchState per thread slot.
	// The main thread always uses states[0]; helpers use states[1..N-1].
	// Allocate enough for the maximum allowed thread count up front so
	// we never allocate during a search.
	const maxAllowedThreads = 256
	states := make([]*SearchState, maxAllowedThreads)
	states[0] = new(SearchState)
	states[0].tt = &mainTT
	states[0].isMainThread = true

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 65536), 65536)

	var searchDone chan struct{}

	// stopSearch signals the running search to abort and waits for it
	// to finish outputting "bestmove" before returning.
	stopSearch := func() {
		atomic.StoreInt32(&abortFlag, 1)
		if searchDone != nil {
			<-searchDone
			searchDone = nil
		}
	}

	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r\n ")
		tokens := strings.Fields(line)
		if len(tokens) == 0 {
			continue
		}

		switch tokens[0] {
		case "uci":
			arch := "Scalar"
			if cpu.X86.HasAVX2 {
				arch = "AVX2"
			}
			fmt.Printf("id name Rodent V %s %s\n", versionString, arch)
			fmt.Println("id author Naman Thanki, Pawel Koziol, based on Sungorus by Pablo Vazquez")
			
			// these options should be always exposed
			fmt.Println("option name Hash type spin default 16 min 1 max 4096")
			fmt.Println("option name Clear Hash type button")
			fmt.Println("option name UCI_LimitStrength type check default false")
			fmt.Printf("option name UCI_Elo type spin default %d min 800 max 3000\n", engineElo)

            if (!noOptions) {
				//fmt.Println("option name PestoEval type check default false")
				fmt.Println("option name OwnBook type check default false")
				fmt.Println("option name Threads type spin default 1 min 1 max 256")
				fmt.Println("option name MultiPV type spin default 1 min 1 max 500")
				printSingleOption(HcePerc)
				printSingleOption(NnuePerc)
				if (readPersonalityFiles) {
					fmt.Println("option name PersonalityFile type string default", personalityFile)
				} else {
					fmt.Println("option name Save Personality type button")
					fmt.Println("option name NnuePath type string default", nnuePath)
					fmt.Println("option name MainBook type string default", mainBookPath)
					fmt.Println("option name GuideBook type string default", guideBookPath)
				}

				printUciOptionsPerColor()

				printSingleOption(LikesClosed)
				printSingleOption(KingTropism)
				printSingleOption(Forwardness)
				printSingleOption(HorizontalMirroring)
			}

			if nnue.Loaded {
				fmt.Printf("info string Loaded NNUE network: %s\n", nnuePath)
			}

			fmt.Println("uciok")

		case "isready":
			stopSearch()
			fmt.Println("readyok")

		case "setoption":
			parseSetOption(tokens[1:])

		case "ucinewgame":
			stopSearch()
			clearTT()
			for i := 0; i < numThreads; i++ {
				if states[i] != nil {
					states[i].clearHistory()
				}
			}
			parseFEN(&p, startFEN)

		case "position":
			stopSearch()
			parsePosition(&p, tokens[1:])

		case "go":
			stopSearch()

			move := 0
			if ownBook {
				move = guideBook.getMove(&p)
				if move == 0 {
					move = mainBook.getMove(&p)
				}
			}
			if move != 0 {
				fmt.Printf("bestmove %s\n", moveToStr(move))
			} else {

				st, mt, md := parseGoParams(tokens[1:], &p)
				softTimeLimit = st
				hardTimeLimit = mt
				pondering = false
				for _, t := range tokens[1:] {
					if t == "ponder" {
						pondering = true
						break
					}
				}
				posForSearch := p // copy: the search owns its own position
				searchDone = make(chan struct{})
				go func(pos Pos, done chan struct{}, maxDepth int) {
					defer close(done)
					think(&pos, states, maxDepth)
				}(posForSearch, searchDone, md)
			}
		case "stop":
			stopSearch()

		case "ponderhit":
			// The move we were pondering was played. Switch from
			// unlimited ponder mode to normal time control.
			pondering = false

		case "quit":
			stopSearch()
			os.Exit(0)

		case "bench":
			stopSearch()
			depth := 14 // default bench depth
			if len(tokens) > 1 {
				if d, err := strconv.Atoi(tokens[1]); err == nil {
					depth = d
				}
			}
			runBench(depth, states[0], false)

		case "perft":
			stopSearch()
			depth := 1
			if len(tokens) > 1 {
				if d, err := strconv.Atoi(tokens[1]); err == nil {
					depth = d
				}
			}
			start := time.Now()
			n := perft(&p, depth)
			elapsed := time.Since(start).Milliseconds()
			if elapsed < 1 {
				elapsed = 1
			}
			fmt.Printf("Nodes: %d  Time: %dms  NPS: %d\n", n, elapsed, n*1000/uint64(elapsed))

		case "print":
			PrintBoard(&p)

		case "eval":
			eval_trace(&p)

		case "help":
			fmt.Println("Available commands:")
			fmt.Println("  uci        - Enter UCI mode (standard GUI communication)")
			fmt.Println("  isready    - Check if the engine is ready")
			fmt.Println("  bench      - Run a performance benchmark over standard FENs")
			fmt.Println("  eval       - Show static evaluation for the current position")
			fmt.Println("  print      - Display the current board")
			fmt.Println("  perft <d>  - Run perft to depth <d>")
			fmt.Println("  quit       - Exit the engine")

		case "nnue":
			{
				var acc Accumulator
				refresh(&p, &acc)
				fmt.Print(acc.getEval(p.side))
			}

		case "measure_scale":
			if len(tokens) >= 2 {
				measureScale(tokens[1])
			} else {
				fmt.Println("info string Usage: measure_scale <epd_file>")
			}

		case "threats":
			PrintThreatDebug(&p)
		}
	}

	// Stdin closed. Wait for any in-progress search to output bestmove.
	if searchDone != nil {
		<-searchDone
	}
}

// ---- Option parsing ----

// parseSetOption handles the "setoption name <n> value <v>" command.
//
//	Hash: resize the transposition table (in megabytes).
//	Clear Hash: zero the transposition table.
func parseSetOption(tokens []string) {
	name := ""
	value := ""
	i := 0

	if i < len(tokens) && strings.EqualFold(tokens[i], "name") {
		i++
	}
	for i < len(tokens) && !strings.EqualFold(tokens[i], "value") {
		if name != "" {
			name += " "
		}
		name += tokens[i]
		i++
	}
	if i < len(tokens) && strings.EqualFold(tokens[i], "value") {
		i++
		value = strings.Join(tokens[i:], " ")
	}

	switch {
	case strings.EqualFold(name, "Hash"):
		if mb, err := strconv.Atoi(value); err == nil {
			allocTT(mb)
		}
		return

	case strings.EqualFold(name, "Clear Hash"):
		clearTT()
		return

	case strings.EqualFold(name, "Save Personality"):
		if noOptions {
			return
		}
		if err := saveOptions("personality.txt"); err != nil {
			fmt.Printf("info string failed to save personality: %v\n", err)
		} else {
			fmt.Println("info string personality saved")
		}
		return

	case strings.EqualFold(name, "PestoEval"):
		if noOptions {
			return
		}
		if b, err := strconv.ParseBool(value); err == nil {
			pestoEval = b
		}
		return

	case strings.EqualFold(name, "Threads"):
		if n, err := strconv.Atoi(value); err == nil {
			numThreads = limitValue(n, 1, 256)
		}
		return

	case strings.EqualFold(name, "UCI_LimitStrength"):
		if b, err := strconv.ParseBool(value); err == nil {
			limitStrength = b
			configureEngineStrength()
		}
		return

	case strings.EqualFold(name, "UCI_Elo"):
		if n, err := strconv.Atoi(value); err == nil {
			engineElo = limitValue(n, 800, 3500)
			configureEngineStrength()
		}
		return

	case strings.EqualFold(name, "nnueScale"):
		if noOptions {
			return
		}
		if n, err := strconv.Atoi(value); err == nil {
			singleOptionValue[NnueScale] = limitValue(n, 10, 2000)
		}
		return

	case strings.EqualFold(name, "hceWeight"):
		if noOptions {
			return
		}
		if n, err := strconv.Atoi(value); err == nil {
			singleOptionValue[HcePerc] = limitValue(n, 0, 256)
		}
		return

	case strings.EqualFold(name, "nnueWeight"):
		if noOptions {
			return
		}
		if n, err := strconv.Atoi(value); err == nil {
			singleOptionValue[NnuePerc] = limitValue(n, 0, 256)
		}
		return

	case strings.EqualFold(name, "personalityFile"):
		if noOptions {
			return
		}
		if value == "" {
			fmt.Println("info string personality not found")
			return
		}

		if readOptions(value) == nil {
			personalityFile = value // only correct values are saved
			fmt.Printf("info string loaded personality: %s\n", value)
		} else {
			fmt.Printf("info string failed to load personality: %s\n", value)
		}
		return

	case strings.EqualFold(name, "nnuePath"):
		if noOptions {
			return
		}
		if value == "" {
			fmt.Println("info string NNUE file path is empty")
			return
		}

		if nnueLoad(value) {
			if noOptions {
				return
			}
			nnuePath = value // only correct values are saved
			fmt.Printf("info string NNUE loaded: %s\n", value)
		} else {
			fmt.Printf("info string failed to load NNUE: %s\n", value)
		}
		return

	case strings.EqualFold(name, "mainBook") || strings.EqualFold(name, "mainBookPath"):
		if noOptions {
			return
		}
		if value == "" {
			fmt.Println("info string main book file path is empty")
			return
		}

		if initMainBook(value) {
			mainBookPath = value
		} else {
			fmt.Printf("info string failed to load main book %q\n", value)
		}
		return

	case strings.EqualFold(name, "guideBook") || strings.EqualFold(name, "guideBookPath"):
		if noOptions {
			return
		}
		if value == "" {
			fmt.Println("info string guide book file path is empty")
			return
		}

		if initGuideBook(value) {
			guideBookPath = value
		} else {
			fmt.Printf("info string failed to load guide book %q\n", value)
		}

		return
		
	case strings.EqualFold(name, "OwnBook"):
		if noOptions {
			return
		}
		ownBook = strings.EqualFold(value, "true")
		return
	}

	// Parse all the single options
	if setSingleOption(name, value) {
		return
	}

	// Parse all the per color (asymmetric) options
	if setPerColorOption(name, value) {
		return
	}
}

// ---- Position setup ----

// parsePosition handles "position startpos [moves ...]" and
// "position fen <fen> [moves ...]".
// After applying the move list, histLen is trimmed whenever an
// irreversible move resets the clock, so repetition detection does
// not look past that point.
func parsePosition(p *Pos, tokens []string) {
	var acc Accumulator
	if len(tokens) == 0 {
		return
	}
	i := 0
	switch tokens[i] {
	case "startpos":
		parseFEN(p, startFEN)
		i++
	case "m8":
		parseFEN(p, m8FEN)
		i++
	case "fen":
		i++
		var parts []string
		for i < len(tokens) && tokens[i] != "moves" {
			parts = append(parts, tokens[i])
			i++
		}
		parseFEN(p, strings.Join(parts, " "))
	}

	if i < len(tokens) && tokens[i] == "moves" {
		i++
		for ; i < len(tokens); i++ {
			move := parseMove(p, tokens[i])
			if move == 0 {
				break
			}
			var u Update
			var r Revert
			makeMove(p, &u, &r, move)
			acc.applyPendingChanges(p, &u)
			if p.clock == 0 {
				p.histLen = 0
			}
		}
	}
	refresh(p, &acc)
}

// ---- Time management ----

// parseGoParams reads the "go" command parameters and returns:
//
//	softLimit (ms): allocated soft time for stopping between iterations; -1 = no limit.
//	hardLimit (ms): absolute deadline for stopping mid-search; -1 = no limit.
//	maxDepth: maximum search depth.
func parseGoParams(tokens []string, p *Pos) (int64, int64, int) {
	wtime := int64(-1)
	btime := int64(-1)
	winc := int64(0)
	binc := int64(0)
	movestogo := int64(16)
	movetime := int64(-1)
	maxDepth := maxPly - 1

	for i := 0; i < len(tokens); i++ {
		switch tokens[i] {
		case "wtime":
			if i+1 < len(tokens) {
				i++
				wtime, _ = strconv.ParseInt(tokens[i], 10, 64)
			}
		case "btime":
			if i+1 < len(tokens) {
				i++
				btime, _ = strconv.ParseInt(tokens[i], 10, 64)
			}
		case "winc":
			if i+1 < len(tokens) {
				i++
				winc, _ = strconv.ParseInt(tokens[i], 10, 64)
			}
		case "binc":
			if i+1 < len(tokens) {
				i++
				binc, _ = strconv.ParseInt(tokens[i], 10, 64)
			}
		case "movestogo":
			if i+1 < len(tokens) {
				i++
				movestogo, _ = strconv.ParseInt(tokens[i], 10, 64)
			}
		case "movetime":
			if i+1 < len(tokens) {
				i++
				movetime, _ = strconv.ParseInt(tokens[i], 10, 64)
			}
		case "depth":
			if i+1 < len(tokens) {
				i++
				if d, err := strconv.Atoi(tokens[i]); err == nil {
					maxDepth = d
				}
			}
		case "infinite":
			return -1, -1, maxPly - 1
		}
	}

	if movetime >= 0 {
		t := movetime - 50 // subtract I/O safety margin
		if t < 0 {
			t = 0
		}
		return t, t, maxDepth
	}

	var myTime, myInc int64
	if p.side == White {
		myTime = wtime
		myInc = winc
	} else {
		myTime = btime
		myInc = binc
	}
	if myTime < 0 {
		return -1, -1, maxDepth // no clock provided -> search indefinitely
	}

	// Reserve a small buffer when only one move remains on the clock.
	if movestogo == 1 {
		dec := myTime / 10
		if dec > 1000 {
			dec = 1000
		}
		myTime -= dec
	}

	// Spread remaining time evenly over the expected number of moves.
	alloc := (myTime + myInc*(movestogo-1)) / movestogo
	if alloc > myTime {
		alloc = myTime
	}
	alloc -= 50 // I/O safety margin
	if alloc < 0 {
		alloc = 0
	}
	// Soft limit: stop comfortably between iterations (75% of base time).
	soft := alloc * 75 / 100
	// Hard limit: allow up to 4x base time in complex struggles before aborting.
	hard := alloc * 4
	if hard > myTime-50 {
		hard = myTime - 50
		if hard < 0 {
			hard = 0
		}
	}
	return soft, hard, maxDepth
}

// ---- Move string conversion ----

// moveToStr converts an integer move to UCI algebraic notation,
// e.g. "e2e4" or "e7e8q" for a queen promotion.
func moveToStr(move int) string {
	from := moveFrom(move)
	to := moveTo(move)
	s := []byte{
		byte('a' + fileOf(from)),
		byte('1' + rankOf(from)),
		byte('a' + fileOf(to)),
		byte('1' + rankOf(to)),
	}
	if isProm(move) {
		const promChars = "nbrq"
		s = append(s, promChars[(move>>12)&3])
	}
	return string(s)
}

// parseMove converts a UCI move string (e.g. "e2e4", "e7e8q") to the
// internal integer representation.  Returns 0 if the string is invalid.
// The move type is inferred from the position and move geometry.
func parseMove(p *Pos, s string) int {
	if len(s) < 4 {
		return 0
	}
	from := makeSquare(int(s[0]-'a'), int(s[1]-'1'))
	to := makeSquare(int(s[2]-'a'), int(s[3]-'1'))
	mt := NORMAL

	switch {
	case p.typeAt(from) == K && abs(to-from) == 2:
		mt = CASTLE
	case p.typeAt(from) == P && to == p.epSquare:
		mt = EP_CAP
	case p.typeAt(from) == P && abs(to-from) == 16:
		mt = EP_SET
	case p.typeAt(from) == P && len(s) > 4:
		switch s[4] {
		case 'n':
			mt = N_PROM
		case 'b':
			mt = B_PROM
		case 'r':
			mt = R_PROM
		case 'q':
			mt = Q_PROM
		}
	}
	return (mt << 12) | (to << 6) | from
}

// pvString converts a PV array to a space-separated string of UCI
// move strings, stopping at the first zero entry.
func pvString(pv []int) string {
	var sb strings.Builder
	for _, move := range pv {
		if move == 0 {
			break
		}
		if sb.Len() > 0 {
			sb.WriteByte(' ')
		}
		sb.WriteString(moveToStr(move))
	}
	return sb.String()
}

// buildPV prepends move to src and writes the result into dst,
// keeping the principal variation buffer current after each alpha improvement.
func buildPV(dst, src []int, move int) {
	dst[0] = move
	i := 1
	for i < len(dst) {
		if src[i-1] == 0 {
			break
		}
		dst[i] = src[i-1]
		i++
	}
	if i < len(dst) {
		dst[i] = 0
	}
}

// ---- Perft ----

// perft counts the total number of leaf nodes at exactly "depth" plies
// from the current position.  Used to verify move generation against
// published tables (e.g. the Chessprogramming Wiki).
//
// Expected node counts from the starting position:
//
//	depth 1 ->        20
//	depth 2 ->       400
//	depth 3 ->     8 902
//	depth 4 ->   197 281
//	depth 5 -> 4 865 609
func perft(p *Pos, depth int) uint64 {
	var posStack [maxPly]Pos
	posStack[0] = *p

	return perftStack(&posStack, 0, depth)
}

func perftStack(
	posStack *[maxPly]Pos,
	ply int,
	depth int,
) uint64 {
	if depth == 0 {
		return 1
	}

	p := &posStack[ply]

	var list [maxMoves]int

	capCount := genCaptures(p, list[:])
	quietCount := genQuiet(p, list[capCount:])
	total := capCount + quietCount

	var nodes uint64

	for i := 0; i < total; i++ {
		move := list[i]

		child := &posStack[ply+1]
		*child = *p

		var u Update
		var r Revert
		makeMove(child, &u, &r, move)
		//nnueApplyPending(child, &u)

		if !child.selfInCheck() {
			nodes += perftStack(
				posStack,
				ply+1,
				depth-1,
			)
		}
	}

	return nodes
}

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
		eval := acc.getEval(p.side)

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
