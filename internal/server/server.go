package server

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"

	"github.com/mathewepstein/devboard/internal/config"
	"github.com/mathewepstein/devboard/internal/seen"
	"github.com/mathewepstein/devboard/internal/state"
	"github.com/mathewepstein/devboard/web"
)

func New(cfg *config.Config, store *state.Store, sn *seen.Store) (http.Handler, error) {
	if store.Get().Repos == nil {
		store.Set(state.Seed(cfg))
	}
	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServerFS(web.FS))
	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(store.Get().WithSeen(sn.Has))
	})
	mux.HandleFunc("POST /api/ack", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			IDs []string `json:"ids"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil || len(body.IDs) == 0 {
			http.Error(w, "expected {\"ids\":[...]}", http.StatusBadRequest)
			return
		}
		if err := sn.Mark(body.IDs...); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	return guardLocalhost(mux), nil
}

func guardLocalhost(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		host = strings.Trim(host, "[]")
		if host != "localhost" && host != "127.0.0.1" && host != "::1" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
