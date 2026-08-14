package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/image/draw"

	"go.etcd.io/bbolt"
)

var clientPath = regexp.MustCompile("client/([^/]+)/(.*)")

// validGridID limits grid IDs to characters that are safe to embed in a file
// path. Grid IDs arrive straight from the client and end up in the on-disk
// tile name, so anything outside this set is rejected.
var validGridID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

type ctxKey int

const userCtxKey ctxKey = iota

// userFrom returns the username the upload token resolved to, for logging.
func userFrom(ctx context.Context) string {
	if u, ok := ctx.Value(userCtxKey).(string); ok && u != "" {
		return u
	}
	return "unknown"
}

const VERSION = "4"

func (m *Map) client(rw http.ResponseWriter, req *http.Request) {
	matches := clientPath.FindStringSubmatch(req.URL.Path)
	if matches == nil {
		http.Error(rw, "Client token not found", http.StatusBadRequest)
		return
	}
	auth := false
	user := ""
	u := User{}
	m.db.View(func(tx *bbolt.Tx) error {
		tb := tx.Bucket([]byte("tokens"))
		if tb == nil {
			return nil
		}
		userName := tb.Get([]byte(matches[1]))
		if userName == nil {
			return nil
		}
		ub := tx.Bucket([]byte("users"))
		if ub == nil {
			return nil
		}
		userRaw := ub.Get(userName)
		if userRaw == nil {
			return nil
		}
		//u = User{}
		json.Unmarshal(userRaw, &u)
		if u.Auths.Has(AUTH_UPLOAD) {
			user = string(userName)
			auth = true
		}
		return nil
	})
	if !auth {
		rw.WriteHeader(http.StatusUnauthorized)
		return
	}

	ctx := context.WithValue(req.Context(), userCtxKey, user)
	req = req.WithContext(ctx)

	switch matches[2] {
	case "locate":
		m.locate(rw, req)
	case "gridUpdate":
		m.gridUpdate(rw, req)
	case "gridUpload":
		m.gridUpload(rw, req)
	case "positionUpdate":
		m.updatePositions(rw, req, u)
	case "markerUpdate":
		m.uploadMarkers(rw, req)
	/*case "mapData":
	m.mapdataIndex(rw, req)*/
	case "":
		http.Redirect(rw, req, "/map/", 302)
	case "checkVersion":
		if req.FormValue("version") == VERSION {
			rw.WriteHeader(200)
		} else {
			rw.WriteHeader(http.StatusBadRequest)
		}
	default:
		rw.WriteHeader(http.StatusNotFound)
	}
}

func (m *Map) updatePositions(rw http.ResponseWriter, req *http.Request, u User) {
	defer req.Body.Close()
	craws := map[string]struct {
		Name   string
		GridID string
		Coords struct {
			X, Y int
		}
		Type string
	}{}
	buf, err := ioutil.ReadAll(req.Body)
	if err != nil {
		log.Println("Error reading position update json: ", err)
		return
	}
	err = json.Unmarshal(buf, &craws)
	if err != nil {
		log.Println("Error decoding position update json: ", err)
		log.Println("Original json: ", string(buf))
		return
	}
	groups := groupArr(u.Auths)
	m.db.View(func(tx *bbolt.Tx) error {
		grids := tx.Bucket([]byte("grids"))
		if grids == nil {
			return nil
		}
		m.chmu.Lock()
		defer m.chmu.Unlock()
		for id, craw := range craws {
			grid := grids.Get([]byte(craw.GridID))
			if grid == nil {
				// Unknown grid: skip this character but keep processing the
				// rest of the batch.
				continue
			}
			gd := GridData{}
			json.Unmarshal(grid, &gd)
			idnum, _ := strconv.Atoi(id)
			c := Character{
				Name: craw.Name,
				ID:   idnum,
				Map:  gd.Map,
				Position: Position{
					X: craw.Coords.X + (gd.Coord.X * 100),
					Y: craw.Coords.Y + (gd.Coord.Y * 100),
				},
				Type:    craw.Type,
				updated: time.Now(),
				group:   groups,
			}
			old, ok := m.characters[id]
			if !ok {
				m.characters[id] = c
			} else {
				if old.Type == "player" {
					if c.Type == "player" {
						m.characters[id] = c
					} else {
						old.Position = c.Position
						old.group = c.group
						m.characters[id] = old
					}
				} else if old.Type != "unknown" {
					if c.Type != "unknown" {
						m.characters[id] = c
					} else {
						old.Position = c.Position
						old.group = c.group
						m.characters[id] = old
					}
				} else {
					m.characters[id] = c
				}
			}
		}
		return nil
	})

}

