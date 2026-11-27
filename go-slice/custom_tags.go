package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const authCookieName = "demanage-token"

type customTag struct {
	ID        string `json:"id"`
	UserID    string `json:"userId"`
	Scope     string `json:"scope"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type customTagRepository interface {
	FindUserSessionVersion(context.Context, string) (int64, bool, error)
	ListCustomTags(context.Context, string, string) ([]customTag, error)
}

type postgresCustomTagRepository struct {
	database *postgresDatabase
}

func (repository postgresCustomTagRepository) FindUserSessionVersion(ctx context.Context, userID string) (int64, bool, error) {
	var version int64
	found := false
	err := repository.withPool(ctx, func(queryContext context.Context, pool *pgxpool.Pool) error {
		err := pool.QueryRow(queryContext, `SELECT "sessionVersion" FROM "User" WHERE id = $1`, userID).Scan(&version)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err == nil {
			found = true
		}
		return err
	})
	if err != nil {
		return 0, false, err
	}
	return version, found, nil
}

func (repository postgresCustomTagRepository) ListCustomTags(ctx context.Context, userID, scope string) ([]customTag, error) {
	var tags []customTag
	err := repository.withPool(ctx, func(queryContext context.Context, pool *pgxpool.Pool) error {
		rows, err := pool.Query(queryContext, `
			SELECT id, "userId", scope, name, color, "createdAt", "updatedAt"
			FROM "CustomTag"
			WHERE "userId" = $1 AND scope = $2
			ORDER BY name ASC`, userID, scope)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var tag customTag
			var createdAt, updatedAt time.Time
			if err := rows.Scan(&tag.ID, &tag.UserID, &tag.Scope, &tag.Name, &tag.Color, &createdAt, &updatedAt); err != nil {
				return err
			}
			tag.CreatedAt = createdAt.UTC().Format("2006-01-02T15:04:05.000Z")
			tag.UpdatedAt = updatedAt.UTC().Format("2006-01-02T15:04:05.000Z")
			tags = append(tags, tag)
		}
		return rows.Err()
	})
	return tags, err
}

func (repository postgresCustomTagRepository) withPool(ctx context.Context, run func(context.Context, *pgxpool.Pool) error) error {
	if repository.database == nil {
		return fmt.Errorf("database is not configured")
	}
	queryContext, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	pool, err := repository.database.pool()
	if err != nil {
		return err
	}
	return run(queryContext, pool)
}

func registerCustomTags(mux *http.ServeMux, jwtKey []byte, repository customTagRepository) {
	mux.HandleFunc("GET /custom-tags", func(w http.ResponseWriter, r *http.Request) {
		userID, sessionVersion, ok := verifiedCustomTagIdentity(r, jwtKey)
		if !ok {
			writeJSON(w, r, http.StatusUnauthorized, struct {
				Error string `json:"error"`
			}{"Não autenticado"})
			return
		}
		if repository == nil {
			writeInternalError(w, r)
			return
		}
		storedVersion, exists, err := repository.FindUserSessionVersion(r.Context(), userID)
		if err != nil {
			writeInternalError(w, r)
			return
		}
		if !exists || storedVersion != sessionVersion {
			writeJSON(w, r, http.StatusUnauthorized, struct {
				Error string `json:"error"`
			}{"Não autenticado"})
			return
		}

		values, present := r.URL.Query()["scope"]
		if present && len(values) != 1 {
			writeJSON(w, r, http.StatusBadRequest, struct {
				Error string `json:"error"`
			}{"Parâmetro inválido: scope"})
			return
		}
		if !present || (values[0] != "expense" && values[0] != "income") {
			writeJSON(w, r, http.StatusBadRequest, struct {
				Error string `json:"error"`
			}{"Query obrigatória: scope=expense|income"})
			return
		}
		tags, err := repository.ListCustomTags(r.Context(), userID, values[0])
		if err != nil {
			writeInternalError(w, r)
			return
		}
		if tags == nil {
			tags = []customTag{}
		}
		writeJSON(w, r, http.StatusOK, tags)
	})
}

func verifiedCustomTagIdentity(r *http.Request, key []byte) (string, int64, bool) {
	if len(key) == 0 {
		return "", 0, false
	}
	cookie, err := r.Cookie(authCookieName)
	if err != nil {
		return "", 0, false
	}
	token, err := jwt.Parse(cookie.Value, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unsupported signing method")
		}
		return key, nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithJSONNumber())
	if err != nil || token == nil || !token.Valid {
		return "", 0, false
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", 0, false
	}
	userID, ok := claims["userId"].(string)
	if !ok {
		return "", 0, false
	}
	version, ok := sessionVersionClaim(claims)
	if !ok {
		return "", 0, false
	}
	return userID, version, true
}

func sessionVersionClaim(claims jwt.MapClaims) (int64, bool) {
	value, present := claims["sessionVersion"]
	if !present || value == nil {
		return 0, true
	}
	number, ok := value.(json.Number)
	if !ok {
		return 0, false
	}
	parsed, err := strconv.ParseFloat(number.String(), 64)
	if err != nil || math.Trunc(parsed) != parsed || math.Abs(parsed) > 9007199254740991 {
		return 0, false
	}
	return int64(parsed), true
}

func writeInternalError(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, r, http.StatusInternalServerError, struct {
		Error string `json:"error"`
	}{"Internal server error"})
}
