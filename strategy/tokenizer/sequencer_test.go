package tokenizer

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/store"
)

func litRegion(key uint8, brightness float64) *store.Region {
	return &store.Region{Key: key, Brightness: brightness}
}

func TestStepEmptyLit(t *testing.T) {
	step := NewSequencer().Step(nil, false)
	if step.Token.String() != "" || step.Silence != StepSilenceEmpty {
		t.Fatalf("empty lit: token=%q silence=%q", step.Token.String(), step.Silence)
	}
}

func TestStepFlatLeader(t *testing.T) {
	step := NewSequencer().Step([]*store.Region{
		litRegion(6, 0.2),
		litRegion(7, 0.9),
		litRegion(8, 0.1),
		litRegion(0, 0.4),
		litRegion(1, 0.3),
		litRegion(2, 0.2),
		litRegion(4, 4),
	}, false)

	if step.Token.String() != "R07.R00" {
		t.Fatalf("cut = %q, want R07.R00", step.Token.String())
	}
}

func TestStepHoldingLeader(t *testing.T) {
	step := NewSequencer().Step([]*store.Region{
		litRegion(6, 5),
		litRegion(0, 5),
		litRegion(8, 0.4),
		litRegion(9, 0.2),
		litRegion(3, 0.1),
		litRegion(4, 0.8),
		litRegion(5, 0.3),
	}, true)

	if step.Token.String() != "R08.R04" {
		t.Fatalf("cut = %q, want R08.R04", step.Token.String())
	}
}

func TestStepDarkBandStaysSilent(t *testing.T) {
	step := NewSequencer().Step([]*store.Region{
		litRegion(7, 0.8),
		litRegion(0, 0),
	}, false)

	if step.Token.String() != "" || step.Silence != StepSilenceEmpty {
		t.Fatalf("token=%q silence=%q", step.Token.String(), step.Silence)
	}
}

func TestStepTieKeepsLowerKey(t *testing.T) {
	step := NewSequencer().Step([]*store.Region{
		litRegion(8, 0.5),
		litRegion(6, 0.5),
		litRegion(2, 0.4),
		litRegion(0, 0.4),
	}, false)

	if step.Token.String() != "R06.R00" {
		t.Fatalf("cut = %q, want R06.R00", step.Token.String())
	}
}

func TestStepRiskVeto(t *testing.T) {
	step := NewSequencer().Step([]*store.Region{
		litRegion(6, 0.6),
		litRegion(1, 0.4),
		litRegion(12, 0.2),
		litRegion(13, 0.5),
	}, false)

	if step.Token.String() != "" || step.Silence != StepSilenceVeto {
		t.Fatalf("token=%q silence=%q", step.Token.String(), step.Silence)
	}
}

func TestStepRiskAtPostureDoesNotVeto(t *testing.T) {
	step := NewSequencer().Step([]*store.Region{
		litRegion(6, 0.6),
		litRegion(1, 0.5),
		litRegion(14, 0.5),
	}, false)

	if step.Token.String() != "R06.R01" {
		t.Fatalf("cut = %q, want R06.R01", step.Token.String())
	}
}

func TestStepIgnoresSequentialDuplicate(t *testing.T) {
	sequencer := NewSequencer()
	lit := []*store.Region{
		litRegion(6, 0.4),
		litRegion(0, 0.3),
	}

	first := sequencer.Step(lit, false)
	second := sequencer.Step(lit, false)

	if first.Token.String() != "R06.R00" {
		t.Fatalf("first = %q", first.Token.String())
	}

	if second.Token.String() != "" || second.Silence != StepSilenceDuplicate {
		t.Fatalf("second token=%q silence=%q", second.Token.String(), second.Silence)
	}
}

func TestStepEmitsAfterLeaderChanges(t *testing.T) {
	sequencer := NewSequencer()

	_ = sequencer.Step([]*store.Region{litRegion(6, 0.4), litRegion(0, 0.3)}, false)
	next := sequencer.Step([]*store.Region{litRegion(7, 0.5), litRegion(1, 0.2)}, false)

	if next.Token.String() != "R07.R01" {
		t.Fatalf("next = %q, want R07.R01", next.Token.String())
	}
}

func TestHoldSpellCompressesOscillation(t *testing.T) {
	sequencer := NewSequencer()
	a := []*store.Region{litRegion(7, 0.5), litRegion(3, 0.4)}
	b := []*store.Region{litRegion(8, 0.5), litRegion(3, 0.4)}

	var last []string
	for range 6 {
		last = sequencer.Step(a, true).Spell
		last = sequencer.Step(b, true).Spell
	}

	if len(last) > 4 {
		t.Fatalf("spell tail = %v, want sequitur-compressed length", last)
	}
}
