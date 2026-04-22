package wire

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseFormat(t *testing.T) {
	cases := []struct {
		in   string
		want Format
	}{
		{"", FormatJSON},
		{"json", FormatJSON},
		{"JSON", FormatJSON},
		{"gcx", FormatGCX},
		{"GCX1", FormatGCX},
		{"text", FormatText},
		{"compact", FormatText},
		{"nope", FormatJSON},
	}
	for _, c := range cases {
		require.Equalf(t, c.want, ParseFormat(c.in), "ParseFormat(%q)", c.in)
	}
}

func TestEscapeUnescape_RoundTrip(t *testing.T) {
	cases := []string{
		"",
		"plain",
		"with\tTab",
		"with\nnewline",
		"back\\slash",
		"\t\n\\mixed",
		"unicode αβγ",
		"embedded \\n literal",
	}
	for _, c := range cases {
		got := unescape(escape(c))
		require.Equalf(t, c, got, "round-trip on %q", c)
	}
}

func TestEscape_StripsCR(t *testing.T) {
	require.Equal(t, `foo\nbar`, escape("foo\r\nbar"))
}

func TestEncoder_WritesHeaderThenRows(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf, Header{
		Tool:   "search_symbols",
		Fields: []string{"id", "kind", "path", "line", "name"},
	})
	require.NoError(t, enc.WriteComment("2 matches"))
	require.NoError(t, enc.WriteRow("1", "func", "a.go", 10, "Hello"))
	require.NoError(t, enc.WriteRow("2", "method", "b.go", 20, "World"))
	require.NoError(t, enc.Close())

	out := buf.String()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	require.Equal(t, "GCX1 tool=search_symbols fields=id,kind,path,line,name", lines[0])
	require.Equal(t, "# 2 matches", lines[1])
	require.Equal(t, "1\tfunc\ta.go\t10\tHello", lines[2])
	require.Equal(t, "2\tmethod\tb.go\t20\tWorld", lines[3])
}

func TestDecoder_ParsesHeaderAndRows(t *testing.T) {
	payload := "GCX1 tool=search_symbols fields=id,kind,path\n" +
		"# 2 matches\n" +
		"\n" +
		"1\tfunc\ta.go\n" +
		"2\tmethod\tb.go\n"
	dec := NewDecoder(strings.NewReader(payload))
	h, err := dec.Header()
	require.NoError(t, err)
	require.Equal(t, "search_symbols", h.Tool)
	require.Equal(t, []string{"id", "kind", "path"}, h.Fields)

	rows, err := dec.All()
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, "1", rows[0]["id"])
	require.Equal(t, "func", rows[0]["kind"])
	require.Equal(t, "a.go", rows[0]["path"])
	require.Equal(t, "method", rows[1]["kind"])
}

func TestDecoder_RejectsBadHeader(t *testing.T) {
	_, err := NewDecoder(strings.NewReader("not-gcx\n")).Header()
	require.Error(t, err)

	_, err = NewDecoder(strings.NewReader("GCX1 noequals\n")).Header()
	require.Error(t, err)

	_, err = NewDecoder(strings.NewReader("GCX1 tool=x\n")).Header()
	require.Error(t, err) // missing fields

	_, err = NewDecoder(strings.NewReader("GCX1 fields=a,b\n")).Header()
	require.Error(t, err) // missing tool
}

func TestDecoder_RejectsOverlongRow(t *testing.T) {
	payload := "GCX1 tool=x fields=a,b\n" +
		"1\t2\t3\n"
	dec := NewDecoder(strings.NewReader(payload))
	_, err := dec.All()
	require.Error(t, err)
}

func TestEncoder_EmbeddedTabsAndNewlinesRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf, Header{
		Tool:   "get_symbol_source",
		Fields: []string{"id", "source"},
	})
	src := "func F() {\n\tfmt.Println(\"hi\\there\")\n}"
	require.NoError(t, enc.WriteRow("sym-1", src))

	dec := NewDecoder(&buf)
	rows, err := dec.All()
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, src, rows[0]["source"])
}

func TestEncoder_RowShorterThanFieldsFillsEmpty(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf, Header{
		Tool:   "x",
		Fields: []string{"a", "b", "c"},
	})
	require.NoError(t, enc.WriteRow("x"))

	dec := NewDecoder(&buf)
	rows, err := dec.All()
	require.NoError(t, err)
	require.Equal(t, map[string]string{"a": "x", "b": "", "c": ""}, rows[0])
}

func TestEncoder_RejectsTooManyValues(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf, Header{Tool: "x", Fields: []string{"a"}})
	require.Error(t, enc.WriteRow("a", "b"))
}

func TestDecoder_HeaderMeta(t *testing.T) {
	payload := "GCX1 tool=x fields=a rows=3 ms=42\n" +
		"1\n2\n3\n"
	dec := NewDecoder(strings.NewReader(payload))
	h, err := dec.Header()
	require.NoError(t, err)
	require.Equal(t, "3", h.Meta["rows"])
	require.Equal(t, "42", h.Meta["ms"])
	rows, err := dec.All()
	require.NoError(t, err)
	require.Len(t, rows, 3)
}

func TestDecoder_MultiSection(t *testing.T) {
	payload := "GCX1 tool=ctx.target fields=id,name\n" +
		"A\tAlpha\n" +
		"GCX1 tool=ctx.callers fields=from,line\n" +
		"C1\t10\n" +
		"C2\t20\n"
	dec := NewDecoder(strings.NewReader(payload))
	h1, err := dec.Header()
	require.NoError(t, err)
	require.Equal(t, "ctx.target", h1.Tool)
	rows, err := dec.All()
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "Alpha", rows[0]["name"])

	h2, err := dec.NextSection()
	require.NoError(t, err)
	require.Equal(t, "ctx.callers", h2.Tool)
	require.Equal(t, []string{"from", "line"}, h2.Fields)
	rows, err = dec.All()
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, "C1", rows[0]["from"])
	require.Equal(t, "20", rows[1]["line"])

	_, err = dec.NextSection()
	require.ErrorIs(t, err, io.EOF)
}

func TestDecoder_EmptyStream(t *testing.T) {
	dec := NewDecoder(strings.NewReader(""))
	_, err := dec.Header()
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
}

func TestEncoder_HeaderMetaStable(t *testing.T) {
	// Meta keys emit in sorted order so test fixtures are deterministic.
	var buf bytes.Buffer
	NewEncoder(&buf, Header{
		Tool:   "x",
		Fields: []string{"a"},
		Meta:   map[string]string{"z": "last", "a": "first", "m": "mid"},
	})
	line := strings.SplitN(buf.String(), "\n", 2)[0]
	require.Equal(t, "GCX1 tool=x fields=a a=first m=mid z=last", line)
}
