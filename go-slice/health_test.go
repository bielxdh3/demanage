package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
)

func TestHealthContractMatchesExpress(t *testing.T) {
	calls := 0
	handler := newHandler(func(context.Context) error {
		calls++
		return errors.New("database password must not escape")
	})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	assertExpressJSONHeaders(t, response)
	var body struct {
		Status    string `json:"status"`
		Service   string `json:"service"`
		Timestamp string `json:"timestamp"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "ok" || body.Service != serviceName || !regexp.MustCompile(`^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d\.\d{3}Z$`).MatchString(body.Timestamp) {
		t.Fatalf("unexpected /health body: %s", response.Body.String())
	}
	if calls != 0 {
		t.Fatalf("liveness called database probe %d times", calls)
	}
}

func TestReadyContractMatchesExpressWhenDatabaseIsUp(t *testing.T) {
	calls := 0
	handler := newHandler(func(context.Context) error { calls++; return nil })
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/ready", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	assertExpressJSONHeaders(t, response)
	if got, want := response.Body.String(), `{"status":"ok","service":"demanage-backend","db":"ready"}`; got != want {
		t.Fatalf("body = %s, want %s", got, want)
	}
	if got, want := response.Header().Get("ETag"), `W/"39-G5ntxmnYDgbHk9jilPE0XNx/Fo8"`; got != want {
		t.Fatalf("ETag = %q, want %q", got, want)
	}
	if calls != 1 {
		t.Fatalf("database probe calls = %d, want 1", calls)
	}
}

func TestReadyContractMatchesExpressWhenDatabaseIsDown(t *testing.T) {
	handler := newHandler(func(context.Context) error { return errors.New("private connection details") })
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/ready", nil))

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
	assertExpressJSONHeaders(t, response)
	if got, want := response.Body.String(), `{"status":"unavailable","service":"demanage-backend","db":"unavailable"}`; got != want {
		t.Fatalf("body = %s, want %s", got, want)
	}
	if regexp.MustCompile(`(?i)private|connection`).Match(response.Body.Bytes()) {
		t.Fatalf("database error leaked in body: %s", response.Body.String())
	}
}

func TestDatabaseProbeFailsClosedWhenURLIsNotConfigured(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	if err := probeDatabase(context.Background()); err == nil {
		t.Fatal("probeDatabase() succeeded without DATABASE_URL")
	}
}

func TestReadyPropagatesRequestContext(t *testing.T) {
	probeSawCanceledContext := false
	handler := newHandler(func(ctx context.Context) error {
		probeSawCanceledContext = ctx.Err() != nil
		return ctx.Err()
	})
	requestContext, cancel := context.WithCancel(context.Background())
	cancel()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/ready", nil).WithContext(requestContext))
	if !probeSawCanceledContext || response.Code != http.StatusServiceUnavailable {
		t.Fatalf("canceled readiness context observed=%t status=%d", probeSawCanceledContext, response.Code)
	}
}

func TestReadySupportsConditionalGet(t *testing.T) {
	handler := newHandler(func(context.Context) error { return nil })
	first := httptest.NewRecorder()
	handler.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/ready", nil))
	conditional := httptest.NewRequest(http.MethodGet, "/ready", nil)
	conditional.Header.Set("If-None-Match", first.Header().Get("ETag"))
	second := httptest.NewRecorder()
	handler.ServeHTTP(second, conditional)
	if second.Code != http.StatusNotModified || second.Body.Len() != 0 {
		t.Fatalf("conditional response = %d %q, want empty 304", second.Code, second.Body.String())
	}

	down := httptest.NewRecorder()
	newHandler(func(context.Context) error { return errors.New("database down") }).ServeHTTP(down, httptest.NewRequest(http.MethodGet, "/ready", nil))
	failedConditional := httptest.NewRequest(http.MethodGet, "/ready", nil)
	failedConditional.Header.Set("If-None-Match", down.Header().Get("ETag"))
	stillUnavailable := httptest.NewRecorder()
	newHandler(func(context.Context) error { return errors.New("database down") }).ServeHTTP(stillUnavailable, failedConditional)
	if stillUnavailable.Code != http.StatusServiceUnavailable {
		t.Fatalf("conditional failure status = %d, want %d", stillUnavailable.Code, http.StatusServiceUnavailable)
	}
}

func TestReadySupportsHeadWithoutBody(t *testing.T) {
	handler := newHandler(func(context.Context) error { return nil })
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodHead, "/ready", nil))
	if response.Code != http.StatusOK || response.Body.Len() != 0 {
		t.Fatalf("HEAD response = %d %q, want empty 200", response.Code, response.Body.String())
	}
	if got, want := response.Header().Get("Content-Length"), "57"; got != want {
		t.Fatalf("Content-Length = %q, want %q", got, want)
	}
}

func assertExpressJSONHeaders(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if got, want := response.Header().Get("X-Powered-By"), "Express"; got != want {
		t.Errorf("X-Powered-By = %q, want %q", got, want)
	}
	if got, want := response.Header().Get("Content-Type"), "application/json; charset=utf-8"; got != want {
		t.Errorf("Content-Type = %q, want %q", got, want)
	}
	if got := response.Header().Get("ETag"); got == "" {
		t.Error("ETag missing; Express JSON responses include one")
	} else if !regexp.MustCompile(`^W/"[0-9a-f]+-[A-Za-z0-9+/]{27}"$`).MatchString(got) {
		t.Errorf("ETag = %q, want Express-style weak ETag", got)
	}
	if got, want := response.Header().Get("Content-Length"), response.Body.Len(); got != fmt.Sprint(want) {
		t.Errorf("Content-Length = %q, want %d", got, want)
	}
}
