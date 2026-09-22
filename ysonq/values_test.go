package main

import (
	"encoding/json"
	"math"
	"math/big"
	"strings"
	"testing"

	"go.ytsaurus.tech/yt/go/yson"
)

// decode reads one YSON document the way the input of a command line does.
func decode(t *testing.T, text string) any {
	t.Helper()

	var value any
	if err := yson.Unmarshal([]byte(text), &value); err != nil {
		t.Fatalf("%q did not parse: %v", text, err)
	}
	return value
}

func TestNumbersToJQ(t *testing.T) {
	cases := []struct {
		name string
		yson string
		want any
	}{
		{"signed", "1", 1},
		{"negative", "-1", -1},
		{"unsigned", "1u", 1},
		{"float", "1.5", 1.5},
		{"zero", "0", 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := toJQ(decode(t, c.yson)); got != c.want {
				t.Errorf("%s read as %#v, want %#v", c.yson, got, c.want)
			}
		})
	}
}

// TestUnsignedAboveInt64 pins the one number the two value models disagree
// about: YSON has unsigned 64-bit integers, jq has signed ones, and a value
// that does not fit into the signed range keeps its value rather than being
// rounded or wrapped.
func TestUnsignedAboveInt64(t *testing.T) {
	got, ok := toJQ(decode(t, "18446744073709551615u")).(*big.Int)
	if !ok {
		t.Fatalf("the largest unsigned integer became %#v", got)
	}
	if got.Cmp(new(big.Int).SetUint64(math.MaxUint64)) != 0 {
		t.Errorf("the largest unsigned integer became %s", got)
	}
}

func TestNaN(t *testing.T) {
	if got := toJQ(decode(t, "%nan")); !math.IsNaN(got.(float64)) {
		t.Errorf("null became %#v", got)
	}
	if got := toJQ(decode(t, "%inf")); !math.IsInf(got.(float64), 1) {
		t.Errorf("infinity became %#v", got)
	}
}

// TestAttributeRoundTrip is the contract the Attrs/Value spelling rests on: a
// node that carries attributes goes through the program as a map, and comes
// back an attributed node rather than a map with two keys in it.
func TestAttributeRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		yson string
		want string
	}{
		{"scalar", "<q=e>1", "<q=e;>1"},
		{"map", "<q=e>{a=1}", "<q=e;>{a=1;}"},
		{"list", "<q=e>[1;2]", "<q=e;>[1;2;]"},
		{"no attributes", "{a=1}", "{a=1;}"},
		{"nested", "{foo=<q=e>{bar=1}}", "{foo=<q=e;>{bar=1;};}"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			text, err := dump(toJQ(decode(t, c.yson)), outputConfig{compact: true})
			if err != nil {
				t.Fatal(err)
			}
			if text != c.want {
				t.Errorf("%s came back as %s, want %s", c.yson, text, c.want)
			}
		})
	}
}

// TestIntegerOutOfRange checks that a number YSON cannot hold is refused
// rather than rounded off: the program can compute whatever it likes, but a
// YSON file has no place to put a number that wide.
func TestIntegerOutOfRange(t *testing.T) {
	tooBig := new(big.Int).Add(new(big.Int).SetUint64(math.MaxUint64), big.NewInt(1))

	_, err := dump(tooBig, outputConfig{compact: true})
	if err == nil {
		t.Fatal("a number wider than YSON's integers was written")
	}
	if !strings.Contains(err.Error(), "does not fit") {
		t.Errorf("the error was %q, which does not say what is wrong", err)
	}
}

// TestNumbersFromJQ checks the values the program produces arrive in YSON's own
// terms: an int is a signed 64-bit integer, and a big integer that fits into
// one of YSON's two is written as that one.
func TestNumbersFromJQ(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  string
	}{
		{"int", 1, "1"},
		{"negative", -1, "-1"},
		{"unsigned", uint64(math.MaxUint64), "18446744073709551615u"},
		{"big unsigned", new(big.Int).SetUint64(math.MaxUint64), "18446744073709551615u"},
		{"big signed", big.NewInt(math.MinInt64), "-9223372036854775808"},
		// a number the program never touched keeps the text it came with
		{"json number", json.Number("1.500"), "1.500"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			text, err := dump(c.value, outputConfig{compact: true})
			if err != nil {
				t.Fatal(err)
			}
			if text != c.want {
				t.Errorf("%#v was written as %s, want %s", c.value, text, c.want)
			}
		})
	}
}
