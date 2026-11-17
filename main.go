// Pengine Server — лобби + UDP-ретранслятор для Pengine Arena (порт игры 27015).
//
//	server                       — лобби на :8080 (список комнат по ID)
//	server -forward 1.2.3.4:27015 — ещё и пробрасывает игровой UDP на хост игры
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Room struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	IP      string    `json:"ip"`
	Port    int       `json:"port"`
	Mode    string    `json:"mode"`
	Map     string    `json:"map"`
	Players int       `json:"players"`
	Seen    time.Time `json:"-"`
}

var (
	mu    sync.Mutex
	rooms = map[string]*Room{}
)

const ttl = 30 * time.Second

func newID() string {
	const a = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	for {
		b := make([]byte, 5)
		for i := range b {
			b[i] = a[rand.Intn(len(a))]
		}
		if _, ok := rooms[string(b)]; !ok {
			return string(b)
		}
	}
}

func clientIP(r *http.Request) string {
	if f := r.Header.Get("X-Forwarded-For"); f != "" {
		return strings.TrimSpace(strings.Split(f, ",")[0])
	}
	h, _, _ := net.SplitHostPort(r.RemoteAddr)
	return h
}

func gc() {
	for {
		time.Sleep(5 * time.Second)
		mu.Lock()
		for k, v := range rooms {
			if time.Since(v.Seen) > ttl {
				delete(rooms, k)
			}
		}
		mu.Unlock()
	}
}

// POST/GET /register?name=&port=27015&mode=&map=&players=[&id=] — хост шлёт раз в 10 c.
func register(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	port, _ := strconv.Atoi(q.Get("port"))
	if port <= 0 || port > 65535 {
		port = 27015
	}
	pl, _ := strconv.Atoi(q.Get("players"))
	mu.Lock()
	defer mu.Unlock()
	id := strings.ToUpper(q.Get("id"))
	rm := rooms[id]
	if rm == nil {
		id = newID()
		rm = &Room{ID: id}
		rooms[id] = rm
	}
	name := q.Get("name")
	if len(name) > 32 {
		name = name[:32]
	}
	ip := q.Get("ip")
	if ip == "" {
		ip = clientIP(r)
	}
	*rm = Room{ID: id, Name: name, IP: ip, Port: port, Mode: q.Get("mode"), Map: q.Get("map"), Players: pl, Seen: time.Now()}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rm)
}

func list(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	out := []*Room{}
	for _, v := range rooms {
		out = append(out, v)
	}
	mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

// GET /room/ID -> "ip:port" (текстом)
func room(w http.ResponseWriter, r *http.Request) {
	id := strings.ToUpper(strings.TrimPrefix(r.URL.Path, "/room/"))
	mu.Lock()
	rm := rooms[id]
	mu.Unlock()
	if rm == nil {
		http.Error(w, "нет такой комнаты", 404)
		return
	}
	fmt.Fprintf(w, "%s:%d", rm.IP, rm.Port)
}

func index(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	mu.Lock()
	defer mu.Unlock()
	fmt.Fprint(w, "<meta charset=utf-8><title>Pengine Server</title><body style='font-family:sans-serif'><h2>Pengine Server</h2><table border=1 cellpadding=6><tr><th>ID<th>Название<th>Адрес<th>Режим<th>Карта<th>Игроков")
	for _, v := range rooms {
		fmt.Fprintf(w, "<tr><td><b>%s</b><td>%s<td>%s:%d<td>%s<td>%s<td>%d", v.ID, v.Name, v.IP, v.Port, v.Mode, v.Map, v.Players)
	}
	fmt.Fprint(w, "</table><p>API: /rooms, /room/ID, /register?name=..&port=27015")
}

// ---- UDP-ретранслятор: клиенты -> этот сервер -> хост игры ----
func forward(listen, target string) {
	pc, err := net.ListenPacket("udp", listen)
	if err != nil {
		log.Fatalf("UDP %s: %v", listen, err)
	}
	dst, err := net.ResolveUDPAddr("udp", target)
	if err != nil {
		log.Fatalf("адрес хоста %s: %v", target, err)
	}
	type sess struct {
		c    *net.UDPConn
		seen time.Time
	}
	var m sync.Mutex
	ss := map[string]*sess{}
	go func() {
		for {
			time.Sleep(10 * time.Second)
			m.Lock()
			for k, s := range ss {
				if time.Since(s.seen) > 60*time.Second {
					s.c.Close()
					delete(ss, k)
				}
			}
			m.Unlock()
		}
	}()
	log.Printf("Ретранслятор UDP %s -> %s", listen, target)
	buf := make([]byte, 65535)
	for {
		n, from, err := pc.ReadFrom(buf)
		if err != nil {
			continue
		}
		key := from.String()
		m.Lock()
		s := ss[key]
		if s == nil {
			c, err := net.DialUDP("udp", nil, dst)
			if err != nil {
				m.Unlock()
				continue
			}
			s = &sess{c: c}
			ss[key] = s
			go func(c *net.UDPConn, back net.Addr) {
				b := make([]byte, 65535)
				for {
					k, err := c.Read(b)
					if err != nil {
						return
					}
					pc.WriteTo(b[:k], back)
				}
			}(c, from)
		}
		s.seen = time.Now()
		m.Unlock()
		s.c.Write(buf[:n])
	}
}

func ips() {
	log.Println("Локальные адреса этого компьютера:")
	as, _ := net.InterfaceAddrs()
	for _, a := range as {
		if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() != nil && !ipn.IP.IsLoopback() {
			log.Printf("   %s", ipn.IP)
		}
	}
	cl := http.Client{Timeout: 4 * time.Second}
	if r, err := cl.Get("https://api.ipify.org"); err == nil {
		b, _ := io.ReadAll(r.Body)
		r.Body.Close()
		log.Printf("Внешний (публичный) адрес: %s", strings.TrimSpace(string(b)))
	} else {
		log.Println("Внешний адрес узнать не удалось (нет интернета?)")
	}
}

func main() {
	httpAddr := flag.String("http", ":8080", "адрес лобби (HTTP)")
	udp := flag.String("udp", ":27015", "UDP-порт ретранслятора")
	fwd := flag.String("forward", "", "адрес хоста игры ip:port — включает ретранслятор")
	flag.Parse()
	log.SetFlags(log.Ltime)
	ips()
	go gc()
	if *fwd != "" {
		go forward(*udp, *fwd)
	}
	http.HandleFunc("/", index)
	http.HandleFunc("/register", register)
	http.HandleFunc("/rooms", list)
	http.HandleFunc("/room/", room)
	log.Printf("Лобби: http://localhost%s  (порт %s открыть в файрволе)", *httpAddr, *httpAddr)
	log.Fatal(http.ListenAndServe(*httpAddr, nil))
}
