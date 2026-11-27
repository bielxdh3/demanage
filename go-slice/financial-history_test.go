package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type fakeFinancialHistoryReader struct {
	userID  string
	options financialHistoryOptions
	page    financialHistoryPage
	err     error
	calls   int
}

func (reader *fakeFinancialHistoryReader) ListFinancialHistoryPage(
	_ context.Context,
	userID string,
	options financialHistoryOptions,
) (financialHistoryPage, error) {
	reader.calls++
	reader.userID = userID
	reader.options = options
	return reader.page, reader.err
}

func TestFinancialHistoryHandlerUsesAuthenticatedOwnerAndStableEmptyPage(t *testing.T) {
	snapshot := "2026-11-26T20:00:00.000Z"
	sessionReader := &fakeCustomTagRepository{userExists: true}
	pageReader := &fakeFinancialHistoryReader{page: financialHistoryPage{Snapshot: snapshot}}
	handler := financialHistoryHandler(testJWTKey, sessionReader, pageReader)
	request := httptest.NewRequest(http.MethodGet, "/financial-history?snapshot="+snapshot+"&limit=2&expenseCursor=end", nil)
	request.AddCookie(&http.Cookie{Name: authCookieName, Value: signTestToken(t, jwt.SigningMethodHS256, map[string]any{
		"userId": "owner-1", "sessionVersion": 0, "exp": 4102444800,
	})})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	want := `{"snapshot":"2026-11-26T20:00:00.000Z","nextExpenseCursor":null,"nextIncomeCursor":null,"expenses":[],"incomes":[]}`
	if response.Code != http.StatusOK || response.Body.String() != want {
		t.Fatalf("response = %d %s, want 200 %s", response.Code, response.Body.String(), want)
	}
	assertExpressJSONHeaders(t, response)
	if pageReader.calls != 1 || pageReader.userID != "owner-1" || pageReader.options.limit != 2 || pageReader.options.snapshot != snapshot {
		t.Fatalf("page request = calls %d owner %q options %#v", pageReader.calls, pageReader.userID, pageReader.options)
	}
	if !pageReader.options.expenseCursor.done || pageReader.options.incomeCursor.done {
		t.Fatalf("independent cursor sentinels = expense %#v income %#v", pageReader.options.expenseCursor, pageReader.options.incomeCursor)
	}
}

func TestFinancialHistoryHandlerRemainsUnmountedFromSharedServer(t *testing.T) {
	handler := newHandler(func(context.Context) error { return nil }, handlerOptions{
		jwtKey:    testJWTKey,
		tagReader: &fakeCustomTagRepository{userExists: true},
	})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/financial-history", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("shared handler status = %d, want 404 while financial history remains unmounted", response.Code)
	}
}

func TestFinancialHistoryHandlerReturnsExpressErrorsWithoutLeakingRepositoryDetails(t *testing.T) {
	sessionReader := &fakeCustomTagRepository{userExists: true}
	pageReader := &fakeFinancialHistoryReader{err: context.DeadlineExceeded}
	handler := financialHistoryHandler(testJWTKey, sessionReader, pageReader)
	token := signTestToken(t, jwt.SigningMethodHS256, map[string]any{
		"userId": "owner-1", "sessionVersion": 0, "exp": 4102444800,
	})

	invalid := httptest.NewRequest(http.MethodGet, "/financial-history?limit=201", nil)
	invalid.AddCookie(&http.Cookie{Name: authCookieName, Value: token})
	invalidResponse := httptest.NewRecorder()
	handler.ServeHTTP(invalidResponse, invalid)
	if invalidResponse.Code != http.StatusBadRequest || invalidResponse.Body.String() != `{"error":"Invalid financial history page limit"}` || pageReader.calls != 0 {
		t.Fatalf("invalid query response = %d %s, repository calls %d", invalidResponse.Code, invalidResponse.Body.String(), pageReader.calls)
	}

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/financial-history", nil))
	if unauthorized.Code != http.StatusUnauthorized || unauthorized.Body.String() != `{"error":"Não autenticado"}` {
		t.Fatalf("unauthenticated response = %d %s", unauthorized.Code, unauthorized.Body.String())
	}

	valid := httptest.NewRequest(http.MethodGet, "/financial-history", nil)
	valid.AddCookie(&http.Cookie{Name: authCookieName, Value: token})
	failed := httptest.NewRecorder()
	handler.ServeHTTP(failed, valid)
	if failed.Code != http.StatusInternalServerError || failed.Body.String() != `{"error":"Internal server error"}` {
		t.Fatalf("repository failure response = %d %s", failed.Code, failed.Body.String())
	}
}

