package redisx

import (
	"context"
	"net"
	"testing"
	"time"
)

// A Redis that is gone must cost a request almost nothing. The library
// defaults retry with backoff, which turned "the cache is unavailable" into a
// request that hung until the browser gave up.
func TestCommandsFailFastWhenRedisIsUnreachable(t *testing.T) {
	// A listener that accepts and then never speaks: dial succeeds, reads hang.
	// Harsher than a refused connection, and the case the timeouts exist for.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			defer conn.Close()
		}
	}()

	c := New(ln.Addr().String())
	defer c.Close()

	start := time.Now()
	if err := c.Get(context.Background(), "any-key").Err(); err == nil {
		t.Fatal("Get succeeded against a server that never replies")
	}
	// A search request budgets 800ms for ranking end to end. A cache lookup
	// that can outlast that is worse than having no cache.
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("a command against a dead Redis took %v; it must fail in well under a second", elapsed)
	} else {
		t.Logf("command failed after %v", elapsed)
	}
}

func TestConnectRetriesUntilContextExpires(t *testing.T) {
	// Nothing is listening here, so every attempt fails immediately. Connect
	// should keep trying rather than give up on the first refusal, because a
	// caller treats failure as fatal and services can start before Redis does.
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 900*time.Millisecond)
	defer cancel()

	if _, err := Connect(ctx, "127.0.0.1:1"); err == nil {
		t.Fatal("Connect succeeded against a closed port")
	}
	if elapsed := time.Since(start); elapsed < 700*time.Millisecond {
		t.Errorf("Connect gave up after %v; it should retry until the context expires", elapsed)
	}
}
