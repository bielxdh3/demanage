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
)

const serviceName = "demanage-backend"

type databaseProbe func(context.Context) error

type handlerOptions struct {
	jwtKey                  []byte
	tagReader               customTagRepository
	expenseReader           expenseListRepository
	entryReader             entryListRepository
	cardReader              cardListRepository
	piggyBankListReader     piggyBankListRepository
	piggyBankReadReader     piggyBankReadRepository
	assetTransactionReader  assetTransactionListRepository
	patrimonySettingsReader patrimonySettingsReader
	financialHistoryReader  financialHistoryPageReader
}

func main() {
	jwtSecret := strings.TrimSpace(os.Getenv("JWT_SECRET"))
	if os.Getenv("NODE_ENV") == "production" && len([]byte(jwtSecret)) < 32 {
		fmt.Fprintln(os.Stderr, "JWT_SECRET must contain at least 32 bytes in production")
		os.Exit(1)
	}
	clock, err := contractClock()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	database := newPostgresDatabase(os.Getenv("DATABASE_URL"))
	defer database.Close()
	handler := newHandler(database.Probe, handlerOptions{
		jwtKey:                  []byte(jwtSecret),
		tagReader:               postgresCustomTagRepository{database: database},
		expenseReader:           postgresExpenseRepository{database: database},
		entryReader:             postgresEntryRepository{database: database},
		cardReader:              postgresCardRepository{database: database, now: clock},
		piggyBankListReader:     postgresPiggyBankRepository{database: database, now: clock},
		piggyBankReadReader:     postgresPiggyBankReadRepository{database: database},
		assetTransactionReader:  postgresAssetTransactionRepository{database: database},
		patrimonySettingsReader: postgresPatrimonySettingsRepository{database: database},
		financialHistoryReader:  postgresFinancialHistoryRepository{database: database},
	})
	server := &http.Server{
		Addr:              "127.0.0.1:8888",
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newHandler(probe databaseProbe, options ...handlerOptions) http.Handler {
	mux := http.NewServeMux()
	if len(options) > 0 {
		registerCustomTags(mux, options[0].jwtKey, options[0].tagReader)
		registerFinancialReads(
			mux,
			options[0].jwtKey,
			options[0].tagReader,
			options[0].expenseReader,
			options[0].entryReader,
			options[0].cardReader,
			options[0].piggyBankListReader,
		)
		registerAssetTransactionReads(mux, options[0].jwtKey, options[0].tagReader, options[0].assetTransactionReader)
		registerPatrimonySettings(mux, options[0].jwtKey, options[0].tagReader, options[0].patrimonySettingsReader)
		if options[0].piggyBankReadReader != nil {
			registerPiggyBankReadProjections(mux, options[0].jwtKey, options[0].tagReader, options[0].piggyBankReadReader)
		}
		if options[0].financialHistoryReader != nil {
			financialHistory := financialHistoryHandler(options[0].jwtKey, options[0].tagReader, options[0].financialHistoryReader)
			mux.HandleFunc("GET /financial-history", financialHistory)
			mux.HandleFunc("GET /financial-history/{$}", financialHistory)
		}
	}
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

func writeJSON(w http.ResponseWriter, r *http.Request, status int, value any) {
	body, err := json.Marshal(value)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("X-Powered-By", "Express")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Permissions-Policy", "camera=(), geolocation=(), microphone=()")
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
