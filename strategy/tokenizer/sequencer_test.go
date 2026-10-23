package tokenizer

import "testing"

func TestSequencerEmitsRepeatedChordAsOneRegime(t *testing.T) {
	sequencer := NewSequencer()
	chord := []Token{
		NewTokenFromRegion(1, 0.8),
		NewTokenFromRegion(2, 0.4),
	}

	first := sequencer.Analyze(chord)
	if len(first.Symbols) != 0 {
		t.Fatalf("first chord emitted %s; a regime waits until the chord repeats", first.String())
	}

	second := sequencer.Analyze(chord)
	if second.String() != "[R01-R02]" {
		t.Fatalf("regime = %q, want [R01-R02]", second.String())
	}

	if second.Brightness != 0.8 {
		t.Fatalf("brightness = %v, want the brighter region 0.8", second.Brightness)
	}
}

func TestSequencerHoldsAUniqueRegion(t *testing.T) {
	sequencer := NewSequencer()

	token := sequencer.Analyze([]Token{NewTokenFromRegion(9, 0.9)})
	if len(token.Symbols) != 0 {
		t.Fatalf("unique region emitted %s", token.String())
	}
}
