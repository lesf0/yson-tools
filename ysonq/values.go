package main

import (
	"fmt"
	"math"
	"math/big"

	"github.com/lesf0/yson-tools/ysonlib"
)

// This file is the border between the two value models: the one yt's YSON
// decoder produces and the one gojq works with (nil, bool, int, float64,
// *big.Int, json.Number, string, []any, map[string]any).
//
// They agree on everything but two things. Numbers: YSON has signed and
// unsigned 64-bit integers, jq has int and *big.Int, so a uint64 that does not
// fit into an int64 becomes a big integer rather than losing its value, and a
// big integer that does not fit into a uint64 on the way back is an error
// rather than a rounding. And attributes: YSON values may carry them, jq values
// cannot, so an attributed node travels through jq as the map
// {"Attrs": …, "Value": …} — the spelling yson-convert documents and ysonq
// queries the same way ('.foo.bar.Attrs.q'). Both directions of that live in
// ysonlib, so the two tools cannot drift apart.
//
// A json.Number passes through untouched: it is what --argjson and friends
// produce, it keeps the text it was written with, and the formatter writes that
// text out as it is.

// toJQ converts a value out of the YSON decoder into a jq value.
func toJQ(v any) any {
	return numbersToJQ(ysonlib.FlattenAttrs(v))
}

func numbersToJQ(v any) any {
	switch v := v.(type) {
	case []any:
		for i, x := range v {
			v[i] = numbersToJQ(x)
		}
		return v
	case map[string]any:
		for k, x := range v {
			v[k] = numbersToJQ(x)
		}
		return v
	case int64:
		return int(v)
	case uint64:
		if v > math.MaxInt64 {
			return new(big.Int).SetUint64(v)
		}
		return int(v)
	default:
		return v
	}
}

// fromJQ converts a jq value into something the YSON formatter can write.
func fromJQ(v any) (any, error) {
	numbers, err := numbersFromJQ(v)
	if err != nil {
		return nil, err
	}
	return ysonlib.RestoreAttrs(numbers)
}

func numbersFromJQ(v any) (any, error) {
	switch v := v.(type) {
	case []any:
		for i, x := range v {
			number, err := numbersFromJQ(x)
			if err != nil {
				return nil, err
			}
			v[i] = number
		}
		return v, nil
	case map[string]any:
		for k, x := range v {
			number, err := numbersFromJQ(x)
			if err != nil {
				return nil, err
			}
			v[k] = number
		}
		return v, nil
	case int:
		return int64(v), nil
	case *big.Int:
		return ysonInteger(v)
	default:
		return v, nil
	}
}

func ysonInteger(number *big.Int) (any, error) {
	switch {
	case number.IsInt64():
		return number.Int64(), nil
	case number.IsUint64():
		return number.Uint64(), nil
	default:
		return nil, fmt.Errorf("%s does not fit into YSON's 64-bit integers", number)
	}
}
