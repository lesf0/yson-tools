package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/lesf0/yson-tools/ysonlib"
	"go.ytsaurus.tech/yt/go/yson"
)

// inputIter is the stream of values the program runs over: gojq asks it for
// values through input/inputs, and the main loop asks it for the value to run
// the program on next. Both share the one iterator, so `input` consumes the
// very values the main loop would have seen next.
//
// An error is yielded as a value and the iterator stops after it, which is how
// gojq reports an input error in the middle of a stream.
type inputIter interface {
	gojq.Iter
	io.Closer
	Name() string
}

// positioned is an input that can say how far it has read, which is what a
// failure while running the program is reported against.
type positioned interface {
	Position() (name string, line int)
}

// positionOf asks an input how far it has read; one that cannot say returns
// nothing, and the message then names no place.
func positionOf(iter inputIter) (string, int) {
	if positioned, ok := iter.(positioned); ok {
		return positioned.Position()
	}
	return "", 0
}

// documentSource makes an iterator over one input's bytes: which kind of
// iterator that is depends on the flags (-R, --seq, -s), so the loop over the
// inputs does not have to know about them.
type documentSource func(data []byte, name string) inputIter

// documentSourceFor is the iterator the flags ask for.
func documentSourceFor(opts *options) documentSource {
	switch {
	case opts.rawInput && opts.slurp:
		// the documents of one input are not documents at all: its whole
		// contents are the one string they are, and wholeInputs joins the
		// inputs into a single one
		return func(data []byte, name string) inputIter {
			return &wholeInput{data: data, name: name}
		}
	case opts.rawInput:
		return func(data []byte, name string) inputIter {
			return newLines(data, name)
		}
	case opts.seq:
		return func(data []byte, name string) inputIter {
			return newRecords(data, name)
		}
	default:
		return func(data []byte, name string) inputIter {
			return newDocuments(data, name)
		}
	}
}

// documents reads a YSON stream: one value per line, or the ';'-separated text
// yt writes for tables, which is what ysonlib.NextValue knows how to split.
type documents struct {
	// original is the whole input, kept for the position an error or a failure
	// is reported at; data is what is left of it
	original []byte
	data     []byte
	// offset is where in original data begins, current where the value last
	// read began
	offset  int
	current int
	name    string
	done    bool
}

func newDocuments(data []byte, name string) *documents {
	return &documents{original: data, data: data, name: name}
}

func (d *documents) Name() string { return d.name }

func (d *documents) Close() error { return nil }

// Position is where the value the program is being run on began, in the terms
// of the input: the line is counted only when somebody asks, so that reading a
// file costs no more than it has to.
func (d *documents) Position() (string, int) {
	line, _ := position(d.original, d.current)
	return d.name, line
}

func (d *documents) Next() (any, bool) {
	if d.done {
		return nil, false
	}

	start := d.offset + ysonlib.DocumentStart(d.data)
	value, rest, err := ysonlib.NextValue(d.data)
	if err != nil {
		d.done = true
		if err == io.EOF {
			return nil, false
		}
		return &inputError{name: d.name, offset: start, data: d.original, err: err}, true
	}

	d.current = start
	d.offset += len(d.data) - len(rest)
	d.data = rest
	return toJQ(value), true
}

// inputError is an error in the input rather than in the program. yt's error
// messages do not say where in the input it went wrong, so it is said here.
// The data is what the position is counted in, and is left out when there is
// nothing to count it in — a file that would not open says so and no more.
type inputError struct {
	name   string
	offset int
	data   []byte
	err    error
}

func (e *inputError) Error() string {
	if e.data == nil {
		return fmt.Sprintf("%s: %v", e.name, e.err)
	}
	line, column := position(e.data, e.offset)
	return fmt.Sprintf("%s:%d:%d: %v", e.name, line, column, e.err)
}

func (e *inputError) Unwrap() error { return e.err }

