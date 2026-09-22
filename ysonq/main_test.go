package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// command runs a whole command line over the given input and returns what it
// wrote, what it complained about, and the code it exited with. The environment
// is empty, and the output is not a terminal, so nothing is coloured.
func command(t *testing.T, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()

	var out, errout bytes.Buffer
	code = run(args, &environment{
		stdin:     strings.NewReader(stdin),
		stdout:    &out,
		stderr:    &errout,
		lookupEnv: func(string) (string, bool) { return "", false },
		environ:   func() []string { return nil },
	})
	return out.String(), errout.String(), code
}

// input writes a file for a command line to read, and returns its name.
func input(t *testing.T, name, contents string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRun(t *testing.T) {
	cases := []struct {
		name  string
		args  []string
		stdin string
		want  string
		code  int
	}{
		{
			name:  "a program over one value",
			args:  []string{".foo"},
			stdin: "{foo={bar=<q=e>%true;baz=qqq}}\n",
			want: `{
    "bar" = <
        "q" = "e";
    > %true;
    "baz" = "qqq";
}
`,
		},
		{
			name:  "no program at all is the identity",
			args:  []string{},
			stdin: "{a=1}\n",
			want:  "{\n    \"a\" = 1;\n}\n",
		},
		{
			name:  "compact",
			args:  []string{"-c", "."},
			stdin: "{a=[1;2]}\n",
			want:  "{a=[1;2;];}\n",
		},
		{
			name:  "an attribute by path",
			args:  []string{"-c", ".foo.bar.Attrs.q"},
			stdin: "{foo={bar=<q=e>%true;baz=qqq}}\n",
			want:  "e\n",
		},
		{
			name:  "raw strings",
			args:  []string{"-r", ".[]"},
			stdin: "[q;w;e]\n",
			want:  "q\nw\ne\n",
		},
		{
			name:  "joined raw strings",
			args:  []string{"-j", ".[]"},
			stdin: "[q;w]\n",
			want:  "qw",
		},
		{
			name:  "one value per line, one line per value",
			args:  []string{"-c", "."},
			stdin: "1\n2\n3\n",
			want:  "1\n2\n3\n",
		},
		{
			name:  "a stream with semicolons",
			args:  []string{"-c", "."},
			stdin: "{a=1};{b=2}\n",
			want:  "{a=1;}\n{b=2;}\n",
		},
		{
			name: "the input of -n is null",
			args: []string{"-n", "-c", "1+1"},
			want: "2\n",
		},
		{
			name:  "slurp",
			args:  []string{"-s", "-c", "."},
			stdin: "1\n2\n",
			want:  "[1;2;]\n",
		},
		{
			name:  "raw input",
			args:  []string{"-R", "-c", "."},
			stdin: "a\nb\n",
			// bare in the compact form, as every other string is
			want: "a\nb\n",
		},
		{
			name:  "raw input with slurp is one string",
			args:  []string{"-Rs", "."},
			stdin: "a\nb\n",
			want:  "\"a\\nb\\n\"\n",
		},
		{
			name:  "stream",
			args:  []string{"--stream", "-c", "."},
			stdin: "{a=[1;2]}\n",
			want:  "[[a;0;];1;]\n[[a;1;];2;]\n",
		},
		{
			name:  "an RS-separated input, read and written",
			args:  []string{"--seq", "-c", "."},
			stdin: "\x1e{a=1}\n\x1e{b=2}\n",
			want:  "\x1e{a=1;}\n\x1e{b=2;}\n",
		},
		{
			name: "a string bound to a variable",
			args: []string{"-n", "-c", "--arg", "x", "v", "$x"},
			want: "v\n",
		},
		{
			name: "a JSON value bound to a variable",
			args: []string{"-n", "-c", "--argjson", "x", "[1,2]", "$x"},
			want: "[1;2;]\n",
		},
		{
			name: "a YSON value bound to a variable",
			args: []string{"-n", "-c", "--argyson", "x", "%false", "$x"},
			want: "%false\n",
		},
		{
			name: "a YSON value with attributes bound to a variable",
			args: []string{"-n", "-c", "--argyson", "x", "<q=e>{}", "$x"},
			want: "<q=e;>{}\n",
		},
		{
			name: "the positional arguments as strings",
			args: []string{"-n", "-c", "--args", "$ARGS.positional", "q", "w"},
			want: "[q;w;]\n",
		},
		{
			name: "the positional arguments as JSON",
			args: []string{"-n", "-c", "--jsonargs", "$ARGS.positional", "[true,null]", "1"},
			want: "[[%true;#;];1;]\n",
		},
		{
			name: "a YSON value as one of the positional arguments",
			args: []string{"-n", "--ysonargs", "-c", "$ARGS.positional", "[foo]", "%false"},
			want: "[[foo;];%false;]\n",
		},
		{
			name: "the last value of a name given twice",
			args: []string{"-n", "-c", "--arg", "x", "1", "--arg", "x", "2", "$x, $ARGS.named.x"},
			want: "\"2\"\n\"2\"\n",
		},
		{
			name:  "the input filename over a file",
			args:  []string{"-c", "."},
			stdin: "1\n",
			want:  "1\n",
		},
		{
			name:  "input_filename is null for the input of a pipe",
			args:  []string{"-n", "-c", "input_filename"},
			stdin: "1\n",
			want:  "#\n",
		},
		{
			name:  "an indentation of two spaces",
			args:  []string{"--indent", "2", "."},
			stdin: "{a=1}\n",
			want:  "{\n  \"a\" = 1;\n}\n",
		},
		{
			name:  "tabs",
			args:  []string{"--tab", "."},
			stdin: "{a=1}\n",
			want:  "{\n\t\"a\" = 1;\n}\n",
		},
		{
			name:  "the keys of an object are written in order",
			args:  []string{"-c", "."},
			stdin: "{z=1;a=2}\n",
			want:  "{a=2;z=1;}\n",
		},
		{
			name: "a float keeps being a float",
			args: []string{"-n", "-c", "1.5e3, 2.0, 0.1+0.2"},
			want: "1500.\n2.\n0.30000000000000004\n",
		},
		{
			name: "the numbers YSON has and JSON does not",
			args: []string{"-n", "-c", "nan, infinite, -infinite"},
			want: "%nan\n%inf\n%-inf\n",
		},
		{
			name: "an unsigned integer keeps its width",
			args: []string{"-n", "-c", "18446744073709551615"},
			want: "18446744073709551615u\n",
		},
		{
			name: "a big integer the program computed",
			args: []string{"-n", "-c", "36893488147419103232 / 4"},
			want: "9223372036854775808u\n",
		},
		{
			name: "raw output writes the bytes of a string",
			args: []string{"-n", "-r", "\"q\\u0000\\u0001\""},
			want: "q\x00\x01\n",
		},
		{
			name:  "raw output0 ends every string with a NUL",
			args:  []string{"--raw-output0", ".[]"},
			stdin: "[q;w]\n",
			want:  "q\x00w\x00",
		},
		{
			name:  "the exit status of a false value",
			args:  []string{"-e", "-c", "."},
			stdin: "%false\n",
			want:  "%false\n",
			code:  exitFalsy,
		},
		{
			name:  "the exit status of nothing at all",
			args:  []string{"-e", "empty"},
			stdin: "1\n",
			code:  exitNoValue,
		},
		{
			name:  "the exit status of a true value",
			args:  []string{"-e", "-c", "."},
			stdin: "1\n",
			want:  "1\n",
			code:  exitOK,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stdout, stderr, code := command(t, c.stdin, c.args...)
			if stdout != c.want {
				t.Errorf("%v printed %q, want %q", c.args, stdout, c.want)
			}
			if code != c.code {
				t.Errorf("%v exited with %d, want %d (it said %q)", c.args, code, c.code, stderr)
			}
		})
	}
}

