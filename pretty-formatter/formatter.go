package formatter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"go.ytsaurus.tech/yt/go/yson"
)

// YsonFormatter represents a formatter for YSON serialization
type YsonFormatter struct {
	buffer      *bytes.Buffer
	indent      string
	sortKeys    bool
	colorOutput bool
	colors      []string

	// Compact writes the whole value on one line, the way yt's compact format
	// does: {a=1;b=[1;2;]}. Set the field after constructing the formatter;
	// leaving it false keeps the pretty layout every existing caller gets.
	Compact bool
}

func parseJQColors(colorsVar string) []string {
	colors := strings.Split(colorsVar, ":")
	ansiCodes := make([]string, len(colors)+1)

	for i, color := range colors {
		ansiCodes[i+1] = fmt.Sprintf("\033[%sm", color)
	}
	ansiCodes[0] = "\033[0m"

	return ansiCodes
}

// NewYsonFormatter creates an instance of YsonFormatter
func NewYsonFormatter(indent int, sortKeys bool, colorOutput bool, colorScheme string) *YsonFormatter {
	var colors []string
	if colorOutput {
		colors = parseJQColors(colorScheme)
	}

	return &YsonFormatter{
		buffer:      &bytes.Buffer{},
		indent:      strings.Repeat(" ", indent),
		sortKeys:    sortKeys,
		colorOutput: colorOutput,
		colors:      colors,
	}
}

// Dump serializes an object to YSON format
func (y *YsonFormatter) Dump(obj interface{}) string {
	y.writeValue(obj, 0)
	return y.buffer.String()
}

// SetIndent replaces the indentation written once per level, which the
// constructor takes as a count of spaces. It is for callers whose indentation
// is only known at run time: --tab wants "\t" and --indent 2 wants two spaces.
// It has no effect in compact mode.
func (y *YsonFormatter) SetIndent(indent string) {
	y.indent = indent
}

// numberColor and resetColor are the ANSI codes for numbers and for turning
// colouring back off. They exist because the values that are written before the
// reflect kind switch is reached — json.Number, *big.Int — still have to be
// coloured like the other numbers.
func (y *YsonFormatter) numberColor() string {
	if !y.colorOutput || len(y.colors) < 6 {
		return ""
	}
	return y.colors[4]
}

func (y *YsonFormatter) resetColor() string {
	if !y.colorOutput || len(y.colors) == 0 {
		return ""
	}
	return y.colors[0]
}

// begin writes an opening delimiter: alone in compact mode, followed by a line
// break in pretty mode.
func (y *YsonFormatter) begin(delimiter string, color string, endColor string) {
	if y.Compact {
		y.buffer.WriteString(color + delimiter + endColor)
	} else {
		y.buffer.WriteString(color + delimiter + "\n" + endColor)
	}
}

// end writes a closing delimiter, indented in pretty mode.
func (y *YsonFormatter) end(delimiter string, level int, color string, endColor string) {
	if !y.Compact {
		y.writeIndent(level)
	}
	y.buffer.WriteString(color + delimiter + endColor)
}

// beforeItem indents an element of a container; in compact mode the elements
// follow each other on the same line.
func (y *YsonFormatter) beforeItem(level int) {
	if !y.Compact {
		y.writeIndent(level)
	}
}

// terminator ends an element of a container: a line break in pretty mode, a
// single semicolon in compact mode.
func (y *YsonFormatter) terminator() string {
	if y.Compact {
		return ";"
	}
	return ";\n"
}

// assign stands between a key and its value; yt's compact format has no spaces
// around the '='.
func (y *YsonFormatter) assign() string {
	if y.Compact {
		return "="
	}
	return " = "
}

