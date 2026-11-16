package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"time"
)

type CreateServerRequest struct {
	MaxPlayers int    `json:"max_players"`
	GameType   string `json:"game_type"`
}

func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	orchestratorURL := os.Getenv("ORCHESTRATOR_URL")
	if orchestratorURL == "" {
		orchestratorURL = "http://YOUR_VPS_PUBLIC_IP:8080"
	}
	apiSecret := os.Getenv("API_SECRET")
	if apiSecret == "" {
		apiSecret = "super-secret-token"
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	req, err := http.NewRequest(http.MethodPost, orchestratorURL+"/api/create", bytes.NewBuffer(bodyBytes))
	if err != nil {
		http.Error(w, "Failed to build upstream request", http.StatusInternalServerError)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiSecret)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "Orchestrator is unreachable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(respBytes)
}
