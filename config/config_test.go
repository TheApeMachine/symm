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

	if cfg.LogFileActive != true {
		t.Fatalf("expected file logging enabled by default")
	}

	if cfg.LogStdoutActive != false {
		t.Fatalf("expected console logging disabled by default")
	}

	if !cfg.UseMakerEntries {
		t.Fatal("expected maker entries enabled by default")
	}

	if cfg.ScalpHoldBeforeExit != 90*time.Second {
		t.Fatalf("expected 90s scalp hold, got %v", cfg.ScalpHoldBeforeExit)
	}

	if cfg.EntryEdgeMultiple != 2 {
		t.Fatalf("expected EntryEdgeMultiple 2, got %v", cfg.EntryEdgeMultiple)
	}

	if cfg.TakeProfitR != 2 {
		t.Fatalf("expected TakeProfitR 2, got %v", cfg.TakeProfitR)
	}

	if cfg.StopVolMultiple != 8 {
		t.Fatalf("expected StopVolMultiple 8, got %v", cfg.StopVolMultiple)
	}

	if cfg.MinExhaustHold != 5*time.Second {
		t.Fatalf("expected MinExhaustHold 5s, got %v", cfg.MinExhaustHold)
	}

	if cfg.AdverseSelectionBPS != 5 {
		t.Fatalf("expected AdverseSelectionBPS 5, got %v", cfg.AdverseSelectionBPS)
	}

	if cfg.RegimeShockMinSamples != 64 {
		t.Fatalf("expected RegimeShockMinSamples 64, got %v", cfg.RegimeShockMinSamples)
	}

	if cfg.ExecutionMakerFallbackTicks != 4 {
		t.Fatalf("expected ExecutionMakerFallbackTicks 4, got %v", cfg.ExecutionMakerFallbackTicks)
	}

	if cfg.UITelemetryBuffer != 512 {
		t.Fatalf("expected UITelemetryBuffer 512, got %v", cfg.UITelemetryBuffer)
	}

	if cfg.UIHeartbeatInterval != 250*time.Millisecond {
		t.Fatalf("expected UIHeartbeatInterval 250ms, got %v", cfg.UIHeartbeatInterval)
	}
}

func TestLogStdoutEnvOverride(t *testing.T) {
	t.Setenv("SYMM_LOG_STDOUT", "1")

	cfg := NewConfig()

	if !cfg.LogStdoutActive {
		t.Fatal("expected SYMM_LOG_STDOUT=1 to enable console logging")
	}
}

func TestAuditFileEnvOverride(t *testing.T) {
	t.Setenv("SYMM_AUDIT_FILE", "runs/audit.jsonl")
	t.Setenv("SYMM_AUDIT_GATE_COOLDOWN", "30s")
	t.Setenv("SYMM_AUDIT_MAX_MB", "16")

	cfg := NewConfig()

	if cfg.AuditFile != "runs/audit.jsonl" {
		t.Fatalf("expected audit file override, got %q", cfg.AuditFile)
	}

	if cfg.AuditGateRejectCooldown != 30*time.Second {
		t.Fatalf("expected 30s gate cooldown, got %v", cfg.AuditGateRejectCooldown)
	}

	if cfg.AuditMaxFileBytes != 16<<20 {
		t.Fatalf("expected 16MB max file, got %d", cfg.AuditMaxFileBytes)
	}
}

func TestExecutionStressEnvOverride(t *testing.T) {
	t.Setenv("SYMM_EXECUTION_STRESS", "1")

	cfg := NewConfig()

	if !cfg.ExecutionStressEnabled {
		t.Fatal("expected SYMM_EXECUTION_STRESS=1 to enable execution stress")
	}
}
