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
	// CORS на случай запросов с других доменов
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	orchestratorURL := os.Getenv("ORCHESTRATOR_URL")
	if orchestratorURL == "" {
		// Если оркестратор еще не настроен, возвращаем понятную ошибку в JSON
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": "Переменная ORCHESTRATOR_URL не задана в настройках Vercel",
		})
		return
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
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": "Не удалось связаться с оркестратором: " + err.Error(),
		})
		return
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(respBytes)
}
