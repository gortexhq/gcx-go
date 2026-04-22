package wire

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEncodeAny_SingleRecord(t *testing.T) {
	var buf bytes.Buffer
	in := map[string]any{"id": "x1", "kind": "func", "fan_in": 3}
	require.NoError(t, EncodeAny(&buf, "get_symbol", in))

	dec := NewDecoder(&buf)
	h, err := dec.Header()
	require.NoError(t, err)
	require.Equal(t, "get_symbol", h.Tool)
	require.Equal(t, []string{"fan_in", "id", "kind"}, h.Fields)

	rows, err := dec.All()
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "x1", rows[0]["id"])
	require.Equal(t, "func", rows[0]["kind"])
	require.Equal(t, "3", rows[0]["fan_in"])
}

func TestEncodeAny_RowList(t *testing.T) {
	var buf bytes.Buffer
	in := []any{
		map[string]any{"id": "1", "kind": "func"},
		map[string]any{"id": "2", "kind": "method", "line": 42},
	}
	require.NoError(t, EncodeAny(&buf, "search_symbols", in))

	dec := NewDecoder(&buf)
	h, err := dec.Header()
	require.NoError(t, err)
	require.Equal(t, []string{"id", "kind", "line"}, h.Fields)
	require.Equal(t, "2", h.Meta["rows"])

	rows, err := dec.All()
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, "1", rows[0]["id"])
	require.Equal(t, "", rows[0]["line"])
	require.Equal(t, "42", rows[1]["line"])
}

func TestEncodeAny_ScalarList(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, EncodeAny(&buf, "x", []any{"a", "b", "c"}))
	dec := NewDecoder(&buf)
	h, _ := dec.Header()
	require.Equal(t, []string{"value"}, h.Fields)
	rows, _ := dec.All()
	require.Len(t, rows, 3)
	require.Equal(t, "a", rows[0]["value"])
}

func TestEncodeAny_Scalar(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, EncodeAny(&buf, "x", 42))
	dec := NewDecoder(&buf)
	rows, _ := dec.All()
	require.Len(t, rows, 1)
	require.Equal(t, "42", rows[0]["value"])
}

func TestEncodeAny_NestedValuesAsJSON(t *testing.T) {
	var buf bytes.Buffer
	in := map[string]any{
		"id":   "x",
		"tags": []string{"a", "b"},
		"meta": map[string]any{"k": "v"},
	}
	require.NoError(t, EncodeAny(&buf, "x", in))
	dec := NewDecoder(&buf)
	rows, err := dec.All()
	require.NoError(t, err)
	require.Equal(t, `["a","b"]`, rows[0]["tags"])
	var meta map[string]any
	require.NoError(t, json.Unmarshal([]byte(rows[0]["meta"]), &meta))
	require.Equal(t, "v", meta["k"])
}

func TestEncodeAny_EmptyList(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, EncodeAny(&buf, "find_usages", []any{}))
	dec := NewDecoder(&buf)
	h, err := dec.Header()
	require.NoError(t, err)
	require.Equal(t, "0", h.Meta["rows"])
	rows, err := dec.All()
	require.NoError(t, err)
	require.Empty(t, rows)
}

func TestEncodeAny_StructInput(t *testing.T) {
	type sym struct {
		ID   string `json:"id"`
		Kind string `json:"kind"`
	}
	var buf bytes.Buffer
	require.NoError(t, EncodeAny(&buf, "x", []sym{{ID: "1", Kind: "func"}}))
	dec := NewDecoder(&buf)
	rows, err := dec.All()
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "1", rows[0]["id"])
}
