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
		l.allow(string(rune('a'+i%26)) + strings.Repeat("x", i))
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

// The exact body a client sent to the live server, which cost every marker in
// the upload: the id is a bare hexadecimal token, so the document could not be
// tokenised at all and even splitting it into entries failed.
const thingwallUpload = `[{"image":"gfx/terobjs/mm/thingwall","name":"Lintreath","x":36,"y":9,"gridID":"-8514535794887855825","id":6a7f4d30000008f5,"type":"shared"}]`

func TestSharedMarkerWithABareHexIDIsRead(t *testing.T) {
	type markerRaw struct {
		Name   string
		GridID flexString
		X, Y   flexInt
		Image  string
	}
	var raws []json.RawMessage
	if _, err := decodeLoose([]byte(thingwallUpload), &raws); err != nil {
		t.Fatalf("the upload should be readable: %v", err)
	}
	if len(raws) != 1 {
		t.Fatalf("got %d markers, want 1", len(raws))
	}
	m := markerRaw{}
	if err := json.Unmarshal(raws[0], &m); err != nil {
		t.Fatalf("decoding the marker: %v", err)
	}
	if m.Image != "gfx/terobjs/mm/thingwall" || m.Name != "Lintreath" {
		t.Errorf("got %q at %q, want the thingwall", m.Name, m.Image)
	}
	if m.X != 36 || m.Y != 9 || string(m.GridID) != "-8514535794887855825" {
		t.Errorf("position came out as %d,%d on grid %q", m.X, m.Y, m.GridID)
	}
}

func TestDecodeLooseReportsWhetherItHadToRepair(t *testing.T) {
	var v []json.RawMessage
	if repaired, err := decodeLoose([]byte(`[{"a":1}]`), &v); err != nil || repaired {
		t.Errorf("valid JSON must parse untouched, got repaired=%v err=%v", repaired, err)
	}
	if repaired, err := decodeLoose([]byte(thingwallUpload), &v); err != nil || !repaired {
		t.Errorf("the broken upload must be reported as repaired, got repaired=%v err=%v", repaired, err)
	}
	if _, err := decodeLoose([]byte(`[{"a":`), &v); err == nil {
		t.Error("genuinely truncated JSON must still be an error")
	}
}

// The repair must not touch anything that is already valid, and must not reach
// inside strings — a marker name is player-supplied text.
func TestRepairLeavesValidJSONExactlyAsItWas(t *testing.T) {
	for _, in := range []string{
		`{"a":1,"b":-2.5e3,"c":true,"d":null,"e":"txt","f":[1,2],"g":{"h":false}}`,
		`[{"name":"a, b: 6a7f4d3","x":1}]`,
		`{"name":"quote \" and backslash \\ inside"}`,
		`  [ 1 , 2 ]  `,
		`[]`,
		``,
	} {
		if got := string(repairLooseJSON([]byte(in))); got != in {
			t.Errorf("repair changed valid input\n  in:  %s\n  out: %s", in, got)
		}
	}
}

func TestRepairQuotesBareTokensOutsideStrings(t *testing.T) {
	cases := map[string]string{
		`{"id":6a7f4d30000008f5}`:  `{"id":"6a7f4d30000008f5"}`,
		`{"id":deadbeef,"x":1}`:    `{"id":"deadbeef","x":1}`,
		`[0x1f]`:                   `["0x1f"]`,
		`{"a":undefined,"b":null}`: `{"a":"undefined","b":null}`,
	}
	for in, want := range cases {
		got := string(repairLooseJSON([]byte(in)))
		if got != want {
			t.Errorf("repair(%s) = %s, want %s", in, got, want)
		}
		if !json.Valid([]byte(got)) {
			t.Errorf("repair(%s) produced invalid JSON: %s", in, got)
		}
	}
}

// A name containing something that looks like a broken value must not let the
// repair rewrite the document around it.
func TestRepairIsNotFooledByStringContents(t *testing.T) {
	in := `[{"name":"weird \"quoted\" }{ id:6a7f","image":"gfx/x"},{"id":6a7f,"image":"gfx/y"}]`
	out := repairLooseJSON([]byte(in))
	if !json.Valid(out) {
		t.Fatalf("repair produced invalid JSON: %s", out)
	}
	var got []struct {
		Name  string
		Image string
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decoding the repaired document: %v", err)
	}
	if len(got) != 2 || got[0].Name != `weird "quoted" }{ id:6a7f` || got[1].Image != "gfx/y" {
		t.Errorf("repair altered the contents: %+v", got)
	}
}
