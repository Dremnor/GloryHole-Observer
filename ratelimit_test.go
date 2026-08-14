package main

import (
	"net/http"
	"testing"
	"time"
)

// fixedClock lets the tests move time forward without sleeping.
type fixedClock struct{ t time.Time }

func (c *fixedClock) now() time.Time          { return c.t }
func (c *fixedClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newTestLimiter(max int, window time.Duration) (*loginLimiter, *fixedClock) {
	clock := &fixedClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	l := newLoginLimiter(max, window)
	l.now = clock.now
	return l, clock
}

func TestLimiterAllowsUntilMaxFailures(t *testing.T) {
	l, _ := newTestLimiter(3, time.Minute)

	for i := 0; i < 3; i++ {
		if !l.allowed("1.2.3.4") {
			t.Fatalf("attempt %d should still be allowed", i+1)
		}
		l.recordFailure("1.2.3.4")
	}
	if l.allowed("1.2.3.4") {
		t.Error("the attempt after the limit must be refused")
	}
}

func TestLimiterIsPerAddress(t *testing.T) {
	l, _ := newTestLimiter(2, time.Minute)

	l.recordFailure("1.2.3.4")
	l.recordFailure("1.2.3.4")
	if l.allowed("1.2.3.4") {
		t.Error("the offending address should be locked out")
	}
	if !l.allowed("5.6.7.8") {
		t.Error("an unrelated address must not be affected")
	}
}

func TestLimiterForgetsAfterWindow(t *testing.T) {
	l, clock := newTestLimiter(2, time.Minute)

	l.recordFailure("1.2.3.4")
	l.recordFailure("1.2.3.4")
	if l.allowed("1.2.3.4") {
		t.Fatal("should be locked out immediately after the failures")
	}

	clock.advance(time.Minute + time.Second)
	if !l.allowed("1.2.3.4") {
		t.Error("the lockout must expire once the window passes")
	}
}

func TestLimiterFailureExtendsTheWindow(t *testing.T) {
	l, clock := newTestLimiter(2, time.Minute)

	l.recordFailure("1.2.3.4")
	l.recordFailure("1.2.3.4")

	// A slow guesser keeps trying just before the window elapses; it must not
	// win back an allowance by waiting.
	clock.advance(50 * time.Second)
	l.recordFailure("1.2.3.4")
	clock.advance(50 * time.Second)
	if l.allowed("1.2.3.4") {
		t.Error("continued failures should keep the address locked out")
	}
}

func TestLimiterSuccessClearsHistory(t *testing.T) {
	l, _ := newTestLimiter(3, time.Minute)

	l.recordFailure("1.2.3.4")
	l.recordFailure("1.2.3.4")
	l.recordSuccess("1.2.3.4")

	for i := 0; i < 3; i++ {
		if !l.allowed("1.2.3.4") {
			t.Fatalf("a successful login should reset the count, failed at %d", i+1)
		}
		l.recordFailure("1.2.3.4")
	}
}

func TestLimiterDisabledWhenMaxIsZero(t *testing.T) {
	l, _ := newTestLimiter(0, time.Minute)

	for i := 0; i < 100; i++ {
		l.recordFailure("1.2.3.4")
	}
	if !l.allowed("1.2.3.4") {
		t.Error("max of 0 must disable the limiter entirely")
	}
}

func TestLimiterCleanupDropsExpiredRecords(t *testing.T) {
	l, clock := newTestLimiter(2, time.Minute)

	l.recordFailure("1.2.3.4")
	clock.advance(2 * time.Minute)
	l.recordFailure("5.6.7.8")

	// Exercise the same sweep cleanup() performs, without its ticker.
	l.mu.Lock()
	for k, r := range l.records {
		if l.now().After(r.expires) {
			delete(l.records, k)
		}
	}
	remaining := len(l.records)
	l.mu.Unlock()

	if remaining != 1 {
		t.Errorf("expected only the fresh record to survive, got %d", remaining)
	}
}

func TestClientAddrIgnoresForwardedHeaderWhenUntrusted(t *testing.T) {
	req := &http.Request{
		RemoteAddr: "10.0.0.9:5555",
		Header:     http.Header{"X-Forwarded-For": []string{"1.2.3.4"}},
	}

	if got := clientAddr(req, false); got != "10.0.0.9" {
		t.Errorf("untrusted proxy header must be ignored, got %q", got)
	}
	if got := clientAddr(req, true); got != "1.2.3.4" {
		t.Errorf("trusted proxy header should be used, got %q", got)
	}
}

func TestClientAddrTakesFirstForwardedEntry(t *testing.T) {
	req := &http.Request{
		RemoteAddr: "10.0.0.9:5555",
		Header:     http.Header{"X-Forwarded-For": []string{"1.2.3.4, 10.0.0.1, 10.0.0.2"}},
	}

	if got := clientAddr(req, true); got != "1.2.3.4" {
		t.Errorf("expected the original client address, got %q", got)
	}
}

func TestClientAddrFallsBackOnUnparseableRemoteAddr(t *testing.T) {
	req := &http.Request{RemoteAddr: "not-a-host-port", Header: http.Header{}}

	if got := clientAddr(req, false); got != "not-a-host-port" {
		t.Errorf("expected the raw value as fallback, got %q", got)
	}
}
