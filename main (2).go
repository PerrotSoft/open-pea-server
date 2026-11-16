package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"
)

type ServerInstance struct {
	ID        string    `json:"id"`
	Host      string    `json:"host"`
	Port      int       `json:"port"`
	IP        string    `json:"ip"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	cmd       *exec.Cmd
}

type Orchestrator struct {
	mu           sync.Mutex
	servers      map[string]*ServerInstance
	nextPort     int
	publicHost   string
	serverBinary string
	apiSecret    string
}

func NewOrchestrator() *Orchestrator {
	publicHost := os.Getenv("PUBLIC_HOST")
	if publicHost == "" {
		publicHost = "127.0.0.1" // Замените на реальный белый IP вашего VPS
	}
	serverBinary := os.Getenv("GAME_SERVER_BIN")
	if serverBinary == "" {
		serverBinary = "../game-server/game-server"
	}
	apiSecret := os.Getenv("API_SECRET")
	if apiSecret == "" {
		apiSecret = "super-secret-token"
	}

	return &Orchestrator{
		servers:      make(map[string]*ServerInstance),
		nextPort:     7777,
		publicHost:   publicHost,
		serverBinary: serverBinary,
		apiSecret:    apiSecret,
	}
}

func (o *Orchestrator) allocatePort() int {
	used := make(map[int]bool)
	for _, s := range o.servers {
		used[s.Port] = true
	}
	port := o.nextPort
	for used[port] {
		port++
	}
	o.nextPort = port + 1
	return port
}

func (o *Orchestrator) handleCreateServer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if r.Header.Get("Authorization") != "Bearer "+o.apiSecret {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	o.mu.Lock()
	// Если уже есть запущенный креатив-сервер, возвращаем его (или уберите этот блок, если нужно создавать новый на каждый клик)
	for _, srv := range o.servers {
		if srv.Status == "running" {
			o.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(srv)
			return
		}
	}

	port := o.allocatePort()
	id := fmt.Sprintf("creative-%d", port)
	orchestratorAddr := "http://127.0.0.1:8080"

	cmd := exec.Command(
		o.serverBinary,
		fmt.Sprintf("-port=%d", port),
		fmt.Sprintf("-id=%s", id),
		fmt.Sprintf("-orchestrator=%s", orchestratorAddr),
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		o.mu.Unlock()
		log.Printf("Failed to start game server: %v", err)
		http.Error(w, "Failed to start game server process", http.StatusInternalServerError)
		return
	}

	instance := &ServerInstance{
		ID:        id,
		Host:      fmt.Sprintf("%s.creative.local", id),
		Port:      port,
		IP:        fmt.Sprintf("%s:%d", o.publicHost, port),
		Status:    "running",
		CreatedAt: time.Now(),
		cmd:       cmd,
	}
	o.servers[id] = instance
	o.mu.Unlock()

	// Отслеживаем завершение процесса (например, после 30 минут простоя)
	go func(inst *ServerInstance) {
		_ = inst.cmd.Wait()
		o.mu.Lock()
		delete(o.servers, inst.ID)
		o.mu.Unlock()
		log.Printf("[Orchestrator] Server %s exited and removed from active list", inst.ID)
	}(instance)

	log.Printf("[Orchestrator] Spawned creative server %s at %s", instance.ID, instance.IP)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(instance)
}

func (o *Orchestrator) handleServerStopped(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err == nil && body.ID != "" {
		o.mu.Lock()
		delete(o.servers, body.ID)
		o.mu.Unlock()
		log.Printf("[Orchestrator] Received idle shutdown notice for %s", body.ID)
	}
	w.WriteHeader(http.StatusOK)
}

func (o *Orchestrator) handleListServers(w http.ResponseWriter, r *http.Request) {
	o.mu.Lock()
	list := make([]*ServerInstance, 0, len(o.servers))
	for _, s := range o.servers {
		list = append(list, s)
	}
	o.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}

func main() {
	orch := NewOrchestrator()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/create", orch.handleCreateServer)
	mux.HandleFunc("/api/servers", orch.handleListServers)
	mux.HandleFunc("/internal/stopped", orch.handleServerStopped)

	addr := ":8080"
	log.Printf("[Orchestrator] Always-on daemon listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
