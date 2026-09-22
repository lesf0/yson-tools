// Package ysonlib holds the YSON value model shared by the yson-tools command
// line programs.
//
// YSON parses into plain Go values: maps become map[string]any, lists become
// []any, numbers become int64/uint64/float64, and a value that carries
// attributes becomes a *yson.ValueWithAttrs. Programs that hand such a value to
// something which only knows maps and scalars — jq, JSON — flatten the
// attributed nodes into {"Attrs": …, "Value": …} maps first; RestoreAttrs is
// the inverse.
//
// That Attrs/Value spelling is part of what ysonq documents to its users, so it
// lives here rather than in either program: the tools have to agree on it, and
// this is the only place that can make them.
package ysonlib

import (
	"encoding/json"
	"fmt"
	"strconv"

	"go.ytsaurus.tech/yt/go/yson"
)

const (
	// ValueKey and AttrsKey spell out an attributed YSON node as a map, which
	// is the only shape a JSON or jq value can take.
	ValueKey = "Value"
	AttrsKey = "Attrs"
)

// FlattenAttrs replaces every attributed node in v with a map of the form
// {"Attrs": …, "Value": …}, recursively. Maps and lists are modified in place.
func FlattenAttrs(v any) any {
	switch v := v.(type) {
	case *yson.ValueWithAttrs:
		return map[string]any{
			ValueKey: FlattenAttrs(v.Value),
			AttrsKey: FlattenAttrs(v.Attrs),
		}
	case []any:
		for i, x := range v {
			v[i] = FlattenAttrs(x)
		}
		return v
	case map[string]any:
		for k, x := range v {
			v[k] = FlattenAttrs(x)
		}
		return v
	default:
		return v
	}
}

// RestoreAttrs is the inverse of FlattenAttrs: it replaces every map that has
// both an "Attrs" and a "Value" key with an attributed node, recursively.
//
// A map with only one of the two keys is an ordinary map that happens to use
// those names, and is returned as it is. A map that has both, but whose
// "Attrs" is not a map, is malformed: there is no attributed node it could have
// stood for, and silently passing it through would turn a mistake in the query
// into unexpected output rather than an error.
func RestoreAttrs(v any) (any, error) {
	switch v := v.(type) {
	case []any:
		for i, x := range v {
			restored, err := RestoreAttrs(x)
			if err != nil {
				return nil, err
			}
			v[i] = restored
		}
		return v, nil
	case map[string]any:
		value, hasValue := v[ValueKey]
		attrs, hasAttrs := v[AttrsKey]

		if hasValue && hasAttrs {
			attrsMap, ok := attrs.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("%q must be a map, got %s", AttrsKey, typeName(attrs))
			}
			restoredValue, err := RestoreAttrs(value)
			if err != nil {
				return nil, err
			}
			restoredAttrs, err := RestoreAttrs(attrsMap)
			if err != nil {
				return nil, err
			}
			return &yson.ValueWithAttrs{
				Value: restoredValue,
				Attrs: restoredAttrs.(map[string]any),
			}, nil
		}

		for k, x := range v {
			restored, err := RestoreAttrs(x)
			if err != nil {
				return nil, err
			}
			v[k] = restored
		}
		return v, nil
	default:
		return v, nil
	}
}

// NumbersToJSON replaces float32 and float64 with json.Number, recursively, so
// that a float keeps the text it was written with instead of picking up binary
// rounding on the way through JSON. Integers are left alone.
func NumbersToJSON(v any) any {
	switch v := v.(type) {
	case []any:
		for i, x := range v {
			v[i] = NumbersToJSON(x)
		}
		return v
	case map[string]any:
		for k, x := range v {
			v[k] = NumbersToJSON(x)
		}
		return v
	case *yson.ValueWithAttrs:
		return &yson.ValueWithAttrs{
			Value: NumbersToJSON(v.Value),
			Attrs: NumbersToJSON(v.Attrs).(map[string]any),
		}
	case float32:
		return json.Number(strconv.FormatFloat(float64(v), 'e', -1, 32))
	case float64:
		return json.Number(strconv.FormatFloat(v, 'e', -1, 64))
	default:
		return v
	}
}

// NumbersFromJSON is the inverse of NumbersToJSON: json.Number becomes int64
// when it holds an integer and float64 otherwise.
func NumbersFromJSON(v any) (any, error) {
	switch v := v.(type) {
	case []any:
		for i, x := range v {
			number, err := NumbersFromJSON(x)
			if err != nil {
				return nil, err
			}
			v[i] = number
		}
		return v, nil
	case map[string]any:
		for k, x := range v {
			number, err := NumbersFromJSON(x)
			if err != nil {
				return nil, err
			}
			v[k] = number
		}
		return v, nil
	case *yson.ValueWithAttrs:
		value, err := NumbersFromJSON(v.Value)
		if err != nil {
			return nil, err
		}
		attrs, err := NumbersFromJSON(v.Attrs)
		if err != nil {
			return nil, err
		}
		return &yson.ValueWithAttrs{Value: value, Attrs: attrs.(map[string]any)}, nil
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return i, nil
		}
		f, err := v.Float64()
		if err != nil {
			return nil, fmt.Errorf("%q is neither an integer nor a float", v)
		}
		return f, nil
	default:
		return v, nil
	}
}

// typeName names a value the way a person would, for error messages: the YSON
// type of the value rather than its Go type.
func typeName(v any) string {
	switch v.(type) {
	case nil:
		return "#"
	case bool:
		return "boolean"
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, json.Number:
		return "a number"
	case string:
		return "a string"
	case []any:
		return "a list"
	case map[string]any:
		return "a map"
	case *yson.ValueWithAttrs:
		return "an attributed value"
	default:
		return fmt.Sprintf("%T", v)
	}
}
