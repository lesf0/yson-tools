// Command ysonq runs jq programs over YSON.
//
// It is jq for YSON in the sense that the program is jq's: the query language
// comes from gojq, so filters, builtins, modules and --arg-style variables all
// behave the way jq's documentation describes. What this command supplies is
// the YSON around that: it reads YSON, hands the values to the program in the
// shape gojq works with, and writes the result back as YSON — attributes
// included, as the Attrs/Value map yson-convert documents.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/lesf0/yson-tools/ysonlib"
	"golang.org/x/term"
)

// name is what the tool calls itself in its messages.
const name = "ysonq"

// version is what --version reports. A release is built with
// -ldflags "-X main.version=<tag>", so a development build says so.
var version = "dev"

// stdinName is what the standard input is called in messages; input_filename
// answers null for it, since a name in the output of a program should be a name
// the program's caller wrote.
const stdinName = "<stdin>"

// The exit codes, the ones jq uses: 0 fine, 1 the last value was false or null
// under -e, 2 a bad command line or an input that could not be read, 3 the
// program does not compile, 4 -e and nothing was produced, 5 a failure while
// running.
const (
	exitOK = iota
	exitFalsy
	exitUsage
	exitCompile
	exitNoValue
	exitRuntime
)

// defaultColors is the scheme jq's and yson-convert's colouring uses when
// JQ_COLORS says nothing: null, false, true, numbers, strings, arrays, objects,
// keys.
const defaultColors = "0;90:0;39:0;39:0;39:0;32:1;39:1;39:1;34"

func main() {
	os.Exit(run(os.Args[1:], newEnvironment()))
}

// environment is everything the command reads from outside itself, so that a
// test can run a whole command line without a process of its own.
type environment struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer

	// lookupEnv reads one variable, environ all of them, which is what `env`
	// and `$ENV` inside a program see
	lookupEnv func(name string) (string, bool)
	environ   func() []string

	// stdoutIsTerminal is what decides whether the output is coloured by
	// default
	stdoutIsTerminal bool
}

func newEnvironment() *environment {
	return &environment{
		stdin:            os.Stdin,
		stdout:           os.Stdout,
		stderr:           os.Stderr,
		lookupEnv:        os.LookupEnv,
		environ:          os.Environ,
		stdoutIsTerminal: term.IsTerminal(int(os.Stdout.Fd())),
	}
}

func run(args []string, env *environment) int {
	opts, rest, err := parseFlags(args)
	if err != nil {
		fmt.Fprintf(env.stderr, "%s: %v\n", name, err)
		fmt.Fprintf(env.stderr, "Run '%s --help' for the options.\n", name)
		return exitUsage
	}
	if opts.help {
		// not fmt.Fprint: the help text shows YSON literals, and a literal like
		// %true reads to the printf checker as a formatting directive
		io.WriteString(env.stdout, usage)
		return exitOK
	}
	if opts.version {
		fmt.Fprintf(env.stdout, "%s-%s\n", name, version)
		return exitOK
	}
	if opts.buildConfiguration {
		fmt.Fprintln(env.stdout, buildConfiguration())
		return exitOK
	}

	program, err := parseProgram(opts, rest)
	if err != nil {
		return fail(env, exitUsage, err)
	}
	names, values, err := bindings(opts, program.positional)
	if err != nil {
		return fail(env, exitUsage, err)
	}

	input := newInput(program.paths, documentSourceFor(opts), reader(env))
	defer input.Close()

	// --stream and --slurp sit between the reading and the program, which is
	// where gojq puts them, so `input` inside a program sees what they produce
	iter := input
	if opts.stream && !opts.rawInput {
		iter = &streamed{iter: iter}
	}
	switch {
	case opts.rawInput && opts.slurp:
		// -R -s is the whole input as one string, however many inputs it came
		// from
		iter = &wholeInputs{iter: iter}
	case opts.slurp:
		iter = &slurp{iter: iter}
	}

	query, err := gojq.Parse(program.query)
	if err != nil {
		fmt.Fprint(env.stderr, programError(program, err))
		return exitCompile
	}

	code, err := gojq.Compile(query,
		gojq.WithModuleLoader(gojq.NewModuleLoader(modulePaths(opts))),
		gojq.WithEnvironLoader(env.environ),
		gojq.WithVariables(names),
		gojq.WithFunction("debug", 0, 0, debugFunction(env)),
		gojq.WithFunction("stderr", 0, 0, stderrFunction(env)),
		gojq.WithFunction("input_filename", 0, 0, inputFilename(iter)),
		gojq.WithInputIter(iter),
	)
	if err != nil {
		fmt.Fprint(env.stderr, programError(program, err))
		return exitCompile
	}

	return execute(env, opts, code, values, iter)
}

