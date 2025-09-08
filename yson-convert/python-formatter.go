package main

import (
	"bytes"
	"fmt"
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
}

// NewYsonFormatter creates an instance of YsonFormatter
func NewYsonFormatter(indent int, sortKeys bool, colorOutput bool) *YsonFormatter {
	return &YsonFormatter{
		buffer:      &bytes.Buffer{},
		indent:      strings.Repeat(" ", indent),
		sortKeys:    sortKeys,
		colorOutput: colorOutput,
	}
}

const (
	resetColor   = "\033[0m"
	numberColor  = "\033[32m" // Green
	boolColor    = "\033[34m" // Blue
	stringColor  = "\033[35m" // Magenta
	nullColor    = "\033[31m" // Red
	bracketColor = "\033[36m" // Cyan
)

// Dump serializes an object to YSON format
func (y *YsonFormatter) Dump(obj interface{}) string {
	y.writeValue(obj, 0)
	return y.buffer.String()
}

func (y *YsonFormatter) writeValue(v interface{}, level int) {
	rv := reflect.ValueOf(v)
	color := ""
	endColor := ""
	if y.colorOutput {
		switch rv.Kind() {
		case reflect.Invalid:
			color = nullColor
		case reflect.Bool:
			color = boolColor
		case reflect.Int, reflect.Int64, reflect.Int32, reflect.Uint, reflect.Uint64, reflect.Uint32, reflect.Float32, reflect.Float64:
			color = numberColor
		case reflect.String:
			color = stringColor
		case reflect.Slice, reflect.Map, reflect.Struct, reflect.Ptr:
			color = bracketColor
		}
		endColor = resetColor
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
		y.writeMap(rv.Interface(), level, color, endColor)
	case reflect.Ptr:
		if rv.Type() == reflect.TypeOf(&yson.ValueWithAttrs{}) {
			y.writeValueWithAttributes(rv.Interface().(*yson.ValueWithAttrs), level, color, endColor)
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
		y.buffer.WriteString(strconv.FormatFloat(f, 'f', -1, 64))
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
			if r < 32 || r > 126 {
				buf.WriteString(fmt.Sprintf("\\x%02X", r))
			} else {
				buf.WriteRune(r)
			}
		}
	}
	return buf.String()
}

func (y *YsonFormatter) writeList(v interface{}, level int, color string, endColor string) {
	y.buffer.WriteString(color + "[\n" + endColor)
	list := reflect.ValueOf(v)
	for i := 0; i < list.Len(); i++ {
		y.writeIndent(level + 1)
		y.writeValue(list.Index(i).Interface(), level+1)
		y.buffer.WriteString(";\n")
	}
	y.writeIndent(level)
	y.buffer.WriteString(color + "]" + endColor)
}

func (y *YsonFormatter) writeMap(v interface{}, level int, color string, endColor string) {
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
		y.writeValue(key.Interface(), level+1)
		y.buffer.WriteString(" = ")
		y.writeValue(mapValue.MapIndex(key).Interface(), level+1)
		y.buffer.WriteString(";\n")
	}

	y.writeIndent(level)
	y.buffer.WriteString(color + "}" + endColor)
}

func (y *YsonFormatter) writeValueWithAttributes(v *yson.ValueWithAttrs, level int, color string, endColor string) {
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
		y.writeValue(key.Interface(), level+1)
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
