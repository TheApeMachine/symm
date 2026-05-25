package engine

import "testing"

func TestPassesBelowMedianLiquidity(t *testing.T) {
	volumes := map[string]float64{
		"LOW/EUR":  100,
		"MID/EUR":  200,
		"HIGH/EUR": 300,
	}

	if !PassesBelowMedianLiquidity(100, volumes, "LOW/EUR", DefaultMinLiquidityPairs) {
		t.Fatal("expected low-volume symbol to pass")
	}

	if PassesBelowMedianLiquidity(300, volumes, "HIGH/EUR", DefaultMinLiquidityPairs) {
		t.Fatal("expected high-volume symbol to fail")
	}

	if PassesBelowMedianLiquidity(100, volumes, "LOW/EUR", DefaultMinLiquidityPairs+1) {
		t.Fatal("expected min pair guard to fail with one peer")
	}
}
