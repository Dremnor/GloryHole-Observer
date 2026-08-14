package main

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// loginLimiter throttles repeated failed logins from a single address. Only
// failures are counted, so somebody who types their password correctly is
// never delayed, while an address guessing passwords runs out of attempts.
type loginLimiter struct {
	mu      sync.Mutex
	records map[string]*failureRecord
	max     int
	window  time.Duration

	// now is swapped out in tests so they need no sleeps.
	now func() time.Time
}

type failureRecord struct {
	count   int
	expires time.Time
}

func newLoginLimiter(max int, window time.Duration) *loginLimiter {
	return &loginLimiter{
		records: map[string]*failureRecord{},
		max:     max,
		window:  window,
		now:     time.Now,
	}
}

// allowed reports whether key may attempt a login right now.
func (l *loginLimiter) allowed(key string) bool {
	if l.max <= 0 {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	r, ok := l.records[key]
	if !ok {
		return true
	}
	if l.now().After(r.expires) {
		delete(l.records, key)
		return true
	}
	return r.count < l.max
}

// recordFailure counts a rejected login. Each failure restarts the window, so
// a steady trickle of guesses stays locked out rather than topping up.
func (l *loginLimiter) recordFailure(key string) {
	if l.max <= 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	r, ok := l.records[key]
	if !ok || l.now().After(r.expires) {
		l.records[key] = &failureRecord{count: 1, expires: l.now().Add(l.window)}
		return
	}
	r.count++
	r.expires = l.now().Add(l.window)
}

// recordSuccess clears the history for key after a successful login.
func (l *loginLimiter) recordSuccess(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.records, key)
}

// cleanup drops expired records so the map cannot grow without bound.
func (l *loginLimiter) cleanup() {
	if l.window <= 0 {
		return
	}
	for range time.Tick(l.window) {
		l.mu.Lock()
		now := l.now()
		for k, r := range l.records {
			if now.After(r.expires) {
				delete(l.records, k)
			}
		}
		l.mu.Unlock()
	}
}

// clientAddr identifies the client for rate limiting. X-Forwarded-For is only
// consulted when the operator has confirmed a reverse proxy sits in front,
// because anyone able to reach the port directly could otherwise forge the
// header and get a fresh allowance on every attempt.
func clientAddr(req *http.Request, trustProxy bool) string {
	if trustProxy {
		if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
			if first := strings.TrimSpace(strings.Split(xff, ",")[0]); first != "" {
				return first
			}
		}
	}
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		return req.RemoteAddr
	}
	return host
}
