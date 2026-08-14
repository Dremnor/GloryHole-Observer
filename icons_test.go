package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"path/filepath"
	"strings"
	"testing"
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
