//go:build go_contract_test

package main

import (
	"testing"
	"time"
)

const parityDatabaseURL = "postgresql://demanage:demanage@localhost:5432/demanage_parity_test?sslmode=disable"

func TestContractClockAcceptsHostedParityOverride(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("DATABASE_URL", parityDatabaseURL)
	t.Setenv("GO_CONTRACT_NOW", "2026-11-01T02:00:00.000Z")

	clock, err := contractClock()
	if err != nil {
		t.Fatalf("contractClock() error = %v", err)
	}
	want, err := time.Parse(time.RFC3339Nano, "2026-11-01T02:00:00.000Z")
	if err != nil {
		t.Fatal(err)
	}
	if got := clock(); !got.Equal(want) {
		t.Fatalf("contract clock = %s, want %s", got, want)
	}
}

func TestContractClockRejectsOverrideOutsideHostedParityTarget(t *testing.T) {
	tests := []struct {
		name        string
		github      string
		databaseURL string
	}{
		{name: "not hosted", github: "false", databaseURL: parityDatabaseURL},
		{name: "other database", github: "true", databaseURL: "postgresql://demanage:demanage@localhost:5432/demanage?sslmode=disable"},
		{name: "remote host", github: "true", databaseURL: "postgresql://demanage:demanage@example.invalid:5432/demanage_parity_test?sslmode=disable"},
		{name: "wrong port", github: "true", databaseURL: "postgresql://demanage:demanage@localhost:5433/demanage_parity_test?sslmode=disable"},
		{name: "schema override", github: "true", databaseURL: "postgresql://demanage:demanage@localhost:5432/demanage_parity_test?schema=public"},
		{name: "unrelated query setting", github: "true", databaseURL: "postgresql://demanage:demanage@localhost:5432/demanage_parity_test?sslmode=disable&application_name=contract"},
		{name: "repeated query setting", github: "true", databaseURL: "postgresql://demanage:demanage@localhost:5432/demanage_parity_test?sslmode=disable&sslmode=disable"},
		{name: "encoded query value", github: "true", databaseURL: "postgresql://demanage:demanage@localhost:5432/demanage_parity_test?sslmode=dis%61ble"},
		{name: "host override", github: "true", databaseURL: "postgresql://demanage:demanage@localhost:5432/demanage_parity_test?sslmode=disable&host=example.invalid"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("GITHUB_ACTIONS", test.github)
			t.Setenv("DATABASE_URL", test.databaseURL)
			t.Setenv("GO_CONTRACT_NOW", "2026-11-01T02:00:00.000Z")
			if _, err := contractClock(); err == nil {
				t.Fatal("contractClock() succeeded with an unsafe override target")
			}
		})
	}
}

func TestContractClockRejectsInvalidTimestamp(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("DATABASE_URL", parityDatabaseURL)
	t.Setenv("GO_CONTRACT_NOW", "not-a-timestamp")

	if _, err := contractClock(); err == nil {
		t.Fatal("contractClock() succeeded with an invalid timestamp")
	}
}
