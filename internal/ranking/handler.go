package ranking

import (
	"log/slog"
	"net/http"

	"fernweh/internal/platform/httpx"
)

// Handler exposes rank, signal ingestion, and profile inspection.
type Handler struct {
	store *SignalStore
	log   *slog.Logger
}

func NewHandler(store *SignalStore, log *slog.Logger) *Handler {
	return &Handler{store: store, log: log}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/rank", h.rank)
	mux.HandleFunc("POST /v1/rank/compare", h.compare)
	mux.HandleFunc("GET /v1/rank/personas", h.personas)
	mux.HandleFunc("POST /v1/signals", h.signals)
	mux.HandleFunc("GET /v1/profile/{session}", h.profile)
}

type rankRequest struct {
	SessionID string `json:"session_id"`
	Items     []Item `json:"items"`
}

func (h *Handler) rank(w http.ResponseWriter, r *http.Request) {
	var req rankRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if len(req.Items) == 0 || len(req.Items) > 200 {
		httpx.Error(w, http.StatusBadRequest, "items must be 1-200")
		return
	}

	// Profile load failure means rank unpersonalized, not fail: the search
	// path must survive a Redis hiccup.
	profile, err := h.store.Load(r.Context(), req.SessionID)
	if err != nil {
		h.log.WarnContext(r.Context(), "profile load failed, ranking cold", "err", err)
		profile = Profile{}
	}

	ranked := Rank(req.Items, profile)
	h.log.InfoContext(r.Context(), "ranked", "session", req.SessionID,
		"items", len(ranked), "profile_events", profile.Events)
	httpx.JSON(w, http.StatusOK, map[string]any{"items": ranked})
}

type compareRequest struct {
	Persona string `json:"persona"`
	Items   []Item `json:"items"`
}

// compare ranks one candidate set twice, cold and against a declared persona.
// The persona is looked up by name rather than accepted as a profile from the
// caller: the point of the comparison is that both sides used a profile that
// is written down in source and can be checked.
func (h *Handler) compare(w http.ResponseWriter, r *http.Request) {
	var req compareRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if len(req.Items) == 0 || len(req.Items) > 200 {
		httpx.Error(w, http.StatusBadRequest, "items must be 1-200")
		return
	}
	persona, ok := PersonaByName(req.Persona)
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "unknown persona")
		return
	}

	cmp := Compare(req.Items, persona)
	h.log.InfoContext(r.Context(), "compared", "persona", persona.Name,
		"items", cmp.Compared, "moved", cmp.Moved)
	httpx.JSON(w, http.StatusOK, cmp)
}

func (h *Handler) personas(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, map[string]any{"personas": Personas()})
}

func (h *Handler) signals(w http.ResponseWriter, r *http.Request) {
	var ev Event
	if err := httpx.Decode(r, &ev); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if ev.SessionID == "" || len(ev.SessionID) > 100 {
		httpx.Error(w, http.StatusBadRequest, "session_id required")
		return
	}
	if err := h.store.Record(r.Context(), ev); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// profile powers the demo UI's "what the engine knows about you" panel -
// the same transparency a real platform owes its users.
func (h *Handler) profile(w http.ResponseWriter, r *http.Request) {
	p, err := h.store.Load(r.Context(), r.PathValue("session"))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "profile unavailable")
		return
	}
	httpx.JSON(w, http.StatusOK, p)
}