// execute runs the program over the input values and writes what it produces.
func execute(env *environment, opts *options, code *gojq.Code, values []any, iter inputIter) int {
	out := newWriter(env.stdout, outputConfig{
		compact:    opts.compact,
		raw:        opts.rawOutput,
		join:       opts.joinOutput,
		raw0:       opts.rawOutput0,
		seq:        opts.seq,
		indent:     indentation(opts),
		colors:     useColors(opts, env),
		scheme:     colorScheme(env),
		unbuffered: opts.unbuffered,
	})

	// -n runs the program once, on null; `input` and `inputs` inside it still
	// read the input, as they do in jq
	var main inputIter = iter
	if opts.nullInput {
		main = &nullInput{}
	}

	exit := exitOK
	wrote := false
	falsy := false

loop:
	for {
		value, ok := main.Next()
		if !ok {
			break
		}
		if err, ok := value.(error); ok {
			// an input that cannot be read or does not parse is reported, and
			// the rest of the input is still read: one bad file should not cost
			// the values of the others
			fmt.Fprintf(env.stderr, "%s: %v\n", name, err)
			if exit == exitOK {
				exit = exitForInput(err)
			}
			continue
		}

		results := code.Run(value, values...)
		for {
			result, ok := results.Next()
			if !ok {
				break
			}
			if err, ok := result.(error); ok {
				var halt *gojq.HaltError
				if errors.As(err, &halt) {
					// halt is the program saying it is done, with the output it
					// has produced so far
					if err := haltOutput(env, halt); err != nil {
						fmt.Fprintf(env.stderr, "%s: %v\n", name, err)
					}
					exit = halt.ExitCode()
					break loop
				}
				fmt.Fprintf(env.stderr, "%s: error (at %s): %v\n", name, inputAt(main), err)
				exit = exitRuntime
				break loop
			}

			if err := out.write(result); err != nil {
				fmt.Fprintf(env.stderr, "%s: %v\n", name, err)
				return exitRuntime
			}
			wrote = true
			falsy = result == nil || result == false
		}
	}

	if err := out.flush(); err != nil {
		fmt.Fprintf(env.stderr, "%s: %v\n", name, err)
		return exitRuntime
	}

	if exit == exitOK && opts.exitStatus {
		switch {
		case !wrote:
			exit = exitNoValue
		case falsy:
			exit = exitFalsy
		}
	}
	return exit
}

// reader is how the command line reads its input: the standard input for the
// names jq gives it, a file for anything else.
func reader(env *environment) func(path string) ([]byte, error) {
	return func(path string) ([]byte, error) {
		if path == "-" {
			return io.ReadAll(env.stdin)
		}
		return os.ReadFile(path)
	}
}

// program is the jq program to run and the arguments that go with it.
type program struct {
	query string
	// name is where the query came from, for the error message of one that does
	// not compile
	name string
	// paths are the input files; empty means the standard input
	paths []string
	// positional is $ARGS.positional, when --args and its relatives are in play
	positional []any
}

