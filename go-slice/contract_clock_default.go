//go:build !go_contract_test

package main

import "time"

func contractClock() (func() time.Time, error) {
	return time.Now, nil
}
