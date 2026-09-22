package main

import (
	"reflect"
	"strings"
	"testing"
)

// count is for the options that are held as a pointer, since a flag can be
// given more than once and the last one wins: two cases with the same number
// need their own pointers for the comparison below.
func count(n int) *int {
	return &n
}

func colorOff() *bool {
	value := false
	return &value
}

func arg(flag, name, value string, kind argKind) namedArg {
	return namedArg{flag: flag, name: name, value: value, kind: kind}
}

func TestParseFlags(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want *options
		rest []string
	}{
		{
			name: "nothing",
			args: []string{},
			want: &options{},
		},
		{
			name: "long and short are the same flag",
			args: []string{"--compact-output", ".", "-r", "/tmp/a.yson"},
			want: &options{compact: true, rawOutput: true},
			rest: []string{".", "/tmp/a.yson"},
		},
		{
			name: "a cluster of short flags",
			args: []string{"-ncs", "."},
			want: &options{nullInput: true, compact: true, slurp: true},
			rest: []string{"."},
		},
		{
			name: "a long flag with its value after an equals sign",
			args: []string{"--indent=2", "."},
			want: &options{indent: count(2)},
			rest: []string{"."},
		},
		{
			name: "a value taken from the next argument",
			args: []string{"--indent", "2", "."},
			want: &options{indent: count(2)},
			rest: []string{"."},
		},
		{
			name: "a short flag with its value attached",
			args: []string{"-L/tmp/modules", "."},
			want: &options{modulePaths: []string{"/tmp/modules"}},
			rest: []string{"."},
		},
		{
			name: "a short flag with its value in the next argument",
			args: []string{"-L", "/tmp/modules", "."},
			want: &options{modulePaths: []string{"/tmp/modules"}},
			rest: []string{"."},
		},
		{
			name: "two module paths, in the order they were given",
			args: []string{"-L", "/a", "-L", "/b", "."},
			want: &options{modulePaths: []string{"/a", "/b"}},
			rest: []string{"."},
		},
		{
			name: "the last of --tab and --indent is the one that counts",
			args: []string{"--indent", "2", "--tab", "."},
			want: &options{tab: true},
			rest: []string{"."},
		},
		{
			name: "the last of --indent and --tab is the one that counts",
			args: []string{"--tab", "--indent", "3", "."},
			want: &options{indent: count(3)},
			rest: []string{"."},
		},
		{
			name: "the last of -C and -M is the one that counts",
			args: []string{"-C", "-M", "."},
			want: &options{colors: colorOff()},
			rest: []string{"."},
		},
		{
			name: "flags may follow the program",
			args: []string{".", "--compact-output"},
			want: &options{compact: true},
			rest: []string{"."},
		},
		{
			name: "a lone dash is an input, not a flag",
			args: []string{".", "-"},
			want: &options{},
			rest: []string{".", "-"},
		},
		{
			name: "everything after -- is an argument",
			args: []string{"--", "-c", "."},
			want: &options{},
			rest: []string{"-c", "."},
		},
		{
			name: "the values of --arg and its relatives",
			args: []string{"-n", "--arg", "a", "1", "--argjson", "b", "[1]", "--argyson", "c", "%false", "."},
			want: &options{nullInput: true, named: []namedArg{
				arg("--arg", "a", "1", argString),
				arg("--argjson", "b", "[1]", argJSON),
				arg("--argyson", "c", "%false", argYSON),
			}},
			rest: []string{"."},
		},
		{
			name: "the two flags that name a file",
			args: []string{"--slurpfile", "a", "/tmp/a", "--rawfile", "b", "/tmp/b", "."},
			want: &options{named: []namedArg{
				arg("--slurpfile", "a", "/tmp/a", argSlurpFile),
				arg("--rawfile", "b", "/tmp/b", argRawFile),
			}},
			rest: []string{"."},
		},
		{
			name: "the kind of the positional arguments",
			args: []string{"-n", "--jsonargs", "."},
			want: &options{nullInput: true, positionalAsArg: asKind(argJSON)},
			rest: []string{"."},
		},
		{
			name: "the last of --args and --jsonargs is the one that counts",
			args: []string{"-n", "--jsonargs", "--args", "."},
			want: &options{nullInput: true, positionalAsArg: asKind(argString)},
			rest: []string{"."},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, rest, err := parseFlags(c.args)
			if err != nil {
				t.Fatalf("%v did not parse: %v", c.args, err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("%v parsed as %+v, want %+v", c.args, got, c.want)
			}
			if len(rest) != len(c.rest) {
				t.Fatalf("%v left %v, want %v", c.args, rest, c.rest)
			}
			for i := range c.rest {
				if rest[i] != c.rest[i] {
					t.Errorf("%v left %v, want %v", c.args, rest, c.rest)
					break
				}
			}
		})
	}
}

// asKind is for the one option that is held as a pointer to a kind.
func asKind(k argKind) *argKind {
	return &k
}

func TestParseFlagsErrors(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"an unknown long flag", []string{"--bogus"}, "unknown flag --bogus"},
		{"an unknown short flag", []string{"-Z"}, "unknown flag -Z"},
		{"an unknown letter in a cluster", []string{"-cZ"}, "unknown flag -Z"},
		{"a value given to a flag that takes none", []string{"--compact-output=1"}, "does not take a value"},
		{"a missing value", []string{"--indent"}, "needs 1 values"},
		{"a missing second value", []string{"--arg", "a"}, "needs 2 values"},
		{"an indentation that is not a number", []string{"--indent", "x"}, "--indent takes a number"},
		{"an indentation that is too large", []string{"--indent", "8"}, "between 0 and 7"},
		{"a negative indentation", []string{"--indent", "-1"}, "between 0 and 7"},
		{"a flag this tool does not have", []string{"--yaml-output"}, "--yaml-output is not supported yet"},
		{"a short flag this tool does not have", []string{"-a"}, "--ascii-output is not supported yet"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := parseFlags(c.args)
			if err == nil {
				t.Fatalf("%v parsed, and should not have", c.args)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("%v failed with %q, which does not say %q", c.args, err, c.want)
			}
		})
	}
}
