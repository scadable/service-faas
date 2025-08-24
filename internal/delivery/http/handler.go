package http

import (
	"encoding/json"
	"net/http"
	"service-faas/internal/core/functions"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"
)

type Handler struct {
	mgr *functions.Manager
	lg  zerolog.Logger
}

func NewHandler(mgr *functions.Manager, lg zerolog.Logger) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	h := &Handler{mgr: mgr, lg: lg}

	r.Route("/functions", func(r chi.Router) {
		r.Post("/", h.handleAddFunction)
		r.Get("/", h.handleListFunctions)
		r.Post("/{functionID}/execute", h.handleExecuteFunction)
		r.Delete("/{functionID}", h.handleRemoveFunction)
	})

	return r
}

// handleAddFunction handles uploading a .py file and creating a function.
func (h *Handler) handleAddFunction(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil { // 10 MB max
		http.Error(w, `{"error": "invalid form data"}`, http.StatusBadRequest)
		return
	}
	file, _, err := r.FormFile("python_file")
	if err != nil {
		http.Error(w, `{"error": "missing 'python_file' in form"}`, http.StatusBadRequest)
		return
	}
	defer file.Close()

	functionName := r.FormValue("function_name")
	if functionName == "" {
		http.Error(w, `{"error": "missing 'function_name' in form"}`, http.StatusBadRequest)
		return
	}

	fn, err := h.mgr.AddFunction(r.Context(), functionName, file)
	if err != nil {
		h.lg.Error().Err(err).Msg("add function")
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, fn)
}

// handleExecuteFunction runs a function with a given payload.
func (h *Handler) handleExecuteFunction(w http.ResponseWriter, r *http.Request) {
	functionID := chi.URLParam(r, "functionID")
	var req struct {
		Payload string `json:"payload"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "invalid json body"}`, http.StatusBadRequest)
		return
	}

	result, err := h.mgr.ExecuteFunction(r.Context(), functionID, req.Payload)
	if err != nil {
		h.lg.Error().Err(err).Msg("execute function")
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]json.RawMessage{"result": result})
}

// handleListFunctions lists all created functions.
func (h *Handler) handleListFunctions(w http.ResponseWriter, r *http.Request) {
	list, err := h.mgr.ListFunctions()
	if err != nil {
		h.lg.Error().Err(err).Msg("list functions")
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// handleRemoveFunction deletes a function.
func (h *Handler) handleRemoveFunction(w http.ResponseWriter, r *http.Request) {
	functionID := chi.URLParam(r, "functionID")
	if err := h.mgr.RemoveFunction(r.Context(), functionID); err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
