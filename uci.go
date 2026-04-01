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
)

// ---- UCI main loop ----

func uciLoop() {
	var p Pos
	parseFEN(&p, startFEN)
	allocTT(16)

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
		line   := strings.TrimRight(scanner.Text(), "\r\n ")
		tokens := strings.Fields(line)
		if len(tokens) == 0 {
			continue
		}

		switch tokens[0] {
		case "uci":
			fmt.Println("id name Rodent V 0.2.5")
			fmt.Println("id author Naman Thanki, Pawel Koziol, based on Sungorus by Pablo Vazquez")
			fmt.Println("option name Hash type spin default 16 min 1 max 4096")
			fmt.Println("option name Clear Hash type button")
			fmt.Println("uciok")

		case "isready":
			stopSearch()
			fmt.Println("readyok")

		case "setoption":
			parseSetOption(tokens[1:])

		case "ucinewgame":
			stopSearch()
			clearTT()
			parseFEN(&p, startFEN)

		case "position":
			stopSearch()
			parsePosition(&p, tokens[1:])

		case "go":
			stopSearch()
			mt, md := parseGoParams(tokens[1:], &p)
			timeLimit = mt
			pondering = false
			for _, t := range tokens[1:] {
				if t == "ponder" {
					pondering = true
					break
				}
			}
			posForSearch := p // copy: the search owns its own position
			searchDone   = make(chan struct{})
			go func(pos Pos, done chan struct{}, maxDepth int) {
				defer close(done)
				think(&pos, maxDepth)
			}(posForSearch, searchDone, md)

		case "stop":
			stopSearch()

		case "ponderhit":
			// The move we were pondering was played. Switch from
			// unlimited ponder mode to normal time control.
			pondering = false

		case "quit":
			stopSearch()
			os.Exit(0)

		case "perft":
			stopSearch()
			depth := 1
			if len(tokens) > 1 {
				if d, err := strconv.Atoi(tokens[1]); err == nil {
					depth = d
				}
			}
			start   := time.Now()
			n       := perft(&p, depth)
			elapsed := time.Since(start).Milliseconds()
			if elapsed < 1 {
				elapsed = 1
			}
			fmt.Printf("Nodes: %d  Time: %dms  NPS: %d\n", n, elapsed, n*1000/uint64(elapsed))

		case "print":
			PrintBoard(&p)

		}
	}

	// Stdin closed. Wait for any in-progress search to output bestmove.
	if searchDone != nil {
		<-searchDone
	}
}

// ---- Option parsing ----

// parseSetOption handles the "setoption name <n> value <v>" command.
//   Hash: resize the transposition table (in megabytes).
//   Clear Hash: zero the transposition table.
func parseSetOption(tokens []string) {
	name  := ""
	value := ""
	i     := 0
	if i < len(tokens) && tokens[i] == "name" {
		i++
	}
	for i < len(tokens) && tokens[i] != "value" {
		if name != "" {
			name += " "
		}
		name += tokens[i]
		i++
	}
	if i < len(tokens) && tokens[i] == "value" {
		i++
		value = strings.Join(tokens[i:], " ")
	}

	switch name {
	case "Hash":
		if mb, err := strconv.Atoi(value); err == nil {
			allocTT(mb)
		}
	case "Clear Hash":
		clearTT()
	}
}

// ---- Position setup ----

// parsePosition handles "position startpos [moves ...]" and
// "position fen <fen> [moves ...]".
// After applying the move list, histLen is trimmed whenever an
// irreversible move resets the clock, so repetition detection does
// not look past that point.
func parsePosition(p *Pos, tokens []string) {
	if len(tokens) == 0 {
		return
	}
	i := 0
	if tokens[i] == "startpos" {
		parseFEN(p, startFEN)
		i++
	} else if tokens[i] == "fen" {
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
			var u Undo
			makeMove(p, move, &u)
			if p.clock == 0 {
				p.histLen = 0
			}
		}
	}
}

// ---- Time management ----

// parseGoParams reads the "go" command parameters and returns:
//   timeLimit (ms): allocated time for this move; -1 = no limit.
//   maxDepth: maximum search depth.
func parseGoParams(tokens []string, p *Pos) (int64, int) {
	wtime     := int64(-1)
	btime     := int64(-1)
	winc      := int64(0)
	binc      := int64(0)
	movestogo := int64(40)
	movetime  := int64(-1)
	maxDepth  := maxPly - 1

	for i := 0; i < len(tokens); i++ {
		switch tokens[i] {
		case "wtime":
			if i+1 < len(tokens) { i++; wtime, _ = strconv.ParseInt(tokens[i], 10, 64) }
		case "btime":
			if i+1 < len(tokens) { i++; btime, _ = strconv.ParseInt(tokens[i], 10, 64) }
		case "winc":
			if i+1 < len(tokens) { i++; winc, _ = strconv.ParseInt(tokens[i], 10, 64) }
		case "binc":
			if i+1 < len(tokens) { i++; binc, _ = strconv.ParseInt(tokens[i], 10, 64) }
		case "movestogo":
			if i+1 < len(tokens) { i++; movestogo, _ = strconv.ParseInt(tokens[i], 10, 64) }
		case "movetime":
			if i+1 < len(tokens) { i++; movetime, _ = strconv.ParseInt(tokens[i], 10, 64) }
		case "depth":
			if i+1 < len(tokens) {
				i++
				if d, err := strconv.Atoi(tokens[i]); err == nil {
					maxDepth = d
				}
			}
		case "infinite":
			return -1, maxPly - 1
		}
	}

	if movetime >= 0 {
		return movetime - 10, maxDepth // subtract I/O safety margin
	}

	var myTime, myInc int64
	if p.side == White {
		myTime = wtime
		myInc  = winc
	} else {
		myTime = btime
		myInc  = binc
	}
	if myTime < 0 {
		return -1, maxDepth // no clock provided -> search indefinitely
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
	alloc -= 10 // I/O safety margin
	if alloc < 0 {
		alloc = 0
	}
	return alloc, maxDepth
}

// ---- Move string conversion ----

// moveToStr converts an integer move to UCI algebraic notation,
// e.g. "e2e4" or "e7e8q" for a queen promotion.
func moveToStr(move int) string {
	from := moveFrom(move)
	to   := moveTo(move)
	s    := []byte{
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
	to   := makeSquare(int(s[2]-'a'), int(s[3]-'1'))
	mt   := NORMAL

	switch {
	case p.typeAt(from) == K && abs(to-from) == 2:
		mt = CASTLE
	case p.typeAt(from) == P && to == p.epSquare:
		mt = EP_CAP
	case p.typeAt(from) == P && abs(to-from) == 16:
		mt = EP_SET
	case p.typeAt(from) == P && len(s) > 4:
		switch s[4] {
		case 'n': mt = N_PROM
		case 'b': mt = B_PROM
		case 'r': mt = R_PROM
		case 'q': mt = Q_PROM
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
//   depth 1 ->        20
//   depth 2 ->       400
//   depth 3 ->     8 902
//   depth 4 ->   197 281
//   depth 5 -> 4 865 609
func perft(p *Pos, depth int) uint64 {
	if depth == 0 {
		return 1
	}

	var list [maxMoves]int
	capCount   := genCaptures(p, list[:])
	quietCount := genQuiet(p, list[capCount:])
	total      := capCount + quietCount

	var n uint64
	for i := 0; i < total; i++ {
		move := list[i]
		var u Undo
		makeMove(p, move, &u)
		if !p.selfInCheck() {
			n += perft(p, depth-1)
		}
		unmakeMove(p, move, &u)
	}
	return n
}
