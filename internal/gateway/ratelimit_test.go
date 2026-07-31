package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIPLimiterEnforcesBurst(t *testing.T) {
	l := newIPLimiter(1, 3) // 1 rps, burst 3
	allowed := 0
	for i := 0; i < 10; i++ {
		if l.allow("1.2.3.4") {
			allowed++
		}
	}
	if allowed != 3 {
		t.Errorf("burst of 3 should allow exactly 3 immediate requests, got %d", allowed)
	}
	// A different IP has its own bucket.
	if !l.allow("5.6.7.8") {
		t.Error("independent IPs must not share buckets")
	}
}

func TestMiddlewareReturns429(t *testing.T) {
	l := newIPLimiter(1, 1)
	h := l.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/search", nil)
	req.RemoteAddr = "9.9.9.9:1234"

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("first request should pass, got %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second request should be limited, got %d", rec.Code)
	}
}

func TestClientIPPrefersForwardedFor(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.1:555"
	if got := clientIP(r); got != "10.0.0.1" {
		t.Errorf("clientIP = %q, want 10.0.0.1", got)
	}
	r.Header.Set("X-Forwarded-For", "203.0.113.7, 10.0.0.1")
	if got := clientIP(r); got != "203.0.113.7" {
		t.Errorf("clientIP with XFF = %q, want 203.0.113.7", got)
	}
}