// TestRunFiles checks the command line reads the files it names, in order, and
// that input_filename knows which one it is reading.
func TestRunFiles(t *testing.T) {
	first := input(t, "first.yson", "{a=1}\n{b=2}\n")
	second := input(t, "second.yson", "{c=3}\n")

	stdout, stderr, code := command(t, "", "-c", ".", first, second)
	if want := "{a=1;}\n{b=2;}\n{c=3;}\n"; stdout != want {
		t.Errorf("two files read as %q, want %q", stdout, want)
	}
	if code != exitOK {
		t.Errorf("two files exited with %d: %q", code, stderr)
	}

	stdout, _, _ = command(t, "", "-c", "input_filename", first, second)
	if want := "\"" + first + "\"\n\"" + first + "\"\n\"" + second + "\"\n"; stdout != want {
		t.Errorf("input_filename printed %q, want %q", stdout, want)
	}

	stdout, _, _ = command(t, "", "-s", "-c", ".", first, second)
	if want := "[{a=1;};{b=2;};{c=3;};]\n"; stdout != want {
		t.Errorf("two slurped files read as %q, want %q", stdout, want)
	}

	// -Rs is the other way round: the inputs are read as text and joined into
	// one string rather than a list of lines
	stdout, _, _ = command(t, "", "-Rs", "-c", ".", first, second)
	if want := `"{a=1}\n{b=2}\n{c=3}\n"` + "\n"; stdout != want {
		t.Errorf("two raw slurped files read as %q, want %q", stdout, want)
	}
}

