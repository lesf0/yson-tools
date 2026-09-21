package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	formatter "github.com/lesf0/yson-tools/pretty-formatter"
	"go.ytsaurus.tech/yt/go/yson"
	"golang.org/x/term"
)

const guessMode = "guess"
const prettifyMode = "pretty"
const json2ysonMode = "j2y"
const yson2jsonMode = "y2j"

const defaultMode = guessMode

const prettyFormat = "pretty"
const compactFormat = "compact"
const binaryFormat = "binary"

const defaultFormat = prettyFormat

// autoColor is set in main: the pretty printer colours its output only when
// that output goes to a terminal, never when it goes into a file.
var autoColor bool

func fromYson(s []byte) (any, error) {
	var ysonData any
	err := yson.Unmarshal(s, &ysonData)

	if err != nil {
		if serr, ok := err.(*yson.SyntaxError); ok {
			if serr.Message == "unexpected end of YSON input" {
				return nil, io.ErrUnexpectedEOF
			}
		}
	}

	return ysonData, err
}

func toYson(d any, format string) (string, error) {
	if format == prettyFormat {
		_, mono := os.LookupEnv("YSON_NO_COLOR")
		_, forceColor := os.LookupEnv("YSON_FORCE_COLOR")
		useColors := forceColor || !mono && autoColor

		colorScheme := ""
		if useColors {
			colorScheme, _ = os.LookupEnv("JQ_COLORS")
			if colorScheme == "" {
				colorScheme = "0;90:0;39:0;39:0;39:0;32:1;39:1;39:1;34" // default
			}
		}

		formatter := formatter.NewYsonFormatter(4, true, useColors, colorScheme)
		return formatter.Dump(d), nil
	}

	var ysonFormat yson.Format
	switch format {
	case compactFormat:
		ysonFormat = yson.FormatText
	case binaryFormat:
		ysonFormat = yson.FormatBinary
	default:
		return "", fmt.Errorf("unexpected yson format: %v", format)
	}
	result, err := yson.MarshalFormat(d, ysonFormat)
	if err != nil {
		return "", err
	}
	return string(result), nil
}

func fromJson(s []byte) (any, error) {
	var jsonData any
	decoder := json.NewDecoder(bytes.NewReader(s))
	decoder.UseNumber()
	err := decoder.Decode(&jsonData)

	if err == nil {
		jsonData = DenormalizeYSON(jsonData)
	}

	return jsonData, err
}

func toJson(d any, format string) (string, error) {
	var marshaler func(any) ([]byte, error)
	switch format {
	case prettyFormat:
		marshaler = func(v any) ([]byte, error) {
			return json.MarshalIndent(v, "", "\t")
		}
	case compactFormat:
		marshaler = json.Marshal
	default:
		return "", fmt.Errorf("json output cannot be written in %s format", format)
	}
	result, err := marshaler(NormalizeYSON(d))
	if err != nil {
		return "", err
	}
	return string(result), nil
}

func chain(input []byte, from func([]byte) (any, error), to func(any) (string, error)) (string, error) {
	buf, err := from(input)
	if err != nil {
		return "", err
	}
	return to(buf)
}

func applyFormat(to func(any, string) (string, error), format string) func(any) (string, error) {
	return func(v any) (string, error) {
		return to(v, format)
	}
}

func apply(input []byte, mode string, format string) (string, error) {
	switch mode {
	case guessMode:
		result, err := chain(input, fromYson, applyFormat(toJson, format))
		if err != nil {
			return chain(input, fromJson, applyFormat(toYson, format))
		}
		return result, nil
	case prettifyMode:
		return chain(input, fromYson, applyFormat(toYson, format))
	case json2ysonMode:
		return chain(input, fromJson, applyFormat(toYson, format))
	case yson2jsonMode:
		return chain(input, fromYson, applyFormat(toJson, format))
	default:
		return "", fmt.Errorf("unknown mode: %v", mode)
	}
}

func seek(input []byte, mode string) (int, error) {
	switch mode {
	case prettifyMode, yson2jsonMode:
		start, mid, end := 1, 1, len(input)
		for start < end {
			mid = (start + end) >> 1
			_, err := apply(input[:mid], mode, compactFormat)

			switch err {
			case nil:
				start = mid + 1
			case io.ErrUnexpectedEOF, io.EOF:
				start = mid + 1
			default:
				end = mid - 1
			}
		}
		if _, err := apply(input[:end], mode, compactFormat); err == nil {
			return end, nil
		}
		return end - 1, nil
	case json2ysonMode:
		var parsed any
		err := json.Unmarshal(input, &parsed)
		if err != nil {
			if serr, ok := err.(*json.SyntaxError); ok {
				return int(serr.Offset) - 1, nil
			}
		}
		return len(input), nil

	default:
		return 0, fmt.Errorf("seek is not implemented for %s mode", mode)
	}
}

