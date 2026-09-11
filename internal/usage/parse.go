// Package usage reads Claude plan usage: credentials, the HTTP fetch, the on-disk cache and
// the parsing of both input formats (the OAuth usage endpoint and the Claude Code status
// line). It contains no UI code.
package usage

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// Window is one usage window as found in the input, in document order.
type Window struct {
	Key      string
	Used     float64 // percent: utilization, or used_percentage in the status line format
	Reset    float64 // Unix seconds; meaningful only when HasReset
	HasReset bool
}

// Extra is the extra_usage block of the endpoint response.
type Extra struct {
	Enabled        bool
	Utilization    float64
	HasUtilization bool
}

// Snapshot is the parsed content of one response.
type Snapshot struct {
	Windows []Window
	Extra   Extra
}

// ErrInvalidJSON is returned for input that is not a single valid JSON value.
var ErrInvalidJSON = errors.New("response is not valid JSON")

type entry struct {
	key   string
	value json.RawMessage
}

// Parse accepts the endpoint format and the status line format (wrapped in "rate_limits"
// or not). Every field is optional: unexpected shapes are skipped, never fatal. Only input
// that is not valid JSON is an error. Windows keep their document order and are not yet
// filtered by window length; that belongs to the pace package.
func Parse(data []byte) (Snapshot, error) {
	if !json.Valid(data) {
		return Snapshot{}, ErrInvalidJSON
	}
	var snap Snapshot
	top, isObj := objectEntries(data)
	if !isObj {
		return snap, nil
	}
	windows := top
	if rl, found := lookup(top, "rate_limits"); found {
		windows, _ = objectEntries(rl)
	}
	for _, e := range windows {
		if w, ok := parseWindow(e); ok {
			snap.Windows = append(snap.Windows, w)
		}
	}
	if ex, found := lookup(top, "extra_usage"); found {
		snap.Extra = parseExtra(ex)
	}
	return snap, nil
}

// IsObject reports whether data is a valid JSON object.
func IsObject(data []byte) bool {
	_, ok := objectEntries(data)
	return ok && json.Valid(data)
}

func parseWindow(e entry) (Window, bool) {
	fields, ok := objectEntries(e.value)
	if !ok {
		return Window{}, false
	}
	w := Window{Key: e.key}
	// jq: .utilization // .used_percentage (null and false fall through)
	util, _ := lookup(fields, "utilization")
	if u, isNum := number(util); isNum {
		w.Used = u
	} else if !nullOrFalse(util) {
		return Window{}, false
	} else if u, isNum := number(lookupRaw(fields, "used_percentage")); isNum {
		w.Used = u
	} else {
		return Window{}, false
	}
	sec, has, err := ParseResetsAt(lookupRaw(fields, "resets_at"))
	if err != nil {
		return Window{}, false
	}
	w.Reset, w.HasReset = sec, has
	return w, true
}

func parseExtra(raw json.RawMessage) Extra {
	fields, ok := objectEntries(raw)
	if !ok || string(bytes.TrimSpace(lookupRaw(fields, "is_enabled"))) != "true" {
		return Extra{}
	}
	ex := Extra{Enabled: true}
	util := lookupRaw(fields, "utilization")
	if u, isNum := number(util); isNum {
		ex.Utilization, ex.HasUtilization = u, true
	} else if !nullOrFalse(util) {
		// the reference's jq program fails on a non-numeric utilization: no footer at all
		return Extra{}
	}
	return ex
}

// objectEntries decodes a JSON object preserving key order, as jq's to_entries does.
// A repeated key keeps its first position and its last value, like jq.
func objectEntries(raw json.RawMessage) ([]entry, bool) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, false
	}
	var out []entry
	index := map[string]int{}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, false
		}
		key, _ := tok.(string) // object keys are always strings in valid JSON
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return nil, false
		}
		if i, dup := index[key]; dup {
			out[i].value = v
			continue
		}
		index[key] = len(out)
		out = append(out, entry{key, v})
	}
	if _, err := dec.Token(); err != nil { // closing '}'
		return nil, false
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, false
	}
	return out, true
}

func lookup(es []entry, key string) (json.RawMessage, bool) {
	for _, e := range es {
		if e.key == key {
			return e.value, true
		}
	}
	return nil, false
}

func lookupRaw(es []entry, key string) json.RawMessage {
	v, _ := lookup(es, key)
	return v
}

func number(raw json.RawMessage) (float64, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || !(raw[0] == '-' || (raw[0] >= '0' && raw[0] <= '9')) {
		return 0, false
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err != nil {
		return 0, false
	}
	return f, true
}

func nullOrFalse(raw json.RawMessage) bool {
	s := string(bytes.TrimSpace(raw))
	return s == "" || s == "null" || s == "false"
}
