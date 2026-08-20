package main

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.etcd.io/bbolt"
)

func TestIconKeyAcceptsRealMarkerPaths(t *testing.T) {
	for _, key := range []string{
		"gfx/terobjs/mm/thingwall",
		"gfx/invobjs/small/bush",
		"gfx/hud/mmap/cave",
		"gfx/kritter/bear/bear",
		"custom",
		"a-b_c/d-e_f",
	} {
		if !iconKeyOK(key) {
			t.Errorf("%q is a real marker image path and must be accepted", key)
		}
	}
}

func TestIconKeyRejectsPathEscapes(t *testing.T) {
	for _, key := range []string{
		"",
		"..",
		"../etc/passwd",
		"gfx/../../etc/passwd",
		"gfx/./thing",
		"/gfx/thing",
		"gfx/thing/",
		"gfx//thing",
		`gfx\thing`,
		"gfx/thing.png",
		"gfx/thing;rm -rf /",
		"gfx/with space",
		"gfx/" + strings.Repeat("a", maxIconKeyLen),
	} {
		if iconKeyOK(key) {
			t.Errorf("%q must be rejected as an icon key", key)
		}
	}
}

// The key becomes a filesystem path, so a rejected key is the only thing
// keeping writes inside the icon directory. This pins that down.
func TestCustomIconPathStaysUnderStorage(t *testing.T) {
	m := &Map{gridStorage: "/srv/map"}
	root := filepath.Join("/srv/map", iconsSubdir)

	for _, key := range []string{"gfx/terobjs/mm/thingwall", "a", "a/b/c/d/e"} {
		got := m.customIconPath(key)
		if !strings.HasPrefix(got, root+string(filepath.Separator)) {
			t.Errorf("path for %q escaped the icon directory: %s", key, got)
		}
		if got != filepath.Clean(got) {
			t.Errorf("path for %q is not already clean: %s", key, got)
		}
	}
}

func encodePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.NRGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encoding fixture: %v", err)
	}
	return buf.Bytes()
}

func TestDecodeIconAcceptsAReasonablePNG(t *testing.T) {
	img, err := decodeIcon(bytes.NewReader(encodePNG(t, 32, 32)))
	if err != nil {
		t.Fatalf("a 32x32 PNG should be accepted: %v", err)
	}
	if img.Bounds().Dx() != 32 {
		t.Errorf("decoded width = %d, want 32", img.Bounds().Dx())
	}
}

func TestDecodeIconRejectsNonPNG(t *testing.T) {
	// A GIF header: the right shape for an image, the wrong format. Trusting the
	// filename would have let this through.
	gif := []byte("GIF89a\x01\x00\x01\x00\x00\xff\x00,")
	if _, err := decodeIcon(bytes.NewReader(gif)); err == nil {
		t.Error("a non-PNG upload must be rejected")
	}
	if _, err := decodeIcon(strings.NewReader("<svg onload=alert(1)>")); err == nil {
		t.Error("markup must not be accepted as an icon")
	}
	if _, err := decodeIcon(bytes.NewReader(nil)); err == nil {
		t.Error("an empty upload must be rejected")
	}
}

func TestDecodeIconRejectsOversizedImage(t *testing.T) {
	big := encodePNG(t, maxIconPixels+1, 8)
	if _, err := decodeIcon(bytes.NewReader(big)); err == nil {
		t.Errorf("an image wider than %dpx must be rejected", maxIconPixels)
	}
	tall := encodePNG(t, 8, maxIconPixels+1)
	if _, err := decodeIcon(bytes.NewReader(tall)); err == nil {
		t.Errorf("an image taller than %dpx must be rejected", maxIconPixels)
	}
	if _, err := decodeIcon(bytes.NewReader(encodePNG(t, maxIconPixels, maxIconPixels))); err != nil {
		t.Errorf("an image exactly at the limit should be accepted: %v", err)
	}
}

