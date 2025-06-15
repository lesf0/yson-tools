package common

import (
	"encoding/json"
	"fmt"

	"go.ytsaurus.tech/yt/go/yson"
)

type Format yson.Format

const (
	FormatBinary = Format(yson.FormatBinary)
	FormatText   = Format(yson.FormatText)
	FormatPretty = Format(yson.FormatPretty)
)

func (f Format) String() string {
	return yson.Format(f).String()
}

func FormatFromString(format string) Format {
	switch format {
	case yson.FormatPretty.String():
		return FormatPretty
	case yson.FormatText.String():
		fallthrough
	case "compact": // for compatibility with previous version
		return FormatText
	case yson.FormatBinary.String():
		return FormatBinary
	default:
		panic(fmt.Errorf("unexpected format: %v", format))
	}
}

func (f Format) YsonMarshaler() func(any) ([]byte, error) {
	return func(d any) ([]byte, error) {
		return yson.MarshalFormat(d, yson.Format(f))
	}
}

func (f Format) JsonMarhaler() func(any) ([]byte, error) {
	switch f {
	case FormatPretty:
		return func(v any) ([]byte, error) {
			return json.MarshalIndent(v, "", "\t")
		}
	case FormatText:
		return json.Marshal
	default:
		panic(fmt.Errorf("unrecognized json format: %v", f))
	}
}
