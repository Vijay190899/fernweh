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
