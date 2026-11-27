//go:build !go_contract_test

package main

import (
	"testing"
	"time"
)

func TestContractClockIgnoresFixedClockOverrideInNormalBuild(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("DATABASE_URL", "postgresql://demanage:demanage@localhost:5432/demanage_parity_test?sslmode=disable")
	t.Setenv("GO_CONTRACT_NOW", "2000-01-01T00:00:00Z")

	clock, err := contractClock()
	if err != nil {
		t.Fatalf("contractClock() error = %v", err)
	}
	before := time.Now()
	got := clock()
	after := time.Now()
	if got.Before(before) || got.After(after) {
		t.Fatalf("normal build clock = %s, want current time between %s and %s", got, before, after)
	}
}
