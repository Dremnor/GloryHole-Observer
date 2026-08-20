package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"go.etcd.io/bbolt"
)

type Config struct {
	Title string   `json:"title"`
	Auths []string `json:"auths"`
}

func (m *Map) getChars(rw http.ResponseWriter, req *http.Request) {
	s := m.getSession(req)
	if s == nil || !s.Auths.Has(AUTH_MAP) {
		rw.WriteHeader(http.StatusUnauthorized)
		return
	}
	if !s.Auths.Has(AUTH_POINTER) {
		json.NewEncoder(rw).Encode([]interface{}{})
		return
	}
	groups := groupArr(s.Auths)
	chars := []Character{}
	m.chmu.RLock()
	defer m.chmu.RUnlock()
	for _, v := range m.characters {
		b := hasCommonElement(groups, v.group)
		if b {
			chars = append(chars, v)
		}
		//log.Printf("Auth: %v, User: %v, Char: %v, %t\n", s.Auths, groups, v.group, b)
	}
	json.NewEncoder(rw).Encode(chars)
}

func (m *Map) getMarkers(rw http.ResponseWriter, req *http.Request) {
	s := m.getSession(req)
	if s == nil || !s.Auths.Has(AUTH_MAP) {
		rw.WriteHeader(http.StatusUnauthorized)
		return
	}
	if !s.Auths.Has(AUTH_MARKERS) {
		json.NewEncoder(rw).Encode([]interface{}{})
		return
	}
	markers := []FrontendMarker{}
	unplaced, firstUnplaced := 0, ""
	m.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("markers"))
		if b == nil {
			return nil
		}
		grid := b.Bucket([]byte("grid"))
		if grid == nil {
			return nil
		}
		grids := tx.Bucket([]byte("grids"))
		if grids == nil {
			return nil
		}
		return grid.ForEach(func(k, v []byte) error {
			m := Marker{}
			json.Unmarshal(v, &m)
			graw := grids.Get([]byte(m.GridID))
			if graw == nil {
				// The marker's position is relative to its grid, so without the
				// grid there is nowhere to put it. Dropping it silently is how a
				// marker type can be counted in the admin icon list and be
				// nowhere on the map, with nothing anywhere saying why.
				unplaced++
				if firstUnplaced == "" {
					firstUnplaced = m.Image
				}
				return nil
			}
			g := GridData{}
			json.Unmarshal(graw, &g)
			markers = append(markers, FrontendMarker{
				Image:    m.Image,
				Hidden:   m.Hidden,
				ID:       m.ID,
				Name:     m.Name,
				Map:      g.Map,
				ShowName: m.ShowName,
				Position: Position{
					X: m.Position.X + g.Coord.X*100,
					Y: m.Position.Y + g.Coord.Y*100,
				},
			})
			return nil
		})
	})
	if unplaced > 0 && markerLog.allow("unplaced") {
		log.Printf("markers: %d of %d not sent, stored against a grid this server has no record of (first: %s)",
			unplaced, unplaced+len(markers), firstUnplaced)
	}
	json.NewEncoder(rw).Encode(markers)
}

func (m *Map) getMaps(rw http.ResponseWriter, req *http.Request) {
	s := m.getSession(req)
	if s == nil || !s.Auths.Has(AUTH_MAP) {
		rw.WriteHeader(http.StatusUnauthorized)
		return
	}
	maps := map[int]*MapInfo{}
	m.db.View(func(tx *bbolt.Tx) error {
		mapB := tx.Bucket([]byte("maps"))
		if mapB == nil {
			return nil
		}
		return mapB.ForEach(func(k, v []byte) error {
			mapid, err := strconv.Atoi(string(k))
			if err != nil {
				return nil
			}
			mi := &MapInfo{}
			json.Unmarshal(v, &mi)
			if mi.Hidden {
				return nil
			}
			maps[mapid] = mi
			return nil
		})
	})
	json.NewEncoder(rw).Encode(maps)
}

func (m *Map) config(rw http.ResponseWriter, req *http.Request) {
	s := m.getSession(req)
	if s == nil || !s.Auths.Has(AUTH_MAP) {
		rw.WriteHeader(http.StatusUnauthorized)
		return
	}
	config := Config{
		Auths: s.Auths,
	}
	m.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("config"))
		if b == nil {
			return nil
		}
		title := b.Get([]byte("title"))
		config.Title = string(title)
		return nil
	})
	json.NewEncoder(rw).Encode(config)
}

func groupArr(a Auths) []int {
	groups := []int{}
	if a.Has(AUTH_GROUP1) {
		groups = append(groups, 1)
	}
	if a.Has(AUTH_GROUP2) {
		groups = append(groups, 2)
	}
	if a.Has(AUTH_GROUP3) {
		groups = append(groups, 3)
	}
	if a.Has(AUTH_GROUP4) {
		groups = append(groups, 4)
	}
	if a.Has(AUTH_GROUP5) {
		groups = append(groups, 5)
	}

	return groups
}

func hasCommonElement(arr1, arr2 []int) bool {
	if len(arr2) == 0 {
		return true
	}
	if len(arr1) == 0 {
		return false
	}
	for _, num1 := range arr1 {
		for _, num2 := range arr2 {
			if num1 == num2 {
				return true
			}
		}
	}

	return false
}
