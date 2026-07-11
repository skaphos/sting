// SPDX-License-Identifier: MIT
package mcpinstall

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// This file implements format-preserving edits of TOML config files (Codex,
// Grok). go-toml v2 has no comment model, so a read-then-remarshal cycle deletes
// every comment, reorders keys, rewrites quoting, and explodes inline tables in
// a user's hand-authored config. Instead we edit the raw text surgically: only
// the [mcp_servers.sting] table (and its [mcp_servers.sting.*] subtables) is
// inserted, replaced, or removed; every other byte is left untouched.
//
// Limitations (documented, and safe): the entry must be a standard
// [mcp_servers.sting] table header — which is how sting and every runtime write
// it. An entry hand-authored as an inline/dotted key is detected and refused
// (never silently duplicated or corrupted). A comment or blank line trailing the
// block is preserved for the following table; a comment interleaved between the
// sting table's own keys is part of the sting table and is replaced with it.

// stingPrefix is the dotted key path of sting's TOML table.
func stingPrefix() []string { return []string{"mcp_servers", serverKey} }

// tomlHeader is a located [table] / [[array-table]] header in a TOML file.
type tomlHeader struct {
	path      []string // dotted key segments, unquoted
	lineStart int      // byte index of the start of the header's physical line
	bodyEnd   int      // byte index of the next header's lineStart, or len(raw)
}

// byteSpan is a half-open [start,end) byte range within a file.
type byteSpan struct{ start, end int }

// upsertTOMLServer inserts or replaces sting's table in path with the keys in
// set (nil-valued keys are deleted), preserving all other bytes and any extra
// keys the user added to the sting table itself.
func upsertTOMLServer(path string, set map[string]any, mode fs.FileMode) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		raw = nil
	}
	existing, err := existingStingTable(raw, path)
	if err != nil {
		return err
	}
	headers, err := scanTOMLHeaders(raw, path)
	if err != nil {
		return err
	}
	spans := matchingSpans(raw, headers, stingPrefix())
	if len(spans) == 0 && existing != nil {
		return fmt.Errorf("refusing to modify %q: sting entry is not a standard [mcp_servers.%s] table; edit manually", path, serverKey)
	}

	merged := map[string]any{}
	for k, v := range existing {
		merged[k] = v
	}
	for k, v := range set {
		if v == nil {
			delete(merged, k)
			continue
		}
		merged[k] = v
	}
	block, err := marshalStingBlock(merged)
	if err != nil {
		return err
	}

	newRaw := spliceBlock(raw, spans, block)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return WriteAtomic(path, newRaw, mode)
}

// deleteTOMLServer removes sting's table (and subtables) from path. It reports
// whether anything was removed.
func deleteTOMLServer(path string, mode fs.FileMode) (bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	existing, err := existingStingTable(raw, path)
	if err != nil {
		return false, err
	}
	headers, err := scanTOMLHeaders(raw, path)
	if err != nil {
		return false, err
	}
	spans := matchingSpans(raw, headers, stingPrefix())
	if len(spans) == 0 {
		if existing != nil {
			return false, fmt.Errorf("refusing to modify %q: sting entry is not a standard [mcp_servers.%s] table; edit manually", path, serverKey)
		}
		return false, nil
	}
	newRaw := removeSpans(raw, spans)
	if err := WriteAtomic(path, newRaw, mode); err != nil {
		return false, err
	}
	return true, nil
}

// existingStingTable parses raw with go-toml and returns mcp_servers.sting as a
// map, nil if absent. It also surfaces malformed-file parse errors so writes
// fail loudly rather than clobbering an unparseable config.
func existingStingTable(raw []byte, path string) (map[string]any, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil
	}
	var doc map[string]any
	if err := toml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse %q: %w", path, err)
	}
	servers, ok := doc["mcp_servers"]
	if !ok || servers == nil {
		return nil, nil
	}
	m, ok := servers.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("parse %q: mcp_servers is not a TOML table (got %T)", path, servers)
	}
	entry, ok := m[serverKey]
	if !ok || entry == nil {
		return nil, nil
	}
	em, ok := entry.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("parse %q: mcp_servers.%s is not a TOML table (got %T)", path, serverKey, entry)
	}
	return em, nil
}

