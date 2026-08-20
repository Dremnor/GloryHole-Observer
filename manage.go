package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"go.etcd.io/bbolt"
	"golang.org/x/crypto/bcrypt"
)

func (m *Map) index(rw http.ResponseWriter, req *http.Request) {
	s := m.getSession(req)
	if s == nil {
		http.Redirect(rw, req, "/login", 302)
		return
	}

	tokens := []string{}
	prefix := "http://example.com"
	m.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("users"))
		if b == nil {
			return nil
		}
		uRaw := b.Get([]byte(s.Username))
		if uRaw == nil {
			return nil
		}
		u := User{}
		json.Unmarshal(uRaw, &u)
		tokens = u.Tokens

		config := tx.Bucket([]byte("config"))
		if config != nil {
			prefix = string(config.Get([]byte("prefix")))
		}
		return nil
	})

	m.ExecuteTemplate(rw, "index.tmpl", struct {
		Page         Page
		Session      *Session
		UploadTokens []string
		Prefix       string
	}{
		Page:         m.getPage(req),
		Session:      s,
		UploadTokens: tokens,
		Prefix:       prefix,
	})
}

func (m *Map) login(rw http.ResponseWriter, req *http.Request) {
	if req.Method == "POST" {
		key := clientAddr(req, *trustProxyHeaders)
		if !m.loginLimit.allowed(key) {
			log.Printf("login: too many failed attempts from %s, refusing", key)
			rw.Header().Set("Retry-After", strconv.Itoa(int(loginWindow.Seconds())))
			http.Error(rw, "Too many failed login attempts. Try again later.", http.StatusTooManyRequests)
			return
		}
		u := m.getUser(req.FormValue("user"), req.FormValue("pass"))
		if u == nil {
			m.loginLimit.recordFailure(key)
		} else {
			m.loginLimit.recordSuccess(key)
			session := make([]byte, 32)
			rand.Read(session)
			http.SetCookie(rw, &http.Cookie{
				Name:    "session",
				Expires: time.Now().Add(time.Hour * 24 * 7),
				Value:   hex.EncodeToString(session),
				Path:    "/",
				// HttpOnly keeps the session out of reach of page scripts;
				// SameSite=Lax stops other sites from riding it.
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
				Secure:   requestIsHTTPS(req),
			})
			s := &Session{
				ID:        hex.EncodeToString(session),
				Username:  req.FormValue("user"),
				TempAdmin: u.Auths.Has("tempadmin"),
			}
			m.saveSession(s)
			http.Redirect(rw, req, "/", 302)
			return
		}
	}
	m.ExecuteTemplate(rw, "login.tmpl", struct {
		Page Page
	}{
		Page: m.getPage(req),
	})
}

func (m *Map) logout(rw http.ResponseWriter, req *http.Request) {
	s := m.getSession(req)
	if s != nil {
		m.deleteSession(s)
	}
	http.Redirect(rw, req, "/login", 302)
	return
}

func (m *Map) generateToken(rw http.ResponseWriter, req *http.Request) {
	if !requirePOST(rw, req) {
		return
	}
	s := m.getSession(req)
	if s == nil || !s.Auths.Has(AUTH_UPLOAD) {
		http.Redirect(rw, req, "/", 302)
		return
	}
	tokenRaw := make([]byte, 16)
	_, err := rand.Read(tokenRaw)
	if err != nil {
		rw.WriteHeader(500)
		return
	}
	token := hex.EncodeToString(tokenRaw)
	m.db.Update(func(tx *bbolt.Tx) error {
		ub, err := tx.CreateBucketIfNotExists([]byte("users"))
		if err != nil {
			return err
		}
		uRaw := ub.Get([]byte(s.Username))
		if uRaw == nil {
			return nil
		}
		u := User{}
		err = json.Unmarshal(uRaw, &u)
		if err != nil {
			return err
		}
		u.Tokens = append(u.Tokens, token)
		buf, err := json.Marshal(u)
		if err != nil {
			return err
		}
		err = ub.Put([]byte(s.Username), buf)
		if err != nil {
			return err
		}
		b, err := tx.CreateBucketIfNotExists([]byte("tokens"))
		if err != nil {
			return err
		}
		return b.Put([]byte(token), []byte(s.Username))
	})
	http.Redirect(rw, req, "/", 302)
}

func (m *Map) changePassword(rw http.ResponseWriter, req *http.Request) {
	s := m.getSession(req)
	if s == nil {
		http.Redirect(rw, req, "/", 302)
		return
	}

	render := func(msg string) {
		m.ExecuteTemplate(rw, "password.tmpl", struct {
			Page    Page
			Session *Session
			Error   string
		}{
			Page:    m.getPage(req),
			Session: s,
			Error:   msg,
		})
	}

	if req.Method == "POST" {
		req.ParseForm()
		password := req.FormValue("pass")
		if password == "" {
			render("New password must not be empty.")
			return
		}
		// Requiring the current password is what stops a stolen session, or a
		// cross-site POST, from turning into an account takeover.
		if m.getUser(s.Username, req.FormValue("current")) == nil {
			render("Current password is incorrect.")
			return
		}

		err := m.db.Update(func(tx *bbolt.Tx) error {
			users, err := tx.CreateBucketIfNotExists([]byte("users"))
			if err != nil {
				return err
			}
			raw := users.Get([]byte(s.Username))
			if raw == nil {
				// Previously this created a fresh user with no roles at all.
				return fmt.Errorf("user %q no longer exists", s.Username)
			}
			u := User{}
			if err := json.Unmarshal(raw, &u); err != nil {
				return err
			}
			u.Pass, err = bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
			if err != nil {
				return err
			}
			raw, err = json.Marshal(u)
			if err != nil {
				return err
			}
			return users.Put([]byte(s.Username), raw)
		})
		if err != nil {
			log.Println("changePassword:", err)
			render("Could not change the password.")
			return
		}
		http.Redirect(rw, req, "/", 302)
		return
	}

	render("")
}
