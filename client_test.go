package main

import "testing"

// window builds a lookup over a fake grids bucket for buildMatches.
func window(known map[string]GridData) func(string) *GridData {
	return func(id string) *GridData {
		gd, ok := known[id]
		if !ok {
			return nil
		}
		return &gd
	}
}

func TestBuildMatchesCountsAgreeingGrids(t *testing.T) {
	// A 2x2 window fully inside map 1, laid out at offset (10, 20).
	grids := [][]string{{"a", "b"}, {"c", "d"}}
	known := map[string]GridData{
		"a": {Map: 1, Coord: Coord{X: 10, Y: 20}},
		"b": {Map: 1, Coord: Coord{X: 10, Y: 21}},
		"c": {Map: 1, Coord: Coord{X: 11, Y: 20}},
		"d": {Map: 1, Coord: Coord{X: 11, Y: 21}},
	}

	matches := buildMatches(grids, window(known))
	if len(matches) != 1 {
		t.Fatalf("expected 1 map, got %d", len(matches))
	}
	mm := matches[1]
	if mm.conflict {
		t.Error("consistent grids should not report a conflict")
	}
	if mm.count != 4 {
		t.Errorf("count = %d, want 4", mm.count)
	}
	if (mm.off != gridOffset{X: 10, Y: 20}) {
		t.Errorf("offset = %+v, want {10 20}", mm.off)
	}
}

func TestBuildMatchesFlagsDisagreeingGrids(t *testing.T) {
	// Both grids claim map 1, but they imply different offsets.
	grids := [][]string{{"a", "b"}}
	known := map[string]GridData{
		"a": {Map: 1, Coord: Coord{X: 10, Y: 20}},
		"b": {Map: 1, Coord: Coord{X: 99, Y: 99}},
	}

	matches := buildMatches(grids, window(known))
	if !matches[1].conflict {
		t.Error("grids implying different offsets must set conflict")
	}
}

func TestBuildMatchesIgnoresUnknownGrids(t *testing.T) {
	grids := [][]string{{"a", "unknown"}}
	known := map[string]GridData{
		"a": {Map: 3, Coord: Coord{X: 0, Y: 0}},
	}

	matches := buildMatches(grids, window(known))
	if len(matches) != 1 || matches[3].count != 1 {
		t.Errorf("unknown grids should be skipped, got %+v", matches)
	}
}

func TestPlanMergesRequiresMinimumOverlap(t *testing.T) {
	// Map 2 shows up on a single grid — not enough evidence to fold it in.
	matches := map[int]*mapMatch{
		1: {off: gridOffset{X: 0, Y: 0}, count: 8},
		2: {off: gridOffset{X: 5, Y: 5}, count: 1},
	}

	decisions := planMerges(matches, 1, 2)
	if len(decisions) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(decisions))
	}
	if decisions[0].mapID != 2 {
		t.Fatalf("decision is about map %d, want 2", decisions[0].mapID)
	}
	if decisions[0].allowed {
		t.Error("a single overlapping grid must not trigger a merge")
	}
	if decisions[0].reason == "" {
		t.Error("a refused merge must carry a reason for the log")
	}
}

func TestPlanMergesAllowsWellSupportedMerge(t *testing.T) {
	matches := map[int]*mapMatch{
		1: {off: gridOffset{X: 0, Y: 0}, count: 5},
		2: {off: gridOffset{X: 5, Y: 5}, count: 4},
	}

	decisions := planMerges(matches, 1, 2)
	if len(decisions) != 1 || !decisions[0].allowed {
		t.Fatalf("4 agreeing grids should permit the merge, got %+v", decisions)
	}
	if (decisions[0].off != gridOffset{X: 5, Y: 5}) {
		t.Errorf("offset = %+v, want {5 5}", decisions[0].off)
	}
}

func TestPlanMergesRefusesConflictingMap(t *testing.T) {
	// Plenty of overlap, but the grids contradict each other.
	matches := map[int]*mapMatch{
		1: {off: gridOffset{X: 0, Y: 0}, count: 5},
		2: {off: gridOffset{X: 5, Y: 5}, count: 9, conflict: true},
	}

	decisions := planMerges(matches, 1, 2)
	if len(decisions) != 1 || decisions[0].allowed {
		t.Fatalf("a conflicting map must never merge, got %+v", decisions)
	}
}

func TestPlanMergesExcludesAnchor(t *testing.T) {
	matches := map[int]*mapMatch{
		1: {off: gridOffset{X: 0, Y: 0}, count: 9},
	}

	if d := planMerges(matches, 1, 2); len(d) != 0 {
		t.Errorf("the anchor map must not be a merge candidate, got %+v", d)
	}
}

func TestPlanMergesIsDeterministic(t *testing.T) {
	matches := map[int]*mapMatch{
		1: {off: gridOffset{}, count: 5},
		4: {off: gridOffset{}, count: 5},
		2: {off: gridOffset{}, count: 5},
		3: {off: gridOffset{}, count: 5},
	}

	// Go randomises map iteration, so run it repeatedly.
	for i := 0; i < 20; i++ {
		decisions := planMerges(matches, 1, 2)
		got := []int{}
		for _, d := range decisions {
			got = append(got, d.mapID)
		}
		if len(got) != 3 || got[0] != 2 || got[1] != 3 || got[2] != 4 {
			t.Fatalf("decisions must be ordered by map ID, got %v", got)
		}
	}
}

func TestPlanMergesTreatsZeroThresholdAsOne(t *testing.T) {
	matches := map[int]*mapMatch{
		1: {off: gridOffset{}, count: 3},
		2: {off: gridOffset{}, count: 1},
	}

	if d := planMerges(matches, 1, 0); len(d) != 1 || !d[0].allowed {
		t.Errorf("threshold below 1 should behave as 1, got %+v", d)
	}
}

func TestValidGridID(t *testing.T) {
	valid := []string{"12345", "-98765", "abcDEF_09", "a"}
	for _, id := range valid {
		if !validGridID.MatchString(id) {
			t.Errorf("%q should be accepted as a grid id", id)
		}
	}

	invalid := []string{
		"",
		"../../etc/passwd",
		"a/b",
		`a\b`,
		"..",
		"a.png",
		"with space",
		"very" + string(make([]byte, 100)),
	}
	for _, id := range invalid {
		if validGridID.MatchString(id) {
			t.Errorf("%q must be rejected as a grid id", id)
		}
	}
}
