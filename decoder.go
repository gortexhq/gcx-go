package wire

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// Decoder reads a GCX1 payload. It parses the header on first call to
// Header() (or implicitly on the first NextRow()), then streams rows
// as ordered field maps. Comments and blank lines are skipped.
//
// Decoder is not safe for concurrent use.
//
// Multi-section payloads are supported: after NextRow returns io.EOF,
// callers can invoke NextSection to read the next header if the stream
// is not exhausted.
type Decoder struct {
	s       *bufio.Scanner
	header  Header
	parsed  bool
	pending bool // a header line has been peeked but not returned
	peeked  string
	err     error
}

// NewDecoder wraps r in a bufio.Scanner with a buffer large enough
// for representative tool responses. The default Scanner line limit
// (64 KiB) is too small for payloads such as get_symbol_source on
// large functions; we raise it to 4 MiB. Payloads exceeding that cap
// should be streamed on the sender side instead.
func NewDecoder(r io.Reader) *Decoder {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 64*1024), 4*1024*1024)
	return &Decoder{s: s}
}

// Header returns the parsed header, reading from the underlying
// reader if it has not been parsed yet.
func (d *Decoder) Header() (Header, error) {
	if d.err != nil {
		return d.header, d.err
	}
	if d.parsed {
		return d.header, nil
	}
	var line string
	if d.pending {
		line = d.peeked
		d.pending = false
	} else if d.s.Scan() {
		line = d.s.Text()
	} else {
		if err := d.s.Err(); err != nil {
			d.err = err
		} else {
			d.err = io.ErrUnexpectedEOF
		}
		return d.header, d.err
	}
	h, err := parseHeader(line)
	if err != nil {
		d.err = err
		return d.header, err
	}
	d.header = h
	d.parsed = true
	return d.header, nil
}

// NextRow returns the next data row as a map keyed by the declared
// field names. It returns (nil, io.EOF) when the current section ends
// (either at end of stream or when the next header line is encountered).
// Comment lines and blank lines are skipped. Rows with fewer values
// than declared fields fill missing keys with "". Rows with more
// values than declared fields return an error.
func (d *Decoder) NextRow() (map[string]string, error) {
	if _, err := d.Header(); err != nil {
		return nil, err
	}
	for d.s.Scan() {
		line := d.s.Text()
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, CommentPrefix) {
			continue
		}
		if strings.HasPrefix(line, Tag) {
			// Start of a new section. Stash and signal EOF for the
			// current section. Callers can read it via NextSection.
			d.pending = true
			d.peeked = line
			return nil, io.EOF
		}
		return parseRow(line, d.header.Fields)
	}
	if err := d.s.Err(); err != nil {
		d.err = err
		return nil, err
	}
	return nil, io.EOF
}

// NextSection advances to the next logical payload and returns its
// header. Callers invoke NextSection after NextRow returns io.EOF to
// check whether the stream contains additional sections. It returns
// io.EOF when no further section exists.
func (d *Decoder) NextSection() (Header, error) {
	if d.err != nil && d.err != io.EOF {
		return d.header, d.err
	}
	if !d.pending {
		// Drain remaining lines of the current section to find a header.
		for d.s.Scan() {
			line := d.s.Text()
			if strings.HasPrefix(line, Tag) {
				d.pending = true
				d.peeked = line
				break
			}
		}
		if !d.pending {
			if err := d.s.Err(); err != nil {
				d.err = err
				return d.header, err
			}
			return d.header, io.EOF
		}
	}
	d.parsed = false
	d.header = Header{}
	return d.Header()
}

// All consumes the stream and returns every row as a slice.
// Convenient for tests and small responses; prefer NextRow for
// streaming.
func (d *Decoder) All() ([]map[string]string, error) {
	h, err := d.Header()
	if err != nil {
		return nil, err
	}
	_ = h
	var rows []map[string]string
	for {
		row, err := d.NextRow()
		if err == io.EOF {
			return rows, nil
		}
		if err != nil {
			return rows, err
		}
		rows = append(rows, row)
	}
}

func parseHeader(line string) (Header, error) {
	h := Header{Meta: map[string]string{}}
	if len(line) < len(Tag) || line[:len(Tag)] != Tag {
		return h, fmt.Errorf("wire: expected header prefix %q, got %.10q", Tag, line)
	}
	h.Version = Version
	rest := strings.TrimSpace(line[len(Tag):])
	// Tokenise on single spaces. Values containing spaces must be
	// escaped; the Go encoder never emits raw spaces in identifiers.
	for p := range strings.SplitSeq(rest, " ") {
		if p == "" {
			continue
		}
		key, val, ok := strings.Cut(p, "=")
		if !ok {
			return h, fmt.Errorf("wire: malformed header token %q (want key=value)", p)
		}
		k := unescape(key)
		v := unescape(val)
		switch k {
		case "tool":
			h.Tool = v
		case "fields":
			raw := strings.Split(v, ",")
			h.Fields = make([]string, len(raw))
			for i, f := range raw {
				h.Fields[i] = unescape(f)
			}
		default:
			h.Meta[k] = v
		}
	}
	if h.Tool == "" {
		return h, fmt.Errorf("wire: header missing tool= key")
	}
	if len(h.Fields) == 0 {
		return h, fmt.Errorf("wire: header missing fields= key")
	}
	return h, nil
}

func parseRow(line string, fields []string) (map[string]string, error) {
	values := strings.Split(line, FieldSep)
	if len(values) > len(fields) {
		return nil, fmt.Errorf("wire: row has %d values but header declared %d fields", len(values), len(fields))
	}
	m := make(map[string]string, len(fields))
	for i, f := range fields {
		if i < len(values) {
			m[f] = unescape(values[i])
		} else {
			m[f] = ""
		}
	}
	return m, nil
}
