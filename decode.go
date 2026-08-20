package main

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Uploads come from game clients this server does not control, and the JSON
// types they use have drifted between client versions: coordinates arrive as
// 1234, as 1234.0 and occasionally quoted, and a grid ID is sometimes a string
// and sometimes a bare 64-bit number. A whole batch used to be decoded into
// strict Go types in one call, so a single value in an unexpected shape threw
// away every character or marker in the request. From the outside that looks
// like "players never appear" or "some marker types are missing" — with nothing
// but one line in the log to say why. These two types accept either spelling.

const maxCoord = 1 << 30

type flexInt int

func (f *flexInt) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	if s == "" || s == "null" {
		*f = 0
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fmt.Errorf("%s is not a number", clip(b))
	}
	if math.IsNaN(v) || math.IsInf(v, 0) || v > maxCoord || v < -maxCoord {
		return fmt.Errorf("%s is out of range for a coordinate", clip(b))
	}
	*f = flexInt(math.Round(v))
	return nil
}

type flexString string

func (f *flexString) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*f = flexString(s)
		return nil
	}
	// json.Number keeps the literal text, so a 64-bit grid ID survives intact
	// rather than going through a float and losing its last digits.
	var n json.Number
	if err := json.Unmarshal(b, &n); err == nil {
		*f = flexString(n.String())
		return nil
	}
	if string(b) == "null" {
		*f = ""
		return nil
	}
	return fmt.Errorf("%s is neither a string nor a number", clip(b))
}

// Some clients write a marker's id as a bare hexadecimal token:
//
//	{"image":"gfx/terobjs/mm/thingwall","name":"Lintreath",…,"id":6a7f4d30000008f5,"type":"shared"}
//
// That is not JSON — the value is neither a number nor a quoted string — so the
// document cannot even be split into entries, and every marker in the upload is
// lost along with the one carrying it. Shared markers are exactly the ones a
// group cares about, and the field is one this server does not read.
//
// repairLooseJSON quotes any bare token sitting where a value belongs and
// leaves valid JSON untouched. It runs only after a strict parse has failed, so
// a well-formed upload never goes near it.
func repairLooseJSON(b []byte) []byte {
	out := make([]byte, 0, len(b)+16)
	inString, escaped := false, false
	for i := 0; i < len(b); {
		c := b[i]
		if inString {
			out = append(out, c)
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			i++
			continue
		}
		if c == '"' {
			inString = true
			out = append(out, c)
			i++
			continue
		}
		if isJSONStructural(c) || isJSONSpace(c) {
			out = append(out, c)
			i++
			continue
		}
		j := i
		for j < len(b) && !isJSONStructural(b[j]) && !isJSONSpace(b[j]) && b[j] != '"' {
			j++
		}
		tok := b[i:j]
		// json.Valid covers every bare token JSON actually allows: a number,
		// true, false and null.
		if json.Valid(tok) {
			out = append(out, tok...)
		} else {
			out = append(out, '"')
			for _, t := range tok {
				if t == '\\' {
					out = append(out, '\\')
				}
				out = append(out, t)
			}
			out = append(out, '"')
		}
		i = j
	}
	return out
}

func isJSONStructural(c byte) bool {
	return c == '{' || c == '}' || c == '[' || c == ']' || c == ',' || c == ':'
}

func isJSONSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

// decodeLoose parses v from b, retrying through repairLooseJSON if the strict
// parse fails. The bool reports whether the repair was needed, so the caller
// can say so once rather than silently accepting broken uploads forever.
func decodeLoose(b []byte, v interface{}) (bool, error) {
	if err := json.Unmarshal(b, v); err == nil {
		return false, nil
	} else if repaired := repairLooseJSON(b); json.Unmarshal(repaired, v) == nil {
		return true, nil
	} else {
		return false, err
	}
}

// clip shortens a value for a log line. Upload bodies carry hundreds of entries
// and the interesting part is always at the front.
func clip(b []byte) string {
	const limit = 300
	if len(b) <= limit {
		return string(b)
	}
	return string(b[:limit]) + "…"
}

// Clients retry every few seconds, so a malformed field would otherwise write
// the same line to the log hundreds of times a minute — enough to bury the rest
// of it and to fill a small disk. Each distinct message is logged at most once
// per interval.
type logThrottle struct {
	mu       sync.Mutex
	last     map[string]time.Time
	interval time.Duration
	now      func() time.Time
}

func newLogThrottle(interval time.Duration) *logThrottle {
	return &logThrottle{
		last:     map[string]time.Time{},
		interval: interval,
		now:      time.Now,
	}
}

func (l *logThrottle) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if t, ok := l.last[key]; ok && now.Sub(t) < l.interval {
		return false
	}
	// Keys are built from the endpoint and the uploading user, so the map is
	// small in practice; this keeps it that way if a key ever includes
	// something more varied.
	if len(l.last) > 256 {
		for k, t := range l.last {
			if now.Sub(t) >= l.interval {
				delete(l.last, k)
			}
		}
	}
	l.last[key] = now
	return true
}

// uploadLog throttles the complaints the client API makes about the data it is
// sent. One line a minute per endpoint and user is enough to notice a client
// sending something this server cannot read.
var uploadLog = newLogThrottle(time.Minute)

// markerLog throttles the "these markers have nowhere to go" line, which would
// otherwise repeat on every poll from every open map.
var markerLog = newLogThrottle(10 * time.Minute)
