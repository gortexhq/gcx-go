# gcx-go — Go reference encoder/decoder for the GCX1 wire format

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](./LICENSE)

GCX1 is a tab-delimited, line-oriented, round-trippable wire format for
MCP (Model Context Protocol) tool responses. It is an opt-in alternative
to JSON that saves a **median −27.4% tiktoken tokens** across 20
representative tool responses, with 100% round-trip integrity.

This package is the Go reference implementation. A TypeScript decoder
ships as [`@gortex/wire`](https://www.npmjs.com/package/@gortex/wire)
on npm (source: [`gortexhq/gcx-ts`](https://github.com/gortexhq/gcx-ts)).

- **Spec:** https://github.com/zzet/gortex/blob/main/docs/wire-format.md
- **Benchmark harness:** https://github.com/zzet/gortex/tree/main/bench/wire-format
- **License:** MIT.

## Install

```sh
go get github.com/gortexhq/gcx-go
```

Go 1.25+. Zero runtime dependencies beyond the standard library.

## Encode

```go
import wire "github.com/gortexhq/gcx-go"

h := wire.Header{
    Tool:   "search_symbols",
    Fields: []string{"id", "kind", "name", "path", "line"},
    Meta:   map[string]string{"total": "5", "truncated": "false"},
}

enc, err := wire.NewEncoder(w, h)
if err != nil { return err }

for _, row := range rows {
    if err := enc.WriteRow(row.ID, row.Kind, row.Name, row.Path, row.Line); err != nil {
        return err
    }
}
```

For tools without a hand-tuned encoder, use the generic fallback:

```go
wire.EncodeAny(w, "my_tool", anyJSONShape)
```

## Decode

```go
dec := wire.NewDecoder(r)
h, err := dec.Header()
if err != nil { return err }

for {
    row, err := dec.NextRow()
    if err == io.EOF { break }
    if err != nil { return err }
    // row is map[string]string keyed by h.Fields
}
```

Multi-section payloads are supported: call `dec.NextSection()` after
`NextRow` returns `io.EOF` to advance to the next header.

## Round-trip guarantee

Every GCX1 payload decodes back to an equivalent JSON value. The
reference [benchmark harness][bench] (in the Gortex repo, where the
fixtures live) scores bytes, tiktoken tokens, gzip size, and
round-trip integrity across 20 representative MCP tool responses —
currently 20/20 pass.

[bench]: https://github.com/zzet/gortex/tree/main/bench/wire-format

## Versioning

The `GCX1` header prefix is stable for the lifetime of major version 1.
Field layouts for declared tools are frozen within `GCX1`. Additions
ship as `GCX2`.
