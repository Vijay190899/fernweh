// Package llm is the platform's single door to language models. Callers ask
// for a JSON completion; this package decides whether that is answered by
// OpenRouter or refused (budget exhausted, no key, provider down), in which
// case callers must use their deterministic fallback. The AI layer being
// optional-at-runtime is a core design goal: no user-facing path may depend
// on an LLM being up.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// ErrUnavailable signals the caller to use its deterministic fallback.
// Reasons: no API key, daily budget exhausted, provider error, timeout.
var ErrUnavailable = errors.New("llm unavailable")

type Client struct {
	apiKey  string
	model   string
	timeout time.Duration
	budget  int
	rdb     *redis.Client
	http    *http.Client
	baseURL string
}

func New(apiKey, model string, timeout time.Duration, dailyBudget int, rdb *redis.Client) *Client {
	return &Client{
		apiKey:  apiKey,
		model:   model,
		timeout: timeout,
		budget:  dailyBudget,
		rdb:     rdb,
		http:    &http.Client{Timeout: timeout},
		baseURL: "https://openrouter.ai/api/v1",
	}
}

// WithBaseURL overrides the API endpoint (tests).
func (c *Client) WithBaseURL(u string) *Client { c.baseURL = u; return c }

// Enabled reports whether a key is configured at all.
func (c *Client) Enabled() bool { return c.apiKey != "" }

// Model returns the configured model identifier.
func (c *Client) Model() string { return c.model }

// CompleteJSON sends system+user prompts expecting a JSON object reply.
// Budget accounting happens before the call; a platform-wide daily cap in
// Redis protects a public demo from draining the API credit.
func (c *Client) CompleteJSON(ctx context.Context, system, user string) (json.RawMessage, error) {
	tracer := otel.Tracer("llm")
	ctx, span := tracer.Start(ctx, "llm.complete")
	defer span.End()
	span.SetAttributes(attribute.String("llm.model", c.model))

	if c.apiKey == "" {
		span.SetAttributes(attribute.String("llm.skip", "no_key"))
		return nil, fmt.Errorf("%w: no API key configured", ErrUnavailable)
	}
	if err := c.spendBudget(ctx); err != nil {
		span.SetAttributes(attribute.String("llm.skip", "budget"))
		return nil, err
	}

	reqBody, _ := json.Marshal(map[string]any{
		"model": c.model,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
		"response_format": map[string]string{"type": "json_object"},
		"temperature":     0.2,
		"max_tokens":      600,
	})

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		span.SetStatus(codes.Error, resp.Status)
		return nil, fmt.Errorf("%w: openrouter status %s", ErrUnavailable, resp.Status)
	}

	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || len(out.Choices) == 0 {
		return nil, fmt.Errorf("%w: malformed response", ErrUnavailable)
	}
	content := extractJSON(out.Choices[0].Message.Content)
	if !json.Valid([]byte(content)) {
		return nil, fmt.Errorf("%w: model returned invalid JSON", ErrUnavailable)
	}
	return json.RawMessage(content), nil
}

// spendBudget increments today's platform-wide call counter and rejects when
// over cap. The key expires after 48h so counters clean themselves up.
func (c *Client) spendBudget(ctx context.Context) error {
	if c.rdb == nil || c.budget <= 0 {
		return nil
	}
	key := "llm:budget:" + time.Now().UTC().Format("2006-01-02")
	n, err := c.rdb.Incr(ctx, key).Result()
	if err != nil {
		return nil // Redis trouble must not block the request path
	}
	c.rdb.Expire(ctx, key, 48*time.Hour)
	if int(n) > c.budget {
		return fmt.Errorf("%w: daily budget of %d calls exhausted", ErrUnavailable, c.budget)
	}
	return nil
}

// extractJSON tolerates models that wrap JSON in markdown fences.
func extractJSON(s string) string {
	start := -1
	depth := 0
	for i, r := range s {
		switch r {
		case '{':
			if depth == 0 {
				start = i
			}
			depth++
		case '}':
			depth--
			if depth == 0 && start >= 0 {
				return s[start : i+1]
			}
		}
	}
	return s
}
