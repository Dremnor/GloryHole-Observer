package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/cediner/hnh-map-vuetify/webapp"

	"go.etcd.io/bbolt"
	"golang.org/x/crypto/bcrypt"
)

type Map struct {
	gridStorage string
	db          *bbolt.DB

	characters map[string]Character
	chmu       sync.RWMutex

	*webapp.WebApp

	gridUpdates  topic
	mergeUpdates mergeTopic

	loginLimit *loginLimiter
}

type Session struct {
	ID        string
	Username  string
	Auths     Auths `json:"-"`
	TempAdmin bool
}

var (
	gridStorage = flag.String("grids", "grids", "directory to store grids in")
	port        = flag.Int("port", func() int {
		if port, ok := os.LookupEnv("HNHMAP_PORT"); ok {
			p, err := strconv.Atoi(port)
			if err != nil {
				log.Fatal(err)
			}
			return p
		}
		return 8080
	}(), "Port to listen on")
	// Merging two maps rewrites the coordinates of every grid in the merged map
	// and cannot be undone, so a single stray grid should not trigger one. A
	// client genuinely crossing between two mapped areas reports several
	// overlapping grids within a request or two.
	mergeMinOverlap = flag.Int("merge-min-overlap", 2,
		"how many overlapping grids must agree before two maps are merged automatically (1 restores the old behaviour)")
	trustProxyHeaders = flag.Bool("trust-proxy-headers", false,
		"read the client address from X-Forwarded-For; only enable behind a reverse proxy whose port is not reachable directly")
	loginMaxFailures = flag.Int("login-max-failures", 10,
		"failed logins from one address before it is locked out (0 disables the limit)")
	loginWindow = flag.Duration("login-window", 15*time.Minute,
		"how long a login lockout lasts, extended by each further failure")
)

func faviconHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "public/favicon.ico")
}

func main() {
	flag.Parse()

	db, err := bbolt.Open(*gridStorage+"/grids.db", 0600, nil)
	if err != nil {
		log.Fatal(err)
	}
	m := Map{
		gridStorage: *gridStorage,
		db:          db,

		characters: map[string]Character{},

		WebApp: webapp.Must(webapp.New().LoadTemplates("./templates/")),

		loginLimit: newLoginLimiter(*loginMaxFailures, *loginWindow),
	}

	err = db.Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("config"))
		if err != nil {
			return err
		}
		vraw := b.Get([]byte("version"))
		v, _ := strconv.Atoi(string(vraw))
		if v < len(migrations) {
			for _, f := range migrations[v:] {
				err = f(tx)
				if err != nil {
					return err
				}
			}
		}
		return b.Put([]byte("version"), []byte(strconv.Itoa(len(migrations))))
	})
	if err != nil {
		log.Fatal(err)
	}

	go m.cleanChars()
	go m.loginLimit.cleanup()

	//http.HandleFunc("/favicon.ico", faviconHandler)
	http.Handle("/favicon.ico", http.FileServer(http.Dir("public")))
	// Mapping client endpoints
	http.HandleFunc("/client/", m.client)

	http.HandleFunc("/login", m.login)
	http.HandleFunc("/logout", m.logout)
	http.HandleFunc("/", m.index)
	http.HandleFunc("/generateToken", m.generateToken)
	http.HandleFunc("/password", m.changePassword)

	// Admin endpoints
	http.HandleFunc("/admin/", m.admin)
	http.HandleFunc("/admin/user", m.adminUser)
	http.HandleFunc("/admin/deleteUser", m.deleteUser)
	http.HandleFunc("/admin/wipe", m.wipe)
	http.HandleFunc("/admin/setPrefix", m.setPrefix)
	http.HandleFunc("/admin/setDefaultHide", m.setDefaultHide)
	http.HandleFunc("/admin/setTitle", m.setTitle)
	http.HandleFunc("/admin/rebuildZooms", m.rebuildZooms)
	http.HandleFunc("/admin/export", m.export)
	http.HandleFunc("/admin/backup", m.backup)
	http.HandleFunc("/admin/merge", m.merge)
	http.HandleFunc("/admin/map", m.adminMap)
	http.HandleFunc("/admin/mapic", m.adminICMap)
	http.HandleFunc("/admin/icons", m.adminIcons)
	http.HandleFunc("/admin/icons/upload", m.uploadIcon)
	http.HandleFunc("/admin/icons/delete", m.deleteIcon)
	http.HandleFunc("/admin/icons/download", m.downloadIcon)
	http.HandleFunc("/admin/icons/export", m.exportIcons)

	// Map frontend endpoints
	http.HandleFunc("/map/api/v1/characters", m.getChars)
	http.HandleFunc("/map/api/v1/markers", m.getMarkers)
	http.HandleFunc("/map/api/config", m.config)
	http.HandleFunc("/map/api/admin/wipeTile", m.wipeTile)
	http.HandleFunc("/map/api/admin/setCoords", m.setCoords)
	http.HandleFunc("/map/api/admin/hideMarker", m.hideMarker)
	http.HandleFunc("/map/api/admin/addMarker", m.addMarker)
	http.HandleFunc("/map/updates", m.watchGridUpdates)
	http.HandleFunc("/map/grids/", m.gridTile)
	http.HandleFunc("/map/api/maps", m.getMaps)
	//http.Handle("/map/grids/", http.StripPrefix("/map/grids", http.FileServer(http.Dir(m.gridStorage))))

	// Registered ahead of the static handler below: ServeMux prefers the longer
	// pattern, so marker artwork goes through the icon override first.
	http.HandleFunc("/map/gfx/", m.gfxAsset)

	http.Handle("/map/", http.StripPrefix("/map", http.FileServer(http.Dir("frontend"))))

	http.Handle("/js/", http.FileServer(http.Dir("public")))

	log.Printf("Listening on port %d", *port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), nil))
}