func TestIconEntriesSortMissingFirst(t *testing.T) {
	entries := []IconEntry{
		{Key: "zzz", Source: IconBuiltin},
		{Key: "bbb", Source: IconMissing},
		{Key: "aaa", Source: IconCustom},
		{Key: "aaa2", Source: IconMissing},
	}
	// Same comparison iconEntries applies once it has gathered the keys.
	rank := map[IconSource]int{IconMissing: 0, IconCustom: 1, IconBuiltin: 2}
	sortEntries(entries, rank)

	got := []string{}
	for _, e := range entries {
		got = append(got, e.Key)
	}
	want := []string{"aaa2", "bbb", "aaa", "zzz"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}

func TestDownloadNameKeepsTheKeyReadable(t *testing.T) {
	cases := map[string]string{
		"gfx/terobjs/mm/thingwall": "gfx_terobjs_mm_thingwall.png",
		"custom":                   "custom.png",
		"a/b":                      "a_b.png",
	}
	for key, want := range cases {
		if got := downloadName(key); got != want {
			t.Errorf("downloadName(%q) = %q, want %q", key, got, want)
		}
	}
}

func TestDownloadNameHasNoPathSeparators(t *testing.T) {
	// The result goes into a Content-Disposition filename, so it must not be
	// able to suggest a path of its own.
	for _, key := range []string{"gfx/terobjs/mm/thingwall", "a/b/c/d"} {
		got := downloadName(key)
		if strings.ContainsAny(got, `/\`) {
			t.Errorf("downloadName(%q) = %q, which still contains a separator", key, got)
		}
	}
}

func TestIconFilePathPrefersTheUpload(t *testing.T) {
	dir := t.TempDir()
	m := &Map{gridStorage: dir}
	key := "gfx/terobjs/mm/probe"

	if _, ok := m.iconFilePath(key); ok {
		t.Fatal("a key with no icon anywhere must report none")
	}

	custom := m.customIconPath(key)
	if err := os.MkdirAll(filepath.Dir(custom), 0755); err != nil {
		t.Fatalf("preparing the icon directory: %v", err)
	}
	if err := os.WriteFile(custom, encodePNG(t, 8, 8), 0644); err != nil {
		t.Fatalf("writing the upload: %v", err)
	}
	got, ok := m.iconFilePath(key)
	if !ok || got != custom {
		t.Errorf("iconFilePath = (%q, %v), want the uploaded file %q", got, ok, custom)
	}
}

// newTestMap opens a Map backed by a scratch database, for the handful of tests
// that need real buckets rather than pure helpers.
func newTestMap(t *testing.T) *Map {
	t.Helper()
	dir := t.TempDir()
	db, err := bbolt.Open(filepath.Join(dir, "grids.db"), 0600, nil)
	if err != nil {
		t.Fatalf("opening the test database: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return &Map{db: db, gridStorage: dir}
}

func putGrid(t *testing.T, m *Map, id string, mapID, x, y int) {
	t.Helper()
	err := m.db.Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("grids"))
		if err != nil {
			return err
		}
		raw, err := json.Marshal(GridData{ID: id, Map: mapID, Coord: Coord{X: x, Y: y}})
		if err != nil {
			return err
		}
		return b.Put([]byte(id), raw)
	})
	if err != nil {
		t.Fatalf("storing grid %q: %v", id, err)
	}
}

func putMarker(t *testing.T, m *Map, key string, mk Marker) {
	t.Helper()
	err := m.db.Update(func(tx *bbolt.Tx) error {
		mb, err := tx.CreateBucketIfNotExists([]byte("markers"))
		if err != nil {
			return err
		}
		g, err := mb.CreateBucketIfNotExists([]byte("grid"))
		if err != nil {
			return err
		}
		raw, err := json.Marshal(mk)
		if err != nil {
			return err
		}
		return g.Put([]byte(key), raw)
	})
	if err != nil {
		t.Fatalf("storing marker %q: %v", key, err)
	}
}

// A marker is stored against the grid it sits on and only gets a position once
// that grid is known here, so counting stored markers told an admin the map was
// showing five of something when it was showing none.
func TestMarkerImageStatsSeparatesWhatTheMapCanDraw(t *testing.T) {
	m := newTestMap(t)
	putGrid(t, m, "known", 2, 0, 0)
	putGrid(t, m, "far", 3, 40, 40)

	const img = "gfx/terobjs/mm/tarpit"
	putMarker(t, m, "known_1_1", Marker{Name: "A", GridID: "known", Image: img})
	putMarker(t, m, "known_2_2", Marker{Name: "B", GridID: "known", Image: img, Hidden: true})
	putMarker(t, m, "gone_3_3", Marker{Name: "C", GridID: "gone", Image: img})
	putMarker(t, m, "far_4_4", Marker{Name: "D", GridID: "far", Image: img})
	putMarker(t, m, "known_5_5", Marker{Name: "E", GridID: "known", Image: "gfx/terobjs/mm/geyser"})

	stats := m.markerImageStats()
	st := stats[img]
	if st == nil {
		t.Fatal("no stats for the image every marker but one carries")
	}
	if st.Stored != 4 || st.OnMap != 2 || st.Hidden != 1 || st.NoGrid != 1 {
		t.Errorf("stats = %+v, want 4 stored, 2 on the map, 1 hidden, 1 without a grid", *st)
	}
	if got := st.mapIDs(); len(got) != 2 || got[0] != 2 || got[1] != 3 {
		t.Errorf("mapIDs = %v, want the two maps its drawable markers are on", got)
	}
	if other := stats["gfx/terobjs/mm/geyser"]; other == nil || other.Stored != 1 || other.OnMap != 1 {
		t.Errorf("the second image should be counted on its own, got %+v", other)
	}
}

// The row is only worth annotating when the stored count is not what the map
// shows, so an ordinary key stays quiet.
func TestIconEntryNoteOnlySpeaksUpWhenSomethingIsOff(t *testing.T) {
	if note := (IconEntry{Markers: 3, OnMap: 3}).Note(); note != "" {
		t.Errorf("a fully drawn key should carry no note, got %q", note)
	}
	// One map is the ordinary case and naming it on every row would be noise.
	if note := (IconEntry{Markers: 3, OnMap: 3, MapIDs: []int{2}}).Note(); note != "" {
		t.Errorf("a key whose markers all sit on one map should carry no note, got %q", note)
	}
	note := (IconEntry{Markers: 4, OnMap: 2, HiddenMarkers: 1, NoGrid: 1, MapIDs: []int{2, 3}}).Note()
	for _, want := range []string{"1 on a grid this server does not know", "1 hidden by an admin", "maps 2, 3"} {
		if !strings.Contains(note, want) {
			t.Errorf("note %q does not mention %q", note, want)
		}
	}
}

// Everything the icon page counts but cannot draw is worth one number at the
// top of the page, so the admin does not have to spot it row by row.
func TestIconEntriesReportsUnplacedMarkers(t *testing.T) {
	m := newTestMap(t)
	putGrid(t, m, "known", 2, 0, 0)
	putMarker(t, m, "known_1_1", Marker{GridID: "known", Image: "gfx/terobjs/mm/tarpit"})
	putMarker(t, m, "gone_1_1", Marker{GridID: "gone", Image: "gfx/terobjs/mm/tarpit"})
	putMarker(t, m, "gone_2_2", Marker{GridID: "gone", Image: "gfx/terobjs/mm/geyser"})

	unplaced := 0
	for _, e := range m.iconEntries(false) {
		unplaced += e.Unplaced()
		if len(e.MapIDs) != 0 {
			t.Errorf("%s: map ids belong on the page only when there is more than one map", e.Key)
		}
	}
	if unplaced != 2 {
		t.Errorf("unplaced total = %d, want 2", unplaced)
	}
}