func (m *Map) uploadMarkers(rw http.ResponseWriter, req *http.Request) {
	defer req.Body.Close()
	markers := []struct {
		Name   string
		GridID string
		X, Y   int
		Image  string
		Type   string
		Color  string
	}{}
	buf, err := ioutil.ReadAll(req.Body)
	if err != nil {
		log.Println("Error reading marker json: ", err)
		return
	}
	err = json.Unmarshal(buf, &markers)
	if err != nil {
		log.Println("Error decoding marker json: ", err)
		log.Println("Original json: ", string(buf))
		return
	}
	err = m.db.Update(func(tx *bbolt.Tx) error {
		mb, err := tx.CreateBucketIfNotExists([]byte("markers"))
		if err != nil {
			return err
		}
		grid, err := mb.CreateBucketIfNotExists([]byte("grid"))
		if err != nil {
			return err
		}
		idB, err := mb.CreateBucketIfNotExists([]byte("id"))
		if err != nil {
			return err
		}

		for _, mraw := range markers {
			if !validGridID.MatchString(mraw.GridID) {
				log.Printf("markerUpdate from %q: skipping marker with invalid grid id %q",
					userFrom(req.Context()), mraw.GridID)
				continue
			}
			key := []byte(fmt.Sprintf("%s_%d_%d", mraw.GridID, mraw.X, mraw.Y))
			if grid.Get(key) != nil {
				continue
			}
			if mraw.Image == "" {
				mraw.Image = "gfx/terobjs/mm/custom"
			}
			id, err := idB.NextSequence()
			if err != nil {
				return err
			}
			idKey := []byte(strconv.Itoa(int(id)))
			m := Marker{
				Name:   mraw.Name,
				ID:     int(id),
				GridID: mraw.GridID,
				Position: Position{
					X: mraw.X,
					Y: mraw.Y,
				},
				Image: mraw.Image,
			}
			raw, _ := json.Marshal(m)
			grid.Put(key, raw)
			idB.Put(idKey, key)
		}
		return nil
	})
	if err != nil {
		log.Println("Error update db: ", err)
		return
	}
}

func (m *Map) locate(rw http.ResponseWriter, req *http.Request) {
	grid := req.FormValue("gridID")
	err := m.db.View(func(tx *bbolt.Tx) error {
		grids := tx.Bucket([]byte("grids"))
		if grids == nil {
			return nil
		}
		curRaw := grids.Get([]byte(grid))
		cur := GridData{}
		if curRaw == nil {
			return fmt.Errorf("grid not found")
		}
		err := json.Unmarshal(curRaw, &cur)
		if err != nil {
			return err
		}
		fmt.Fprintf(rw, "%d;%d;%d", cur.Map, cur.Coord.X, cur.Coord.Y)
		return nil
	})
	if err != nil {
		rw.WriteHeader(404)
	}
}

type GridUpdate struct {
	Grids [][]string `json:"grids"`
}

type GridRequest struct {
	GridRequests []string `json:"gridRequests"`
	Map          int      `json:"map"`
	Coords       Coord    `json:"coords"`
}

type gridOffset struct{ X, Y int }

// mapMatch records how an already known map lines up with the grid window a
// client just reported: the offset its grids imply, how many grids in the
// window agree on that offset, and whether any grid contradicted it.
type mapMatch struct {
	off      gridOffset
	count    int
	conflict bool
}

// errInconsistentGrids aborts a grid update whose own anchor map reports
// contradictory offsets. Writing anything based on such a window would corrupt
// the map layout, so the whole transaction is rolled back.
var errInconsistentGrids = errors.New("grid window reports inconsistent offsets")

