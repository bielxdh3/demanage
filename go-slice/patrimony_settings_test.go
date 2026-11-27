package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

type fakePatrimonySettingsReader struct {
	userID   string
	settings *patrimonySettings
	err      error
}

func (reader *fakePatrimonySettingsReader) ReadPatrimonySettings(_ context.Context, userID string) (*patrimonySettings, error) {
	reader.userID = userID
	return reader.settings, reader.err
}

func TestPatrimonySettingsRouteReturnsNullOrPrismaProjection(t *testing.T) {
	tests := []struct {
		name     string
		settings *patrimonySettings
		wantBody string
	}{
		{name: "missing settings", wantBody: "null"},
		{
			name: "existing settings",
			settings: &patrimonySettings{
				BaseDate:       "2026-02-03",
				OpeningCashBrl: "1234567890123456.78",
			},
			wantBody: `{"baseDate":"2026-02-03","openingCashBrl":"1234567890123456.78"}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mux := http.NewServeMux()
			sessionReader := &fakeCustomTagRepository{userExists: true}
			settingsReader := &fakePatrimonySettingsReader{settings: test.settings}
			registerPatrimonySettings(mux, testJWTKey, sessionReader, settingsReader)
			response := servePatrimonySettings(t, mux, map[string]any{
				"userId":         "owner-exact",
				"sessionVersion": 0,
				"exp":            4102444800,
			})
			if response.Code != http.StatusOK || response.Body.String() != test.wantBody {
				t.Fatalf("response = %d %s, want 200 %s", response.Code, response.Body.String(), test.wantBody)
			}
			if settingsReader.userID != "owner-exact" {
				t.Fatalf("settings query owner = %q, want exact authenticated owner", settingsReader.userID)
			}
			assertExpressJSONHeaders(t, response)
		})
	}
}

func TestPatrimonySettingsRouteSanitizesRepositoryErrors(t *testing.T) {
	mux := http.NewServeMux()
	sessionReader := &fakeCustomTagRepository{userExists: true}
	settingsReader := &fakePatrimonySettingsReader{err: errors.New("private database detail")}
	registerPatrimonySettings(mux, testJWTKey, sessionReader, settingsReader)
	response := servePatrimonySettings(t, mux, map[string]any{
		"userId":         "owner-exact",
		"sessionVersion": 0,
		"exp":            4102444800,
	})
	if response.Code != http.StatusInternalServerError || response.Body.String() != `{"error":"Internal server error"}` {
		t.Fatalf("repository error response = %d %s", response.Code, response.Body.String())
	}
}

func TestPatrimonySettingsRouteRequiresAuthentication(t *testing.T) {
	mux := http.NewServeMux()
	registerPatrimonySettings(mux, testJWTKey, nil, &fakePatrimonySettingsReader{})
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/patrimony/settings", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want 401", response.Code)
	}
}

func servePatrimonySettings(
	t testing.TB,
	mux *http.ServeMux,
	claims map[string]any,
) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/patrimony/settings", nil)
	request.AddCookie(&http.Cookie{Name: authCookieName, Value: signTestToken(t, jwt.SigningMethodHS256, claims)})
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	return response
}