// marshalStingBlock renders entry as a self-contained [mcp_servers.sting] block
// (plus a [mcp_servers.sting.env] subtable when present). go-toml emits an empty
// parent "[mcp_servers]" header first; we drop it so we never redefine a table
// the user may already own.
func marshalStingBlock(entry map[string]any) (string, error) {
	raw, err := toml.Marshal(map[string]any{"mcp_servers": map[string]any{serverKey: entry}})
	if err != nil {
		return "", err
	}
	s := strings.TrimPrefix(string(raw), "[mcp_servers]\n")
	return strings.TrimRight(s, "\n") + "\n", nil
}

// matchingSpans returns the byte spans of tables whose path is, or descends
// from, prefix. Consecutive matching headers (e.g. sting followed by sting.env)
// are merged into a single span so the whole subtree is replaced or removed as a
// unit. Trailing blank/comment lines that lead into the next table are excluded
// so a following table keeps its leading comment.
func matchingSpans(raw []byte, headers []tomlHeader, prefix []string) []byteSpan {
	var spans []byteSpan
	for i := 0; i < len(headers); {
		if !hasPrefix(headers[i].path, prefix) {
			i++
			continue
		}
		j := i
		for j+1 < len(headers) && hasPrefix(headers[j+1].path, prefix) {
			j++
		}
		start := headers[i].lineStart
		end := blockContentEnd(raw, headers[j].lineStart, headers[j].bodyEnd)
		spans = append(spans, byteSpan{start, end})
		i = j + 1
	}
	return spans
}

// hasPrefix reports whether path equals prefix or is a descendant of it,
// compared segment-by-segment (so "sting_other" does not match "sting").
func hasPrefix(path, prefix []string) bool {
	if len(path) < len(prefix) {
		return false
	}
	for i := range prefix {
		if path[i] != prefix[i] {
			return false
		}
	}
	return true
}

// spliceBlock replaces the given spans with block, inserting at the first span's
// position; with no spans it appends block to the end of the file.
func spliceBlock(raw []byte, spans []byteSpan, block string) []byte {
	if len(spans) == 0 {
		var buf bytes.Buffer
		buf.Write(raw)
		if len(raw) > 0 && raw[len(raw)-1] != '\n' {
			buf.WriteByte('\n')
		}
		if len(bytes.TrimSpace(raw)) > 0 {
			buf.WriteByte('\n') // blank line before the appended table
		}
		buf.WriteString(block)
		return buf.Bytes()
	}
	var buf bytes.Buffer
	prev := 0
	inserted := false
	for _, s := range spans {
		buf.Write(raw[prev:s.start])
		if !inserted {
			buf.WriteString(block)
			inserted = true
		}
		prev = s.end
	}
	buf.Write(raw[prev:])
	return buf.Bytes()
}

// removeSpans deletes the given spans from raw.
func removeSpans(raw []byte, spans []byteSpan) []byte {
	var buf bytes.Buffer
	prev := 0
	for _, s := range spans {
		buf.Write(raw[prev:s.start])
		prev = s.end
	}
	buf.Write(raw[prev:])
	return buf.Bytes()
}

// blockContentEnd returns the byte index at which a table block's own content
// ends, trimming trailing blank and comment lines within [lineStart,bodyEnd) so
// they can be attributed to the following table.
func blockContentEnd(raw []byte, lineStart, bodyEnd int) int {
	end := lineStart
	for i := lineStart; i < bodyEnd; {
		lineEnd := i
		for lineEnd < bodyEnd && raw[lineEnd] != '\n' {
			lineEnd++
		}
		if lineEnd < bodyEnd {
			lineEnd++ // include the newline
		}
		if t := bytes.TrimSpace(raw[i:lineEnd]); len(t) > 0 && t[0] != '#' {
			end = lineEnd
		}
		i = lineEnd
	}
	return end
}

// scanTOMLHeaders returns every table header in raw, in file order. It tracks
// string and array state so a '[' inside a value or a multiline string is never
// mistaken for a table header.
func scanTOMLHeaders(raw []byte, path string) ([]tomlHeader, error) {
	var headers []tomlHeader
	n := len(raw)
	lineStartIdx := 0
	atLineStart := true // only whitespace seen so far on this logical line
	arrayDepth := 0
	for i := 0; i < n; {
		c := raw[i]
		switch {
		case c == '\n':
			i++
			if arrayDepth == 0 {
				lineStartIdx = i
				atLineStart = true
			}
		case c == ' ' || c == '\t' || c == '\r':
			i++
		case c == '#':
			for i < n && raw[i] != '\n' {
				i++
			}
		case c == '"' || c == '\'':
			i = skipTOMLString(raw, i)
			atLineStart = false
		case c == '[' && atLineStart && arrayDepth == 0:
			h, next, err := parseTOMLHeader(raw, i, lineStartIdx, path)
			if err != nil {
				return nil, err
			}
			headers = append(headers, h)
			i = next
			atLineStart = false
		case c == '[':
			arrayDepth++
			atLineStart = false
			i++
		case c == ']':
			if arrayDepth > 0 {
				arrayDepth--
			}
			atLineStart = false
			i++
		default:
			atLineStart = false
			i++
		}
	}
	for idx := range headers {
		if idx+1 < len(headers) {
			headers[idx].bodyEnd = headers[idx+1].lineStart
		} else {
			headers[idx].bodyEnd = n
		}
	}
	return headers, nil
}

