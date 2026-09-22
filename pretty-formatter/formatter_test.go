package formatter

import (
	"encoding/json"
	"math"
	"math/big"
	"strings"
	"testing"

	"go.ytsaurus.tech/yt/go/yson"
)

func pretty(v interface{}) string {
	return NewYsonFormatter(4, true, false, "").Dump(v)
}

func compact(v interface{}) string {
	f := NewYsonFormatter(4, true, false, "")
	f.Compact = true
	return f.Dump(v)
}

func TestPretty(t *testing.T) {
	got := pretty(&yson.ValueWithAttrs{
		Value: true,
		Attrs: map[string]any{"q": "e"},
	})

	want := `<
    "q" = "e";
> %true`

	if got != want {
		t.Errorf("Result was incorrect, got:\n%s\nwant:\n%s", got, want)
	}
}

func TestCompact(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  string
	}{
		{"empty containers", map[string]any{"c": map[string]any{}, "d": []any{}}, "{c={};d=[];}"},
		{"list", []any{int64(1), int64(2)}, "[1;2;]"},
		{"nested", map[string]any{"a": map[string]any{"b": []any{int64(1)}}}, "{a={b=[1;];};}"},
		{"attributed", &yson.ValueWithAttrs{Value: true, Attrs: map[string]any{"q": "e"}}, "<q=e;>%true"},
		{"attributed without attrs", &yson.ValueWithAttrs{Value: true, Attrs: map[string]any{}}, "<>%true"},
		{"entity", nil, "#"},
		{"unsigned", uint64(math.MaxUint64), "18446744073709551615u"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := compact(c.value); got != c.want {
				t.Errorf("Result was incorrect, got: %s, want: %s.", got, c.want)
			}
		})
	}
}

// A compact value is one line of YSON: the output has to parse back into the
// value it was made from.
func TestCompactRoundTrip(t *testing.T) {
	original := map[string]any{
		"a": "text",
		"b": []any{int64(1), 1.5, true, nil},
		"c": &yson.ValueWithAttrs{Value: "v", Attrs: map[string]any{"q": "e"}},
	}

	text := compact(original)

	var parsed any
	if err := yson.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("Output %q did not parse: %v", text, err)
	}
	if roundTrip := compact(parsed); roundTrip != text {
		t.Errorf("Round trip was incorrect, got: %s, want: %s.", roundTrip, text)
	}
}

// The compact layout is what yson-convert -f compact has always printed, which
// is yt's own text format: comparing against yt keeps the two from drifting.
// Floats are left out on purpose — yt writes them with %f, so 1.5 comes out as
// 1.500000, and this formatter writes the shortest text instead.
func TestCompactMatchesYt(t *testing.T) {
	cases := []any{
		map[string]any{"a": "qqq"},
		map[string]any{"a b": "with space"},
		map[string]any{"": "empty key"},
		map[string]any{"1a": "leading digit"},
		map[string]any{"_a": "underscore"},
		map[string]any{"ключ": "non-ascii"},
		map[string]any{"a": "true"},
		map[string]any{"a": "nan"},
		map[string]any{"a": ""},
		map[string]any{"a": []any{}},
		map[string]any{"a": map[string]any{}},
		[]any{int64(1), uint64(2), true, nil, "bare", "not bare"},
		&yson.ValueWithAttrs{Value: "v", Attrs: map[string]any{"q": "e"}},
		&yson.ValueWithAttrs{Value: true, Attrs: map[string]any{}},
		"a string with \"quotes\"\nand a newline",
		"\xff",
	}

	for _, c := range cases {
		want, err := yson.MarshalFormat(c, yson.FormatText)
		if err != nil {
			t.Fatalf("%#v did not marshal: %v", c, err)
		}
		if got := compact(c); got != string(want) {
			t.Errorf("Result was incorrect for %#v, got: %s, want: %s.", c, got, want)
		}
	}
}

