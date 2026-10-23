package tokenizer

import "testing"

func TestChordTokenStablePerTick(t *testing.T) {
	chord := ChordToken([]Token{
		NewTokenFromRegion(2, 0.4),
		NewTokenFromRegion(0, 0.9),
		NewTokenFromRegion(1, 0.7),
	})

	if chord.String() != "[R00-R01-R02]" {
		t.Fatalf("chord = %q, want sorted deduped regions", chord.String())
	}

	if chord.Brightness != 0.9 {
		t.Fatalf("brightness = %v, want max 0.9", chord.Brightness)
	}
}
