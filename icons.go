package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"go.etcd.io/bbolt"
)

// Marker icons are addressed by the path the client reports, e.g.
// "gfx/terobjs/mm/thingwall", which the frontend turns into a request for
// "<key>.png". The images shipped with the frontend are baked into the build,
// so adding one for a newly added game object used to mean editing the repo and
// redeploying. Icons uploaded here live in the map volume instead: they survive
// rebuilds, and they take precedence over a built-in image of the same name.

// validIconKey matches those paths. Dots are excluded from the character set
// altogether, which is what makes ".." impossible rather than something to
// filter out afterwards.
var validIconKey = regexp.MustCompile(`^[A-Za-z0-9_-]+(/[A-Za-z0-9_-]+)*$`)

const (
	maxIconKeyLen  = 200
	maxIconBytes   = 1 << 20 // 1 MiB; these are small sprites
	maxIconPixels  = 512     // per side
	iconsSubdir    = "icons"
	frontendSubdir = "frontend"
)

func iconKeyOK(key string) bool {
	return key != "" && len(key) <= maxIconKeyLen && validIconKey.MatchString(key)
}

// customIconPath is where an uploaded override for key is stored.
func (m *Map) customIconPath(key string) string {
	return filepath.Join(m.gridStorage, iconsSubdir, filepath.FromSlash(key)+".png")
}

// builtinIconPath is the image shipped with the frontend build, if any.
func builtinIconPath(key string) string {
	return filepath.Join(frontendSubdir, filepath.FromSlash(key)+".png")
}

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

type IconSource string

const (
	IconCustom  IconSource = "custom"
	IconBuiltin IconSource = "builtin"
	IconMissing IconSource = "missing"
)

type IconEntry struct {
	Key     string
	Source  IconSource
	Markers int
}

func (e IconEntry) Missing() bool { return e.Source == IconMissing }
func (e IconEntry) Custom() bool  { return e.Source == IconCustom }

// iconFilePath resolves the file currently serving key, preferring an upload
// over the image shipped with the build. ok is false when neither exists.
func (m *Map) iconFilePath(key string) (string, bool) {
	if p := m.customIconPath(key); fileExists(p) {
		return p, true
	}
	if p := builtinIconPath(key); fileExists(p) {
		return p, true
	}
	return "", false
}

// downloadName flattens a key into a filename that still says which key the
// image belongs to, so a folder of downloaded icons stays sortable.
func downloadName(key string) string {
	return strings.ReplaceAll(key, "/", "_") + ".png"
}

// iconSource reports where the image for key would be served from.
func (m *Map) iconSource(key string) IconSource {
	if fileExists(m.customIconPath(key)) {
		return IconCustom
	}
	if fileExists(builtinIconPath(key)) {
		return IconBuiltin
	}
	return IconMissing
}

// markerImageCounts tallies how many markers use each image key.
func (m *Map) markerImageCounts() map[string]int {
	counts := map[string]int{}
	m.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("markers"))
		if b == nil {
			return nil
		}
		grid := b.Bucket([]byte("grid"))
		if grid == nil {
			return nil
		}
		return grid.ForEach(func(k, v []byte) error {
			mk := Marker{}
			if json.Unmarshal(v, &mk) != nil || mk.Image == "" {
				return nil
			}
			counts[mk.Image]++
			return nil
		})
	})
	return counts
}

// uploadedIconKeys lists overrides already on disk, so icons uploaded for a key
// no marker currently uses can still be seen and removed.
func (m *Map) uploadedIconKeys() []string {
	root := filepath.Join(m.gridStorage, iconsSubdir)
	keys := []string{}
	filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(p, ".png") {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return nil
		}
		key := strings.TrimSuffix(filepath.ToSlash(rel), ".png")
		if iconKeyOK(key) {
			keys = append(keys, key)
		}
		return nil
	})
	return keys
}