// position turns a byte offset into the line and column jq reports.
func position(data []byte, offset int) (line, column int) {
	if offset > len(data) {
		offset = len(data)
	}
	line = 1
	start := 0
	for i := 0; i < offset; i++ {
		if data[i] == '\n' {
			line++
			start = i + 1
		}
	}
	return line, offset - start + 1
}

// records reads the RS-separated chunks of jq's --seq, one value each.
type records struct {
	chunks [][]byte
	name   string
}

func newRecords(data []byte, name string) *records {
	var chunks [][]byte
	for _, chunk := range bytes.Split(data, []byte{'\x1e'}) {
		if len(bytes.TrimSpace(chunk)) == 0 {
			continue
		}
		chunks = append(chunks, chunk)
	}
	return &records{chunks: chunks, name: name}
}

func (r *records) Name() string { return r.name }

func (r *records) Close() error { return nil }

func (r *records) Next() (any, bool) {
	if len(r.chunks) == 0 {
		return nil, false
	}

	chunk := r.chunks[0]
	r.chunks = r.chunks[1:]

	var value any
	if err := yson.Unmarshal(chunk, &value); err != nil {
		r.chunks = nil
		return &inputError{name: r.name, data: chunk, err: err}, true
	}
	return toJQ(value), true
}

// streamEvents turns a value into the sequence of [path, leaf] pairs jq
// --stream describes: every leaf on its own, a leaf being a scalar or an empty
// container, with the path that leads to it.
func streamEvents(value any) []any {
	var events []any
	var walk func(value any, path []any)
	walk = func(value any, path []any) {
		switch value := value.(type) {
		case []any:
			if len(value) == 0 {
				events = append(events, []any{path, []any{}})
				return
			}
			for i, item := range value {
				walk(item, appendPath(path, i))
			}
		case map[string]any:
			if len(value) == 0 {
				events = append(events, []any{path, map[string]any{}})
				return
			}
			for _, key := range sortedKeys(value) {
				walk(value[key], appendPath(path, key))
			}
		default:
			events = append(events, []any{path, value})
		}
	}
	walk(value, []any{})
	return events
}

// appendPath extends a path without letting two paths share a backing array:
// every event keeps its own, since a document of a thousand leaves is a
// thousand paths that are alive at once.
func appendPath(path []any, step any) []any {
	extended := make([]any, len(path)+1)
	copy(extended, path)
	extended[len(path)] = step
	return extended
}

// sortedKeys keeps the order of the entries of an object stable from run to
// run: a YSON map is a Go map, which does not remember the order its keys were
// written in.
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// lines yields one value per line of input, with the line break taken off:
// jq's --raw-input.
type lines struct {
	reader *bufio.Reader
	name   string
}

func newLines(data []byte, name string) *lines {
	return &lines{reader: bufio.NewReader(bytes.NewReader(data)), name: name}
}

func (l *lines) Name() string { return l.name }

func (l *lines) Close() error { return nil }

func (l *lines) Next() (any, bool) {
	line, err := l.reader.ReadString('\n')
	if line == "" && err != nil {
		return nil, false
	}
	line = strings.TrimSuffix(line, "\n")
	line = strings.TrimSuffix(line, "\r")
	return line, true
}

// slurp collects every value of the input into a single list, however many
// inputs it came from: jq's --slurp runs the program once, on that list.
type slurp struct {
	iter inputIter
}

func (s *slurp) Name() string { return s.iter.Name() }

func (s *slurp) Close() error { return s.iter.Close() }

func (s *slurp) Position() (string, int) {
	if s.iter == nil {
		return "", 0
	}
	return positionOf(s.iter)
}

func (s *slurp) Next() (any, bool) {
	if s.iter == nil {
		return nil, false
	}
	iter := s.iter
	s.iter = nil

	values := []any{}
	for {
		value, ok := iter.Next()
		if !ok {
			return values, true
		}
		if err, ok := value.(error); ok {
			return err, true
		}
		values = append(values, value)
	}
}

