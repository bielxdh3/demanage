package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type fakeProfileReadRepository struct {
	userID  string
	profile *profileRead
	err     error
	calls   int
}

func (repository *fakeProfileReadRepository) ReadProfile(_ context.Context, userID string) (*profileRead, error) {
	repository.calls++
	repository.userID = userID
	return repository.profile, repository.err
}

func TestProfileReadReturnsPublicContractWithDefaultSalaryAndNoPrivateData(t *testing.T) {
	profile := &profileRead{
		ID:               "owner-a",
		Name:             "Ana",
		Email:            "ana@example.test",
		HasRecoveryCode:  false,
		Salary:           0,
		SalaryReceiveDay: nil,
		Notes:            nil,
		CreatedAt:        "2026-01-02T03:04:05.006Z",
		UpdatedAt:        "2026-02-03T04:05:06.007Z",
	}
	repository := &fakeProfileReadRepository{profile: profile}
	sessions := &fakeCustomTagRepository{userExists: true, sessionVersion: 7}
	mux := http.NewServeMux()
	registerProfileRead(mux, testJWTKey, sessions, repository)

	response := serveProfileRead(t, mux, map[string]any{
		"userId": "owner-a", "sessionVersion": 7, "exp": 4102444800,
	})
	want := `{"user":{"id":"owner-a","name":"Ana","email":"ana@example.test","hasRecoveryCode":false,"salary":0,"salaryReceiveDay":null,"notes":null,"createdAt":"2026-01-02T03:04:05.006Z","updatedAt":"2026-02-03T04:05:06.007Z"}}`
	if response.Code != http.StatusOK || response.Body.String() != want {
		t.Fatalf("response = %d %s, want 200 %s", response.Code, response.Body.String(), want)
	}
	if repository.calls != 1 || repository.userID != "owner-a" || sessions.userID != "owner-a" {
		t.Fatalf("read calls=%d owner=%q session owner=%q, want one read for authenticated owner", repository.calls, repository.userID, sessions.userID)
	}
	for _, privateField := range []string{"passwordHash", "recoveryCodeHash", "sessionVersion", "userId"} {
		if strings.Contains(response.Body.String(), `"`+privateField+`"`) {
			t.Fatalf("response leaked private field %q: %s", privateField, response.Body.String())
		}
	}
	assertExpressJSONHeaders(t, response)
}

func TestProfileReadSerializesSyntheticSalaryAndNullableFields(t *testing.T) {
	receiveDay := int32(5)
	notes := "meta privada do perfil"
	profile := &profileRead{
		ID:               "owner-b",
		Name:             "Bia",
		Email:            "bia@example.test",
		HasRecoveryCode:  true,
		Salary:           1234.5,
		SalaryReceiveDay: &receiveDay,
		Notes:            &notes,
		CreatedAt:        "2026-03-04T05:06:07.008Z",
		UpdatedAt:        "2026-04-05T06:07:08.009Z",
	}
	repository := &fakeProfileReadRepository{profile: profile}
	mux := http.NewServeMux()
	registerProfileRead(mux, testJWTKey, &fakeCustomTagRepository{userExists: true}, repository)
	response := serveProfileRead(t, mux, map[string]any{
		"userId": "owner-b", "sessionVersion": 0, "exp": 4102444800,
	})
	want := `{"user":{"id":"owner-b","name":"Bia","email":"bia@example.test","hasRecoveryCode":true,"salary":1234.5,"salaryReceiveDay":5,"notes":"meta privada do perfil","createdAt":"2026-03-04T05:06:07.008Z","updatedAt":"2026-04-05T06:07:08.009Z"}}`
	if response.Code != http.StatusOK || response.Body.String() != want {
		t.Fatalf("response = %d %s, want 200 %s", response.Code, response.Body.String(), want)
	}
}

func TestProfileReadRequiresCurrentAuthenticatedSessionBeforeRepositoryRead(t *testing.T) {
	repository := &fakeProfileReadRepository{}
	sessions := &fakeCustomTagRepository{userExists: true, sessionVersion: 8}
	mux := http.NewServeMux()
	registerProfileRead(mux, testJWTKey, sessions, repository)
	response := serveProfileRead(t, mux, map[string]any{
		"userId": "owner-a", "sessionVersion": 7, "exp": 4102444800,
	})
	if response.Code != http.StatusUnauthorized || repository.calls != 0 {
		t.Fatalf("stale session response=%d calls=%d, want 401 without profile read", response.Code, repository.calls)
	}
}

func TestProfileReadRejectsRepositoryProfileThatDoesNotBelongToAuthenticatedOwner(t *testing.T) {
	repository := &fakeProfileReadRepository{profile: &profileRead{ID: "owner-b", Name: "private profile"}}
	mux := http.NewServeMux()
	registerProfileRead(mux, testJWTKey, &fakeCustomTagRepository{userExists: true}, repository)
	response := serveProfileRead(t, mux, map[string]any{
		"userId": "owner-a", "sessionVersion": 0, "exp": 4102444800,
	})
	if response.Code != http.StatusUnauthorized || strings.Contains(response.Body.String(), "private profile") {
		t.Fatalf("mismatched owner response = %d %s, want 401 without profile data", response.Code, response.Body.String())
	}
}

