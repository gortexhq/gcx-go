package wire

import (
	"strings"
)

// escape encodes a field value so it can be safely written between
// FieldSep / RowSep. The only characters that require escaping are
// backslash, tab, and newline. An empty value encodes to an empty
// string so consecutive delimiters represent consecutive empty fields.
//
// The escape alphabet is deliberately minimal — a two-byte sequence
// per occurrence — because code-intelligence payloads rarely contain
// tabs or raw newlines inside field values, and the escape cost stays
// bounded when they do.
func escape(s string) string {
	if s == "" {
		return ""
	}
	if !strings.ContainsAny(s, "\\\t\n\r") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 4)
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\\':
			b.WriteString(`\\`)
		case '\t':
			b.WriteString(`\t`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			// Strip bare \r so Windows CRLF round-trips as \n only.
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// unescape is the inverse of escape. Unknown escape sequences decode
// to the literal following byte so a pathological payload cannot wedge
// the decoder. Callers should still treat decoded values as untrusted.
func unescape(s string) string {
	if s == "" {
		return ""
	}
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '\\' || i+1 >= len(s) {
			b.WriteByte(c)
			continue
		}
		i++
		switch s[i] {
		case '\\':
			b.WriteByte('\\')
		case 't':
			b.WriteByte('\t')
		case 'n':
			b.WriteByte('\n')
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}
