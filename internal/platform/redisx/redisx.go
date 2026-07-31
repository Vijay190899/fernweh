// Package redisx builds the shared Redis client.
package redisx

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// Connect opens a client and verifies connectivity.
func Connect(ctx context.Context, addr string) (*redis.Client, error) {
	c := redis.NewClient(&redis.Options{Addr: addr})
	if err := c.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping %s: %w", addr, err)
	}
	return c, nil
}
