package wire

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// Encoder writes a GCX1 payload. It emits the header immediately on
// construction so callers can start streaming rows without a separate
// Start() call. Row values are stringified with fmt.Sprint-compatible
// rules (bool → "true"/"false", int → decimal, float → %g, nil → "").
//
// Encoder is not safe for concurrent use.
type Encoder struct {
	w   io.Writer
	err error
	h   Header
}

// NewEncoder writes the header for h to w and returns a ready encoder.
// Any error from writing the header is sticky: subsequent WriteRow and
// WriteComment calls return it without attempting further I/O.
func NewEncoder(w io.Writer, h Header) *Encoder {
	if h.Version == 0 {
		h.Version = Version
	}
	e := &Encoder{w: w, h: h}
	e.writeHeader()
	return e
}

// Tool returns the tool name declared in the header.
func (e *Encoder) Tool() string { return e.h.Tool }

// Fields returns the declared field order.
func (e *Encoder) Fields() []string { return e.h.Fields }

// WriteComment writes a "# ..." line. Intermediate newlines in s are
// escaped so the comment stays on one physical line.
func (e *Encoder) WriteComment(s string) error {
	if e.err != nil {
		return e.err
	}
	line := CommentPrefix + " " + strings.ReplaceAll(s, "\n", " ") + RowSep
	_, e.err = io.WriteString(e.w, line)
	return e.err
}

// WriteRow writes one record. The number of values must match the
// declared field count. Pass fewer for trailing empty columns; excess
// values are an error.
func (e *Encoder) WriteRow(values ...any) error {
	if e.err != nil {
		return e.err
	}
	if len(values) > len(e.h.Fields) {
		e.err = fmt.Errorf("wire: row has %d values but header declared %d fields", len(values), len(e.h.Fields))
		return e.err
	}
	var sb strings.Builder
	for i, field := range e.h.Fields {
		if i > 0 {
			sb.WriteString(FieldSep)
		}
		if i >= len(values) {
			continue
		}
		sb.WriteString(escape(stringify(values[i])))
		_ = field
	}
	sb.WriteString(RowSep)
	_, e.err = io.WriteString(e.w, sb.String())
	return e.err
}

// Close is a no-op retained for symmetry with bufio-style encoders
// and to let callers evolve to a buffering impl without churn.
func (e *Encoder) Close() error { return e.err }

func (e *Encoder) writeHeader() {
	var sb strings.Builder
	sb.WriteString(Tag)
	sb.WriteString(" tool=")
	sb.WriteString(escape(e.h.Tool))
	sb.WriteString(" fields=")
	for i, f := range e.h.Fields {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(escape(f))
	}
	// Emit metadata in a stable order so fixtures are reproducible.
	if len(e.h.Meta) > 0 {
		keys := make([]string, 0, len(e.h.Meta))
		for k := range e.h.Meta {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			sb.WriteString(" ")
			sb.WriteString(escape(k))
			sb.WriteString("=")
			sb.WriteString(escape(e.h.Meta[k]))
		}
	}
	sb.WriteString(RowSep)
	_, e.err = io.WriteString(e.w, sb.String())
}

// stringify renders a Go value as its GCX-visible form. Keep the rules
// tight: the benchmark scorer depends on output being deterministic.
func stringify(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case int:
		return strconv.Itoa(x)
	case int32:
		return strconv.FormatInt(int64(x), 10)
	case int64:
		return strconv.FormatInt(x, 10)
	case uint:
		return strconv.FormatUint(uint64(x), 10)
	case uint32:
		return strconv.FormatUint(uint64(x), 10)
	case uint64:
		return strconv.FormatUint(x, 10)
	case float32:
		return strconv.FormatFloat(float64(x), 'g', -1, 32)
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64)
	case []byte:
		return string(x)
	case fmt.Stringer:
		return x.String()
	default:
		return fmt.Sprint(v)
	}
}
