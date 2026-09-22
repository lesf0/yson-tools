package main

import (
	"fmt"
	"strconv"
	"strings"
)

// argKind says how the value of a --arg-like flag is read.
type argKind int

const (
	argString argKind = iota
	argJSON
	argYSON
	argSlurpFile
	argRawFile
)

// namedArg is a variable the command line binds by name. The flag it came from
// is kept for the message when its value cannot be read.
type namedArg struct {
	flag  string
	name  string
	value string
	kind  argKind
}

// options is what the command line asked for.
type options struct {
	// input
	nullInput bool
	rawInput  bool
	slurp     bool
	stream    bool
	seq       bool

	// output
	compact    bool
	rawOutput  bool
	joinOutput bool
	rawOutput0 bool
	sortKeys   bool
	tab        bool
	indent     *int
	unbuffered bool
	// colors is -C or -M when either was given, and nil when neither was: the
	// output is then coloured according to the environment and the terminal
	colors *bool

	// status
	exitStatus         bool
	help               bool
	version            bool
	buildConfiguration bool

	// query
	fromFile        bool
	modulePaths     []string
	named           []namedArg
	positionalAsArg *argKind
}

// flagDef is one flag: its names, how many values it takes, and what it does
// with them.
type flagDef struct {
	long   []string
	short  string
	values int
	set    func(o *options, values []string) error
}

func boolean(long []string, short string, apply func(o *options)) flagDef {
	return flagDef{
		long:  long,
		short: short,
		set: func(o *options, _ []string) error {
			apply(o)
			return nil
		},
	}
}

// one is a flag that takes a value, and needs one name: its handler gets that
// value.
func one(long []string, short string, apply func(o *options, value string) error) flagDef {
	return flagDef{
		long:   long,
		short:  short,
		values: 1,
		set: func(o *options, values []string) error {
			return apply(o, values[0])
		},
	}
}

// two is --arg and its relatives: a name and a value.
func two(long []string, kind argKind) flagDef {
	return flagDef{
		long:   long,
		values: 2,
		set: func(o *options, values []string) error {
			o.named = append(o.named, namedArg{
				flag:  "--" + long[0],
				name:  values[0],
				value: values[1],
				kind:  kind,
			})
			return nil
		},
	}
}

// unsupported is a flag of jq this tool has not grown yet. It is reported as
// such rather than ignored, so that a command line which relies on it fails
// instead of quietly producing something else.
func unsupported(long []string, short string) flagDef {
	name := short
	if len(long) > 0 {
		name = "--" + long[0]
	}
	return flagDef{
		long:  long,
		short: short,
		set: func(*options, []string) error {
			return fmt.Errorf("%s is not supported yet", name)
		},
	}
}

// positionalArgs makes the arguments after the query into $ARGS.positional,
// read as kind, rather than into input files.
func positionalArgs(long []string, kind argKind) flagDef {
	return boolean(long, "", func(o *options) {
		chosen := kind
		o.positionalAsArg = &chosen
	})
}

// yes and no are what -C and -M set the colouring to; a variable is taken
// because the last of the two on the command line is the one that counts.
var (
	yes = true
	no  = false
)

