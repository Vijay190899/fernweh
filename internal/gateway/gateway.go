package gateway

import (
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"fernweh/internal/platform/config"
	"fernweh/internal/platform/httpx"
	"fernweh/internal/platform/otelx"
)

// NewMux wires the public surface: embedded frontend at /, JSON APIs under
// /api/* proxied to the internal services with tracing and rate limiting.
func NewMux(cfg config.Config, staticFS fs.FS, log *slog.Logger) *http.ServeMux {
	mux := http.NewServeMux()

	// API routes → internal services. The gateway is the only thing the
	// internet talks to; service topology stays private.
	routes := map[string]string{
		"search":  cfg.SearchURL,
		"compare": cfg.SearchURL,
		"signals": cfg.RankingURL,
		"profile": cfg.RankingURL,
		"enrich":  cfg.EnrichURL,
	}
	limiter := newIPLimiter(cfg.RateLimitRPS, cfg.RateLimitBurst)
	mux.Handle("/api/", limiter.middleware(securityHeaders(apiProxy(routes, log))))

	// Frontend config: where the demo UI should link to Jaeger.
	mux.HandleFunc("GET /api-config", func(w http.ResponseWriter, r *http.Request) {
		httpx.JSON(w, http.StatusOK, map[string]string{"jaeger_url": cfg.JaegerUIURL})
	})

	httpx.Health(mux, nil)

	// Static frontend (embedded in the binary, one artifact, no CDN).
	// Method-less on purpose: "GET /" would conflict with the method-less
	// "/api/" subtree under Go 1.22 mux precedence rules.
	mux.Handle("/", securityHeaders(http.FileServerFS(staticFS)))
	return mux
}

// apiProxy rewrites /api/<service>/... to <target>/v1/<service>/... and
// forwards with trace propagation. Unknown services 404 without leaking
// topology.
func apiProxy(routes map[string]string, log *slog.Logger) http.Handler {
	proxy := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			path := strings.TrimPrefix(r.In.URL.Path, "/api/")
			service := path
			if i := strings.IndexByte(path, '/'); i > 0 {
				service = path[:i]
			}
			target, ok := routes[service]
			if !ok {
				return // Out.URL stays empty → RoundTrip fails → 502 handled below
			}
			u, err := url.Parse(target)
			if err != nil {
				return
			}
			r.SetURL(u)
			r.Out.URL.Path = "/v1/" + path
			r.Out.Host = u.Host
			otelx.Inject(r.In.Context(), r.Out)
		},
		// The edge already stamped X-Trace-Id from its own server span, and the
		// upstream stamps the same id from its child span. ReverseProxy copies
		// upstream headers by appending, so the browser was receiving
		// "abc, abc" and building a Jaeger link out of it that resolved to
		// nothing. One id, set at the edge, is what a caller can use.
		ModifyResponse: func(resp *http.Response) error {
			resp.Header.Del("X-Trace-Id")
			return nil
		},
		// Every proxy failure used to answer 502 "upstream unavailable",
		// including the two that have nothing to do with an upstream: a request
		// too large, and a path that names no service. Both told a caller to go
		// look for an outage that was not happening.
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				httpx.Error(w, http.StatusRequestEntityTooLarge, "request body too large")
				return
			}
			log.WarnContext(r.Context(), "proxy error", "path", r.URL.Path, "err", err)
			httpx.Error(w, http.StatusBadGateway, "upstream unavailable")
		},
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Reject an unrouted path here rather than letting it fall through to a
		// failed dial. The reply names nothing about the topology behind it.
		if _, ok := routes[serviceOf(r.URL.Path)]; !ok {
			httpx.Error(w, http.StatusNotFound, "no such endpoint")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		proxy.ServeHTTP(w, r)
	})
}

// serviceOf pulls the first path segment after /api/, which is the name the
// route table is keyed by.
func serviceOf(path string) string {
	rest := strings.TrimPrefix(path, "/api/")
	if i := strings.IndexByte(rest, '/'); i > 0 {
		return rest[:i]
	}
	return rest
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		// Everything the page needs ships with the binary: no external image
		// host, no font CDN, no script origin beyond self.
		h.Set("Content-Security-Policy",
			"default-src 'self'; img-src 'self' data:; font-src 'self'; "+
				"style-src 'self' 'unsafe-inline'; script-src 'self'; "+
				"connect-src 'self'; base-uri 'none'; form-action 'self'; "+
				"frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}
