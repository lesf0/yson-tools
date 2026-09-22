package ysonlib

import (
	"errors"
	"io"
	"testing"

	"go.ytsaurus.tech/yt/go/yson"
)

// values reads the whole stream and compares it with the values the documents
// hold when parsed on their own.
func values(t *testing.T, data string) []string {
	t.Helper()

	var got []string
	rest := []byte(data)
	for {
		value, next, err := NextValue(rest)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			got = append(got, "<"+err.Error()+">")
			break
		}

		text, err := yson.MarshalFormat(value, yson.FormatText)
		if err != nil {
			t.Fatalf("%q did not marshal back: %v", string(data), err)
		}
		got = append(got, string(text))
		rest = next
	}
	return got
}

func checkValues(t *testing.T, data string, want ...string) {
	t.Helper()

	got := values(t, data)
	if len(got) != len(want) {
		t.Fatalf("%q read as %v, want %v", data, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%q read as %v, want %v", data, got, want)
			return
		}
	}
}

func TestNextValue(t *testing.T) {
	cases := []struct {
		name string
		data string
		want []string
	}{
		{"empty", "", nil},
		{"separators only", " \n\t;", nil},
		{"one value", "{a=1}", []string{"{a=1;}"}},
		{"surrounded by space", "  {a=1}  ", []string{"{a=1;}"}},
		{"one per line", "{a=1}\n{b=2}\n", []string{"{a=1;}", "{b=2;}"}},
		{"no separator at all", "{a=1}{b=2}", []string{"{a=1;}", "{b=2;}"}},
		{"semicolons", "{a=1};{b=2};", []string{"{a=1;}", "{b=2;}"}},
		{"literals separated by semicolons", "1;2;3;", []string{"1", "2", "3"}},
		{"literals separated by newlines", "1\n2\n3\n", []string{"1", "2", "3"}},
		{"a semicolon inside a string", `"a;b";2`, []string{`"a;b"`, "2"}},
		{"a semicolon inside a quoted key", `{a=";"};2`, []string{"{a=\";\";}", "2"}},
		{"empty containers", "[];{}", []string{"[]", "{}"}},
		{"attributes", "<q=e>%true\n{a=1}", []string{"<q=e;>%true", "{a=1;}"}},
		{"attributes and semicolons", "<q=e>%true;{a=1}", []string{"<q=e;>%true", "{a=1;}"}},
		{"nested attributes", "<q=e>{a=<r=t>[1;2;]}", []string{"<q=e;>{a=<r=t;>[1;2;];}"}},
		{"exponent notation", "1.5e3 4", []string{"1500.000000", "4"}},
		{"unsigned and nan", "18446744073709551615u %nan", []string{"18446744073709551615u", "%nan"}},
		{"entity", "#;%false", []string{"#", "%false"}},
		{"bare strings", "qqq\nbar", []string{"qqq", "bar"}},
		{"trailing separator", "1;", []string{"1"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			checkValues(t, c.data, c.want...)
		})
	}
}

// NextValue has to report the rest of the input, so that a caller which stops
// early (or reports a position in an error) knows where it stands.
func TestNextValueRest(t *testing.T) {
	_, rest, err := NextValue([]byte("{a=1}\n{b=2}\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(rest) != "\n{b=2}\n" {
		t.Errorf("rest was %q, want %q", rest, "\n{b=2}\n")
	}
}

func TestNextValueRejectsGarbage(t *testing.T) {
	_, _, err := NextValue([]byte("{a="))
	if err == nil {
		t.Fatal("expected an error for an unterminated map")
	}
	if _, ok := err.(*yson.SyntaxError); !ok {
		t.Errorf("error was %T (%v), want a *yson.SyntaxError", err, err)
	}
}