// parseProgram takes the program off the command line: the first argument,
// unless -f says it is the contents of the file it names. What is left over is
// either a list of input files or, with --args and its relatives, the values of
// $ARGS.positional.
func parseProgram(opts *options, args []string) (*program, error) {
	p := &program{query: ".", name: "<arg>"}

	switch {
	case opts.fromFile:
		if len(args) == 0 {
			return nil, errors.New("expected the name of a program file after -f")
		}
		source, err := os.ReadFile(args[0])
		if err != nil {
			return nil, err
		}
		p.query, p.name, args = string(source), args[0], args[1:]
	case len(args) > 0:
		p.query, args = strings.TrimSpace(args[0]), args[1:]
	}

	if opts.positionalAsArg == nil {
		p.paths = args
		return p, nil
	}
	for _, text := range args {
		value, err := argValue(*opts.positionalAsArg, text)
		if err != nil {
			return nil, err
		}
		p.positional = append(p.positional, value)
	}
	return p, nil
}

// bindings makes the variables the program may name: those the flags bound, and
// $ARGS, which holds them by name together with the positional arguments.
//
// A name given twice is bound once, to the value given last, as in jq, and
// $ARGS.named holds that value like the variable does.
func bindings(opts *options, positional []any) (names []string, values []any, err error) {
	order := []string{}
	named := map[string]any{}
	for _, arg := range opts.named {
		value, err := namedValue(arg)
		if err != nil {
			return nil, nil, fmt.Errorf("%s %s: %v", arg.flag, arg.name, err)
		}
		if _, seen := named[arg.name]; !seen {
			order = append(order, arg.name)
		}
		named[arg.name] = value
	}
	for _, name := range order {
		names = append(names, "$"+name)
		values = append(values, named[name])
	}

	names = append(names, "$ARGS")
	values = append(values, map[string]any{"named": named, "positional": positional})
	return names, values, nil
}

// namedValue reads what a --arg and its relatives bind: the contents of a whole
// file for the two flags that name one, a value read the way the flag says for
// the rest.
func namedValue(arg namedArg) (any, error) {
	switch arg.kind {
	case argSlurpFile:
		return readValues(arg.value)
	case argRawFile:
		data, err := os.ReadFile(arg.value)
		if err != nil {
			return nil, err
		}
		return string(data), nil
	default:
		return argValue(arg.kind, arg.value)
	}
}

// argValue reads one value from its text, the way the flag it came from says to
// read it.
func argValue(kind argKind, text string) (any, error) {
	switch kind {
	case argJSON:
		decoder := json.NewDecoder(strings.NewReader(text))
		// a number keeps the text it was written with: rounding it would lose
		// what a caller wrote, and the formatter can write it as it is
		decoder.UseNumber()
		var value any
		if err := decoder.Decode(&value); err != nil {
			return nil, fmt.Errorf("%q is not JSON: %v", text, err)
		}
		return value, nil
	case argYSON:
		value, rest, err := ysonlib.NextValue([]byte(text))
		if err != nil {
			return nil, fmt.Errorf("%q is not YSON: %v", text, err)
		}
		if ysonlib.DocumentStart(rest) != len(rest) {
			return nil, fmt.Errorf("%q is more than one YSON value", text)
		}
		return toJQ(value), nil
	default:
		return text, nil
	}
}

// readValues reads every YSON value of a file into a list, which is what
// --slurpfile binds: the values of a file, not its text. A file holding one
// value — the usual case — gives a list of one.
func readValues(path string) ([]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	values := []any{}
	for {
		value, rest, err := ysonlib.NextValue(data)
		if err == io.EOF {
			return values, nil
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %v", path, err)
		}
		values = append(values, toJQ(value))
		data = rest
	}
}

// modulePaths is where a program's modules are looked for: the directories -L
// named, or the places jq and gojq look — the user's own directory, and the two
// beside the binary.
func modulePaths(opts *options) []string {
	if len(opts.modulePaths) > 0 {
		return opts.modulePaths
	}
	return []string{"~/.jq", "$ORIGIN/../lib/ysonq", "$ORIGIN/../lib"}
}

// indentation is what one level of the pretty output is written with.
func indentation(opts *options) string {
	switch {
	case opts.tab:
		return "\t"
	case opts.indent != nil:
		return strings.Repeat(" ", *opts.indent)
	default:
		return "    "
	}
}

