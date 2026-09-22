package ysonlib

import (
	"encoding/json"
	"testing"

	"go.ytsaurus.tech/yt/go/yson"
)

func TestFlattenAttrs(t *testing.T) {
	got := FlattenAttrs(&yson.ValueWithAttrs{
		Value: true,
		Attrs: map[string]any{"q": "e"},
	})

	want := map[string]any{
		ValueKey: true,
		AttrsKey: map[string]any{"q": "e"},
	}

	if !equal(got, want) {
		t.Errorf("Result was incorrect, got: %#v, want: %#v.", got, want)
	}
}

func TestFlattenAttrsNested(t *testing.T) {
	// an attributed value inside a map inside a list, the shape a real
	// document takes
	got := FlattenAttrs(map[string]any{
		"foo": []any{
			&yson.ValueWithAttrs{Value: int64(1), Attrs: map[string]any{"q": "e"}},
		},
	})

	want := map[string]any{
		"foo": []any{
			map[string]any{ValueKey: int64(1), AttrsKey: map[string]any{"q": "e"}},
		},
	}

	if !equal(got, want) {
		t.Errorf("Result was incorrect, got: %#v, want: %#v.", got, want)
	}
}

func TestRestoreAttrs(t *testing.T) {
	got, err := RestoreAttrs(map[string]any{
		ValueKey: true,
		AttrsKey: map[string]any{"q": "e"},
	})
	if err != nil {
		t.Fatalf("Should not produce an error, got: %v", err)
	}

	want := &yson.ValueWithAttrs{Value: true, Attrs: map[string]any{"q": "e"}}
	if !equal(got, want) {
		t.Errorf("Result was incorrect, got: %#v, want: %#v.", got, want)
	}
}

func TestRestoreAttrsRoundTrip(t *testing.T) {
	original := &yson.ValueWithAttrs{
		Value: map[string]any{"a": []any{int64(1), "two"}},
		Attrs: map[string]any{"q": "e"},
	}

	got, err := RestoreAttrs(FlattenAttrs(original))
	if err != nil {
		t.Fatalf("Should not produce an error, got: %v", err)
	}
	if !equal(got, original) {
		t.Errorf("Round trip was incorrect, got: %#v, want: %#v.", got, original)
	}
}

// A map that has only one of the two keys is an ordinary map: the convention
// cannot have been meant by it.
func TestRestoreAttrsIgnoresPartialConvention(t *testing.T) {
	for _, v := range []map[string]any{
		{ValueKey: 1},
		{AttrsKey: map[string]any{"q": "e"}},
	} {
		got, err := RestoreAttrs(v)
		if err != nil {
			t.Fatalf("Should not produce an error, got: %v", err)
		}
		if _, isNode := got.(*yson.ValueWithAttrs); isNode {
			t.Errorf("%#v was taken for an attributed value", v)
		}
	}
}

// Both keys are there but Attrs cannot stand for attributes, so the map is
// malformed rather than something to guess at.
func TestRestoreAttrsRejectsNonMapAttrs(t *testing.T) {
	if _, err := RestoreAttrs(map[string]any{ValueKey: 1, AttrsKey: 2}); err == nil {
		t.Error("expected an error for a non-map Attrs")
	}
}

func TestNumbersToJSON(t *testing.T) {
	got := NumbersToJSON(map[string]any{"a": 1.5, "b": int64(2)})

	want := map[string]any{"a": json.Number("1.5e+00"), "b": int64(2)}
	if !equal(got, want) {
		t.Errorf("Result was incorrect, got: %#v, want: %#v.", got, want)
	}
}

func TestNumbersFromJSON(t *testing.T) {
	got, err := NumbersFromJSON(map[string]any{
		"i": json.Number("1234"),
		"f": json.Number("1.5"),
	})
	if err != nil {
		t.Fatalf("Should not produce an error, got: %v", err)
	}

	want := map[string]any{"i": int64(1234), "f": 1.5}
	if !equal(got, want) {
		t.Errorf("Result was incorrect, got: %#v, want: %#v.", got, want)
	}
}

func TestNumbersFromJSONRejectsGarbage(t *testing.T) {
	if _, err := NumbersFromJSON(json.Number("nonsense")); err == nil {
		t.Error("expected an error for a number that is not a number")
	}
}

// equal compares two values by their YSON form, which is insensitive to the
// int64/uint64/float64 differences the parser is free to choose between.
func equal(a, b any) bool {
	x, err := yson.Marshal(a)
	if err != nil {
		panic(err)
	}
	y, err := yson.Marshal(b)
	if err != nil {
		panic(err)
	}
	return string(x) == string(y)
}
