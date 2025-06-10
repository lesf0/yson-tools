package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/andrew-d/go-termutil"
	"go.ytsaurus.tech/yt/go/yson"
	"go.ytsaurus.tech/yt/go/yson/yson2json"
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

	return ysonData, err
}

func toYson(d any, format string) (string, error) {
	var ysonFormat yson.Format
	switch format {
	case prettyFormat:
		ysonFormat = yson.FormatPretty
	case compactFormat:
		ysonFormat = yson.FormatText
	case binaryFormat:
		ysonFormat = yson.FormatBinary
	default:
		panic(fmt.Errorf("unrecognized yson format: %v", format))
	}
	result, err := yson.MarshalFormat(d, ysonFormat)
	if err != nil {
		return "", err
	}
	return string(result), nil
}

func fromJson(s []byte) (any, error) {
	return yson2json.RawMessage{
		JSON:      s,
		UseInt64:  true,
		UseUint64: true,
	}, nil
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

func main() {
	var mode string
	flag.StringVar(&mode, "mode", defaultMode, "work mode")
	flag.StringVar(&mode, "m", defaultMode, "work mode (shorthand)")

	var format string
	flag.StringVar(&format, "format", defaultFormat, "format")
	flag.StringVar(&format, "f", defaultFormat, "format (shorthand)")

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

	result, err := apply(input, mode, format)
	if err != nil {
		panic(fmt.Errorf("conversion resulted in error: %v", err))
	}

	fmt.Println(result)
}
