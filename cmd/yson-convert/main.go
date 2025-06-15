package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/andrew-d/go-termutil"
	"github.com/lesf0/yson-tools/pkg/common"
)

const guessMode = "guess"
const prettifyMode = "pretty"
const json2ysonMode = "j2y"
const yson2jsonMode = "y2j"

const defaultMode = guessMode

const defaultFormat = common.FormatPretty

func apply(input []byte, mode string, format common.Format) ([]byte, error) {
	switch mode {
	case guessMode:
		result, err := common.Chain(common.YSON, common.JSON, format)(input)
		if err != nil {
			return common.Chain(common.JSON, common.YSON, format)(input)
		}
		return result, nil
	case prettifyMode:
		return common.Chain(common.YSON, common.YSON, format)(input)
	case json2ysonMode:
		return common.Chain(common.JSON, common.YSON, format)(input)
	case yson2jsonMode:
		return common.Chain(common.YSON, common.JSON, format)(input)
	default:
		panic(fmt.Errorf("unknown mode: %v", mode))
	}
}

func main() {
	var mode string
	flag.StringVar(&mode, "mode", defaultMode, "work mode")
	flag.StringVar(&mode, "m", defaultMode, "work mode (shorthand)")

	var format string
	flag.StringVar(&format, "format", defaultFormat.String(), "format")
	flag.StringVar(&format, "f", defaultFormat.String(), "format (shorthand)")

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

	result, err := apply(input, mode, common.FormatFromString(format))
	if err != nil {
		panic(fmt.Errorf("conversion resulted in error: %v", err))
	}

	fmt.Println(string(result))
}