func (y *YsonFormatter) writeValue(v interface{}, level int) {
	// json.Number has the reflect kind of a string and *big.Int the kind of a
	// pointer, so both have to be recognised before the kind switch rather than
	// in it
	if number, ok := v.(json.Number); ok {
		y.buffer.WriteString(y.numberColor() + number.String() + y.resetColor())
		return
	}
	if number, ok := v.(*big.Int); ok {
		y.writeBigInt(number)
		return
	}

	rv := reflect.ValueOf(v)
	color := ""
	keyColor := ""
	endColor := ""
	if y.colorOutput {
		switch rv.Kind() {
		case reflect.Invalid:
			color = y.colors[1]
		case reflect.Bool:
			if rv.Bool() {
				color = y.colors[3]
			} else {
				color = y.colors[2]
			}
		case reflect.Int, reflect.Int64, reflect.Int32, reflect.Uint, reflect.Uint64, reflect.Uint32, reflect.Float32, reflect.Float64:
			color = y.colors[4]
		case reflect.String:
			color = y.colors[5]
		case reflect.Slice:
			color = y.colors[6]
		case reflect.Map, reflect.Struct, reflect.Ptr:
			color = y.colors[7]
			keyColor = y.colors[8]
		}
		endColor = y.colors[0]
	}

	switch rv.Kind() {
	case reflect.Invalid:
		y.buffer.WriteString(color + "#" + endColor)
	case reflect.Bool:
		if rv.Bool() {
			y.buffer.WriteString(fmt.Sprintf("%s%%true%s", color, endColor))
		} else {
			y.buffer.WriteString(fmt.Sprintf("%s%%false%s", color, endColor))
		}
	case reflect.Int, reflect.Int64, reflect.Int32:
		y.buffer.WriteString(fmt.Sprintf("%s%s%s", color, strconv.FormatInt(rv.Int(), 10), endColor))
	case reflect.Uint, reflect.Uint64, reflect.Uint32:
		y.buffer.WriteString(fmt.Sprintf("%s%su%s", color, strconv.FormatUint(rv.Uint(), 10), endColor))
	case reflect.Float32, reflect.Float64:
		y.buffer.WriteString(color)
		y.writeFloat(rv.Float())
		y.buffer.WriteString(endColor)
	case reflect.String:
		y.buffer.WriteString(color)
		y.writeStringValue(rv.String())
		y.buffer.WriteString(endColor)
	case reflect.Slice:
		y.writeList(rv.Interface(), level, color, endColor)
	case reflect.Map:
		y.writeMap(rv.Interface(), level, color, endColor, keyColor)
	case reflect.Ptr:
		if rv.Type() == reflect.TypeOf(&yson.ValueWithAttrs{}) {
			y.writeValueWithAttributes(rv.Interface().(*yson.ValueWithAttrs), level, color, endColor, keyColor)
		} else {
			y.writeValue(rv.Elem().Interface(), level)
		}
	default:
		panic(fmt.Sprintf("%v is not YSON serializable", v))
	}
}

// writeBigInt writes an integer that did not fit into the int64 or uint64 the
// parser produces, which happens once a value has been through jq. YSON has no
// integer type wider than 64 bits, so anything above that is a value the format
// cannot hold rather than something to round off quietly.
func (y *YsonFormatter) writeBigInt(number *big.Int) {
	written := number.String()
	switch {
	case number.IsInt64():
	case number.IsUint64():
		written += "u"
	default:
		panic(fmt.Sprintf("%s is out of YSON's integer range", written))
	}
	y.buffer.WriteString(y.numberColor() + written + y.resetColor())
}

func (y *YsonFormatter) writeFloat(f float64) {
	switch {
	case math.IsNaN(f):
		y.buffer.WriteString("%nan")
	case math.IsInf(f, 1):
		y.buffer.WriteString("%inf")
	case math.IsInf(f, -1):
		y.buffer.WriteString("%-inf")
	default:
		// the shortest text that reads back as the same float; YSON tells an
		// integer from a float by the spelling, so there has to be something in
		// it that says float
		str := strconv.FormatFloat(f, 'g', -1, 64)
		if !strings.ContainsAny(str, ".eE") {
			str += "."
		}
		y.buffer.WriteString(str)
	}
}

func (y *YsonFormatter) writeString(s string) {
	y.buffer.WriteString("\"")
	y.buffer.WriteString(escapeString(s))
	y.buffer.WriteString("\"")
}

// writeStringValue writes a string in its value position: quoted in the pretty
// layout, and bare in the compact one when YSON reads it back as the same
// string. This is the rule yt's own text and compact formats use, and it is
// what the compact output of the tools has always looked like ({a=qqq;}), so
// the two layouts quote different amounts by design.
func (y *YsonFormatter) writeStringValue(s string) {
	if y.Compact && needsNoQuotes(s) {
		y.buffer.WriteString(s)
		return
	}
	y.writeString(s)
}

