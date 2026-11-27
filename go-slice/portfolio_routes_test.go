package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewHandlerRegistersPortfolioGetRoutesBehindAuthentication(t *testing.T) {
	handler := newHandler(
		func(context.Context) error { return nil },
		handlerOptions{
			jwtKey:    testJWTKey,
			tagReader: &fakeCustomTagRepository{userExists: true},
		},
	)

	for _, path := range []string{"/assets/BTC/transactions", "/patrimony/settings"} {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("unauthenticated response = %d %s, want 401", response.Code, response.Body.String())
			}
		})
	}
}
