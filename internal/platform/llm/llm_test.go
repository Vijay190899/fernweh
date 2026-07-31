package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func fakeOpenRouter(t *testing.T, status int, content string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer key_test" {
			t.Errorf("missing auth header")
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": content}},
			},
		})
	}))
}

func client(baseURL string) *Client {
	c := New("key_test", "test-model", 2*time.Second, 0, nil)
	return c.WithBaseURL(baseURL)
}

func TestCompleteJSONHappyPath(t *testing.T) {
	srv := fakeOpenRouter(t, 200, `{"category":"beach"}`)
	defer srv.Close()

	raw, err := client(srv.URL).CompleteJSON(context.Background(), "sys", "user")
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]string
	if err := json.Unmarshal(raw, &out); err != nil || out["category"] != "beach" {
		t.Fatalf("bad payload: %s", raw)
	}
}

func TestCompleteJSONUnwrapsMarkdownFences(t *testing.T) {
	srv := fakeOpenRouter(t, 200, "```json\n{\"month\": 3}\n```")
	defer srv.Close()

	raw, err := client(srv.URL).CompleteJSON(context.Background(), "sys", "user")
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(raw) {
		t.Fatalf("fenced reply not unwrapped: %s", raw)
	}
}

func TestCompleteJSONWithoutKeyIsUnavailable(t *testing.T) {
	c := New("", "model", time.Second, 0, nil)
	_, err := c.CompleteJSON(context.Background(), "sys", "user")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("no key must yield ErrUnavailable, got %v", err)
	}
}

func TestCompleteJSONProviderErrorIsUnavailable(t *testing.T) {
	srv := fakeOpenRouter(t, 500, "boom")
	defer srv.Close()

	_, err := client(srv.URL).CompleteJSON(context.Background(), "sys", "user")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("5xx must map to ErrUnavailable, got %v", err)
	}
}

func TestCompleteJSONGarbageIsUnavailable(t *testing.T) {
	srv := fakeOpenRouter(t, 200, "certainly! here are your results")
	defer srv.Close()

	_, err := client(srv.URL).CompleteJSON(context.Background(), "sys", "user")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("non-JSON reply must map to ErrUnavailable, got %v", err)
	}
}

func TestExtractJSON(t *testing.T) {
	tests := []struct{ in, want string }{
		{`{"a":1}`, `{"a":1}`},
		{"prose before {\"a\": {\"b\": 2}} prose after", `{"a": {"b": 2}}`},
		{"```json\n{\"x\":true}\n```", `{"x":true}`},
		{"no json here", "no json here"},
	}
	for _, tt := range tests {
		if got := extractJSON(tt.in); got != tt.want {
			t.Errorf("extractJSON(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