type Character struct {
	Name     string   `json:"name"`
	ID       int      `json:"id"`
	Map      int      `json:"map"`
	Position Position `json:"position"`
	Type     string   `json:"type"`
	group    Group
	updated  time.Time
}

type Marker struct {
	Name     string   `json:"name"`
	ID       int      `json:"id"`
	GridID   string   `json:"gridID"`
	Position Position `json:"position"`
	Image    string   `json:"image"`
	Hidden   bool     `json:"hidden"`
	// Set on markers placed from the map view, where the point of the marker
	// is usually the label rather than the icon.
	ShowName bool `json:"showName"`
}

type FrontendMarker struct {
	Name     string   `json:"name"`
	ID       int      `json:"id"`
	Map      int      `json:"map"`
	Position Position `json:"position"`
	Image    string   `json:"image"`
	Hidden   bool     `json:"hidden"`
	ShowName bool     `json:"showName"`
}

type MapInfo struct {
	ID       int
	Name     string
	Hidden   bool
	Priority bool
}

type GridData struct {
	ID         string
	Coord      Coord
	NextUpdate time.Time
	Map        int
}

type Coord struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type Position struct {
	X int `json:"x"`
	Y int `json:"y"`
}

func (c Coord) Name() string {
	return fmt.Sprintf("%d_%d", c.X, c.Y)
}

func (c Coord) Parent() Coord {
	if c.X < 0 {
		c.X--
	}
	if c.Y < 0 {
		c.Y--
	}
	return Coord{
		X: c.X / 2,
		Y: c.Y / 2,
	}
}

type Auths []string

type Group []int

func (a Auths) Has(auth string) bool {
	for _, v := range a {
		if v == auth {
			return true
		}
	}
	return false
}

const (
	AUTH_ADMIN   = "admin"
	AUTH_WRITER  = "writer"
	AUTH_POINTER = "point"
	AUTH_MAP     = "map"
	AUTH_MARKERS = "markers"
	AUTH_UPLOAD  = "upload"
	AUTH_GROUP1  = "g1"
	AUTH_GROUP2  = "g2"
	AUTH_GROUP3  = "g3"
	AUTH_GROUP4  = "g4"
	AUTH_GROUP5  = "g5"
)

