package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"time"

	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/mock"
)

type agentService struct {
	pluginDirectory       string
	pluginKeys            map[string]ed25519.PublicKey
	pluginMu              sync.Mutex
	kafkaPasswordKeychain bool
	mqttPasswordKeychain  bool
	amqpPasswordKeychain  bool
	redisPasswordKeychain bool
	mockMu                sync.Mutex
	mockRuntime           *mock.Runtime
	sqliteTarget          string
	sqlPasswords          execution.SQLPasswordBinding
	addr                  string
	pairing               [sha256.Size]byte
	origins               map[string]bool
	history               *historyStore

	mu            sync.Mutex
	sessions      map[[sha256.Size]byte]session
	jobs          map[string]*agentJob
	socketTickets map[[sha256.Size]byte]socketTicket
	socketSlots   chan struct{}
}

type session struct {
	expires time.Time
	origin  string
}

func newAgent(addr, token, origins string, history *historyStore) (*agentService, error) {
	if len(token) < 24 || len(token) > 1024 || strings.ContainsFunc(token, func(c rune) bool { return c < 33 || c > 126 }) {
		return nil, errors.New("set PROTOVOY_AGENT_TOKEN or -token-file to a pairing token of 24 to 1024 printable characters")
	}
	allowed := make(map[string]bool)
	for item := range strings.SplitSeq(origins, ",") {
		origin := strings.TrimSpace(item)
		if origin == "" {
			continue
		}
		parsed, err := url.Parse(origin)
		if err != nil || parsed.Hostname() == "" || parsed.User != nil || origin != parsed.Scheme+"://"+parsed.Host ||
			(parsed.Scheme != "http" && parsed.Scheme != "https" && parsed.Scheme != "tauri") {
			return nil, errors.New("configure exact Local Agent origins without paths, wildcards, or credentials")
		}
		if strings.Contains(parsed.Host, "*") {
			return nil, errors.New("configure exact Local Agent origins without wildcards")
		}
		allowed[origin] = true
	}
	return &agentService{
		addr: addr, pairing: sha256.Sum256([]byte(token)), origins: allowed, history: history,
		sessions: make(map[[sha256.Size]byte]session), jobs: make(map[string]*agentJob),
		socketTickets: make(map[[sha256.Size]byte]socketTicket), socketSlots: make(chan struct{}, 16),
	}, nil
}

func (agent *agentService) authorize(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		remote, _, err := net.SplitHostPort(r.RemoteAddr)
		ip, ipErr := netip.ParseAddr(remote)
		if err != nil || ipErr != nil || !ip.Unmap().IsLoopback() || r.Host != agent.addr {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Add("Vary", "Origin")
		origins := r.Header.Values("Origin")
		if len(origins) > 0 {
			if len(origins) != 1 || !agent.origins[origins[0]] {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origins[0])
			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept, If-Match, If-None-Match, "+versionHeader+", X-ProtoVoy-Client, X-ProtoVoy-Client-Version")
				w.WriteHeader(http.StatusNoContent)
				return
			}
			w.Header().Set("Access-Control-Expose-Headers", "ETag")
		}
		// Upgrades authenticate with their one-use ticket after these same
		// loopback, Host and Origin checks.
		ticketUpgrade := r.Method == http.MethodGet && r.URL.Path == execution.AgentWebSocketPath
		if r.URL.Path != "/health" && !ticketUpgrade {
			bearer, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			key := sha256.Sum256([]byte(bearer))
			pairing := subtle.ConstantTimeCompare(key[:], agent.pairing[:]) == 1
			if !ok || (!pairing && (r.URL.Path == "/v1/session" || !agent.validSession(key, r.Header.Get("Origin")))) {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			if version := r.Header.Get(versionHeader); version != "" && version != apiVersion {
				http.Error(w, "protocol API version mismatch", http.StatusUpgradeRequired)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (agent *agentService) validSession(key [sha256.Size]byte, origin string) bool {
	agent.mu.Lock()
	defer agent.mu.Unlock()
	entry, exists := agent.sessions[key]
	if exists && !time.Now().Before(entry.expires) {
		delete(agent.sessions, key)
		return false
	}
	return exists && entry.origin == origin
}

func (agent *agentService) createSession(w http.ResponseWriter, r *http.Request) {
	const lifetime = 8 * time.Hour
	var random [32]byte
	rand.Read(random[:])
	token := hex.EncodeToString(random[:])
	now := time.Now()
	agent.mu.Lock()
	for key, entry := range agent.sessions {
		if !now.Before(entry.expires) {
			delete(agent.sessions, key)
		}
	}
	if len(agent.sessions) >= 128 {
		agent.mu.Unlock()
		http.Error(w, "too many active sessions", http.StatusTooManyRequests)
		return
	}
	agent.sessions[sha256.Sum256([]byte(token))] = session{expires: now.Add(lifetime), origin: r.Header.Get("Origin")}
	agent.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "expiresInSeconds": int(lifetime.Seconds())})
}
