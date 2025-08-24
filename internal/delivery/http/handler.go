package http

import (
	"encoding/json"
	"net/http"
	"service-faas/internal/core/functions"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"
	httpSwagger "github.com/swaggo/http-swagger"
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

	// --- API Routes ---
	r.Route("/functions", func(r chi.Router) {
		r.Post("/", h.handleAddFunction)
		r.Get("/", h.handleListFunctions)
		r.Post("/{functionID}/execute", h.handleExecuteFunction)
		r.Delete("/{functionID}", h.handleRemoveFunction)
	})

	// --- Swagger Docs Route ---
	r.Get("/docs", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/docs/index.html", http.StatusMovedPermanently)
	})
	r.Get("/docs/*", httpSwagger.WrapHandler)
	return r
}

// @Summary      Add a new function
// @Description  Uploads a Python file, creates a new FaaS function container, and returns its details.
// @Tags         functions
// @Accept       multipart/form-data
// @Produce      json
// @Param        python_file    formData  file   true   "The Python file containing the function handler"
// @Param        function_name  formData  string true   "The name of the function to execute (e.g., 'handle')"
// @Success      201  {object}  functions.Function
// @Failure      400  {string}  string "Bad Request"
// @Failure      500  {string}  string "Internal Server Error"
// @Router       /functions [post]
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

// @Summary      Execute a function
// @Description  Sends a JSON payload to a function and returns the result.
// @Tags         functions
// @Accept       json
// @Produce      json
// @Param        functionID path string true "Function ID"
// @Param        body body string true "Payload for the function"
// @Success      200  {object}  object "{"result": "..."}"
// @Failure      400  {string}  string "Bad Request"
// @Failure      500  {string}  string "Internal Server Error"
// @Router       /functions/{functionID}/execute [post]
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

// @Summary      List all functions
// @Description  Retrieves a list of all registered functions.
// @Tags         functions
// @Produce      json
// @Success      200  {array}   functions.Function
// @Failure      500  {string}  string "Internal Server Error"
// @Router       /functions [get]
func (h *Handler) handleListFunctions(w http.ResponseWriter, r *http.Request) {
	list, err := h.mgr.ListFunctions()
	if err != nil {
		h.lg.Error().Err(err).Msg("list functions")
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// @Summary      Remove a function
// @Description  Stops the function's container and removes its record from the database.
// @Tags         functions
// @Produce      json
// @Param        functionID path string true "Function ID"
// @Success      204  {string}  string "No Content"
// @Failure      500  {string}  string "Internal Server Error"
// @Router       /functions/{functionID} [delete]
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