// useColors decides whether the output is coloured: what the flags say first,
// then the environment the rest of the toolkit reads — YSON_FORCE_COLOR,
// YSON_NO_COLOR, NO_COLOR, a dumb terminal — and otherwise whether the output
// is a terminal at all, since colour is for reading, not for piping into
// another tool.
func useColors(opts *options, env *environment) bool {
	switch {
	case opts.colors != nil:
		return *opts.colors
	case isSet(env, "YSON_FORCE_COLOR"):
		return true
	case isSet(env, "YSON_NO_COLOR"):
		return false
	case isSet(env, "NO_COLOR"):
		return false
	case env.getenv("TERM") == "dumb":
		return false
	default:
		return env.stdoutIsTerminal
	}
}

// isSet reports whether a variable is in the environment at all: the two
// variables the toolkit reads are looked for rather than read, the way
// yson-convert reads them.
func isSet(env *environment, name string) bool {
	_, ok := env.lookupEnv(name)
	return ok
}

func (env *environment) getenv(name string) string {
	value, _ := env.lookupEnv(name)
	return value
}

// colorScheme is the colouring to use: JQ_COLORS, as jq reads it, or the
// default jq and yson-convert share.
func colorScheme(env *environment) string {
	if scheme := env.getenv("JQ_COLORS"); scheme != "" {
		return scheme
	}
	return defaultColors
}

// exitForInput says which failure an input error is. A file that cannot be read
// is a problem with the command line, and gets the exit code jq gives it; a
// document that does not parse is a failure while running, as in jq too.
func exitForInput(err error) int {
	var input *inputError
	if errors.As(err, &input) && input.data == nil {
		return exitUsage
	}
	return exitRuntime
}

// inputAt names the place the program was reading when it failed, which is what
// jq brackets after an error: a failure belongs to one of the values of the
// input, as often as not the one that was not what the program expected.
func inputAt(iter inputIter) string {
	name, line := positionOf(iter)
	if name == "" {
		return "<unknown>"
	}
	return fmt.Sprintf("%s:%d", name, line)
}

// programError formats a program that does not compile: what is wrong with it,
// the line it is on, and a caret under the token that caused it, which is the
// way jq reports one.
func programError(p *program, err error) string {
	var parseErr *gojq.ParseError
	if !errors.As(err, &parseErr) {
		return fmt.Sprintf("%s: error: %v\n", name, err)
	}

	offset := parseErr.Offset - len(parseErr.Token) + 1
	if offset < 0 {
		offset = 0
	}
	if offset > len(p.query) {
		offset = len(p.query)
	}
	line, column := position([]byte(p.query), offset)

	where := "<top-level>"
	if p.name != "<arg>" {
		where = p.name
	}

	var text strings.Builder
	fmt.Fprintf(&text, "%s: error: %v at %s, line %d:\n", name, parseErr, where, line)
	fmt.Fprintf(&text, "  %s\n  %s^\n", lineAt(p.query, line), strings.Repeat(" ", column-1))
	fmt.Fprintf(&text, "%s: 1 compile error\n", name)
	return text.String()
}

// lineAt is one line of the program, counting from one.
func lineAt(query string, line int) string {
	lines := strings.Split(query, "\n")
	if line < 1 || line > len(lines) {
		return ""
	}
	return lines[line-1]
}

// debugFunction is jq's debug: it writes the value it was given, in the compact
// form, to the error output and passes it on.
func debugFunction(env *environment) func(any, []any) any {
	return func(value any, _ []any) any {
		text, err := dump([]any{"DEBUG:", value}, outputConfig{compact: true})
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintln(env.stderr, text); err != nil {
			return err
		}
		return value
	}
}

// stderrFunction is jq's stderr: it writes the value it was given to the error
// output, without a newline, and passes it on. A string is written as it is, so
// that what lands on the error output is the text a program meant to print.
func stderrFunction(env *environment) func(any, []any) any {
	return func(value any, _ []any) any {
		text, ok := value.(string)
		if !ok {
			var err error
			if text, err = dump(value, outputConfig{compact: true}); err != nil {
				return err
			}
		}
		if _, err := io.WriteString(env.stderr, text); err != nil {
			return err
		}
		return value
	}
}