func TestFinancialHistoryRejectsInvalidSnapshotAndIndependentCursors(t *testing.T) {
	now := time.Date(2026, time.November, 27, 12, 0, 0, 0, time.UTC)
	snapshot := "2026-11-26T20:00:00.000Z"
	cursor := testFinancialHistoryCursor("2026-11-26T19:00:00.000Z", "income-1")
	options, err := parseFinancialHistoryOptions(url.Values{
		"snapshot":      {snapshot},
		"limit":         {"2"},
		"incomeCursor":  {cursor},
		"expenseCursor": {"end"},
	}, now)
	if err != nil {
		t.Fatalf("parse valid request: %v", err)
	}
	if options.limit != 2 || options.snapshot != snapshot || !options.expenseCursor.done || options.expenseCursor.cursor != nil || options.incomeCursor.done {
		t.Fatalf("parsed independent cursors = %#v", options)
	}
	if options.incomeCursor.cursor == nil || options.incomeCursor.cursor.id != "income-1" || options.incomeCursor.cursor.createdAt.Format(historyTimestampLayout) != "2026-11-26T19:00:00.000Z" {
		t.Fatalf("parsed income cursor = %#v", options.incomeCursor)
	}

	invalid := []struct {
		name      string
		query     url.Values
		wantError string
	}{
		{name: "unknown key", query: url.Values{"offset": {"1"}}, wantError: "Invalid financial history query"},
		{name: "duplicate limit", query: url.Values{"limit": {"1", "2"}}, wantError: "Invalid financial history query"},
		{name: "empty limit", query: url.Values{"limit": {""}}, wantError: "Invalid financial history query"},
		{name: "zero limit", query: url.Values{"limit": {"0"}}, wantError: "Invalid financial history page limit"},
		{name: "leading zero", query: url.Values{"limit": {"01"}}, wantError: "Invalid financial history page limit"},
		{name: "oversized limit", query: url.Values{"limit": {"201"}}, wantError: "Invalid financial history page limit"},
		{name: "snapshot without milliseconds", query: url.Values{"snapshot": {"2026-11-26T20:00:00Z"}}, wantError: "Invalid financial history snapshot"},
		{name: "non UTC snapshot", query: url.Values{"snapshot": {"2026-11-26T20:00:00.000+00:00"}}, wantError: "Invalid financial history snapshot"},
		{name: "pre epoch snapshot", query: url.Values{"snapshot": {"1969-12-31T23:59:59.999Z"}}, wantError: "Invalid financial history snapshot"},
		{name: "future snapshot", query: url.Values{"snapshot": {"2026-11-27T12:01:01.000Z"}}, wantError: "Invalid financial history snapshot"},
		{name: "cursor without snapshot", query: url.Values{"expenseCursor": {"end"}}, wantError: "Invalid financial history snapshot"},
		{name: "malformed cursor", query: url.Values{"snapshot": {snapshot}, "expenseCursor": {"not-base64!"}}, wantError: "Invalid financial history cursor"},
		{name: "cursor fields are case sensitive", query: url.Values{"snapshot": {snapshot}, "expenseCursor": {base64.RawURLEncoding.EncodeToString([]byte(`{"CreatedAt":"2026-11-26T19:00:00.000Z","ID":"income-1"}`))}}, wantError: "Invalid financial history cursor"},
		{name: "cursor after snapshot", query: url.Values{"snapshot": {snapshot}, "incomeCursor": {testFinancialHistoryCursor("2026-11-26T21:00:00.000Z", "income-2")}}, wantError: "Invalid financial history cursor snapshot"},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			if _, err := parseFinancialHistoryOptions(test.query, now); err == nil || err.Error() != test.wantError {
				t.Fatalf("parse error = %v, want %q", err, test.wantError)
			}
		})
	}
}

func TestFinancialHistoryDefaultsToCanonicalUTCMillisecondSnapshotAndMaximumPage(t *testing.T) {
	now := time.Date(2026, time.November, 27, 5, 2, 3, 456789000, time.UTC)
	options, err := parseFinancialHistoryOptions(url.Values{}, now)
	if err != nil {
		t.Fatalf("parse default request: %v", err)
	}
	if options.limit != financialHistoryMaxPageSize || options.snapshot != "2026-11-27T05:02:03.456Z" || options.snapshotTime.Format(historyTimestampLayout) != options.snapshot {
		t.Fatalf("default page options = %#v", options)
	}
}

func testFinancialHistoryCursor(createdAt, id string) string {
	value, err := json.Marshal(struct {
		CreatedAt string `json:"createdAt"`
		ID        string `json:"id"`
	}{createdAt, id})
	if err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(value)
}
