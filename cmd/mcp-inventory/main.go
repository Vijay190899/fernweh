// Command mcp-inventory exposes the travel inventory over the Model Context
// Protocol, so an assistant can query the catalogue through typed tools
// instead of being handed a shell and a database password.
//
// It speaks JSON-RPC 2.0 over stdio and reuses the same repository the
// services use, which means a tool call and a production query go through
// identical code. There is no second implementation to drift.
//
// Registered for this repository in .mcp.json.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"fernweh/internal/inventory"
	"fernweh/internal/platform/config"
	"fernweh/internal/platform/db"
	"fernweh/internal/platform/logging"
)

const protocolVersion = "2024-11-05"

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type tool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema any    `json:"inputSchema"`
}

func obj(props map[string]any, required ...string) map[string]any {
	s := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func str(desc string) map[string]any  { return map[string]any{"type": "string", "description": desc} }
func num(desc string) map[string]any  { return map[string]any{"type": "number", "description": desc} }

var tools = []tool{
	{
		Name: "search_inventory",
		Description: "Search travel listings by structured filters. Every argument is " +
			"optional; omitting all of them returns the highest rated listings. " +
			"Budgets are euros per night.",
		InputSchema: obj(map[string]any{
			"destination": str("City or region, for example Crete or Algarve"),
			"country":     str("Full country name, for example Portugal"),
			"category":    map[string]any{"type": "string", "description": "Trip type", "enum": []string{"beach", "city", "ski", "wellness", "countryside", "adventure"}},
			"budget_max":  num("Maximum euros per night"),
			"budget_min":  num("Minimum euros per night"),
			"month":       num("Month of travel, 1 to 12"),
			"min_rating":  num("Minimum guest rating, 0 to 5"),
			"limit":       num("Maximum results, default 10"),
		}),
	},
	{
		Name:        "get_listing",
		Description: "Fetch one listing by id, including its full description, amenities and content status.",
		InputSchema: obj(map[string]any{"id": str("Listing id, for example lst_0042")}, "id"),
	},
	{
		Name: "inventory_stats",
		Description: "Report catalogue size and content completeness, broken down by " +
			"content status. Useful for checking whether enrichment has work outstanding.",
		InputSchema: obj(map[string]any{}),
	},
	{
		Name: "listing_audit",
		Description: "Show the enrichment history for one listing: what its content was " +
			"before, what it became, and whether a model or a template produced it.",
		InputSchema: obj(map[string]any{"id": str("Listing id")}, "id"),
	},
}

type server struct {
	repo *inventory.Repo
}

func (s *server) callTool(ctx context.Context, name string, args map[string]any) (string, error) {
	getStr := func(k string) string {
		if v, ok := args[k].(string); ok {
			return strings.TrimSpace(v)
		}
		return ""
	}
	getInt := func(k string) int {
		if v, ok := args[k].(float64); ok {
			return int(v)
		}
		return 0
	}

	switch name {
	case "search_inventory":
		limit := getInt("limit")
		if limit <= 0 || limit > 50 {
			limit = 10
		}
		f := inventory.Filter{
			Destination: getStr("destination"),
			Country:     getStr("country"),
			Category:    getStr("category"),
			BudgetMax:   getInt("budget_max") * 100,
			BudgetMin:   getInt("budget_min") * 100,
			Month:       getInt("month"),
			Limit:       limit,
		}
		if v, ok := args["min_rating"].(float64); ok {
			f.MinRating = v
		}
		listings, err := s.repo.Search(ctx, f)
		if err != nil {
			return "", err
		}
		if len(listings) == 0 {
			return "No listings match those filters. Try widening the budget or dropping the month.", nil
		}
		var b strings.Builder
		fmt.Fprintf(&b, "%d listings:\n\n", len(listings))
		for _, l := range listings {
			fmt.Fprintf(&b, "%s  %s\n  %s, %s · %s · €%d/night · rated %.1f from %d reviews\n  amenities: %s\n  content: %s\n\n",
				l.ID, l.Name, l.Destination, l.Country, l.Category,
				l.PricePerNightCents/100, l.Rating, l.ReviewCount,
				strings.Join(l.Amenities, ", "), l.ContentStatus)
		}
		return b.String(), nil

	case "get_listing":
		l, err := s.repo.Get(ctx, getStr("id"))
		if err != nil {
			return "", fmt.Errorf("no listing with id %q", getStr("id"))
		}
		out, _ := json.MarshalIndent(l, "", "  ")
		return string(out), nil

	case "inventory_stats":
		st, err := s.repo.Stats(ctx)
		if err != nil {
			return "", err
		}
		var b strings.Builder
		fmt.Fprintf(&b, "%d listings total\n", st.Total)
		for status, n := range st.ByStatus {
			pct := 0.0
			if st.Total > 0 {
				pct = float64(n) / float64(st.Total) * 100
			}
			fmt.Fprintf(&b, "  %-18s %4d  (%.1f%%)\n", status, n, pct)
		}
		return b.String(), nil

	case "listing_audit":
		entries, err := s.repo.RecentAudit(ctx, getStr("id"), 10)
		if err != nil {
			return "", err
		}
		if len(entries) == 0 {
			return "No enrichment history for that listing; its content arrived complete.", nil
		}
		var b strings.Builder
		for _, e := range entries {
			fmt.Fprintf(&b, "%s · field %s · source %s %s\n  before: %s\n  after:  %s\n\n",
				e.CreatedAt, e.Field, e.Source, e.Model,
				truncate(e.Before, 200), truncate(e.After, 300))
		}
		return b.String(), nil
	}
	return "", fmt.Errorf("unknown tool %q", name)
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "(empty)"
	}
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func main() {
	// Logs go to stderr; stdout carries the protocol and nothing else.
	log := logging.New("mcp-inventory")
	cfg := config.Load("mcp-inventory")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	cancel()
	if err != nil {
		fmt.Fprintf(os.Stderr, "mcp-inventory: cannot reach the database: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	srv := &server{repo: inventory.NewRepo(pool)}
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	out := json.NewEncoder(os.Stdout)

	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			continue
		}

		// Notifications carry no id and expect no reply.
		if len(req.ID) == 0 {
			continue
		}

		resp := response{JSONRPC: "2.0", ID: req.ID}

		switch req.Method {
		case "initialize":
			resp.Result = map[string]any{
				"protocolVersion": protocolVersion,
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "fernweh-inventory", "version": "1.0.0"},
			}

		case "tools/list":
			resp.Result = map[string]any{"tools": tools}

		case "tools/call":
			var p struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			}
			_ = json.Unmarshal(req.Params, &p)

			callCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			text, err := srv.callTool(callCtx, p.Name, p.Arguments)
			cancel()

			if err != nil {
				// Tool failures are reported inside the result so the model can
				// read and recover from them, not as transport errors.
				resp.Result = map[string]any{
					"content": []any{map[string]any{"type": "text", "text": "Error: " + err.Error()}},
					"isError": true,
				}
			} else {
				resp.Result = map[string]any{
					"content": []any{map[string]any{"type": "text", "text": text}},
				}
			}

		default:
			resp.Error = &rpcError{Code: -32601, Message: "method not found: " + req.Method}
		}

		if err := out.Encode(resp); err != nil {
			log.Error("write failed", "err", err)
			return
		}
	}
}
