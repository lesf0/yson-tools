package main

import "go.ytsaurus.tech/yt/go/yson"

const (
	valueKey = "Value"
	attrsKey = "Attrs"
)

func NormalizeYSON(v any) any {
	switch v := v.(type) {
	case *yson.ValueWithAttrs:
		return map[string]any{
			valueKey: NormalizeYSON(v.Value),
			attrsKey: NormalizeYSON(v.Attrs),
		}
	case []any:
		for i, x := range v {
			v[i] = NormalizeYSON(x)
		}
		return v
	case map[string]any:
		for k, x := range v {
			v[k] = NormalizeYSON(x)
		}
		return v
	default:
		return v
	}
}

func DenormalizeYSON(v any) any {
	switch v := v.(type) {
	case []any:
		for i, x := range v {
			v[i] = DenormalizeYSON(x)
		}
		return v
	case map[string]any:
		// check if map is actually a value with attributes
		value, hasValue := v[valueKey]
		attrs, hasAttrs := v[attrsKey]

		if hasValue && hasAttrs {
			return &yson.ValueWithAttrs{
				Value: DenormalizeYSON(value),
				Attrs: DenormalizeYSON(attrs).(map[string]any),
			}
		}

		for k, x := range v {
			v[k] = DenormalizeYSON(x)
		}
		return v
	default:
		return v
	}
}
