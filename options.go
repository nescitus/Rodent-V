// ================================================================
//
//	R O D E N T   V
//
// ================================================================
//
//	A Go chess engine by Naman Thanki and Pawel Koziol.
//	Based on Sungorus 1.4 by Pablo Vazquez (2013).
//
//	Authors        : Naman Thanki, Pawel Koziol
//	Date           : 2026
//
//	Every file is a short lesson in chess engine design. Follow the
//	table of contents below to understand the full pipeline from
//	board representation to UCI output.
//
//	Protocol: Universal Chess Interface (UCI)
//	Build:    go build -o rodent-v .

package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

var noOptions = false // prevents from changing default settings if true
var readPersonalityFiles bool = true
var personalityFile string // default path to personality file
var nnuePath string      // default path to NNUE file
var guideBookPath string // path to repertoire Polyglot book, default is empty
var mainBookPath string  // path to main Polyglot book, default is empty
var ownBook bool = false // whether to use the internal opening book

var pestoEval bool            // are we using pesto eval?
var adjustEvalByCorrhist bool // are we using corrhist
// (corrhist makes engine stronger but can mask eval properties)

// Rodent has two kinds of eval options: asymmetric, set independently
// for engine's and opponent's side, and symmetrical, affecting both
// sides to the same extent

// symmetrical, side-independent options
type SingleOption int

const (
	HcePerc SingleOption = iota
	NnuePerc
	NodesLimit
	NnueScale
	LikesClosed
	KingTropism
	Forwardness
	HorizontalMirroring
	NofSingleOptions
)

var singleOptionName [NofSingleOptions]string
var singleOptionValue [NofSingleOptions]int
var singleOptionMin [NofSingleOptions]int
var singleOptionMax [NofSingleOptions]int
var singleOptionVisible[NofSingleOptions] bool

// Asymmetric, side-dependent options (EvaComponent) are defined
// in EvalData. They need to be indexed by engine/non engine side,
// not white/black, hence we need to set engineSide variable
// at the beginning of each search.
var optionPerColorValues [2][EvalComponentN]int

var engineSide int

const weightOwn = 0
const weightOpp = 1

// 131072 nodes - 2680

// default settings
func init() {

	personalityFile = "personalities/rodent.txt" 
	guideBookPath = "books/empty.bin"
	mainBookPath = "books/empty.bin"
	nnuePath = "nets/rodent_hm_512hl_1.bin"

	registerSingleOption(HcePerc, "hceWeight", 0, 0, 256, !readPersonalityFiles)
	registerSingleOption(NnuePerc, "nnueWeight", 100, 0, 256, !readPersonalityFiles)
	registerSingleOption(NnueScale, "nnueScale", 400, 10, 2000, !readPersonalityFiles)
	registerSingleOption(NodesLimit, "nodesLimit", 0, 65356, 1000*1000*1000, !readPersonalityFiles)
	registerSingleOption(LikesClosed, "likesClosed", 0, -256, 256, !readPersonalityFiles)
	registerSingleOption(KingTropism, "kingTropism", 0, -256, 256, !readPersonalityFiles)
	registerSingleOption(Forwardness, "forwardness", 0, -256, 256, !readPersonalityFiles)
	registerSingleOption(HorizontalMirroring, "horizontalMirroring", 1, 0, 1, !readPersonalityFiles)

	pestoEval = false
	adjustEvalByCorrhist = true

	for c := EvalComponent(0); c < EvalComponentN; c++ {
		optionPerColorValues[weightOwn][c] = 100
		optionPerColorValues[weightOpp][c] = 100
	}
}

// set value, min, max and name for an option
func registerSingleOption(
	option SingleOption,
	name string,
	defaultValue int,
	minValue int,
	maxValue int,
	visible bool,
) {
	singleOptionName[option] = name
	singleOptionValue[option] = defaultValue
	singleOptionMin[option] = minValue
	singleOptionMax[option] = maxValue
	singleOptionVisible[option] = visible
}