// parseTOMLHeader parses a table header beginning at raw[start] ('['), on a line
// starting at lineStart. It returns the header and the index just past the
// closing bracket(s).
func parseTOMLHeader(raw []byte, start, lineStart int, path string) (tomlHeader, int, error) {
	n := len(raw)
	i := start + 1
	arrayTable := false
	if i < n && raw[i] == '[' {
		arrayTable = true
		i++
	}
	var key []byte
	for i < n {
		c := raw[i]
		if c == '"' || c == '\'' {
			j := skipTOMLString(raw, i)
			key = append(key, raw[i:j]...)
			i = j
			continue
		}
		if c == ']' || c == '\n' {
			break
		}
		key = append(key, c)
		i++
	}
	if i >= n || raw[i] != ']' {
		return tomlHeader{}, 0, fmt.Errorf("parse %q: unterminated table header", path)
	}
	i++
	if arrayTable {
		if i >= n || raw[i] != ']' {
			return tomlHeader{}, 0, fmt.Errorf("parse %q: unterminated array-table header", path)
		}
		i++
	}
	segs, err := splitTOMLKey(string(key))
	if err != nil {
		return tomlHeader{}, 0, fmt.Errorf("parse %q: %w", path, err)
	}
	return tomlHeader{path: segs, lineStart: lineStart}, i, nil
}

// splitTOMLKey splits a dotted TOML key into unquoted segments, honoring quoted
// segments that may themselves contain dots.
func splitTOMLKey(s string) ([]string, error) {
	var parts []string
	var cur []byte
	b := []byte(s)
	for i := 0; i < len(b); {
		c := b[i]
		switch c {
		case '"', '\'':
			j := skipTOMLString(b, i)
			cur = append(cur, unquoteTOMLKey(b[i:j])...)
			i = j
		case '.':
			parts = append(parts, strings.TrimSpace(string(cur)))
			cur = nil
			i++
		default:
			cur = append(cur, c)
			i++
		}
	}
	parts = append(parts, strings.TrimSpace(string(cur)))
	for _, p := range parts {
		if p == "" {
			return nil, errors.New("empty key segment in table header")
		}
	}
	return parts, nil
}

// unquoteTOMLKey strips the quotes from a quoted key segment, unescaping the
// common sequences in a basic (double-quoted) key.
func unquoteTOMLKey(b []byte) string {
	if len(b) < 2 || b[0] != b[len(b)-1] {
		return string(b)
	}
	inner := string(b[1 : len(b)-1])
	if b[0] == '"' {
		inner = strings.ReplaceAll(inner, `\"`, `"`)
		inner = strings.ReplaceAll(inner, `\\`, `\`)
	}
	return inner
}

// skipTOMLString advances past the TOML string beginning at raw[i] (a quote
// char) and returns the index just past its close. Handles basic ("), literal
// ('), and multiline (""" / ”') strings.
func skipTOMLString(raw []byte, i int) int {
	n := len(raw)
	q := raw[i]
	if i+2 < n && raw[i+1] == q && raw[i+2] == q {
		// multiline string
		i += 3
		for i < n {
			if raw[i] == q && i+2 < n && raw[i+1] == q && raw[i+2] == q {
				return i + 3
			}
			if q == '"' && raw[i] == '\\' && i+1 < n {
				i += 2
				continue
			}
			i++
		}
		return n
	}
	// single-line string
	i++
	for i < n {
		c := raw[i]
		if c == '\n' {
			return i // unterminated; stop at line boundary
		}
		if q == '"' && c == '\\' && i+1 < n {
			i += 2
			continue
		}
		if c == q {
			return i + 1
		}
		i++
	}
	return n
}
