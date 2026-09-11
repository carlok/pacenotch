package usage

import (
	"bytes"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"time"
)

var (
	isoRe    = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(\.\d+)?(.*)$`)
	offsetRe = regexp.MustCompile(`^([+-])(\d{2}):?(\d{2})$`)

	errTimestamp = errors.New("invalid resets_at timestamp")
)

// ParseResetsAt converts a resets_at JSON value to Unix seconds, exactly like the
// reference's jq `epoch` function: numbers are used as they are (fractions kept), ISO 8601
// strings drop their fractional seconds and honour a Z, ±HH:MM or ±HHMM offset.
// ok is false (with a nil error) when the value is null or missing.
func ParseResetsAt(raw json.RawMessage) (sec float64, ok bool, err error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return 0, false, nil
	}
	switch raw[0] {
	case '"':
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return 0, false, errTimestamp
		}
		t, err := ParseISO(s)
		if err != nil {
			return 0, false, err
		}
		return t, true, nil
	case '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		var f float64
		if err := json.Unmarshal(raw, &f); err != nil {
			return 0, false, errTimestamp
		}
		return f, true, nil
	}
	return 0, false, errTimestamp
}

// ParseISO parses "YYYY-MM-DDTHH:MM:SS[.frac][Z|±HH:MM|±HHMM]" into Unix seconds.
func ParseISO(s string) (float64, error) {
	m := isoRe.FindStringSubmatch(s)
	if m == nil {
		return 0, errTimestamp
	}
	n := make([]int, 6)
	for i := range n {
		n[i], _ = strconv.Atoi(m[i+1]) // the regexp guarantees digits
	}
	if n[1] < 1 || n[1] > 12 || n[2] < 1 || n[2] > 31 || n[3] > 23 || n[4] > 59 || n[5] > 60 {
		return 0, errTimestamp
	}
	t := float64(time.Date(n[0], time.Month(n[1]), n[2], n[3], n[4], n[5], 0, time.UTC).Unix())
	tz := m[8]
	if tz == "" || tz == "Z" {
		return t, nil
	}
	o := offsetRe.FindStringSubmatch(tz)
	if o == nil {
		return 0, errTimestamp
	}
	h, _ := strconv.Atoi(o[2])
	mm, _ := strconv.Atoi(o[3])
	sign := 1.0
	if o[1] == "-" {
		sign = -1
	}
	return t - sign*float64(h*3600+mm*60), nil
}
