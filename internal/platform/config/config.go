// Package config loads service configuration from the environment and fails
// fast on invalid values. Environment-only config keeps local and production
// identical: same binaries, different env.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	ServiceName string

	DatabaseURL string
	RedisAddr   string

	OTLPEndpoint  string
	TracesEnabled bool

	OpenRouterKey  string
	LLMModel       string
	LLMTimeout     time.Duration
	LLMDailyBudget int

	SearchURL  string
	RankingURL string
	EnrichURL  string

	RateLimitRPS   float64
	RateLimitBurst int
}

// Load reads configuration for the named service. addrEnv names the env var
// holding this service's listen address (empty for non-listening commands).
func Load(service string) Config {
	return Config{
		ServiceName:    service,
		DatabaseURL:    getStr("DATABASE_URL", "postgres://fernweh:fernweh@localhost:5432/fernweh?sslmode=disable"),
		RedisAddr:      getStr("REDIS_ADDR", "localhost:6379"),
		OTLPEndpoint:   getStr("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318"),
		TracesEnabled:  getBool("OTEL_TRACES_ENABLED", true),
		OpenRouterKey:  os.Getenv("OPENROUTER_API_KEY"),
		LLMModel:       getStr("LLM_MODEL", "anthropic/claude-haiku-4.5"),
		LLMTimeout:     time.Duration(getInt("LLM_TIMEOUT_MS", 2500)) * time.Millisecond,
		LLMDailyBudget: getInt("LLM_DAILY_CALL_BUDGET", 2000),
		SearchURL:      getStr("SEARCH_URL", "http://localhost:8081"),
		RankingURL:     getStr("RANKING_URL", "http://localhost:8082"),
		EnrichURL:      getStr("ENRICH_URL", "http://localhost:8083"),
		RateLimitRPS:   getFloat("RATE_LIMIT_RPS", 5),
		RateLimitBurst: getInt("RATE_LIMIT_BURST", 15),
	}
}

// Addr returns the listen address from env var name, e.g. "SEARCH_ADDR".
func Addr(envVar, fallback string) string {
	return getStr(envVar, fallback)
}

func getStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		panic(fmt.Sprintf("config: %s must be an integer, got %q", key, v))
	}
	return n
}

func getFloat(key string, fallback float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		panic(fmt.Sprintf("config: %s must be a number, got %q", key, v))
	}
	return f
}

func getBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		panic(fmt.Sprintf("config: %s must be a boolean, got %q", key, v))
	}
	return b
}
