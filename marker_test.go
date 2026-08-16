package main

import "testing"

// Go's integer division truncates towards zero, so a point west or north of
// the origin would land on the grid next door and the marker would show up a
// whole grid away from where it was placed.
func TestMarkerPlacementHandlesNegativeCoordinates(t *testing.T) {
	cases := []struct {
		x, y     int
		wantGrid Coord
		wantIn   Position
	}{
		{0, 0, Coord{0, 0}, Position{0, 0}},
		{50, 50, Coord{0, 0}, Position{50, 50}},
		{99, 99, Coord{0, 0}, Position{99, 99}},
		{100, 100, Coord{1, 1}, Position{0, 0}},
		{250, 380, Coord{2, 3}, Position{50, 80}},
		{-1, -1, Coord{-1, -1}, Position{99, 99}},
		{-50, -50, Coord{-1, -1}, Position{50, 50}},
		{-100, -100, Coord{-1, -1}, Position{0, 0}},
		{-101, -101, Coord{-2, -2}, Position{99, 99}},
		{-750, 320, Coord{-8, 3}, Position{50, 20}},
	}
	for _, c := range cases {
		gc, in := markerPlacement(c.x, c.y)
		if gc != c.wantGrid || in != c.wantIn {
			t.Errorf("markerPlacement(%d, %d) = %v %v, want %v %v",
				c.x, c.y, gc, in, c.wantGrid, c.wantIn)
		}
	}
}

// Whatever the split, putting the two halves back together has to land on the
// point that was clicked — that is the whole contract with getMarkers, which
// rebuilds the absolute position exactly this way.
func TestMarkerPlacementRoundTrips(t *testing.T) {
	for x := -420; x <= 420; x += 7 {
		for y := -420; y <= 420; y += 11 {
			gc, in := markerPlacement(x, y)
			gotX := in.X + gc.X*GridSize
			gotY := in.Y + gc.Y*GridSize
			if gotX != x || gotY != y {
				t.Fatalf("round trip of %d,%d came back as %d,%d", x, y, gotX, gotY)
			}
			if in.X < 0 || in.X >= GridSize || in.Y < 0 || in.Y >= GridSize {
				t.Fatalf("offset for %d,%d is outside the grid: %v", x, y, in)
			}
		}
	}
}

func TestFloorDivRoundsTowardsNegativeInfinity(t *testing.T) {
	cases := map[[2]int]int{
		{7, 100}: 0, {100, 100}: 1, {199, 100}: 1, {200, 100}: 2,
		{-1, 100}: -1, {-100, 100}: -1, {-101, 100}: -2, {0, 100}: 0,
	}
	for in, want := range cases {
		if got := floorDiv(in[0], in[1]); got != want {
			t.Errorf("floorDiv(%d, %d) = %d, want %d", in[0], in[1], got, want)
		}
	}
}
