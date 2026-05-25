package adaptive

import "testing"

func TestBlendEMA(t *testing.T) {
	blended := BlendEMA(0, 10, 0.2)

	if blended != 2 {
		t.Fatalf("expected 2, got %v", blended)
	}

	next := BlendEMA(blended, 10, 0.2)

	if next <= blended || next >= 10 {
		t.Fatalf("expected move toward 10, got %v", next)
	}
}

func BenchmarkBlendEMA(b *testing.B) {
	current := 0.0

	for b.Loop() {
		current = BlendEMA(current, 1.5, 0.2)
	}
}
