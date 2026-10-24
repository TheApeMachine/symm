package strategy

import (
	"testing"

	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/strategy/tokenizer"
)

func TestAppendEmitReadoutPassed(t *testing.T) {
	out := data.NewValue("training", "step", "BTC/USD")
	appendEmitReadout(out, tokenizer.StepResult{
		Token: tokenizer.NewToken([]string{"R01"}, 1.2, 0),
		Peak:  1.2,
	}, true)

	var gate stepGate
	out.Lookup(&gate, "gate")

	if !gate.Passed || gate.Warm {
		t.Fatalf("gate = %+v", gate)
	}

	var blocked bool
	out.Lookup(&blocked, "blocked")
	if blocked {
		t.Fatal("expected not blocked on pass")
	}
}

func TestAppendEmitReadoutBrightnessBlocked(t *testing.T) {
	out := data.NewValue("training", "step", "BTC/USD")
	appendEmitReadout(out, tokenizer.StepResult{
		Silence: tokenizer.StepSilenceBrightness,
		Peak:    0.8,
	}, true)

	var gate stepGate
	out.Lookup(&gate, "gate")

	if gate.Passed || gate.Cold != "brightness" {
		t.Fatalf("gate = %+v", gate)
	}

	var blocked bool
	out.Lookup(&blocked, "blocked")
	if !blocked {
		t.Fatal("expected blocked")
	}

	var reason string
	out.Lookup(&reason, "blockedReason")
	if reason != "brightness" {
		t.Fatalf("reason = %q", reason)
	}
}