// A string that is written bare has to read back as the same string, not as a
// number or a keyword.
func TestCompactBareStringsRoundTrip(t *testing.T) {
	for _, s := range []string{"qqq", "a1", "true", "nan", "inf", "q"} {
		text := compact(s)

		var parsed any
		if err := yson.Unmarshal([]byte(text), &parsed); err != nil {
			t.Fatalf("Output %q did not parse: %v", text, err)
		}
		if parsed != s {
			t.Errorf("%q was written as %q and read back as %#v", s, text, parsed)
		}
	}
}

// 0x7FF0000000000000 is the bit pattern of +Inf, but as an integer constant it
// is 2^63: comparing a float against it turned every large float into %inf.
func TestLargeFloatsAreNotInfinity(t *testing.T) {
	cases := []struct {
		value float64
		want  string
	}{
		{1e30, "1e+30"},
		{1e300, "1e+300"},
		{1.5, "1.5"},
		{1e10, "1e+10"},
		{0, "0."},
		{1e-300, "1e-300"},
		{math.Inf(1), "%inf"},
		{math.Inf(-1), "%-inf"},
		{math.NaN(), "%nan"},
	}

	for _, c := range cases {
		if got := pretty(c.value); got != c.want {
			t.Errorf("Formatting %v gave %q, want %q.", c.value, got, c.want)
		}

		// and the text has to read back as the very same float
		text := compact(c.value)
		var parsed any
		if err := yson.Unmarshal([]byte(text), &parsed); err != nil {
			t.Errorf("Output %q for %v did not parse: %v", text, c.value, err)
			continue
		}
		if f, ok := parsed.(float64); !ok || (f != c.value && !(math.IsNaN(f) && math.IsNaN(c.value))) {
			t.Errorf("%v was written as %q and read back as %#v", c.value, text, parsed)
		}
	}
}

// Values from a jq program are not the types the YSON parser produces: a
// json.Number has the kind of a string, a big integer the kind of a pointer.
func TestNumbersFromJQ(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  string
	}{
		{"json.Number integer", json.Number("1234"), "1234"},
		{"json.Number float", json.Number("1.5"), "1.5"},
		{"big.Int", big.NewInt(-9223372036854775808), "-9223372036854775808"},
		{"big.Int unsigned", new(big.Int).SetUint64(math.MaxUint64), "18446744073709551615u"},
		{"big.Int in a map", map[string]any{"a": big.NewInt(1)}, "{a=1;}"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := compact(c.value); got != c.want {
				t.Errorf("Result was incorrect, got: %s, want: %s.", got, c.want)
			}
		})
	}
}

func TestBigIntOutOfRange(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected a panic for a number YSON cannot hold")
		}
	}()

	tooBig, _ := new(big.Int).SetString("99999999999999999999999999", 10)
	compact(tooBig)
}

// A byte that is not UTF-8 came out of a binary string: it has to survive the
// printing, not turn into U+FFFD.
func TestInvalidUTF8(t *testing.T) {
	got := compact("\xff\xfe")

	if want := `"\xFF\xFE"`; got != want {
		t.Errorf("Result was incorrect, got: %s, want: %s.", got, want)
	}
}

func TestValidUTF8IsKept(t *testing.T) {
	got := compact("привет")

	if want := `"привет"`; got != want {
		t.Errorf("Result was incorrect, got: %s, want: %s.", got, want)
	}
}

func TestEscapes(t *testing.T) {
	got := compact("a\"b\\c\nd\te")

	if want := `"a\"b\\c\nd\te"`; got != want {
		t.Errorf("Result was incorrect, got: %s, want: %s.", got, want)
	}
}

// The pretty layout is what the tools printed before Compact existed, down to
// the indentation of a nested value.
func TestPrettyLayoutUnchanged(t *testing.T) {
	got := pretty(map[string]any{
		"foo": map[string]any{"bar": []any{int64(1), int64(2)}},
	})

	want := strings.TrimPrefix(`{
    "foo" = {
        "bar" = [
            1;
            2;
        ];
    };
}`, "")

	if got != want {
		t.Errorf("Result was incorrect, got:\n%s\nwant:\n%s", got, want)
	}
}
