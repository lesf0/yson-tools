package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

	formatter "github.com/lesf0/yson-tools/pretty-formatter"
)

// outputConfig is everything the flags decided about the shape of the output.
type outputConfig struct {
	compact bool
	// raw writes strings as they are, without quotes or escapes; join is raw
	// without the newline after each value, raw0 is raw with a NUL instead of
	// it (--raw-output, --join-output, --raw-output0)
	raw  bool
	join bool
	raw0 bool
	// seq puts an RS before every value, as --seq does
	seq    bool
	indent string
	colors bool
	scheme string
	// unbuffered is --unbuffered: every value is written as soon as it is
	// produced, for a filter whose output is watched while it runs
	unbuffered bool
}

type writer struct {
	buffer *bufio.Writer
	config outputConfig
}

func newWriter(out io.Writer, config outputConfig) *writer {
	return &writer{buffer: bufio.NewWriter(out), config: config}
}

// write outputs one value of the query, with whatever terminator the flags ask
// for.
func (w *writer) write(value any) error {
	text, err := w.render(value)
	if err != nil {
		return err
	}

	if w.config.seq {
		if err := w.buffer.WriteByte('\x1e'); err != nil {
			return err
		}
	}
	if _, err := w.buffer.WriteString(text); err != nil {
		return err
	}

	switch {
	case w.config.raw0:
		err = w.buffer.WriteByte(0)
	case w.config.join:
		// --join-output leaves the value unterminated, so that a run of
		// results comes out as one line
	default:
		err = w.buffer.WriteByte('\n')
	}
	if err != nil {
		return err
	}

	if w.config.unbuffered {
		return w.buffer.Flush()
	}
	return nil
}

func (w *writer) flush() error { return w.buffer.Flush() }

func (w *writer) render(value any) (string, error) {
	if text, ok := value.(string); ok && (w.config.raw || w.config.join || w.config.raw0) {
		if w.config.raw0 && strings.ContainsRune(text, 0) {
			return "", errors.New("cannot write a string containing a NUL byte with --raw-output0")
		}
		return text, nil
	}
	return dump(value, w.config)
}

// dump writes a value as YSON in the layout the flags chose.
func dump(value any, config outputConfig) (text string, err error) {
	// a jq value is not quite a YSON one: attributes have to be put back, and
	// an integer too big for YSON has to be refused rather than rounded off
	value, err = fromJQ(value)
	if err != nil {
		return "", err
	}

	// the formatter panics on a value it cannot write rather than half-writing
	// it; a command line tool reports that the way it reports every other value
	// it cannot write
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("cannot write value: %v", recovered)
		}
	}()

	ys := formatter.NewYsonFormatter(4, true, config.colors, config.scheme)
	ys.Compact = config.compact
	ys.SetIndent(config.indent)
	return ys.Dump(value), nil
}
