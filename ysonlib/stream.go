package ysonlib

import (
	"bytes"
	"io"

	"go.ytsaurus.tech/yt/go/yson"
)

// separators are the bytes a document may be preceded by: whitespace, and the
// ';' and ',' that separate the values of a YSON stream.
func isSeparator(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r', ';', ',':
		return true
	default:
		return false
	}
}

// DocumentStart returns where the next document of a stream begins in data:
// past the separators that precede it, or len(data) when nothing is left but
// separators. It is for callers that have to say where in the input a bad
// document is, since an error from NextValue says nothing about position.
func DocumentStart(data []byte) int {
	start := 0
	for start < len(data) && isSeparator(data[start]) {
		start++
	}
	return start
}

// NextValue decodes the YSON value at the front of data and returns it together
// with the rest of the input. It returns io.EOF when data holds no value.
//
// yt's yson package parses one value: its reader stops at the end of the first
// top-level value and cannot be resumed, and Unmarshal rejects anything after a
// value with "after top-level value". A stream of values — one per line, or the
// ';'-separated text yt writes for tables — is not something the library reads.
// This walks such a stream one document at a time, and it finds where each
// document ends by asking the library rather than by scanning for delimiters
// itself: the reader is given the remaining input and asked for the raw span of
// its first value, which is that value's text verbatim. The next document
// starts right after it, so no lexer of ours can disagree with the parser about
// where a document ends — a bare string, a quoted one with a ';' in it, a
// number in exponent notation and a nested attributed node are all just bytes
// the reader walked over.
//
// The document is then parsed from those same bytes with yson.Unmarshal, so the
// values are exactly the ones the rest of the toolkit produces.
//
// The stream kind is a list fragment, which is the shape yt gives a stream of
// values: it is what lets a ';' after a bare literal end a document, as it does
// between table rows.
func NextValue(data []byte) (value any, rest []byte, err error) {
	start := DocumentStart(data)
	if start == len(data) {
		return nil, nil, io.EOF
	}

	document := data[start:]
	raw, err := yson.NewReaderKind(bytes.NewReader(document), yson.StreamListFragment).NextRawValue()
	if err != nil {
		return nil, data, err
	}

	end := documentEnd(document, raw)
	if end < 0 {
		// unreachable: raw came out of this very input
		return nil, data, io.ErrUnexpectedEOF
	}

	if err := yson.Unmarshal(raw, &value); err != nil {
		return nil, data, err
	}
	return value, document[end:], nil
}

// documentEnd returns the length of the document that ends with raw.
//
// raw is that document's text, so it is a prefix of document; the reader may
// have skipped separators of its own before it, though, in which case the
// document starts a few bytes in.
func documentEnd(document, raw []byte) int {
	if bytes.HasPrefix(document, raw) {
		return len(raw)
	}
	if offset := bytes.Index(document, raw); offset >= 0 {
		return offset + len(raw)
	}
	return -1
}