// buildMatches maps each already-known map appearing in the client's grid
// window to the offset those grids imply. lookup returns nil for unknown grids.
func buildMatches(gridRows [][]string, lookup func(string) *GridData) map[int]*mapMatch {
	matches := map[int]*mapMatch{}
	for x, row := range gridRows {
		for y, grid := range row {
			gd := lookup(grid)
			if gd == nil {
				continue
			}
			off := gridOffset{X: gd.Coord.X - x, Y: gd.Coord.Y - y}
			mm, ok := matches[gd.Map]
			if !ok {
				matches[gd.Map] = &mapMatch{off: off, count: 1}
				continue
			}
			if mm.off != off {
				// Two grids of the same map disagree about where the client is.
				// The window cannot be trusted for this map.
				mm.conflict = true
				continue
			}
			mm.count++
		}
	}
	return matches
}

type mergeDecision struct {
	mapID   int
	off     gridOffset
	allowed bool
	reason  string
}

// planMerges decides which of the other maps in the window may be folded into
// the anchor map. A merge rewrites the coordinates of every grid in the merged
// map and cannot be undone, so it requires minOverlap grids agreeing on a
// single offset. Decisions are returned in map-ID order so the outcome does not
// depend on Go's map iteration order.
func planMerges(matches map[int]*mapMatch, anchorID, minOverlap int) []mergeDecision {
	if minOverlap < 1 {
		minOverlap = 1
	}
	ids := make([]int, 0, len(matches))
	for id := range matches {
		if id != anchorID {
			ids = append(ids, id)
		}
	}
	sort.Ints(ids)

	decisions := make([]mergeDecision, 0, len(ids))
	for _, id := range ids {
		mm := matches[id]
		d := mergeDecision{mapID: id, off: mm.off}
		switch {
		case mm.conflict:
			d.reason = "its grids report inconsistent offsets"
		case mm.count < minOverlap:
			d.reason = fmt.Sprintf("only %d overlapping grid(s), need %d", mm.count, minOverlap)
		default:
			d.allowed = true
		}
		decisions = append(decisions, d)
	}
	return decisions
}

