// Package wire implements the GCX1 compact wire format for Gortex MCP
// tool responses. GCX1 is a tab-delimited, line-oriented, round-trippable
// format with a self-describing header. See docs/wire-format.md.
package wire

import (
	"fmt"
	"strings"
)

// Format selects the output encoding for an MCP tool response.
type Format int

const (
	// FormatJSON is the default MCP response encoding.
	FormatJSON Format = iota
	// FormatGCX is the GCX1 compact round-trippable wire format.
	FormatGCX
	// FormatText is the legacy one-line-per-result text output
	// previously selected by `compact: true`. Lossy.
	FormatText
)

// ParseFormat maps an MCP tool-argument string to a Format. Unknown
// values return FormatJSON to stay safe. The caller should separately
// honour the deprecated `compact: true` boolean by mapping it to
// FormatText.
func ParseFormat(s string) Format {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "gcx", "gcx1":
		return FormatGCX
	case "text", "compact":
		return FormatText
	case "", "json":
		return FormatJSON
	default:
		return FormatJSON
	}
}

func (f Format) String() string {
	switch f {
	case FormatGCX:
		return "gcx"
	case FormatText:
		return "text"
	default:
		return "json"
	}
}

// Version is the current GCX protocol version. Bump on any
// backward-incompatible change to the header syntax or escape rules.
const Version = 1

// Tag is the four-byte header magic that starts every GCX payload.
// It is a literal prefix ("GCX" + Version as a decimal digit); the
// decoder rejects anything that does not match.
var Tag = fmt.Sprintf("GCX%d", Version)

// Header is the first line of a GCX1 payload. It declares the tool
// that produced the payload, the field order of the row stream, and
// optional free-form metadata.
//
// Wire layout:
//
//	GCX1 tool=<tool> fields=<f1>,<f2>,... [k=v]...
type Header struct {
	Version int               // always Version for now
	Tool    string            // MCP tool name
	Fields  []string          // declared column order for the row stream
	Meta    map[string]string // additional k=v pairs (rows=, ms=, ...)
}

// CommentPrefix marks human-readable comment lines. Comments have no
// effect on decoding and may be dropped by intermediaries.
const CommentPrefix = "#"

// FieldSep is the row column delimiter. Tab never appears in code
// symbol names and is treated as whitespace by GPT tokenizers.
const FieldSep = "\t"

// RowSep terminates a row. Newline is also tokenizer-friendly and
// survives transports that re-flow whitespace but preserve line breaks.
const RowSep = "\n"
