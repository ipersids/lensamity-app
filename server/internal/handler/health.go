package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

type healthResponse struct {
	Status string `json:"status"`
	Time   string `json:"time"`
}

func HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	body := healthResponse{
		Status: "healthy",
		Time:   time.Now().UTC().Format(time.RFC3339),
	}
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Error("failed to encode health check body", "error", err)
	}
}
