package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/andrew-d/go-termutil"
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
		_, color := os.LookupEnv("YSON_FORCE_COLOR")
		formatter := NewYsonFormatter(4, true, color || !mono && !testing.Testing() && term.IsTerminal(int(os.Stdout.Fd())))
		return formatter.Dump(d), nil
	}

	var ysonFormat yson.Format
	switch format {
	case compactFormat:
		ysonFormat = yson.FormatText
	case binaryFormat:
		ysonFormat = yson.FormatBinary
	default:
		panic(fmt.Errorf("unexpected yson format: %v", format))
	}
	result, err := yson.MarshalFormat(d, ysonFormat)
	if err != nil {
		return "", err
	}
	return string(result), nil
}

func fromJson(s []byte) (any, error) {
	var jsonData any
	err := json.Unmarshal(s, &jsonData)

	if err != nil {
		if serr, ok := err.(*json.SyntaxError); ok {
			if serr.Error() == "unexpected end of JSON input" {
				return nil, io.ErrUnexpectedEOF
			}
		}
	} else {
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
		panic(fmt.Errorf("unrecognized json format: %v", format))
	}
	result, err := marshaler(d)
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
		panic(fmt.Errorf("unknown mode: %v", mode))
	}
}

func handle(input []byte, mode string, format string, readAsSeq bool) (string, error) {
	if !readAsSeq {
		return apply(input, mode, format)
	} else {
		// if it's stupid but it works it's not stupid
		var results []string
		ok := false
		var lastErr error

		for len(input) != 0 {
			var result string

			start, mid, end := 1, 1, len(input)
			for start < end {
				mid = (start + end) >> 1
				_, err := apply(input[:mid], mode, format)

				if err == nil {
					end = mid
				} else if err == io.ErrUnexpectedEOF || err == io.EOF {
					start = mid + 1
				} else {
					end = mid - 1
				}
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
}

func main() {
	var mode string
	flag.StringVar(&mode, "mode", defaultMode, "work mode")
	flag.StringVar(&mode, "m", defaultMode, "work mode (shorthand)")

	var format string
	flag.StringVar(&format, "format", defaultFormat, "format")
	flag.StringVar(&format, "f", defaultFormat, "format (shorthand)")

	readAsSeq := flag.Bool("seq", false, "attempt to read the input as a sequence of (Y/J)SON's")

	flag.Parse()

	var input []byte

	if termutil.Isatty(os.Stdin.Fd()) {
		if flag.NArg() != 1 {
			panic(fmt.Errorf("expected single arg with data"))
		}
		input = []byte(flag.Arg(0))
	} else {
		fi, err := os.Stdin.Stat()
		if err != nil {
			panic(err)
		}
		if fi.Mode()&os.ModeNamedPipe == 0 {
			// stdin is empty, try reading args
			if flag.NArg() != 1 {
				panic(fmt.Errorf("expected either non-empty pipe or single arg with data"))
			}
			input = []byte(flag.Arg(0))
		} else {
			if flag.NArg() != 0 {
				panic(fmt.Errorf("expected no positional args"))
			}

			input, err = io.ReadAll(os.Stdin)
			if err != nil {
				panic(fmt.Errorf("error reading from stdin: %v", err))
			}
		}
	}

	result, err := handle(input, mode, format, *readAsSeq)
	if err != nil {
		panic(fmt.Errorf("conversion resulted in error: %v", err))
	}
	fmt.Println(result)
}
