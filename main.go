package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

const idleTimeout = 30 * time.Minute

type ClientInfo struct {
	Addr     *net.UDPAddr
	LastSeen time.Time
}

func main() {
	port := flag.Int("port", 7777, "UDP port for game traffic")
	serverID := flag.String("id", "creative-1", "Unique server ID")
	orchestratorURL := flag.String("orchestrator", "http://localhost:8080", "Orchestrator callback URL")
	flag.Parse()

	addr := net.UDPAddr{
		Port: *port,
		IP:   net.ParseIP("0.0.0.0"),
	}
	conn, err := net.ListenUDP("udp", &addr)
	if err != nil {
		log.Fatalf("Failed to bind UDP port %d: %v", *port, err)
	}
	defer conn.Close()

	log.Printf("[%s] Creative server started on UDP :%d (Idle timeout: %v)", *serverID, *port, idleTimeout)

	var lastActivityUnix int64
	atomic.StoreInt64(&lastActivityUnix, time.Now().Unix())

	clients := make(map[string]*ClientInfo)
	var mu sync.Mutex

	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			now := time.Now()
			mu.Lock()
			for key, client := range clients {
				if now.Sub(client.LastSeen) > 60*time.Second {
					log.Printf("[%s] Player disconnected (timeout): %s", *serverID, key)
					delete(clients, key)
				}
			}
			activePlayers := len(clients)
			if activePlayers > 0 {
				atomic.StoreInt64(&lastActivityUnix, now.Unix())
			}
			mu.Unlock()

			lastAct := time.Unix(atomic.LoadInt64(&lastActivityUnix), 0)
			idleDuration := now.Sub(lastAct)

			if activePlayers == 0 && idleDuration >= idleTimeout {
				log.Printf("[%s] No players for %v. Shutting down creative server...", *serverID, idleDuration.Round(time.Second))
				notifyOrchestrator(*orchestratorURL, *serverID)
				conn.Close()
				os.Exit(0)
			}
		}
	}()

	buf := make([]byte, 2048)
	for {
		n, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			return
		}

		now := time.Now()
		atomic.StoreInt64(&lastActivityUnix, now.Unix())

		clientKey := remoteAddr.String()
		mu.Lock()
		if _, exists := clients[clientKey]; !exists {
			log.Printf("[%s] Player connected: %s", *serverID, clientKey)
		}
		clients[clientKey] = &ClientInfo{
			Addr:     remoteAddr,
			LastSeen: now,
		}
		mu.Unlock()

		msg := string(buf[:n])
		reply := fmt.Sprintf("ACK [%s]: %s", *serverID, msg)
		_, _ = conn.WriteToUDP([]byte(reply), remoteAddr)
	}
}

func notifyOrchestrator(baseURL, serverID string) {
	payload, _ := json.Marshal(map[string]string{"id": serverID})
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(baseURL+"/internal/stopped", "application/json", bytes.NewBuffer(payload))
	if err != nil {
		log.Printf("Warning: could not notify orchestrator: %v", err)
		return
	}
	defer resp.Body.Close()
}
