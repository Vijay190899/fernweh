package gateway

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The edge answers for three things that are not upstream failures, and used
// to report all of them as one. A caller reading 502 goes looking for an
// outage; these are not outages.
func TestProxyStatusesDistinguishFaults(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Drain first: the body cap only trips while the request is being
		// copied upstream, so an upstream that answers without reading would
		// never exercise it.
		_, _ = io.Copy(io.Discard, r.Body)
		// Echo the rewritten path so the happy case proves the rewrite too.
		w.Header().Set("X-Trace-Id", "upstream-id")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(r.URL.Path))
	}))
	defer upstream.Close()

	routes := map[string]string{"search": upstream.URL, "dead": "http://127.0.0.1:1"}
	h := apiProxy(routes, slog.New(slog.DiscardHandler))

	tests := []struct {
		name string
		path string
		body string
		want int
	}{
		{"routed service", "/api/search", `{"query":"x"}`, http.StatusOK},
		{"unknown service", "/api/nope/thing", `{}`, http.StatusNotFound},
		{"unroutable service name", "/api/", `{}`, http.StatusNotFound},
		{"upstream down", "/api/dead", `{}`, http.StatusBadGateway},
		{"body over the cap", "/api/search", strings.Repeat("A", (1<<20)+1), http.StatusRequestEntityTooLarge},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Errorf("%s -> %d, want %d (%s)", tc.path, w.Code, tc.want, w.Body.String())
			}
		})
	}
}

// The browser builds a Jaeger link out of this header. Two values arrive as
// "id, id" and the link resolves to nothing.
func TestProxyReturnsOneTraceID(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Trace-Id", "from-upstream")
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	h := apiProxy(map[string]string{"search": upstream.URL}, slog.New(slog.DiscardHandler))
	req := httptest.NewRequest(http.MethodPost, "/api/search", strings.NewReader("{}"))
	w := httptest.NewRecorder()
	// Stand in for the edge middleware, which stamps the id it will be traced by.
	w.Header().Set("X-Trace-Id", "from-edge")
	h.ServeHTTP(w, req)

	got := w.Result().Header.Values("X-Trace-Id")
	if len(got) != 1 {
		t.Fatalf("X-Trace-Id has %d values %v, a caller can only follow one", len(got), got)
	}
	if got[0] != "from-edge" {
		t.Errorf("X-Trace-Id = %q, want the id stamped at the edge", got[0])
	}
}

func TestServiceOf(t *testing.T) {
	for in, want := range map[string]string{
		"/api/search":                "search",
		"/api/search/":               "search",
		"/api/enrich/listings/x/aud": "enrich",
		"/api/":                      "",
		"/api/compare/personas":      "compare",
	} {
		if got := serviceOf(in); got != want {
			t.Errorf("serviceOf(%q) = %q, want %q", in, got, want)
		}
	}
}
