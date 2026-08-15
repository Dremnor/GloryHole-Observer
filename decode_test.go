package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestFlexIntAcceptsTheShapesClientsSend(t *testing.T) {
	cases := map[string]int{
		"1234":    1234,
		"-1234":   -1234,
		"0":       0,
		"1234.0":  1234,
		"1234.6":  1235,
		"-1234.6": -1235,
		`"1234"`:  1234,
		"null":    0,
		"1e3":     1000,
	}
	for in, want := range cases {
		var got flexInt
		if err := json.Unmarshal([]byte(in), &got); err != nil {
			t.Errorf("%s should decode as a coordinate: %v", in, err)
			continue
		}
		if int(got) != want {
			t.Errorf("%s decoded to %d, want %d", in, got, want)
		}
	}
}

func TestFlexIntRejectsNonsense(t *testing.T) {
	for _, in := range []string{`"abc"`, `{}`, `[]`, `true`, `1e400`, `-1e400`} {
		var got flexInt
		if err := json.Unmarshal([]byte(in), &got); err == nil {
			t.Errorf("%s decoded to %d, but is not a coordinate", in, got)
		}
	}
}

func TestFlexStringTakesGridIDsEitherWay(t *testing.T) {
	cases := map[string]string{
		`"-8834948214763"`: "-8834948214763",
		`-8834948214763`:   "-8834948214763",
		`""`:               "",
		`null`:             "",
	}
	for in, want := range cases {
		var got flexString
		if err := json.Unmarshal([]byte(in), &got); err != nil {
			t.Errorf("%s should decode as a grid id: %v", in, err)
			continue
		}
		if string(got) != want {
			t.Errorf("%s decoded to %q, want %q", in, got, want)
		}
	}
}

// A grid ID is a 64-bit number. Going through a float would round the last
// digits off and quietly point markers at a grid that does not exist.
func TestFlexStringKeepsFullPrecisionOfANumericGridID(t *testing.T) {
	const id = "9007199254740993" // 2^53 + 1: not representable as a float64
	var got flexString
	if err := json.Unmarshal([]byte(id), &got); err != nil {
		t.Fatalf("decoding %s: %v", id, err)
	}
	if string(got) != id {
		t.Errorf("grid id decoded to %q, want %q", got, id)
	}
}

// The point of decoding entry by entry: what a client sends alongside a bad
// value should still arrive.
func TestOneBadEntryDoesNotDiscardTheBatch(t *testing.T) {
	body := `[
		{"name":"good","gridID":"12","x":1,"y":2,"image":"gfx/terobjs/mm/thingwall"},
		{"name":"odd colour","gridID":"12","x":3,"y":4,"image":"gfx/terobjs/mm/claypit","color":{"r":1}},
		{"name":"broken","gridID":"12","x":"east","y":4,"image":"gfx/terobjs/mm/tarpit"},
		{"name":"float coords","gridID":34,"x":5.0,"y":6.0,"image":"gfx/terobjs/mm/cave"}
	]`
	type markerRaw struct {
		Name   string
		GridID flexString
		X, Y   flexInt
		Image  string
	}
	raws := []json.RawMessage{}
	if err := json.Unmarshal([]byte(body), &raws); err != nil {
		t.Fatalf("splitting the batch: %v", err)
	}
	got := []string{}
	for _, raw := range raws {
		mr := markerRaw{}
		if err := json.Unmarshal(raw, &mr); err != nil {
			continue
		}
		got = append(got, mr.Name)
	}
	want := []string{"good", "odd colour", "float coords"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("kept %v, want %v", got, want)
	}
}

func TestLogThrottleLetsOneThroughPerInterval(t *testing.T) {
	now := time.Unix(0, 0)
	l := newLogThrottle(time.Minute)
	l.now = func() time.Time { return now }

	if !l.allow("a") {
		t.Fatal("the first message must be logged")
	}
	if l.allow("a") {
		t.Error("a repeat within the interval must be suppressed")
	}
	if !l.allow("b") {
		t.Error("a different message must not be suppressed by another one")
	}
	now = now.Add(time.Minute)
	if !l.allow("a") {
		t.Error("the message must be logged again once the interval has passed")
	}
}

func TestLogThrottleDoesNotGrowWithoutBound(t *testing.T) {
	now := time.Unix(0, 0)
	l := newLogThrottle(time.Minute)
	l.now = func() time.Time { return now }

	for i := 0; i < 500; i++ {
		l.allow(string(rune('a' + i%26)) + strings.Repeat("x", i))
	}
	now = now.Add(2 * time.Minute)
	l.allow("trigger the prune")
	if len(l.last) > 256 {
		t.Errorf("throttle is holding %d expired keys", len(l.last))
	}
}

func TestClipKeepsLogLinesShort(t *testing.T) {
	long := []byte(strings.Repeat("x", 1000))
	got := clip(long)
	if len(got) > 320 {
		t.Errorf("clipped value is %d bytes long", len(got))
	}
	short := []byte(`{"x":1}`)
	if clip(short) != `{"x":1}` { //nolint
		t.Errorf("a short value must be logged as it is, got %q", clip(short))
	}
}
