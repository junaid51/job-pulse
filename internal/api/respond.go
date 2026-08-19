package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("write response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// serverError logs the cause and tells the client only that it failed: a SQL
// error is for the log, not for the phone.
func serverError(w http.ResponseWriter, doing string, err error) {
	slog.Error(doing, "error", err)
	writeError(w, http.StatusInternalServerError, doing+" failed")
}
