package wire

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
)

// EncodeAny writes v to w as a GCX1 payload using a generic
// shape-inference encoder. It is the fallback used when no hand-tuned
// encoder has been registered for the tool. Supported shapes:
//
//   - A JSON object ("record") becomes a single header + single row
//     where fields are the top-level keys (sorted alphabetically) and
//     values are their scalar representations. Nested objects / arrays
//     are JSON-serialised into the cell.
//
//   - A JSON array of objects ("rows") becomes a header with the union
//     of top-level keys across elements (sorted) and one row per
//     element.
//
//   - A JSON array of scalars becomes a header with a single "value"
//     field and one row per element.
//
//   - A scalar value becomes a header with a single "value" field and
//     one row.
//
// Anything else is rejected so an unencodable shape cannot silently
// degrade the wire format. Callers should prefer hand-tuned encoders
// for hot-path tools; the generic fallback exists for breadth.
func EncodeAny(w io.Writer, tool string, v any) error {
	// Canonicalise through encoding/json so we reason about one
	// normalised shape (map[string]any / []any / scalars) regardless
	// of whether v came from a struct, map, or already-decoded JSON.
	buf, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("wire: marshal input: %w", err)
	}
	var decoded any
	dec := json.NewDecoder(bytes.NewReader(buf))
	dec.UseNumber()
	if err := dec.Decode(&decoded); err != nil {
		return fmt.Errorf("wire: normalise input: %w", err)
	}

	switch x := decoded.(type) {
	case []any:
		return encodeRowsGeneric(w, tool, x)
	case map[string]any:
		return encodeRecordGeneric(w, tool, x)
	default:
		// Scalar — single row with one field.
		e := NewEncoder(w, Header{Tool: tool, Fields: []string{"value"}})
		if err := e.WriteRow(renderCell(x)); err != nil {
			return err
		}
		return e.Close()
	}
}

func encodeRecordGeneric(w io.Writer, tool string, m map[string]any) error {
	keys := sortedKeys(m)
	e := NewEncoder(w, Header{Tool: tool, Fields: keys})
	values := make([]any, len(keys))
	for i, k := range keys {
		values[i] = renderCell(m[k])
	}
	if err := e.WriteRow(values...); err != nil {
		return err
	}
	return e.Close()
}

func encodeRowsGeneric(w io.Writer, tool string, rows []any) error {
	if len(rows) == 0 {
		// Empty rows: emit header with a single "value" field and no data.
		e := NewEncoder(w, Header{
			Tool:   tool,
			Fields: []string{"value"},
			Meta:   map[string]string{"rows": "0"},
		})
		return e.Close()
	}

	// Detect whether every element is an object. If so, union the key
	// set. Otherwise collapse to a single-column scalar list.
	objectMode := true
	keySet := map[string]struct{}{}
	for _, r := range rows {
		obj, ok := r.(map[string]any)
		if !ok {
			objectMode = false
			break
		}
		for k := range obj {
			keySet[k] = struct{}{}
		}
	}

	if !objectMode {
		e := NewEncoder(w, Header{
			Tool:   tool,
			Fields: []string{"value"},
			Meta:   map[string]string{"rows": fmt.Sprintf("%d", len(rows))},
		})
		for _, r := range rows {
			if err := e.WriteRow(renderCell(r)); err != nil {
				return err
			}
		}
		return e.Close()
	}

	fields := make([]string, 0, len(keySet))
	for k := range keySet {
		fields = append(fields, k)
	}
	sort.Strings(fields)

	e := NewEncoder(w, Header{
		Tool:   tool,
		Fields: fields,
		Meta:   map[string]string{"rows": fmt.Sprintf("%d", len(rows))},
	})
	for _, r := range rows {
		obj := r.(map[string]any)
		values := make([]any, len(fields))
		for i, f := range fields {
			values[i] = renderCell(obj[f])
		}
		if err := e.WriteRow(values...); err != nil {
			return err
		}
	}
	return e.Close()
}

// renderCell flattens nested values for display in a GCX cell. Scalars
// go through stringify; maps / slices serialise to compact JSON so the
// cell stays on one physical line and the decoder can re-hydrate the
// nested shape by JSON-parsing it.
func renderCell(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case json.Number:
		return x.String()
	case bool, int, int32, int64, uint, uint32, uint64, float32, float64:
		return stringify(v)
	case []any, map[string]any:
		b, err := json.Marshal(x)
		if err != nil {
			return fmt.Sprint(v)
		}
		return string(b)
	default:
		return stringify(v)
	}
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
