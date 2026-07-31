// Command gateway is the public entry point: demo frontend + API edge with
// rate limiting and trace propagation.
package main

import (
	"context"
	"io/fs"
	"os"
	"time"

	fernweh "fernweh"
	"fernweh/internal/gateway"
	"fernweh/internal/platform/betterstack"
	"fernweh/internal/platform/config"
	"fernweh/internal/platform/httpx"
	"fernweh/internal/platform/logging"
	"fernweh/internal/platform/otelx"
)

func main() {
	const service = "gateway"
	log := logging.New(service)
	cfg := config.Load(service)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	shutdown, err := otelx.Setup(ctx, service, cfg.OTLPEndpoint, cfg.TracesEnabled)
	if err != nil {
		log.Error("otel setup failed", "err", err)
		os.Exit(1)
	}
	defer shutdown(context.Background()) //nolint:errcheck

	static, err := fs.Sub(fernweh.WebFS, "web")
	if err != nil {
		log.Error("static assets missing", "err", err)
		os.Exit(1)
	}

	go betterstack.Heartbeat(context.Background(), cfg.HeartbeatURL, time.Minute, log)

	mux := gateway.NewMux(cfg, static, log)
	srv := httpx.NewServer(config.Addr("GATEWAY_ADDR", ":8080"), service, mux)
	if err := httpx.Serve(srv, log); err != nil {
		log.Error("server error", "err", err)
		os.Exit(1)
	}
}