// saveOptions writes the current engine options as UCI setoption commands,
// one option per line.
func saveOptions(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create options file %q: %w", path, err)
	}

	fmt.Println("info string saving personality ", path)

	writer := bufio.NewWriter(file)

	writeErr := func() error {
		// Header comments are ignored by readOptions.
		if _, err := fmt.Fprintln(writer, "; Rodent V options"); err != nil {
			return err
		}

		if _, err := fmt.Fprintf(
			writer,
			"setoption name nnuePath value %s\n",
			nnuePath,
		); err != nil {
			return err
		}

		if _, err := fmt.Fprintf(
			writer,
			"setoption name mainBookPath value %s\n",
			mainBookPath,
		); err != nil {
			return err
		}

		if _, err := fmt.Fprintf(
			writer,
			"setoption name guideBookPath value %s\n",
			guideBookPath,
		); err != nil {
			return err
		}

		for option := SingleOption(0); option < NofSingleOptions; option++ {
			if _, err := fmt.Fprintf(
				writer,
				"setoption name %s value %d\n",
				singleOptionName[option],
				singleOptionValue[option],
			); err != nil {
				return err
			}
		}

		for component := EvalComponent(0); component < EvalComponentN; component++ {
			if _, err := fmt.Fprintf(
				writer,
				"setoption name Own%s value %d\n",
				evalComponentName[component],
				optionPerColorValues[weightOwn][component],
			); err != nil {
				return err
			}

			if _, err := fmt.Fprintf(
				writer,
				"setoption name Opp%s value %d\n",
				evalComponentName[component],
				optionPerColorValues[weightOpp][component],
			); err != nil {
				return err
			}
		}

		return writer.Flush()
	}()

	closeErr := file.Close()

	if writeErr != nil {
		return fmt.Errorf("write options file %q: %w", path, writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close options file %q: %w", path, closeErr)
	}

	return nil
}

// readOptions reads UCI setoption commands from a file.
//
// Empty lines and lines whose first non-space character is ';' are ignored.
// Unknown or malformed commands are skipped. Option values may contain spaces.
func readOptions(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open options file %q: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	// Permit long paths or future string options.
	scanner.Buffer(make([]byte, 1024), 1024*1024)

	lineNumber := 0

	for scanner.Scan() {
		lineNumber++

		line := strings.TrimSpace(scanner.Text())

		// Remove a possible UTF-8 BOM from the beginning of the file.
		if lineNumber == 1 {
			line = strings.TrimPrefix(line, "\uFEFF")
		}

		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}

		tokens := strings.Fields(line)
		if len(tokens) == 0 {
			continue
		}

		if !strings.EqualFold(tokens[0], "setoption") {
			continue
		}

		// parseSetOption expects everything after the "setoption" token.
		parseSetOption(tokens[1:])
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read options file %q: %w", path, err)
	}

	return nil
}

// printUciOptionsPerColor prints color-separated options
func printUciOptionsPerColor() {

	if noOptions {
		return
	}

	for c := EvalComponent(0); c < EvalComponentN; c++ {
		if (!readPersonalityFiles) {
			printPerColorOption(c)
		}
	}
}

// printPerColorOption prints option that has separate values
// for engine's and its opponents' side
func printPerColorOption(component EvalComponent) {

	if noOptions {
		return
	}

	fmt.Printf(
		"option name Own%s type spin default %d min 0 max 500\n",
		evalComponentName[component],
		optionPerColorValues[weightOwn][component],
	)

	fmt.Printf(
		"option name Opp%s type spin default %d min 0 max 500\n",
		evalComponentName[component],
		optionPerColorValues[weightOpp][component],
	)
}

func setPerColorOption(name, value string) bool {
	v, err := strconv.Atoi(value)
	if err != nil {
		return false
	}

	if noOptions {
		return false
	}

	for c := EvalComponent(0); c < EvalComponentN; c++ {
		switch {
		case strings.EqualFold(name, "Own"+evalComponentName[c]):
			optionPerColorValues[weightOwn][c] = limitValue(v, 0, 500)
			//fmt.Println("info string setting Own"+evalComponentName[c],"at",v)
			return true

		case strings.EqualFold(name, "Opp"+evalComponentName[c]):
			optionPerColorValues[weightOpp][c] = limitValue(v, 0, 500)
			//fmt.Println("info string setting Opp"+evalComponentName[c],"at",v)
			return true
		}
	}

	return false
}

// prints spin option applicable to both sides
// NOTE: we are relying on Println adding spaces between arguments
func printSingleOption(option SingleOption) {

	if noOptions {
		return
	}

	if (!readPersonalityFiles) {
		fmt.Println(
			"option name", singleOptionName[option],
			"type spin default", singleOptionValue[option],
			"min", singleOptionMin[option],
			"max", singleOptionMax[option],
		)
	}
}

func setSingleOption(name, value string) bool {

	if noOptions {
		return false
	}

	for option := SingleOption(0); option < NofSingleOptions; option++ {
		if !strings.EqualFold(name, singleOptionName[option]) {
			continue
		}

		n, err := strconv.Atoi(value)
		if err != nil {
			fmt.Printf("info string invalid value %q for option %s\n",
				value, singleOptionName[option])
			return true
		}

		singleOptionValue[option] = limitValue(n,
			singleOptionMin[option],
			singleOptionMax[option])
		return true
	}

	return false
}