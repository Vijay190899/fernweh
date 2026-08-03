// Package httpx is the shared HTTP plumbing: server lifecycle with graceful
// shutdown, JSON helpers, health endpoints, and a traced client. Plain
// net/http, Go 1.22+ routing makes a framework unnecessary.
package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"fernweh/internal/platform/otelx"
)

// Serve runs srv until SIGINT/SIGTERM, then drains connections gracefully.
func Serve(srv *http.Server, log *slog.Logger) error {
	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return err
	case sig := <-stop:
		log.Info("shutting down", "signal", sig.String())
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(ctx)
	}
}

// NewServer applies the standard middleware stack (tracing outermost) and
// sane timeouts.
func NewServer(addr, service string, mux http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           otelx.Middleware(service, mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

// Health registers /healthz (liveness) and /readyz (dependency checks).
func Health(mux *http.ServeMux, ready func(context.Context) error) {
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if ready != nil {
			if err := ready(ctx); err != nil {
				Error(w, http.StatusServiceUnavailable, "not ready: "+err.Error())
				return
			}
		}
		w.WriteHeader(http.StatusOK)
	})
}

// JSON writes v as a JSON response.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// Error writes a JSON error envelope.
func Error(w http.ResponseWriter, status int, msg string) {
	JSON(w, status, map[string]string{"error": msg})
}

// Decode reads a JSON body with a size cap.
func Decode(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// Client is a shared HTTP client with timeouts for service-to-service calls.
var Client = &http.Client{Timeout: 5 * time.Second}

// StatusError is a non-2xx reply from an internal service. Callers that
// forward a result to a browser need the distinction: a 400 upstream means the
// caller sent something wrong and must not be reported to the user as an
// outage, which is what a bare error collapses it into.
type StatusError struct {
	URL    string
	Status int
	Reason string
}

func (e *StatusError) Error() string { return e.URL + " returned " + e.Reason }

// ClientFault reports whether err was an upstream 4xx, meaning the request was
// at fault rather than the service.
func ClientFault(err error) bool {
	var se *StatusError
	return errors.As(err, &se) && se.Status >= 400 && se.Status < 500
}

// PostJSON performs a traced service-to-service POST, decoding into out.
func PostJSON(ctx context.Context, url string, in, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return do(ctx, http.MethodPost, url, bytes.NewReader(body), out)
}

// GetJSON performs a traced service-to-service GET, decoding into out.
func GetJSON(ctx context.Context, url string, out any) error {
	return do(ctx, http.MethodGet, url, nil, out)
}

func do(ctx context.Context, method, url string, body io.Reader, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	otelx.Inject(ctx, req)
	resp, err := Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return &StatusError{URL: url, Status: resp.StatusCode, Reason: resp.Status}
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