type User struct {
	Pass   []byte
	Auths  Auths
	Tokens []string
}

// requirePOST rejects a state-changing request that did not arrive as a POST.
// Together with the SameSite=Lax session cookie this is what stops cross-site
// requests: Lax withholds the cookie from cross-site POSTs, while requiring
// POST stops a plain link — which Lax still sends the cookie with — from
// triggering a destructive action.
func requirePOST(rw http.ResponseWriter, req *http.Request) bool {
	if req.Method != http.MethodPost {
		http.Error(rw, "method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	return true
}

// requestIsHTTPS reports whether the client reached us over TLS, either
// directly or through a terminating reverse proxy.
func requestIsHTTPS(req *http.Request) bool {
	return req.TLS != nil || req.Header.Get("X-Forwarded-Proto") == "https"
}

func (m *Map) getSession(req *http.Request) *Session {
	c, err := req.Cookie("session")
	if err != nil {
		return nil
	}
	var s *Session
	m.db.View(func(tx *bbolt.Tx) error {
		sessions := tx.Bucket([]byte("sessions"))
		if sessions == nil {
			return nil
		}
		session := sessions.Get([]byte(c.Value))
		if session == nil {
			return nil
		}
		err := json.Unmarshal(session, &s)
		if err != nil {
			return err
		}
		if s.TempAdmin {
			s.Auths = Auths{AUTH_ADMIN}
			return nil
		}
		users := tx.Bucket([]byte("users"))
		if users == nil {
			return nil
		}
		raw := users.Get([]byte(s.Username))
		if raw == nil {
			s = nil
			return nil
		}
		u := User{}
		err = json.Unmarshal(raw, &u)
		if err != nil {
			s = nil
			return err
		}
		s.Auths = u.Auths
		return nil
	})
	return s
}

func (m *Map) deleteSession(s *Session) {
	m.db.Update(func(tx *bbolt.Tx) error {
		sessions, err := tx.CreateBucketIfNotExists([]byte("sessions"))
		if err != nil {
			return err
		}
		return sessions.Delete([]byte(s.ID))
	})
}

func (m *Map) saveSession(s *Session) {
	m.db.Update(func(tx *bbolt.Tx) error {
		sessions, err := tx.CreateBucketIfNotExists([]byte("sessions"))
		if err != nil {
			return err
		}
		buf, err := json.Marshal(s)
		if err != nil {
			return err
		}
		return sessions.Put([]byte(s.ID), buf)
	})
}

type Page struct {
	Title string `json:"title"`
}

func (m *Map) getPage(req *http.Request) Page {
	p := Page{}
	m.db.View(func(tx *bbolt.Tx) error {
		c := tx.Bucket([]byte("config"))
		if c == nil {
			return nil
		}
		p.Title = string(c.Get([]byte("title")))
		return nil
	})
	return p
}

func (m *Map) getUser(user, pass string) (u *User) {
	m.db.View(func(tx *bbolt.Tx) error {
		users := tx.Bucket([]byte("users"))
		if users == nil {
			if user == "admin" && pass == "admin" {
				u = &User{
					Auths: Auths{"admin", "tempadmin"},
				}
			}
			return nil
		}
		raw := users.Get([]byte(user))
		if raw != nil {
			json.Unmarshal(raw, &u)
			if bcrypt.CompareHashAndPassword(u.Pass, []byte(pass)) != nil {
				u = nil
				return nil
			}
		}
		return nil
	})
	return u
}

func (m *Map) cleanChars() {
	for range time.Tick(time.Second * 10) {
		m.chmu.Lock()
		for n, c := range m.characters {
			if c.updated.Before(time.Now().Add(-10 * time.Second)) {
				delete(m.characters, n)
			}
		}
		m.chmu.Unlock()
	}
}
