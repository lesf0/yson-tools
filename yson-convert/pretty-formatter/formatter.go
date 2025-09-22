package formatter

import (
	"bytes"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"go.ytsaurus.tech/yt/go/yson"
)

// YsonFormatter represents a formatter for YSON serialization
type YsonFormatter struct {
	buffer      *bytes.Buffer
	indent      string
	sortKeys    bool
	colorOutput bool
	colors      []string
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
func NewYsonFormatter(indent int, sortKeys bool, colorOutput bool) *YsonFormatter {
	jqColors, found := os.LookupEnv("JQ_COLORS")
	if !found {
		jqColors = "0;90:0;39:0;39:0;39:0;32:1;39:1;39:1;34" // default
	}

	return &YsonFormatter{
		buffer:      &bytes.Buffer{},
		indent:      strings.Repeat(" ", indent),
		sortKeys:    sortKeys,
		colorOutput: colorOutput,
		colors:      parseJQColors(jqColors),
	}
}

// Dump serializes an object to YSON format
func (y *YsonFormatter) Dump(obj interface{}) string {
	y.writeValue(obj, 0)
	return y.buffer.String()
}

func (y *YsonFormatter) writeValue(v interface{}, level int) {
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
		y.writeString(rv.String())
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

func (y *YsonFormatter) writeFloat(f float64) {
	switch {
	case f != f:
		y.buffer.WriteString("%nan")
	case f > 0 && (f > 0x7FF0000000000000):
		y.buffer.WriteString("%inf")
	case f < 0 && (f < -0x7FF0000000000000):
		y.buffer.WriteString("%-inf")
	default:
		str := strconv.FormatFloat(f, 'f', -1, 64)
		y.buffer.WriteString(str)
		if !strings.ContainsRune(str, '.') {
			y.buffer.WriteRune('.')
		}
	}
}

func (y *YsonFormatter) writeString(s string) {
	y.buffer.WriteString("\"")
	y.buffer.WriteString(escapeString(s))
	y.buffer.WriteString("\"")
}

func escapeString(s string) string {
	var buf strings.Builder
	for _, r := range s {
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
				buf.WriteString(fmt.Sprintf("\\x%02X", r))
			} else {
				buf.WriteRune(r)
			}
		}
	}
	return buf.String()
}

func (y *YsonFormatter) writeList(v interface{}, level int, color string, endColor string) {
	list := reflect.ValueOf(v)

	if list.Len() == 0 {
		y.buffer.WriteString(color + "[]" + endColor)
		return
	}

	y.buffer.WriteString(color + "[\n" + endColor)
	for i := 0; i < list.Len(); i++ {
		y.writeIndent(level + 1)
		y.writeValue(list.Index(i).Interface(), level+1)
		y.buffer.WriteString(";\n")
	}
	y.writeIndent(level)
	y.buffer.WriteString(color + "]" + endColor)
}

func (y *YsonFormatter) writeMap(v interface{}, level int, color string, endColor string, keyColor string) {
	mapValue := reflect.ValueOf(v)
	keys := mapValue.MapKeys()

	if len(keys) == 0 {
		y.buffer.WriteString(color + "{}" + endColor)
		return
	}

	y.buffer.WriteString(color + "{\n" + endColor)

	if y.sortKeys {
		sort.Slice(keys, func(i, j int) bool {
			return fmt.Sprint(keys[i].Interface()) < fmt.Sprint(keys[j].Interface())
		})
	}

	for _, key := range keys {
		y.writeIndent(level + 1)
		y.buffer.WriteString(keyColor)
		y.writeString(key.String())
		y.buffer.WriteString(endColor)
		y.buffer.WriteString(" = ")
		y.writeValue(mapValue.MapIndex(key).Interface(), level+1)
		y.buffer.WriteString(";\n")
	}

	y.writeIndent(level)
	y.buffer.WriteString(color + "}" + endColor)
}

func (y *YsonFormatter) writeValueWithAttributes(v *yson.ValueWithAttrs, level int, color string, endColor string, keyColor string) {
	y.buffer.WriteString(color + "<\n" + endColor)
	mapValue := reflect.ValueOf(v.Attrs)
	keys := mapValue.MapKeys()

	if y.sortKeys {
		sort.Slice(keys, func(i, j int) bool {
			return fmt.Sprint(keys[i].Interface()) < fmt.Sprint(keys[j].Interface())
		})
	}

	for _, key := range keys {
		y.writeIndent(level + 1)
		y.buffer.WriteString(keyColor)
		y.writeString(key.String())
		y.buffer.WriteString(endColor)
		y.buffer.WriteString(" = ")
		y.writeValue(mapValue.MapIndex(key).Interface(), level+1)
		y.buffer.WriteString(";\n")
	}

	y.writeIndent(level)
	y.buffer.WriteString(color + "> " + endColor)
	y.writeValue(v.Value, level)
}

func (y *YsonFormatter) writeIndent(level int) {
	y.buffer.WriteString(strings.Repeat(y.indent, level))
}
