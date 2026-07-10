package tests

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
)

/*
Decimal parses a decimal string and fails the test on error.
*/
func Decimal(t testing.TB, value string) decimal.Decimal {
	t.Helper()

	parsed, err := decimal.NewFromString(value)

	if err != nil {
		t.Fatal(err)
	}

	return *parsed
}
