// ================================================================
//                       R O D E N T   V
// ================================================================
//
//   A Go chess engine by Naman Thanki and Pawel Koziol.
//   Based on Sungorus 1.4 by Pablo Vazquez (2013).
//
//   Authors        : Naman Thanki, Pawel Koziol
//   Date           : 2026
//
//   Every file is a short lesson in chess engine design. Follow the
//   table of contents below to understand the full pipeline from
//   board representation to UCI output.
//
//   Protocol: Universal Chess Interface (UCI)
//   Build:    go build -o rodent-v .
//
// ================================================================
//
//   TABLE OF CONTENTS  (one file per subsystem)
//   -------------------------------------------------------------
//   tables.go  - S1  constants, bit helpers, precomputed tables
//   pos.go     - S2  board representation and FEN parsing
//   attacks.go - S3  attack detection (is a square safe?)
//   moves.go   - S4  make / unmake move (incremental updates)
//   gen.go     - S5  move generation (captures and quiet moves)
//   legal.go   - S6  move legality validation
//   eval.go    - S7  static evaluation (material, mobility, pawns)
//   next.go    - S8  move ordering (TT -> good caps -> killers -> quiet)
//   trans.go   - S9  transposition table (4-bucket hash with aging)
//   swap.go    - S10 static exchange evaluation (SEE)
//   search.go  - S11 principal variation search + quiescence
//   uci.go     - S12 UCI protocol (commands, time management, perft)
//   main.go    -     entry point
//
// ================================================================

package main

import (
	"fmt"
	"os"
	"runtime/pprof"
	"strconv"
)

const versionString = "1.1.02"

// init() is guaranteed to run before main()
func init() {
	engineSide = White
	initTables()
}

func main() {
	// Opt-in CPU profiling for regenerating default.pgo.
	if profPath := os.Getenv("RODENT_CPUPROFILE"); profPath != "" {
		f, err := os.Create(profPath)
		if err != nil {
			fmt.Println("could not create CPU profile:", err)
		} else if err := pprof.StartCPUProfile(f); err != nil {
			fmt.Println("could not start CPU profile:", err)
		} else {
			defer pprof.StopCPUProfile()
		}
	}

	if len(os.Args) > 1 && os.Args[1] == "genmagics" {
		FindMagics()
		return
	}

	var p Pos
	parseFEN(&p, startFEN)
	if !nnueInitEmbedded() {
		nnueLoad(nnuePath)
	}
	if !nnue.Loaded {
		fmt.Println("nnue not loaded")
	}

	// Tuner workflows are opt-in only and must be explicitly requested.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "tune":
			ctTune("d:/epd/lichess-sym.epd", 400, 0.25, 0)
			return
		case "tunefile":
			if len(os.Args) < 3 {
				fmt.Println("usage: rodent-v tunefile <epd-or-book-file>")
				return
			}
			ctTune(os.Args[2], 5000, 1.0, 1.0)
			return
		case "datagen":
			if len(os.Args) < 5 {
				fmt.Println("usage: rodent-v datagen <target_positions> <threads> <nodes_per_move> [book_file]")
				return
			}
			target, _ := strconv.Atoi(os.Args[2])
			threads, _ := strconv.Atoi(os.Args[3])
			nodesPerMove, _ := strconv.Atoi(os.Args[4])
			bookFile := ""
			if len(os.Args) > 5 {
				bookFile = os.Args[5]
			}
			runDatagen(target, threads, nodesPerMove, bookFile)
			return

		case "filter":
			if len(os.Args) < 4 {
				fmt.Println("usage: rodent-v filter <input.txt> <output.txt>")
				return
			}
			err := filterQuietBulletFile(os.Args[2], os.Args[3])
			if err != nil {
				fmt.Println("Error:", err)
			}
			return
		case "fit":
			ss := new(SearchState)
			ss.tt = &mainTT
			ss.isMainThread = true
			ss.isUsingNNUE = false

			fit, err := texelFitFile("d:/epd/zuri_orig.epd", func(p *Pos) int {
				return eval_internal(p, false, ss)
			})
			if err == nil {
				fmt.Println(fit)
			}
			return
		case "alloptions":
			// Engine will hide personality path and print UCI options.
			// No return statement, we want to use Rodent like that!
			readPersonalityFiles = false
		case "nooptions":
			// Engine will hide all the personality options.
			// No return statement, we want to use Rodent like that!
			noOptions = true
		case "pesto":
			// Engine will run a piece/square only eval.
			// No return statement, we want to use Rodent like that!
			pestoEval = true
			noOptions = true

			// case "loadsnapshot":
			// 	if len(os.Args) < 3 {
			// 		fmt.Println("usage: rodent-v loadsnapshot <snapshot-file>")
			// 		return
			// 	}
			// 	loadSnapshotFile(os.Args[2])
			// 	return
		case "bench":
			depth := 14
			if len(os.Args) > 2 {
				if d, err := strconv.Atoi(os.Args[2]); err == nil {
					depth = d
				}
			}
			allocTT(16)
			ss := new(SearchState)
			ss.tt = &mainTT
			runBench(depth, ss, true)
			return
		}
	}

	// default is no opening books
	initBooks("books/empty.bin", "books/empty.bin")

	// some ASCII art
	PrintHeader()

	// main program loop
	uciLoop()
}

func PrintHeader() {
	fmt.Printf(`
   ___          __         __   _   __
  / _ \___  ___/ /__ ___  / /_ | | / /
 / , _/ _ \/ _  / -_) _ \/ __/ | |/ /
/_/|_|\___/\_,_/\__/_//_/\__/  |___/

         Rodent V %s
      Type 'uci' or 'help'
`, versionString)
}