var flagDefs = []flagDef{
	boolean([]string{"null-input"}, "n", func(o *options) { o.nullInput = true }),
	boolean([]string{"raw-input"}, "R", func(o *options) { o.rawInput = true }),
	boolean([]string{"slurp"}, "s", func(o *options) { o.slurp = true }),
	boolean([]string{"stream"}, "", func(o *options) { o.stream = true }),
	boolean([]string{"seq"}, "", func(o *options) { o.seq = true }),

	boolean([]string{"compact-output"}, "c", func(o *options) { o.compact = true }),
	boolean([]string{"raw-output"}, "r", func(o *options) { o.rawOutput = true }),
	boolean([]string{"join-output"}, "j", func(o *options) { o.joinOutput = true }),
	boolean([]string{"raw-output0"}, "", func(o *options) { o.rawOutput0 = true }),
	boolean([]string{"sort-keys"}, "S", func(o *options) { o.sortKeys = true }),
	boolean([]string{"tab"}, "", func(o *options) {
		o.tab = true
		o.indent = nil
	}),
	one([]string{"indent"}, "", func(o *options, value string) error {
		count, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("--indent takes a number, got %q", value)
		}
		if count < 0 || count > 7 {
			return fmt.Errorf("--indent takes a number between 0 and 7, got %d", count)
		}
		o.indent = &count
		o.tab = false
		return nil
	}),
	boolean([]string{"unbuffered"}, "", func(o *options) { o.unbuffered = true }),
	boolean([]string{"color-output"}, "C", func(o *options) { o.colors = &yes }),
	boolean([]string{"monochrome-output"}, "M", func(o *options) { o.colors = &no }),

	boolean([]string{"exit-status"}, "e", func(o *options) { o.exitStatus = true }),
	boolean([]string{"help"}, "h", func(o *options) { o.help = true }),
	// jq spells it -V, gojq -v; both are accepted, since a command line written
	// for either should not have to be changed
	boolean([]string{"version"}, "Vv", func(o *options) { o.version = true }),
	boolean([]string{"build-configuration"}, "", func(o *options) { o.buildConfiguration = true }),

	boolean([]string{"from-file"}, "f", func(o *options) { o.fromFile = true }),
	one([]string{"library-path"}, "L", func(o *options, value string) error {
		o.modulePaths = append(o.modulePaths, value)
		return nil
	}),

	two([]string{"arg"}, argString),
	two([]string{"argjson"}, argJSON),
	two([]string{"argyson"}, argYSON),
	two([]string{"slurpfile"}, argSlurpFile),
	two([]string{"rawfile"}, argRawFile),

	positionalArgs([]string{"args"}, argString),
	positionalArgs([]string{"jsonargs"}, argJSON),
	positionalArgs([]string{"ysonargs"}, argYSON),
}

var unsupportedFlags = []flagDef{
	unsupported([]string{"yaml-input"}, ""),
	unsupported([]string{"yaml-output"}, ""),
	unsupported([]string{"stream-errors"}, ""),
	unsupported([]string{"ascii-output"}, "a"),
}

// parseFlags reads the command line the way jq does: options and positional
// arguments may follow each other in any order, a lone "-" is a positional
// argument (stdin), clusters of short options are one argument, and a long
// option may take its value with "=".
func parseFlags(args []string) (*options, []string, error) {
	options := &options{}
	rest := []string{}

	byLong := map[string]*flagDef{}
	byShort := map[byte]*flagDef{}
	for _, defs := range [][]flagDef{flagDefs, unsupportedFlags} {
		for i := range defs {
			def := &defs[i]
			for _, name := range def.long {
				byLong[name] = def
			}
			for i := 0; i < len(def.short); i++ {
				byShort[def.short[i]] = def
			}
		}
	}

	positional := false
	for i := 0; i < len(args); i++ {
		arg := args[i]

		if positional || arg == "-" || !strings.HasPrefix(arg, "-") {
			rest = append(rest, arg)
			continue
		}
		if arg == "--" {
			positional = true
			continue
		}

		var def *flagDef
		values := []string{}
		switch {
		case strings.HasPrefix(arg, "--"):
			name := arg[2:]
			value := ""
			hasValue := false
			if j := strings.IndexByte(name, '='); j >= 0 {
				name, value, hasValue = name[:j], name[j+1:], true
			}
			def = byLong[name]
			if def == nil {
				return nil, nil, fmt.Errorf("unknown flag --%s", name)
			}
			if def.values == 0 && hasValue {
				return nil, nil, fmt.Errorf("--%s does not take a value", name)
			}
			if hasValue {
				values = append(values, value)
			}
		default:
			cluster := arg[1:]
			letter := 0
			for ; letter < len(cluster); letter++ {
				candidate := byShort[cluster[letter]]
				if candidate == nil {
					return nil, nil, fmt.Errorf("unknown flag -%c", cluster[letter])
				}
				if candidate.values == 0 {
					if err := candidate.set(options, nil); err != nil {
						return nil, nil, err
					}
					continue
				}
				// the flag takes a value, so it is the last one of the cluster;
				// what follows it here is that value
				def = candidate
				if value := cluster[letter+1:]; value != "" {
					values = append(values, value)
				}
				break
			}
			if letter == len(cluster) {
				// every letter of the cluster was a flag of its own
				continue
			}
		}

		for len(values) < def.values {
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("%s needs %d values", flagName(def), def.values)
			}
			i++
			values = append(values, args[i])
		}

		if err := def.set(options, values); err != nil {
			return nil, nil, err
		}
	}

	return options, rest, nil
}

func flagName(def *flagDef) string {
	if len(def.long) > 0 {
		return "--" + def.long[0]
	}
	return "-" + def.short
}