// TestRunRawOutput0 checks the NUL-separated output: a string that has a NUL of
// its own is refused rather than written, since a stream like that could not be
// told apart from the separators.
func TestRunRawOutput0(t *testing.T) {
	stdout, stderr, code := command(t, "", "-n", "--raw-output0", `"a\u0000b"`)
	if code != exitRuntime {
		t.Errorf("a string with a NUL in it exited with %d, want %d", code, exitRuntime)
	}
	if stdout != "" {
		t.Errorf("a string with a NUL in it wrote %q", stdout)
	}
	if !strings.Contains(stderr, "NUL") {
		t.Errorf("a string with a NUL in it said %q, which does not say why", stderr)
	}
}

// TestRunDashIsStdin checks that "-" means the standard input rather than being
// taken for a flag.
func TestRunDashIsStdin(t *testing.T) {
	stdout, stderr, code := command(t, "1\n", "-c", ".", "-")
	if stdout != "1\n" || code != exitOK {
		t.Errorf("- read as %q with code %d: %q", stdout, code, stderr)
	}
}

// TestRunMissingFile checks that a file that cannot be read is a problem with
// the command line — the exit code jq uses for it — while the other files still
// produce their values.
func TestRunMissingFile(t *testing.T) {
	good := input(t, "good.yson", "1\n")

	stdout, stderr, code := command(t, "", "-c", ".", "/nonexistent/yson", good)
	if code != exitUsage {
		t.Errorf("a missing file exited with %d, want %d", code, exitUsage)
	}
	if !strings.Contains(stderr, "no such file") {
		t.Errorf("a missing file said %q, which does not say why", stderr)
	}
	if stdout != "1\n" {
		t.Errorf("the file after a missing one read as %q", stdout)
	}
}

// TestRunBadDocument checks that a document that does not parse is reported with
// the place in the input it was found, and that the exit code is the one jq
// uses for an input that is not the input a program expects.
func TestRunBadDocument(t *testing.T) {
	stdout, stderr, code := command(t, "1\n{a=\n", "-c", ".")
	if code != exitRuntime {
		t.Errorf("a bad document exited with %d, want %d", code, exitRuntime)
	}
	if !strings.Contains(stderr, "<stdin>:2:1") {
		t.Errorf("a bad document said %q, which does not say where", stderr)
	}
	if stdout != "1\n" {
		t.Errorf("the values before a bad document read as %q", stdout)
	}
}

