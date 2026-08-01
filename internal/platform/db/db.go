// Package db provides the pgx connection pool and a minimal, forward-only
// migration runner over SQL files embedded in the binary, no external
// migration tool to install or version-drift.
package db

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect opens a pool and verifies connectivity.
func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("db connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db ping: %w", err)
	}
	return pool, nil
}

// Bootstrap brings the schema and demo data up to date, holding a Postgres
// advisory lock so that every service can call this at start-up and exactly
// one of them does the work while the rest wait a moment and continue.
//
// This replaces a one-shot seed container. A container that migrates and then
// exits zero is correct on a laptop and fragile under an orchestrator: many
// platforms apply their own restart policy to every service in a stack, which
// turns a clean exit into a restart loop, and anything waiting on
// service_completed_successfully then waits forever. Making start-up
// self-sufficient removes that whole class of deployment failure.
func Bootstrap(ctx context.Context, pool *pgxpool.Pool, migrations embed.FS, log *slog.Logger,
	seed func(context.Context, *pgxpool.Pool, *slog.Logger) error) error {

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("bootstrap acquire: %w", err)
	}
	defer conn.Release()

	// Any constant works so long as every service uses the same one.
	const lockID int64 = 8274531
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", lockID); err != nil {
		return fmt.Errorf("bootstrap lock: %w", err)
	}
	defer func() {
		// Released on its own context: the request context may already be done.
		_, _ = conn.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", lockID)
	}()

	if err := Migrate(ctx, pool, migrations, log); err != nil {
		return err
	}
	if seed == nil {
		return nil
	}
	return seed(ctx, pool, log)
}

// Migrate applies embedded migrations in filename order, tracking versions in
// schema_migrations. Idempotent; safe to run on every boot.
func Migrate(ctx context.Context, pool *pgxpool.Pool, migrations embed.FS, log *slog.Logger) error {
	_, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`)
	if err != nil {
		return fmt.Errorf("migrations table: %w", err)
	}

	files, err := fs.Glob(migrations, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(files)

	for _, f := range files {
		var applied bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, f).Scan(&applied); err != nil {
			return err
		}
		if applied {
			continue
		}
		sql, err := migrations.ReadFile(f)
		if err != nil {
			return err
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, string(sql)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("migration %s: %w", f, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, f); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		log.Info("migration applied", "file", f)
	}
	return nil
}
