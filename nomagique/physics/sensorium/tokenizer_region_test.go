package sensorium

import "testing"

func TestMakeRegionLitBatch(t *testing.T) {
	tokenizer, err := NewTokenizer(8, 8, 8, 64)
	if err != nil {
		t.Fatal(err)
	}

	state, err := tokenizer.MakeRegionLitBatch(1, []RegionLit{
		{Key: 0, Brightness: 0.5},
		{Key: 1, Brightness: 0.8},
	}, 0)
	if err != nil {
		t.Fatal(err)
	}

	if state == nil || state.N < 2 {
		t.Fatalf("expected particles, got N=%d", state.N)
	}
}