// TestRunRuntimeError checks the failure of the program itself: it is reported
// against the input it happened on, and the code is jq's for a failure while
// running.
func TestRunRuntimeError(t *testing.T) {
	_, stderr, code := command(t, "1\n2\n", ".a")
	if code != exitRuntime {
		t.Errorf("a failure exited with %d, want %d", code, exitRuntime)
	}
	if !strings.Contains(stderr, "at <stdin>:1") {
		t.Errorf("a failure said %q, which does not say where", stderr)
	}
}

func TestRunProgramDoesNotCompile(t *testing.T) {
	_, stderr, code := command(t, "1\n", ".foo |")
	if code != exitCompile {
		t.Errorf("a program that does not compile exited with %d, want %d", code, exitCompile)
	}
	if !strings.Contains(stderr, "compile error") {
		t.Errorf("a program that does not compile said %q", stderr)
	}
	if !strings.Contains(stderr, "<top-level>") {
		t.Errorf("a program that does not compile said %q, which does not say where", stderr)
	}
}

func TestRunBadCommandLine(t *testing.T) {
	cases := [][]string{
		{"--bogus"},
		{"--yaml-output"},
		{"--indent", "9"},
	}
	for _, args := range cases {
		_, stderr, code := command(t, "", args...)
		if code != exitUsage {
			t.Errorf("%v exited with %d, want %d", args, code, exitUsage)
		}
		if stderr == "" {
			t.Errorf("%v said nothing about itself", args)
		}
	}
}

// TestRunProgramFile checks -f reads the program from a file.
func TestRunProgramFile(t *testing.T) {
	path := input(t, "program.jq", ".a\n")

	stdout, stderr, code := command(t, "{a=7}\n", "-c", "-f", path)
	if stdout != "7\n" || code != exitOK {
		t.Errorf("-f read as %q with code %d: %q", stdout, code, stderr)
	}
}

// TestRunFileVariables checks the two flags that bind a file: --slurpfile binds
// the values of a YSON file as a list, --rawfile binds its text as a string.
func TestRunFileVariables(t *testing.T) {
	values := input(t, "values.yson", "{a=1}\n{b=2}\n")
	text := input(t, "text.txt", "a\nb\n")

	stdout, stderr, code := command(t, "", "-n", "-c", "--slurpfile", "v", values, "$v")
	if want := "[{a=1;};{b=2;};]\n"; stdout != want || code != exitOK {
		t.Errorf("--slurpfile read as %q with code %d: %q", stdout, code, stderr)
	}

	stdout, stderr, code = command(t, "", "-n", "-c", "--rawfile", "v", text, "$v")
	if want := "\"a\\nb\\n\"\n"; stdout != want || code != exitOK {
		t.Errorf("--rawfile read as %q with code %d: %q", stdout, code, stderr)
	}
}