// needsNoQuotes reports whether a string can be written without quotes: it has
// to start with a letter and hold nothing but letters and digits, so that it
// cannot be read back as a number, an entity or a keyword. Everything else —
// including anything non-ASCII — is quoted.
func needsNoQuotes(s string) bool {
	if len(s) == 0 || !isAlphaByte(s[0]) {
		return false
	}
	for i := 1; i < len(s); i++ {
		if b := s[i]; !isAlphaByte(b) && !isDigitByte(b) {
			return false
		}
	}
	return true
}

func isAlphaByte(b byte) bool {
	return 'a' <= b && b <= 'z' || 'A' <= b && b <= 'Z'
}

func isDigitByte(b byte) bool {
	return '0' <= b && b <= '9'
}

func escapeString(s string) string {
	var buf strings.Builder
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size <= 1 {
			// a text string is UTF-8 by definition, so a byte that stands on
			// its own here came out of a binary string: escape it instead of
			// writing U+FFFD, which would lose it
			fmt.Fprintf(&buf, "\\x%02X", s[i])
			i++
			continue
		}
		switch r {
		case '\\':
			buf.WriteString("\\\\")
		case '"':
			buf.WriteString("\\\"")
		case '\n':
			buf.WriteString("\\n")
		case '\r':
			buf.WriteString("\\r")
		case '\t':
			buf.WriteString("\\t")
		default:
			if r < 32 {
				fmt.Fprintf(&buf, "\\x%02X", r)
			} else {
				buf.WriteRune(r)
			}
		}
		i += size
	}
	return buf.String()
}

func (y *YsonFormatter) writeList(v interface{}, level int, color string, endColor string) {
	list := reflect.ValueOf(v)

	if list.Len() == 0 {
		y.buffer.WriteString(color + "[]" + endColor)
		return
	}

	y.begin("[", color, endColor)
	for i := 0; i < list.Len(); i++ {
		y.beforeItem(level + 1)
		y.writeValue(list.Index(i).Interface(), level+1)
		y.buffer.WriteString(y.terminator())
	}
	y.end("]", level, color, endColor)
}

func (y *YsonFormatter) writeMap(v interface{}, level int, color string, endColor string, keyColor string) {
	mapValue := reflect.ValueOf(v)
	keys := mapValue.MapKeys()

	if len(keys) == 0 {
		y.buffer.WriteString(color + "{}" + endColor)
		return
	}

	y.begin("{", color, endColor)

	if y.sortKeys {
		sort.Slice(keys, func(i, j int) bool {
			return fmt.Sprint(keys[i].Interface()) < fmt.Sprint(keys[j].Interface())
		})
	}

	for _, key := range keys {
		y.beforeItem(level + 1)
		y.buffer.WriteString(keyColor)
		y.writeStringValue(key.String())
		y.buffer.WriteString(endColor)
		y.buffer.WriteString(y.assign())
		y.writeValue(mapValue.MapIndex(key).Interface(), level+1)
		y.buffer.WriteString(y.terminator())
	}

	y.end("}", level, color, endColor)
}

func (y *YsonFormatter) writeValueWithAttributes(v *yson.ValueWithAttrs, level int, color string, endColor string, keyColor string) {
	y.buffer.WriteString(color + "<" + endColor)
	if !y.Compact {
		y.buffer.WriteString("\n")
	}

	mapValue := reflect.ValueOf(v.Attrs)
	keys := mapValue.MapKeys()

	if y.sortKeys {
		sort.Slice(keys, func(i, j int) bool {
			return fmt.Sprint(keys[i].Interface()) < fmt.Sprint(keys[j].Interface())
		})
	}

	for _, key := range keys {
		y.beforeItem(level + 1)
		y.buffer.WriteString(keyColor)
		y.writeStringValue(key.String())
		y.buffer.WriteString(endColor)
		y.buffer.WriteString(y.assign())
		y.writeValue(mapValue.MapIndex(key).Interface(), level+1)
		y.buffer.WriteString(y.terminator())
	}

	if y.Compact {
		y.buffer.WriteString(color + ">" + endColor)
	} else {
		y.writeIndent(level)
		y.buffer.WriteString(color + "> " + endColor)
	}
	y.writeValue(v.Value, level)
}

func (y *YsonFormatter) writeIndent(level int) {
	y.buffer.WriteString(strings.Repeat(y.indent, level))
}