func handle(input []byte, mode string, format string, readAsSeq bool) (string, error) {
	if !readAsSeq {
		return apply(input, mode, format)
	}

	// if it's stupid but it works it's not stupid
	var results []string
	ok := false
	var lastErr error

	for len(input) != 0 {
		end, err := seek(input, mode)
		if err != nil {
			return "", err
		}

		result, err := apply(input[:end], mode, format)
		input = input[end:]

		if err == nil {
			results = append(results, result)
			ok = true
		} else {
			if len(bytes.TrimSpace(input)) > 0 {
				return "", fmt.Errorf("illegal characters at the end of input")
			}

			lastErr = err
			break
		}
	}

	if !ok {
		return "", fmt.Errorf("unable to parse input: %v", lastErr)
	}

	return strings.Join(results, "\n"), nil
}

// checkMode rejects a mode or format the tool cannot produce, rather than
// failing half way through the conversion with an empty result.
func checkMode(mode string, format string, readAsSeq bool) error {
	switch mode {
	case guessMode, prettifyMode, json2ysonMode, yson2jsonMode:
	default:
		return fmt.Errorf("unknown mode: %v", mode)
	}

	switch format {
	case prettyFormat, compactFormat, binaryFormat:
	default:
		return fmt.Errorf("unknown format: %v", format)
	}

	// splitting a sequence in guess mode cannot work: past the end of a value
	// both guesses fail, so a truncated prefix is indistinguishable from an
	// invalid one and the search would cut the input in the wrong place
	if readAsSeq && mode == guessMode {
		return fmt.Errorf("-seq needs an explicit mode, pass -m y2j, -m j2y or -m pretty")
	}

	return nil
}

// readInput returns the value to convert: the single positional argument as
// data, the -i file, or stdin when it is not a terminal.
func readInput(path string, args []string) ([]byte, error) {
	if len(args) > 1 {
		return nil, fmt.Errorf("expected at most one positional argument with data, got %d (flags go before it)", len(args))
	}

	if len(args) == 1 {
		if path != "" {
			return nil, fmt.Errorf("give either a value argument or -i, not both")
		}
		return []byte(args[0]), nil
	}

	if path != "" && path != "-" {
		return os.ReadFile(path)
	}

	// "-" asks for stdin explicitly, an unset -i only takes it when something
	// is actually there: without a terminal to type into, an interactive run
	// would otherwise hang waiting for input that never comes
	if path == "" && term.IsTerminal(int(os.Stdin.Fd())) {
		return nil, fmt.Errorf("expected a value argument or data on stdin")
	}

	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil, fmt.Errorf("reading stdin: %w", err)
	}
	return data, nil
}

// writeOutput writes the converted value. The file is opened here, after the
// input has been read and converted, which is what makes "-i f -o f" safe.
func writeOutput(path string, result string) error {
	if path == "" || path == "-" {
		fmt.Println(result)
		return nil
	}

	// truncating an existing file keeps its inode and its permissions, so this
	// is an edit in place rather than a replacement
	file, err := os.Create(path)
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintln(file, result); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

// fail reports a fatal error the way a command line tool should: one line on
// stderr and a non-zero exit, no stack trace.
func fail(err error) {
	fmt.Fprintf(os.Stderr, "yson-convert: %v\n", err)
	os.Exit(1)
}

func main() {
	var mode string
	flag.StringVar(&mode, "mode", defaultMode, "work mode")
	flag.StringVar(&mode, "m", defaultMode, "work mode (shorthand)")

	var format string
	flag.StringVar(&format, "format", defaultFormat, "format")
	flag.StringVar(&format, "f", defaultFormat, "format (shorthand)")

	readAsSeq := flag.Bool("seq", false, "attempt to read the input as a sequence of (Y/J)SON's")

	var input string
	flag.StringVar(&input, "input", "", "read the value from a file, \"-\" for stdin")
	flag.StringVar(&input, "i", "", "read the value from a file (shorthand)")

	var output string
	flag.StringVar(&output, "output", "", "write the result to a file, \"-\" for stdout")
	flag.StringVar(&output, "o", "", "write the result to a file (shorthand)")

	flag.Parse()

	if err := checkMode(mode, format, *readAsSeq); err != nil {
		fail(err)
	}

	// colours are decided before the output is opened: a file, a pipe or a
	// redirection must never receive escape sequences
	autoColor = (output == "" || output == "-") && term.IsTerminal(int(os.Stdout.Fd()))

	data, err := readInput(input, flag.Args())
	if err != nil {
		fail(err)
	}

	result, err := handle(data, mode, format, *readAsSeq)
	if err != nil {
		fail(fmt.Errorf("conversion resulted in error: %v", err))
	}

	if err := writeOutput(output, result); err != nil {
		fail(err)
	}
}