func TestProfileReadSanitizesRepositoryErrors(t *testing.T) {
	repository := &fakeProfileReadRepository{err: errors.New("private database detail")}
	mux := http.NewServeMux()
	registerProfileRead(mux, testJWTKey, &fakeCustomTagRepository{userExists: true}, repository)
	response := serveProfileRead(t, mux, map[string]any{
		"userId": "owner-a", "sessionVersion": 0, "exp": 4102444800,
	})
	if response.Code != http.StatusInternalServerError || response.Body.String() != `{"error":"Internal server error"}` {
		t.Fatalf("repository error response = %d %s", response.Code, response.Body.String())
	}
}

func TestProfileReadRouteRemainsIsolatedFromSharedServer(t *testing.T) {
	response := httptest.NewRecorder()
	newHandler(func(context.Context) error { return nil }).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/auth/me", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("shared server GET /auth/me status = %d, want unmounted 404", response.Code)
	}
}

func serveProfileRead(t testing.TB, mux *http.ServeMux, claims map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	request.AddCookie(&http.Cookie{Name: authCookieName, Value: signTestToken(t, jwt.SigningMethodHS256, claims)})
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	return response
}

type fakeProfileReadQueryer struct {
	query string
	args  []any
	row   fakeProfileReadRow
}

func (queryer *fakeProfileReadQueryer) QueryRow(_ context.Context, query string, args ...any) pgx.Row {
	queryer.query = query
	queryer.args = args
	return queryer.row
}

type fakeProfileReadRow struct {
	hasRecoveryCode  bool
	salary           pgtype.Text
	salaryReceiveDay pgtype.Int4
	notes             pgtype.Text
}

func (row fakeProfileReadRow) Scan(dest ...any) error {
	*dest[0].(*string) = "owner-a"
	*dest[1].(*string) = "Ana"
	*dest[2].(*string) = "ana@example.test"
	*dest[3].(*bool) = row.hasRecoveryCode
	*dest[4].(*pgtype.Text) = row.salary
	*dest[5].(*pgtype.Int4) = row.salaryReceiveDay
	*dest[6].(*pgtype.Text) = row.notes
	*dest[7].(*time.Time) = time.Date(2026, 1, 2, 3, 4, 5, 6_000_000, time.UTC)
	*dest[8].(*time.Time) = time.Date(2026, 2, 3, 4, 5, 6, 7_000_000, time.UTC)
	return nil
}

func TestReadProfileParsesSalaryAsPublicNumber(t *testing.T) {
	queryer := &fakeProfileReadQueryer{row: fakeProfileReadRow{
		hasRecoveryCode: true,
		salary:          pgtype.Text{String: "1234.50", Valid: true},
		salaryReceiveDay: pgtype.Int4{Int32: 12, Valid: true},
		notes:            pgtype.Text{String: "synthetic note", Valid: true},
		},
	}
	profile, err := readProfile(context.Background(), queryer, "owner-a")
	if err != nil {
		t.Fatal(err)
	}
	if profile == nil || profile.Salary != 1234.5 || profile.SalaryReceiveDay == nil || *profile.SalaryReceiveDay != 12 || profile.Notes == nil || *profile.Notes != "synthetic note" || !profile.HasRecoveryCode {
		t.Fatalf("profile = %#v, want numeric salary, day, note, and recovery-code boolean", profile)
	}
}

func TestReadProfileUsesExpressDefaultsAndDeterministicCanonicalSalary(t *testing.T) {
	queryer := &fakeProfileReadQueryer{}
	profile, err := readProfile(context.Background(), queryer, "owner-a")
	if err != nil {
		t.Fatal(err)
	}
	if profile == nil || profile.ID != "owner-a" || profile.Salary != 0 || profile.SalaryReceiveDay != nil || profile.Notes != nil {
		t.Fatalf("profile = %#v, want owner profile with salary default 0 and nullable fields", profile)
	}
	if profile.CreatedAt != "2026-01-02T03:04:05.006Z" || profile.UpdatedAt != "2026-02-03T04:05:06.007Z" {
		t.Fatalf("profile timestamps = %q / %q", profile.CreatedAt, profile.UpdatedAt)
	}
	if len(queryer.args) != 1 || queryer.args[0] != "owner-a" {
		t.Fatalf("profile query args = %#v, want authenticated owner", queryer.args)
	}
	for _, expected := range []string{
		`COALESCE(NULLIF(u."recoveryCodeHash", ''), '') <> ''`,
		`ORDER BY e."createdAt" ASC, e.id ASC`,
		`WHERE u.id = $1`,
	} {
		if !strings.Contains(queryer.query, expected) {
			t.Fatalf("profile query does not contain %q: %s", expected, queryer.query)
		}
	}
	for _, privateProjection := range []string{`u."passwordHash"`, `u."recoveryCodeHash",`} {
		if strings.Contains(queryer.query, privateProjection) {
			t.Fatalf("profile query projects private data %q", privateProjection)
		}
	}
}
