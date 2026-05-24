package config

import (
	"testing"

	"github.com/theapemachine/symm/kraken/market"
)

func TestSlippageFillUsesDepthVWAP(t *testing.T) {
	cfg := NewConfig()
	asks := []market.BookLevel{
		{Price: 100, Volume: 1},
		{Price: 102, Volume: 1},
	}

	fill := cfg.SlippageFill(100, 99, 101, "buy", 0, 150, nil, asks)

	if fill <= 100 || fill >= 102 {
		t.Fatalf("expected depth VWAP between 100 and 102, got %v", fill)
	}
}

func TestSlippageFillFallsBackToHalfSpread(t *testing.T) {
	cfg := NewConfig()
	fill := cfg.SlippageFill(100, 99, 101, "buy", 0, 500, nil, nil)

	if fill != 101 {
		t.Fatalf("expected half-spread fallback 101, got %v", fill)
	}
}