func (m *Map) gridUpdate(rw http.ResponseWriter, req *http.Request) {
	defer req.Body.Close()
	dec := json.NewDecoder(req.Body)
	grup := GridUpdate{}
	err := dec.Decode(&grup)
	if err != nil {
		log.Println("Error decoding grid request json: ", err)
		http.Error(rw, "Error decoding request", http.StatusBadRequest)
		return
	}

	user := userFrom(req.Context())

	if len(grup.Grids) == 0 {
		http.Error(rw, "empty grid set", http.StatusBadRequest)
		return
	}
	for _, row := range grup.Grids {
		for _, grid := range row {
			if !validGridID.MatchString(grid) {
				log.Printf("gridUpdate from %q: rejected, invalid grid id %q", user, grid)
				http.Error(rw, "invalid grid id", http.StatusBadRequest)
				return
			}
		}
	}
	log.Printf("gridUpdate from %q: %v", user, grup)

	ops := []struct {
		mapid int
		x, y  int
		f     string
	}{}

	greq := GridRequest{}

	err = m.db.Update(func(tx *bbolt.Tx) error {
		grids, err := tx.CreateBucketIfNotExists([]byte("grids"))
		if err != nil {
			return err
		}
		tiles, err := tx.CreateBucketIfNotExists([]byte("tiles"))
		if err != nil {
			return err
		}

		mapB, err := tx.CreateBucketIfNotExists([]byte("maps"))
		if err != nil {
			return err
		}

		configb, err := tx.CreateBucketIfNotExists([]byte("config"))
		if err != nil {
			return err
		}

		matches := buildMatches(grup.Grids, func(grid string) *GridData {
			gridRaw := grids.Get([]byte(grid))
			if gridRaw == nil {
				return nil
			}
			gd := &GridData{}
			if err := json.Unmarshal(gridRaw, gd); err != nil {
				return nil
			}
			return gd
		})

		if len(matches) == 0 {
			seq, err := mapB.NextSequence()
			if err != nil {
				return err
			}
			mi := MapInfo{
				ID:     int(seq),
				Name:   strconv.Itoa(int(seq)),
				Hidden: configb.Get([]byte("defaultHide")) != nil,
			}
			raw, _ := json.Marshal(mi)
			err = mapB.Put([]byte(strconv.Itoa(int(seq))), raw)
			if err != nil {
				return err
			}
			log.Println("Client made mapid ", seq)
			for x, row := range grup.Grids {
				for y, grid := range row {

					cur := GridData{}
					cur.ID = grid
					cur.Map = int(seq)
					cur.Coord.X = x - 1
					cur.Coord.Y = y - 1

					raw, err := json.Marshal(cur)
					if err != nil {
						return err
					}
					grids.Put([]byte(grid), raw)
					greq.GridRequests = append(greq.GridRequests, grid)
				}
			}
			greq.Coords = Coord{0, 0}
			return nil
		}

		mapid := -1
		var anchor *mapMatch
		for id, mm := range matches {
			mi := MapInfo{}
			mraw := mapB.Get([]byte(strconv.Itoa(id)))
			if mraw != nil {
				json.Unmarshal(mraw, &mi)
			}
			if mi.Priority {
				mapid = id
				anchor = mm
				break
			}
			if mapid == -1 || id < mapid {
				mapid = id
				anchor = mm
			}
		}

		// If the map the client is standing on cannot agree with itself about
		// where the client is, every coordinate derived from this window would
		// be wrong. Roll back rather than write a corrupted layout.
		if anchor.conflict {
			log.Printf("gridUpdate from %q: rejected, map %d reports inconsistent offsets", user, mapid)
			return errInconsistentGrids
		}
		offset := anchor.off

		mergeable := map[int]gridOffset{}
		for _, d := range planMerges(matches, mapid, *mergeMinOverlap) {
			if !d.allowed {
				log.Printf("gridUpdate from %q: refusing to merge map %d into %d: %s",
					user, d.mapID, mapid, d.reason)
				continue
			}
			mergeable[d.mapID] = d.off
		}

		log.Println("Client in mapid ", mapid)

		for x, row := range grup.Grids {
			for y, grid := range row {
				cur := GridData{}
				if curRaw := grids.Get([]byte(grid)); curRaw != nil {
					json.Unmarshal(curRaw, &cur)
					if time.Now().After(cur.NextUpdate) {
						greq.GridRequests = append(greq.GridRequests, grid)
					}
					continue
				}

				cur.ID = grid
				cur.Map = mapid
				cur.Coord.X = x + offset.X
				cur.Coord.Y = y + offset.Y
				raw, err := json.Marshal(cur)
				if err != nil {
					return err
				}
				grids.Put([]byte(grid), raw)
				greq.GridRequests = append(greq.GridRequests, grid)
			}
		}
		// The client reports a window centred on its own position; the centre
		// grid is what it wants coordinates for. Short windows are tolerated.
		if len(grup.Grids) > 1 && len(grup.Grids[1]) > 1 {
			if curRaw := grids.Get([]byte(grup.Grids[1][1])); curRaw != nil {
				cur := GridData{}
				json.Unmarshal(curRaw, &cur)
				greq.Map = cur.Map
				greq.Coords = cur.Coord
			}
		}
		if len(mergeable) > 0 {
			grids.ForEach(func(k, v []byte) error {
				gd := GridData{}
				json.Unmarshal(v, &gd)
				if gd.Map == mapid {
					return nil
				}
				if merge, ok := mergeable[gd.Map]; ok {
					var td *TileData
					mapb, err := tiles.CreateBucketIfNotExists([]byte(strconv.Itoa(gd.Map)))
					if err != nil {
						return err
					}
					zoom, err := mapb.CreateBucketIfNotExists([]byte(strconv.Itoa(0)))
					if err != nil {
						return err
					}
					tileraw := zoom.Get([]byte(gd.Coord.Name()))
					if tileraw != nil {
						json.Unmarshal(tileraw, &td)
					}

					gd.Map = mapid
					gd.Coord.X += offset.X - merge.X
					gd.Coord.Y += offset.Y - merge.Y
					raw, _ := json.Marshal(gd)
					if td != nil {
						ops = append(ops, struct {
							mapid int
							x     int
							y     int
							f     string
						}{
							mapid: mapid,
							x:     gd.Coord.X,
							y:     gd.Coord.Y,
							f:     td.File,
						})
					}
					grids.Put(k, raw)
				}
				return nil
			})
		}
		for mergeid, merge := range mergeable {
			mapB.Delete([]byte(strconv.Itoa(mergeid)))
			log.Printf("Merging map %d into %d (requested by %q)", mergeid, mapid, user)
			m.reportMerge(mergeid, mapid, Coord{X: offset.X - merge.X, Y: offset.Y - merge.Y})
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, errInconsistentGrids) {
			http.Error(rw, "inconsistent grid data", http.StatusConflict)
			return
		}
		log.Println(err)
		return
	}
	needProcess := map[zoomproc]struct{}{}
	for _, op := range ops {
		m.SaveTile(op.mapid, Coord{X: op.x, Y: op.y}, 0, op.f, time.Now().UnixNano())
		needProcess[zoomproc{c: Coord{X: op.x, Y: op.y}.Parent(), m: op.mapid}] = struct{}{}
	}
	for z := 1; z <= 6; z++ {
		process := needProcess
		needProcess = map[zoomproc]struct{}{}
		for p := range process {
			m.updateZoomLevel(p.m, p.c, z)
			needProcess[zoomproc{p.c.Parent(), p.m}] = struct{}{}
		}
	}
	log.Println(greq)
	json.NewEncoder(rw).Encode(greq)
}

/*
func (m *Map) mapdataIndex(rw http.ResponseWriter, req *http.Request) {
	err := m.db.View(func(tx *bbolt.Tx) error {
		grids := tx.Bucket([]byte("grids"))
		if grids == nil {
			return fmt.Errorf("grid not found")
		}
		return grids.ForEach(func(k, v []byte) error {
			cur := GridData{}
			err := json.Unmarshal(v, &cur)
			if err != nil {
				return err
			}
			fmt.Fprintf(rw, "%s,%d,%d,%d\n", cur.ID, cur.Map, cur.Coord.X, cur.Coord.Y)
			return nil
		})
	})
	if err != nil {
		rw.WriteHeader(404)
	}
}
*/

type ExtraData struct {
	Season int
}

func (m *Map) gridUpload(rw http.ResponseWriter, req *http.Request) {
	if strings.Count(req.Header.Get("Content-Type"), "=") >= 2 && strings.Count(req.Header.Get("Content-Type"), "\"") == 0 {
		parts := strings.SplitN(req.Header.Get("Content-Type"), "=", 2)
		req.Header.Set("Content-Type", parts[0]+"=\""+parts[1]+"\"")
	}

	err := req.ParseMultipartForm(100000000)
	if err != nil {
		log.Println(err)
		return
	}

	id := req.FormValue("id")
	// The grid ID becomes part of the tile's path on disk. gridUpdate already
	// rejects unsafe IDs, but this path is reachable on its own so it revalidates.
	if !validGridID.MatchString(id) {
		log.Printf("gridUpload from %q: rejected, invalid grid id %q", userFrom(req.Context()), id)
		http.Error(rw, "invalid grid id", http.StatusBadRequest)
		return
	}

	extraData := req.FormValue("extraData")
	if extraData != "" {
		ed := ExtraData{}
		json.Unmarshal([]byte(extraData), &ed)
		if ed.Season == 3 {
			needTile := false
			m.db.Update(func(tx *bbolt.Tx) error {
				b, err := tx.CreateBucketIfNotExists([]byte("grids"))
				if err != nil {
					return err
				}
				curRaw := b.Get([]byte(id))
				if curRaw == nil {
					return fmt.Errorf("Unknown grid id: %s", id)
				}
				cur := GridData{}
				err = json.Unmarshal(curRaw, &cur)
				if err != nil {
					return err
				}

				tiles, err := tx.CreateBucketIfNotExists([]byte("tiles"))
				if err != nil {
					return err
				}
				maps, err := tiles.CreateBucketIfNotExists([]byte(strconv.Itoa(cur.Map)))
				if err != nil {
					return err
				}
				zooms, err := maps.CreateBucketIfNotExists([]byte("0"))
				if err != nil {
					return err
				}

				tdRaw := zooms.Get([]byte(cur.Coord.Name()))
				if tdRaw == nil {
					needTile = true
					return nil
				}
				td := TileData{}
				err = json.Unmarshal(tdRaw, &td)
				if err != nil {
					return err
				}
				if td.File == "" {
					needTile = true
					return nil
				}

				if time.Now().After(cur.NextUpdate) {
					cur.NextUpdate = time.Now().Add(time.Minute * 30)
				}

				raw, err := json.Marshal(cur)
				if err != nil {
					return err
				}
				b.Put([]byte(id), raw)

				return nil
			})
			if !needTile {
				log.Println("ignoring tile upload: winter")
				return
			} else {
				log.Println("Missing tile, using winter version")
			}
		}
	}

	file, _, err := req.FormFile("file")
	if err != nil {
		log.Println(err)
		return
	}

	log.Printf("gridUpload from %q: tile for grid %s", userFrom(req.Context()), id)

	updateTile := false
	cur := GridData{}

	mapid := 0

	m.db.Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("grids"))
		if err != nil {
			return err
		}
		curRaw := b.Get([]byte(id))
		if curRaw == nil {
			return fmt.Errorf("Unknown grid id: %s", id)
		}
		err = json.Unmarshal(curRaw, &cur)
		if err != nil {
			return err
		}

		updateTile = time.Now().After(cur.NextUpdate)
		mapid = cur.Map

		if updateTile {
			cur.NextUpdate = time.Now().Add(time.Minute * 30)
		}

		raw, err := json.Marshal(cur)
		if err != nil {
			return err
		}
		b.Put([]byte(id), raw)

		return nil
	})

	if updateTile {
		// 0755, not 0600: without the execute bit nothing can descend into the
		// directory to read the tiles back out.
		if err := os.MkdirAll(fmt.Sprintf("%s/grids", m.gridStorage), 0755); err != nil {
			log.Println("gridUpload: mkdir:", err)
			return
		}
		f, err := os.Create(fmt.Sprintf("%s/grids/%s.png", m.gridStorage, cur.ID))
		if err != nil {
			log.Println("gridUpload: create:", err)
			return
		}
		_, err = io.Copy(f, file)
		if err != nil {
			f.Close()
			return
		}
		f.Close()

		m.SaveTile(mapid, cur.Coord, 0, fmt.Sprintf("grids/%s.png", cur.ID), time.Now().UnixNano())

		c := cur.Coord
		for z := 1; z <= 6; z++ {
			c = c.Parent()
			m.updateZoomLevel(mapid, c, z)
		}
	}
}

