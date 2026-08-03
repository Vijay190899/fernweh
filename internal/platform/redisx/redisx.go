// Package redisx builds the shared Redis client.
package redisx

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Everything this client is used for is advisory: the intent cache, session
// profiles, and the LLM budget counter. Each caller already has an answer for
// Redis being unreachable, so none of them should ever be made to wait for it.
//
// The library defaults do not agree. A 5s dial timeout with retries and
// backoff means a Redis that is simply gone costs upwards of twenty seconds
// per command, and the architecture doc's promise that "intent extraction
// skips cache" quietly became "intent extraction hangs, then the request times
// out". Bounding this here rather than at each call site means a new caller
// cannot reintroduce the stall by forgetting to wrap its lookup.
//
// The budget is deliberately far below any human patience threshold: a cache
// that answers slowly is worth less than no cache at all.
const (
	dialTimeout = 250 * time.Millisecond
	opTimeout   = 250 * time.Millisecond
)

// New builds the client without touching the network. Separate from Connect so
// the timeout policy can be tested for what it actually guarantees, which is
// about commands rather than about the opening handshake.
func New(addr string) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:         addr,
		DialTimeout:  dialTimeout,
		ReadTimeout:  opTimeout,
		WriteTimeout: opTimeout,
		PoolTimeout:  opTimeout,
		// -1 disables retries. Retrying a cache read costs the request more
		// than the cache could ever save it.
		MaxRetries: -1,
	})
}

// Connect opens a client and verifies connectivity.
//
// The opening handshake and the steady state get different budgets on purpose.
// Callers treat a Connect failure as fatal, so giving startup the same 250ms
// the request path gets would make booting depend on Redis already being up:
// correct on this compose file, which waits for a health check, and fragile
// anywhere ordering is not guaranteed. So the ping is retried until ctx runs
// out, while every command after it stays bounded.
func Connect(ctx context.Context, addr string) (*redis.Client, error) {
	c := New(addr)

	var err error
	for attempt := 0; ; attempt++ {
		if err = c.Ping(ctx).Err(); err == nil {
			return c, nil
		}
		wait := time.Duration(1<<min(attempt, 5)) * 100 * time.Millisecond
		select {
		case <-ctx.Done():
			c.Close()
			return nil, fmt.Errorf("redis ping %s: %w", addr, err)
		case <-time.After(wait):
		}
	}
}
