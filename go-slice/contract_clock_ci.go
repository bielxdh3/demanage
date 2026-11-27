//go:build go_contract_test

package main

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

func contractClock() (func() time.Time, error) {
	value := strings.TrimSpace(os.Getenv("GO_CONTRACT_NOW"))
	if value == "" {
		return time.Now, nil
	}
	if os.Getenv("GITHUB_ACTIONS") != "true" {
		return nil, fmt.Errorf("GO_CONTRACT_NOW is only allowed in hosted synthetic CI")
	}
	if !isParityDatabaseURL(os.Getenv("DATABASE_URL")) {
		return nil, fmt.Errorf("GO_CONTRACT_NOW requires the disposable parity database")
	}
	fixed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return nil, fmt.Errorf("invalid GO_CONTRACT_NOW: %w", err)
	}
	return func() time.Time { return fixed }, nil
}

func isParityDatabaseURL(value string) bool {
	target, err := url.Parse(value)
	if err != nil || (target.Scheme != "postgres" && target.Scheme != "postgresql") || target.Fragment != "" {
		return false
	}
	hostname := strings.ToLower(target.Hostname())
	if hostname != "localhost" && hostname != "127.0.0.1" && hostname != "::1" {
		return false
	}
	if target.RawQuery != "sslmode=disable" {
		return false
	}
	return target.Port() == "5432" && target.EscapedPath() == "/demanage_parity_test"
}
