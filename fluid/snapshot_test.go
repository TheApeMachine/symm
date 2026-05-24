package fluid

import "testing"

func TestAggregateFieldRowsIgnoresWarmingRows(t *testing.T) {
	rows := []SymbolSnapshot{
		{Symbol: "WARM/EUR", ChangePct: 1.2},
		{Symbol: "WARM/B", ChangePct: 0.4},
		{
			Symbol: "PUMP/EUR",
			Vol:    10,
			Visc:   5,
			Div:    120,
			Vort:   0.002,
			Turb:   0.001,
			Re:     0.004,
		},
		{
			Symbol: "FLOW/EUR",
			Vol:    8,
			Visc:   4,
			Div:    80,
			Vort:   0.006,
			Turb:   0.003,
			Re:     0.008,
		},
	}

	aggregate, sampledCount := aggregateFieldRows(rows)

	if sampledCount != 2 {
		t.Fatalf("expected 2 sampled rows, got %d", sampledCount)
	}

	if aggregate.Div != 5 {
		t.Fatalf("expected normalized div activity 5, got %v", aggregate.Div)
	}

	if aggregate.Vort != 2 {
		t.Fatalf("expected normalized vort activity 2, got %v", aggregate.Vort)
	}

	if aggregate.Re != 3 {
		t.Fatalf("expected normalized re activity 3, got %v", aggregate.Re)
	}
}

func BenchmarkAggregateFieldRows(b *testing.B) {
	rows := make([]SymbolSnapshot, 600)

	for index := range rows {
		if index%3 == 0 {
			rows[index] = SymbolSnapshot{Symbol: "WARM/EUR", ChangePct: 1}
			continue
		}

		rows[index] = SymbolSnapshot{
			Symbol: "PUMP/EUR",
			Vol:    10,
			Visc:   5,
			Div:    2,
			Vort:   0.002,
			Turb:   0.001,
			Re:     0.004,
		}
	}

	b.ReportAllocs()

	for b.Loop() {
		aggregateFieldRows(rows)
	}
}