// iconEntries is everything the admin page lists: every image key markers refer
// to, plus any override on disk that nothing currently uses.
func (m *Map) iconEntries() []IconEntry {
	counts := m.markerImageCounts()
	seen := map[string]bool{}
	entries := []IconEntry{}

	add := func(key string, n int) {
		if seen[key] || !iconKeyOK(key) {
			return
		}
		seen[key] = true
		entries = append(entries, IconEntry{Key: key, Source: m.iconSource(key), Markers: n})
	}
	for key, n := range counts {
		add(key, n)
	}
	for _, key := range m.uploadedIconKeys() {
		add(key, counts[key])
	}

	sortEntries(entries, listingRank)
	return entries
}

// listingRank puts what needs attention first: images with no icon at all, then
// uploaded overrides, then the ones the build already covers.
var listingRank = map[IconSource]int{IconMissing: 0, IconCustom: 1, IconBuiltin: 2}

func sortEntries(entries []IconEntry, rank map[IconSource]int) {
	sort.Slice(entries, func(i, j int) bool {
		if rank[entries[i].Source] != rank[entries[j].Source] {
			return rank[entries[i].Source] < rank[entries[j].Source]
		}
		return entries[i].Key < entries[j].Key
	})
}

func (m *Map) adminIcons(rw http.ResponseWriter, req *http.Request) {
	s := m.getSession(req)
	if s == nil || !s.Auths.Has(AUTH_ADMIN) {
		http.Redirect(rw, req, "/", 302)
		return
	}

	entries := m.iconEntries()
	missing := 0
	for _, e := range entries {
		if e.Missing() {
			missing++
		}
	}

	m.ExecuteTemplate(rw, filepath.FromSlash("admin/icons.tmpl"), struct {
		Page    Page
		Session *Session
		Icons   []IconEntry
		Missing int
		Error   string
	}{
		Page:    m.getPage(req),
		Session: s,
		Icons:   entries,
		Missing: missing,
		Error:   req.FormValue("error"),
	})
}

// decodeIcon reads an uploaded file as a PNG. The upload is decoded rather than
// trusted by extension, and re-encoded on the way out, so only bytes this
// server produced are ever served back to a browser.
func decodeIcon(r io.Reader) (image.Image, error) {
	img, err := png.Decode(io.LimitReader(r, maxIconBytes+1))
	if err != nil {
		return nil, fmt.Errorf("not a readable PNG: %w", err)
	}
	b := img.Bounds()
	if b.Dx() > maxIconPixels || b.Dy() > maxIconPixels {
		return nil, fmt.Errorf("image is %dx%d, larger than %dpx per side", b.Dx(), b.Dy(), maxIconPixels)
	}
	if b.Dx() == 0 || b.Dy() == 0 {
		return nil, fmt.Errorf("image has no pixels")
	}
	return img, nil
}

func (m *Map) uploadIcon(rw http.ResponseWriter, req *http.Request) {
	if !requirePOST(rw, req) {
		return
	}
	s := m.getSession(req)
	if s == nil || !s.Auths.Has(AUTH_ADMIN) {
		http.Redirect(rw, req, "/", 302)
		return
	}

	fail := func(msg string) {
		log.Printf("icon upload by %q failed: %s", s.Username, msg)
		http.Redirect(rw, req, "/admin/icons?error="+url.QueryEscape(msg), 302)
	}

	if err := req.ParseMultipartForm(2 * maxIconBytes); err != nil {
		fail("could not read the upload")
		return
	}
	key := strings.TrimSuffix(strings.TrimSpace(req.FormValue("key")), ".png")
	if !iconKeyOK(key) {
		fail(fmt.Sprintf("%q is not a valid icon key", key))
		return
	}

	file, _, err := req.FormFile("file")
	if err != nil {
		fail("no file was attached")
		return
	}
	defer file.Close()

	img, err := decodeIcon(file)
	if err != nil {
		fail(err.Error())
		return
	}

	dest := m.customIconPath(key)
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		fail("could not create the icon directory")
		return
	}
	// Write beside the target and rename, so a failure part-way through cannot
	// leave a half-written icon in place of a working one.
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".upload-*")
	if err != nil {
		fail("could not create a temporary file")
		return
	}
	if err := png.Encode(tmp, img); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		fail("could not write the image")
		return
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		fail("could not finish writing the image")
		return
	}
	if err := os.Rename(tmp.Name(), dest); err != nil {
		os.Remove(tmp.Name())
		fail("could not save the icon")
		return
	}

	log.Printf("icon %q uploaded by %q", key, s.Username)
	http.Redirect(rw, req, "/admin/icons", 302)
}

