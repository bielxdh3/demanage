package main

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const serviceName = "demanage-backend"

type databaseProbe func(context.Context) error

func main() {
	server := &http.Server{
		Addr:              "127.0.0.1:8888",
		Handler:           newHandler(probeDatabase),
		ReadHeaderTimeout: 5 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newHandler(probe databaseProbe) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, r, http.StatusOK, struct {
			Status    string `json:"status"`
			Service   string `json:"service"`
			Timestamp string `json:"timestamp"`
		}{"ok", serviceName, time.Now().UTC().Format("2006-01-02T15:04:05.000Z")})
	})
	mux.HandleFunc("GET /ready", func(w http.ResponseWriter, r *http.Request) {
		if err := probe(r.Context()); err != nil {
			writeJSON(w, r, http.StatusServiceUnavailable, struct {
				Status  string `json:"status"`
				Service string `json:"service"`
				DB      string `json:"db"`
			}{"unavailable", serviceName, "unavailable"})
			return
		}
		writeJSON(w, r, http.StatusOK, struct {
			Status  string `json:"status"`
			Service string `json:"service"`
			DB      string `json:"db"`
		}{"ok", serviceName, "ready"})
	})
	return mux
}

func probeDatabase(requestContext context.Context) error {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return fmt.Errorf("DATABASE_URL is not configured")
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return err
	}
	config.MaxConns = 1
	ctx, cancel := context.WithTimeout(requestContext, 2*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return err
	}
	defer pool.Close()
	connection, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer connection.Release()
	_, err = connection.Exec(ctx, "SELECT 1")
	return err
}

func writeJSON(w http.ResponseWriter, r *http.Request, status int, value any) {
	body, err := json.Marshal(value)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("X-Powered-By", "Express")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	etag := expressETag(body)
	w.Header().Set("ETag", etag)
	if status >= 200 && status < 300 && (r.Method == http.MethodGet || r.Method == http.MethodHead) && isFresh(r.Header.Get("If-None-Match"), etag) {
		w.Header().Del("Content-Type")
		w.Header().Del("Content-Length")
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.WriteHeader(status)
	if r.Method != http.MethodHead {
		_, _ = w.Write(body)
	}
}

func expressETag(body []byte) string {
	hash := sha1.Sum(body)
	encoded := base64.StdEncoding.EncodeToString(hash[:])
	return fmt.Sprintf("W/\"%x-%s\"", len(body), encoded[:27])
}

func isFresh(ifNoneMatch, etag string) bool {
	for _, candidate := range strings.Split(ifNoneMatch, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" || strings.TrimPrefix(candidate, "W/") == strings.TrimPrefix(etag, "W/") {
			return candidate != ""
		}
	}
	return false
}