func (m *Map) updateZoomLevel(mapid int, c Coord, z int) {
	img := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	draw.Draw(img, img.Bounds(), image.Transparent, image.Point{}, draw.Src)
	for x := 0; x <= 1; x++ {
		for y := 0; y <= 1; y++ {
			subC := c
			subC.X *= 2
			subC.Y *= 2
			subC.X += x
			subC.Y += y
			td := m.GetTile(mapid, subC, z-1)
			if td == nil || td.File == "" {
				continue
			}
			subf, err := os.Open(filepath.Join(m.gridStorage, td.File))
			if err != nil {
				continue
			}
			subimg, _, err := image.Decode(subf)
			subf.Close()
			if err != nil {
				continue
			}
			draw.BiLinear.Scale(img, image.Rect(50*x, 50*y, 50*x+50, 50*y+50), subimg, subimg.Bounds(), draw.Src, nil)
		}
	}
	if err := os.MkdirAll(fmt.Sprintf("%s/%d/%d", m.gridStorage, mapid, z), 0755); err != nil {
		log.Println("updateZoomLevel: mkdir:", err)
		return
	}
	name := fmt.Sprintf("%d/%d/%s.png", mapid, z, c.Name())
	f, err := os.Create(filepath.Join(m.gridStorage, name))
	if err != nil {
		log.Println("updateZoomLevel: create:", err)
		return
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		log.Println("updateZoomLevel: encode:", err)
		return
	}
	if err := f.Close(); err != nil {
		log.Println("updateZoomLevel: close:", err)
		return
	}
	// Only announce the tile once it is actually on disk.
	m.SaveTile(mapid, c, z, name, time.Now().UnixNano())
}