// TestRunModules checks that -L makes a module findable, which is what a program
// spread over several files needs.
func TestRunModules(t *testing.T) {
	directory := t.TempDir()
	module := filepath.Join(directory, "answer.jq")
	if err := os.WriteFile(module, []byte("def answer: 42;\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := command(t, "", "-n", "-c", "-L", directory, `include "answer"; answer`)
	if want := "42\n"; stdout != want || code != exitOK {
		t.Errorf("a module read as %q with code %d: %q", stdout, code, stderr)
	}
}

// TestRunDebug checks the two programs that write to the error output: debug
// writes the value as YSON and passes it on, stderr writes a string as it is.
func TestRunDebug(t *testing.T) {
	stdout, stderr, code := command(t, "1\n", "-c", "debug")
	if stdout != "1\n" {
		t.Errorf("debug printed %q, want the value it was given", stdout)
	}
	if stderr != "[\"DEBUG:\";1;]\n" {
		t.Errorf("debug said %q", stderr)
	}
	if code != exitOK {
		t.Errorf("debug exited with %d", code)
	}

	stdout, stderr, _ = command(t, "1\n", "-c", `"note" | stderr`)
	if stdout != "note\n" {
		t.Errorf("the program printed %q, want the value it was given", stdout)
	}
	if stderr != "note" {
		t.Errorf("stderr said %q, want the string as it is", stderr)
	}
}

// TestRunColors checks that colouring follows the flags: off for piped output,
// on with -C, and off again with -M.
func TestRunColors(t *testing.T) {
	stdout, _, _ := command(t, "{a=1}\n", "-c", ".")
	if strings.Contains(stdout, "\x1b[") {
		t.Errorf("output that is not a terminal was coloured: %q", stdout)
	}

	stdout, _, _ = command(t, "{a=1}\n", "-C", "-c", ".")
	if !strings.Contains(stdout, "\x1b[") {
		t.Errorf("-C wrote no colour: %q", stdout)
	}

	stdout, _, _ = command(t, "{a=1}\n", "-C", "-M", "-c", ".")
	if strings.Contains(stdout, "\x1b[") {
		t.Errorf("-M after -C left the colour on: %q", stdout)
	}
}

// TestRunEnvironment checks that a program reads the environment, and that the
// toolkit variables that turn colouring on and off are honoured. Colour is a
// pipe's business as much as a terminal's: a flag says what to do, and the
// variables say what to do when no flag did.
func TestRunEnvironment(t *testing.T) {
	withEnvironment := func(variables map[string]string, args ...string) string {
		t.Helper()

		var out bytes.Buffer
		run(args, &environment{
			stdin:  strings.NewReader("{a=1}\n"),
			stdout: &out,
			stderr: &out,
			lookupEnv: func(name string) (string, bool) {
				value, ok := variables[name]
				return value, ok
			},
			environ: func() []string {
				var all []string
				for name, value := range variables {
					all = append(all, name+"="+value)
				}
				return all
			},
		})
		return out.String()
	}

	if got := withEnvironment(map[string]string{"FOO": "bar"}, "-n", "-c", "$ENV.FOO"); got != "bar\n" {
		t.Errorf("$ENV.FOO was %q", got)
	}
	if got := withEnvironment(map[string]string{"YSON_FORCE_COLOR": "1"}, "-c", "."); !strings.Contains(got, "\x1b[") {
		t.Errorf("YSON_FORCE_COLOR wrote no colour: %q", got)
	}
	// the two variables are read in the order yson-convert reads them: the one
	// that asks for colour is looked for first
	if got := withEnvironment(map[string]string{"YSON_FORCE_COLOR": "1", "YSON_NO_COLOR": "1"}, "-c", "."); !strings.Contains(got, "\x1b[") {
		t.Errorf("YSON_FORCE_COLOR lost to YSON_NO_COLOR: %q", got)
	}
	if got := withEnvironment(map[string]string{"YSON_NO_COLOR": "1"}, "-C", "-c", "."); !strings.Contains(got, "\x1b[") {
		t.Errorf("-C lost to YSON_NO_COLOR: %q", got)
	}
	if got := withEnvironment(map[string]string{"YSON_FORCE_COLOR": "1"}, "-M", "-c", "."); strings.Contains(got, "\x1b[") {
		t.Errorf("-M left the colour of YSON_FORCE_COLOR on: %q", got)
	}
}

// TestRunHelpAndVersion checks the two flags that print something about the
// tool itself instead of running a program.
func TestRunHelpAndVersion(t *testing.T) {
	stdout, _, code := command(t, "", "--help")
	if code != exitOK || !strings.Contains(stdout, "Usage:") {
		t.Errorf("--help printed %q with code %d", stdout, code)
	}

	stdout, _, code = command(t, "", "--version")
	if code != exitOK || !strings.HasPrefix(stdout, name+"-") {
		t.Errorf("--version printed %q with code %d", stdout, code)
	}
}
