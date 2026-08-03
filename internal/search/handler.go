package search

import (
	"context"
	"net/http"
	"strings"

	"fernweh/internal/platform/httpx"
)

// Handler exposes the search API over plain net/http.
type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/search", h.search)
	mux.HandleFunc("POST /v1/compare", h.compare)
	mux.HandleFunc("GET /v1/compare/personas", h.personas)
}

type compareRequest struct {
	Query   string `json:"query"`
	Persona string `json:"persona"`
}

func (h *Handler) compare(w http.ResponseWriter, r *http.Request) {
	var req compareRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.Query = strings.TrimSpace(req.Query)
	if req.Query == "" || len(req.Query) > 500 {
		httpx.Error(w, http.StatusBadRequest, "query must be 1-500 characters")
		return
	}
	if h.svc.comparer == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "comparison unavailable")
		return
	}

	// Ten a side: enough to show movement, short enough to read without
	// scrolling one column against the other.
	resp, err := h.svc.Compare(r.Context(), req.Query, req.Persona, 10)
	if err != nil {
		// An unknown persona is the caller's mistake, and saying "upstream
		// unavailable" would send someone looking for an outage that is not
		// there.
		if httpx.ClientFault(err) {
			httpx.Error(w, http.StatusBadRequest, "unknown persona")
			return
		}
		httpx.Error(w, http.StatusBadGateway, "comparison failed")
		return
	}
	httpx.JSON(w, http.StatusOK, resp)
}

// personas proxies the declared persona list so the page can render the
// choices from source rather than hardcoding a copy that can drift.
func (h *Handler) personas(w http.ResponseWriter, r *http.Request) {
	if h.svc.comparer == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "comparison unavailable")
		return
	}
	ps, err := h.svc.comparer.Personas(r.Context())
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "personas unavailable")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"personas": ps})
}

type searchRequest struct {
	Query     string `json:"query"`
	SessionID string `json:"session_id"`
}

func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	var req searchRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.Query = strings.TrimSpace(req.Query)
	if req.Query == "" || len(req.Query) > 500 {
		httpx.Error(w, http.StatusBadRequest, "query must be 1-500 characters")
		return
	}

	resp, err := h.svc.Search(r.Context(), req.Query, req.SessionID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "search failed")
		return
	}
	httpx.JSON(w, http.StatusOK, resp)
}

// RankingClient calls the ranking service over HTTP with trace propagation.
type RankingClient struct {
	baseURL string
}

func NewRankingClient(baseURL string) *RankingClient { return &RankingClient{baseURL: baseURL} }

func (c *RankingClient) Rank(ctx context.Context, sessionID string, items []RankItem) ([]RankedItem, error) {
	var out struct {
		Items []RankedItem `json:"items"`
	}
	err := httpx.PostJSON(ctx, c.baseURL+"/v1/rank", map[string]any{
		"session_id": sessionID,
		"items":      items,
	}, &out)
	if err != nil {
		return nil, err
	}
	return out.Items, nil
}

func (c *RankingClient) Compare(ctx context.Context, persona string, items []RankItem) (Comparison, error) {
	var out Comparison
	err := httpx.PostJSON(ctx, c.baseURL+"/v1/rank/compare", map[string]any{
		"persona": persona,
		"items":   items,
	}, &out)
	return out, err
}

func (c *RankingClient) Personas(ctx context.Context) ([]map[string]any, error) {
	var out struct {
		Personas []map[string]any `json:"personas"`
	}
	if err := httpx.GetJSON(ctx, c.baseURL+"/v1/rank/personas", &out); err != nil {
		return nil, err
	}
	return out.Personas, nil
}
