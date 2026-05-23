package config

import (
	"testing"
	"time"
)

func TestNewConfigDefaults(t *testing.T) {
	cfg := NewConfig()

	if cfg.ExitEvery != 10*time.Millisecond {
		t.Fatalf("expected 10ms exit ticker, got %v", cfg.ExitEvery)
	}

	if cfg.WSPingInterval != 30*time.Second {
		t.Fatalf("expected 30s ping interval, got %v", cfg.WSPingInterval)
	}
}

func TestSlippagePriceUsesHalfSpread(t *testing.T) {
	cfg := NewConfig()
	fill := cfg.SlippagePrice(100, 99, 101, "buy", 0)

	if fill != 101 {
		t.Fatalf("expected buy fill 101, got %v", fill)
	}
}