// streamed turns an input into the events jq's --stream describes, one value at
// a time.
type streamed struct {
	iter  inputIter
	queue []any
}

func (s *streamed) Name() string { return s.iter.Name() }

func (s *streamed) Close() error { return s.iter.Close() }

func (s *streamed) Position() (string, int) { return positionOf(s.iter) }

func (s *streamed) Next() (any, bool) {
	for {
		if len(s.queue) > 0 {
			value := s.queue[0]
			s.queue = s.queue[1:]
			return value, true
		}

		value, ok := s.iter.Next()
		if !ok {
			return nil, false
		}
		if err, ok := value.(error); ok {
			return err, true
		}
		// one value is many events, so they wait their turn here
		s.queue = streamEvents(value)
	}
}

// wholeInput is one input as the one string it is: --raw-input with --slurp.
type wholeInput struct {
	data []byte
	name string
	done bool
}

func (w *wholeInput) Name() string { return w.name }

func (w *wholeInput) Close() error { return nil }

func (w *wholeInput) Next() (any, bool) {
	if w.done {
		return nil, false
	}
	w.done = true
	return string(w.data), true
}

// wholeInputs joins the inputs of --raw-input --slurp into the single string
// jq reads, since there is one of them however many files it took.
type wholeInputs struct {
	iter inputIter
	done bool
}

func (w *wholeInputs) Name() string { return w.iter.Name() }

func (w *wholeInputs) Close() error { return w.iter.Close() }

func (w *wholeInputs) Position() (string, int) { return positionOf(w.iter) }

func (w *wholeInputs) Next() (any, bool) {
	if w.done {
		return nil, false
	}
	w.done = true

	var text strings.Builder
	for {
		value, ok := w.iter.Next()
		if !ok {
			return text.String(), true
		}
		if err, ok := value.(error); ok {
			return err, true
		}
		text.WriteString(value.(string))
	}
}

// nullInput is -n: the program runs once, on null, and the main loop reads
// nothing — input and inputs still do, as they do in jq.
type nullInput struct {
	done bool
}

func (*nullInput) Name() string { return "" }

func (*nullInput) Close() error { return nil }

func (n *nullInput) Next() (any, bool) {
	if n.done {
		return nil, false
	}
	n.done = true
	return nil, true
}

// newInput is the input of a command line: the files it names, or the standard
// input when it names none.
func newInput(paths []string, source documentSource, read func(path string) ([]byte, error)) inputIter {
	if len(paths) == 0 {
		paths = []string{"-"}
	}
	return &files{paths: paths, source: source, read: read}
}

// inputName is what an input is called in messages: the standard input has no
// name of its own.
func inputName(path string) string {
	if path == "-" {
		return stdinName
	}
	return path
}

// files reads the inputs in order; input_filename reports the one being read.
// An input that cannot be read is reported as an error and the next one is
// tried, so one missing file does not take the rest of the command line with
// it.
type files struct {
	paths   []string
	source  documentSource
	read    func(path string) ([]byte, error)
	current inputIter
	at      int
}

func (f *files) Name() string {
	if f.current != nil {
		return f.current.Name()
	}
	return ""
}

func (f *files) Close() error {
	if f.current != nil {
		return f.current.Close()
	}
	return nil
}

func (f *files) Position() (string, int) {
	if f.current == nil {
		return "", 0
	}
	return positionOf(f.current)
}

func (f *files) Next() (any, bool) {
	for {
		if f.current != nil {
			if value, ok := f.current.Next(); ok {
				return value, true
			}
			f.current.Close()
			f.current = nil
		}
		if f.at >= len(f.paths) {
			return nil, false
		}

		path := f.paths[f.at]
		f.at++

		data, err := f.read(path)
		if err != nil {
			return &inputError{name: inputName(path), err: err}, true
		}
		f.current = f.source(data, inputName(path))
	}
}
