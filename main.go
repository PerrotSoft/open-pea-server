package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"time"
)

//go:embed public/*
var publicFS embed.FS

const idleTimeout = 30 * time.Minute

type CreativeServer struct {
	ID           string    `json:"id"`
	Host         string    `json:"host"`
	IP           string    `json:"ip"`
	Status       string    `json:"status"`
	Players      int       `json:"players"`
	CreatedAt    time.Time `json:"created_at"`
	LastActivity time.Time `json:"last_activity"`
	ExpiresIn    string    `json:"expires_in"`
}

var (
	mu            sync.Mutex
	activeServers = make(map[string]*CreativeServer)
)

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/create-server", handleCreateServer)
	mux.HandleFunc("/api/heartbeat", handleHeartbeat)
	mux.HandleFunc("/api/servers", handleListServers)

	subFS, err := fs.Sub(publicFS, "public")
	if err != nil {
		log.Fatal(err)
	}
	mux.Handle("/", http.FileServer(http.FS(subFS)))

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	log.Printf("Open Pea Server listening on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}

func cleanupExpired() {
	now := time.Now()
	for id, srv := range activeServers {
		if srv.Players == 0 && now.Sub(srv.LastActivity) >= idleTimeout {
			delete(activeServers, id)
		}
	}
}

func handleCreateServer(w http.ResponseWriter, r *http.Request) {
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

	domain := r.Host
	if domain == "" {
		domain = "open-pea-server.vercel.app"
	}

	// Визначаємо IP-адресу домену за допомогою DNS-резолвера
	resolvedIP := domain
	// Відкидаємо порт, якщо він є в r.Host
	hostOnly, _, err := net.SplitHostPort(domain)
	if err == nil {
		domain = hostOnly
	}

	ips, err := net.LookupIP(domain)
	if err == nil && len(ips) > 0 {
		// Шукаємо IPv4 для зручності підключення
		for _, ip := range ips {
			if ipv4 := ip.To4(); ipv4 != nil {
				resolvedIP = ipv4.String()
				break
			}
		}
	}

	mu.Lock()
	defer mu.Unlock()

	cleanupExpired()

	for _, srv := range activeServers {
		if srv.Status == "running" {
			remaining := idleTimeout - time.Since(srv.LastActivity)
			if remaining < 0 {
				remaining = 0
			}
			srv.ExpiresIn = fmt.Sprintf("%d хв", int(remaining.Minutes()))
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(srv)
			return
		}
	}

	// Використовуємо порт 27015 за замовчуванням
	gamePort := 27015
	id := fmt.Sprintf("creative-%d", gamePort)
	now := time.Now()

	srv := &CreativeServer{
		ID:           id,
		Host:         fmt.Sprintf("%s:%d", domain, gamePort),
		IP:           fmt.Sprintf("%s:%d", resolvedIP, gamePort),
		Status:       "running",
		Players:      0,
		CreatedAt:    now,
		LastActivity: now,
		ExpiresIn:    "30 хв",
	}
	activeServers[id] = srv

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(srv)
}

func handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ID      string `json:"id"`
		Players int    `json:"players"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	mu.Lock()
	defer mu.Unlock()

	cleanupExpired()

	srv, exists := activeServers[req.ID]
	if !exists {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Сервер закрито через неактивність (30 хв)"})
		return
	}

	srv.Players = req.Players
	if req.Players > 0 {
		srv.LastActivity = time.Now()
	}

	remaining := idleTimeout - time.Since(srv.LastActivity)
	if remaining < 0 {
		remaining = 0
	}
	srv.ExpiresIn = fmt.Sprintf("%d хв", int(remaining.Minutes()))

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(srv)
}

func handleListServers(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()

	cleanupExpired()

	list := make([]*CreativeServer, 0, len(activeServers))
	for _, s := range activeServers {
		remaining := idleTimeout - time.Since(s.LastActivity)
		if remaining < 0 {
			remaining = 0
		}
		s.ExpiresIn = fmt.Sprintf("%d хв", int(remaining.Minutes()))
		list = append(list, s)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}