// inputFilename is jq's input_filename: the file being read, or null while the
// input is the standard input.
func inputFilename(iter inputIter) func(any, []any) any {
	return func(any, []any) any {
		if name := iter.Name(); name != "" && name != stdinName {
			return name
		}
		return nil
	}
}

// haltOutput writes what halt_error was given: a string as it is, any other
// value in the compact form, on the error output.
func haltOutput(env *environment, halt *gojq.HaltError) error {
	value := halt.Value()
	if value == nil {
		return nil
	}
	if text, ok := value.(string); ok {
		_, err := io.WriteString(env.stderr, text)
		return err
	}
	text, err := dump(value, outputConfig{compact: true})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(env.stderr, text)
	return err
}

// buildConfiguration says what this binary is, for --build-configuration: the
// version, and the settings the Go toolchain recorded while building it.
func buildConfiguration() string {
	var text strings.Builder
	fmt.Fprintf(&text, "%s-%s (%s %s/%s)", name, version, runtime.Version(), runtime.GOOS, runtime.GOARCH)
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			switch setting.Key {
			case "CGO_ENABLED", "vcs.revision", "vcs.modified":
				fmt.Fprintf(&text, " %s=%s", setting.Key, setting.Value)
			}
		}
	}
	return text.String()
}

func fail(env *environment, code int, err error) int {
	fmt.Fprintf(env.stderr, "%s: %v\n", name, err)
	return code
}

const usage = `ysonq - jq for YSON

Usage: ` + name + ` [OPTIONS] FILTER [FILES...]
       ` + name + ` [OPTIONS] -f FILE [FILES...]

Reads YSON values, runs the program over each of them, and writes the result
back as YSON. A value that carries attributes is the Attrs/Value map
yson-convert writes, so '.foo.bar.Attrs.q' is the attribute q of .foo.bar.

Example:
  echo '{foo={bar=<q=e>%true}}' | ysonq '.foo.bar.Attrs.q'
Input:
  -n, --null-input          run the program once, on null
  -R, --raw-input           read the input as lines of text, not as YSON
  -s, --slurp               read all of the input into one array
  --stream                  read the input as [path, leaf] pairs
  --seq                     read an RS-separated YSON stream

Output:
  -c, --compact-output      write each value on one line
  -r, --raw-output          write strings without quotes or escapes
  -j, --join-output         as -r, without a line break after each value
  --raw-output0             as -r, with a NUL after each value
  -S, --sort-keys           write the keys of an object in order
  --tab                     indent with tabs
  --indent N                indent with N spaces, from 0 to 7
  --unbuffered              write each value as soon as it is produced
  -C, --color-output        colour the output even when it is not a terminal
  -M, --monochrome-output   do not colour the output
  --seq                     write an RS before each value

Values:
  --arg NAME VALUE          bind $NAME to a string
  --argjson NAME VALUE      bind $NAME to a JSON value
  --argyson NAME VALUE      bind $NAME to a YSON value
  --slurpfile NAME FILE     bind $NAME to the YSON values of FILE, as a list
  --rawfile NAME FILE       bind $NAME to the contents of FILE, as a string
  --args                    the rest of the arguments are $ARGS.positional strings
  --jsonargs                the rest of the arguments are $ARGS.positional JSON
  --ysonargs                the rest of the arguments are $ARGS.positional YSON

Program:
  -f, --from-file           the first argument names a file holding the program
  -L, --library-path DIR    where to look for modules, instead of ~/.jq
  -e, --exit-status         exit 1 when the last value is false or null

Other:
  -h, --help                this text
  -V, --version             the version
  --build-configuration     how this binary was built

jq has flags this tool does not have yet: they are refused rather than ignored.
They are --yaml-input, --yaml-output, --stream-errors and -a/--ascii-output.
`
