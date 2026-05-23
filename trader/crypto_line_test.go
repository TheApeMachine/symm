package trader

import "testing"

func TestBatchEntryLineUsesCrossSectionFence(t *testing.T) {
	candidates := []tradeCandidate{
		{symbol: "A/EUR", confidence: 0.4},
		{symbol: "B/EUR", confidence: 0.6},
		{symbol: "C/EUR", confidence: 0.9},
		{symbol: "D/EUR", confidence: 1.2},
	}

	line := batchEntryLine(candidates)

	if line.median <= 0 {
		t.Fatalf("expected positive median, got %v", line.median)
	}

	if line.line <= line.median {
		t.Fatalf("expected line above median, line=%v median=%v", line.line, line.median)
	}

	crypto := &Crypto{holds: make(map[string]position), wallet: &Wallet{Balance: 200}}

	if crypto.meetsEntryLine(candidates[0], line) {
		t.Fatal("expected weakest candidate below entry line")
	}

	if !crypto.meetsEntryLine(candidates[3], line) {
		t.Fatal("expected strongest candidate above entry line")
	}
}

func TestBatchEntryLineSingleCandidate(t *testing.T) {
	candidates := []tradeCandidate{{symbol: "A/EUR", confidence: 0.7}}
	line := batchEntryLine(candidates)

	if line.line != 0.7 {
		t.Fatalf("expected line 0.7 for single candidate, got %v", line.line)
	}

	crypto := &Crypto{holds: make(map[string]position), wallet: &Wallet{Balance: 200}}

	if !crypto.meetsEntryLine(candidates[0], line) {
		t.Fatal("expected single candidate to meet entry line")
	}
}

func BenchmarkBatchEntryLine(b *testing.B) {
	candidates := make([]tradeCandidate, 64)

	for index := range candidates {
		candidates[index] = tradeCandidate{
			symbol:     "SYM/EUR",
			confidence: float64(index) * 0.01,
		}
	}

	b.ReportAllocs()

	for b.Loop() {
		if batchEntryLine(candidates).line <= 0 {
			b.Fatal("expected line")
		}
	}
}