// downloadIcon hands back the image currently in use for one key, so it can be
// edited and uploaded again.
func (m *Map) downloadIcon(rw http.ResponseWriter, req *http.Request) {
	s := m.getSession(req)
	if s == nil || !s.Auths.Has(AUTH_ADMIN) {
		http.Redirect(rw, req, "/", 302)
		return
	}

	key := strings.TrimSpace(req.FormValue("key"))
	if !iconKeyOK(key) {
		http.Error(rw, "invalid icon key", http.StatusBadRequest)
		return
	}
	src, ok := m.iconFilePath(key)
	if !ok {
		http.Error(rw, "no icon for that key", http.StatusNotFound)
		return
	}

	rw.Header().Set("Content-Type", "image/png")
	rw.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", downloadName(key)))
	http.ServeFile(rw, req, src)
}

// exportIcons zips every icon in use, laid out by key, so a batch can be edited
// without downloading them one at a time. Keys with no icon are skipped —
// there is nothing to send for those.
func (m *Map) exportIcons(rw http.ResponseWriter, req *http.Request) {
	s := m.getSession(req)
	if s == nil || !s.Auths.Has(AUTH_ADMIN) {
		http.Redirect(rw, req, "/", 302)
		return
	}

	rw.Header().Set("Content-Type", "application/zip")
	rw.Header().Set("Content-Disposition", `attachment; filename="marker-icons.zip"`)

	zw := zip.NewWriter(rw)
	defer zw.Close()

	for _, e := range m.iconEntries() {
		src, ok := m.iconFilePath(e.Key)
		if !ok {
			continue
		}
		f, err := os.Open(src)
		if err != nil {
			log.Printf("icon export: %s: %v", e.Key, err)
			continue
		}
		// The archive mirrors the keys, so an edited file can be matched back
		// to the marker image it belongs to.
		w, err := zw.Create(e.Key + ".png")
		if err != nil {
			f.Close()
			log.Printf("icon export: %s: %v", e.Key, err)
			return
		}
		if _, err := io.Copy(w, f); err != nil {
			log.Printf("icon export: %s: %v", e.Key, err)
		}
		f.Close()
	}
}

func (m *Map) deleteIcon(rw http.ResponseWriter, req *http.Request) {
	if !requirePOST(rw, req) {
		return
	}
	s := m.getSession(req)
	if s == nil || !s.Auths.Has(AUTH_ADMIN) {
		http.Redirect(rw, req, "/", 302)
		return
	}

	key := strings.TrimSpace(req.FormValue("key"))
	if !iconKeyOK(key) {
		http.Redirect(rw, req, "/admin/icons?error="+url.QueryEscape("invalid icon key"), 302)
		return
	}
	if err := os.Remove(m.customIconPath(key)); err != nil && !os.IsNotExist(err) {
		log.Printf("icon %q could not be removed: %v", key, err)
		http.Redirect(rw, req, "/admin/icons?error="+url.QueryEscape("could not remove the icon"), 302)
		return
	}
	log.Printf("icon %q removed by %q", key, s.Username)
	http.Redirect(rw, req, "/admin/icons", 302)
}

var frontendFiles = http.StripPrefix("/map", http.FileServer(http.Dir(frontendSubdir)))

// gfxAsset serves marker artwork, preferring an uploaded override over the
// image shipped with the frontend. Anything that is not a valid icon request
// falls through to the static files unchanged.
func (m *Map) gfxAsset(rw http.ResponseWriter, req *http.Request) {
	rel := strings.TrimPrefix(path.Clean(req.URL.Path), "/map/")
	if key := strings.TrimSuffix(rel, ".png"); strings.HasSuffix(rel, ".png") && iconKeyOK(key) {
		if custom := m.customIconPath(key); fileExists(custom) {
			// Overrides can be replaced from the admin page, so they must not
			// sit in a browser cache afterwards.
			rw.Header().Set("Cache-Control", "no-cache")
			http.ServeFile(rw, req, custom)
			return
		}
	}
	frontendFiles.ServeHTTP(rw, req)
}
