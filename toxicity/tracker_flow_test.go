package toxicity

import (
	"testing"

	"github.com/theapemachine/symm/engine"
)

func TestTrackerAmendPriceChangeClassifiesRemoval(t *testing.T) {
	tr := NewTracker()
	tr.ObserveMid(sym, pair, 100)
	tr.ApplyOrder(sym, pair, "add", "filler", 'b', 99.5, 50, base, base)
	tr.ApplyOrder(sym, pair, "add", "wall", 'b', 99.95, 10, base, base)
	tr.ApplyOrder(sym, pair, "amend", "wall", 'b', 99.90, 10, base, base)

	if !tr.IsToxic(sym, 99.95, base) {
		t.Fatal("a price-change amend should classify the old level removal as toxic")
	}
}

func TestTrackerDuplicateAddIsIgnored(t *testing.T) {
	tr := NewTracker()
	tr.ObserveMid(sym, pair, 100)
	tr.ApplyOrder(sym, pair, "add", "wall", 'b', 99.95, 10, base, base)
	tr.ApplyOrder(sym, pair, "add", "wall", 'b', 99.95, 20, base, base)

	m, ok := tr.Measure(sym, base)

	if ok || m.Type != engine.MeasurementType(0) {
		t.Fatalf("duplicate add must not disturb flow state, got %+v ok=%v", m, ok)
	}
}

func TestTrackerL2LevelGrowth(t *testing.T) {
	tr := NewTracker()
	tr.ObserveMid(sym, pair, 100)
	tr.ApplyBookLevel(sym, pair, 'a', 100.05, 5, base)
	tr.ApplyBookLevel(sym, pair, 'a', 100.05, 15, base)

	m, ok := tr.Measure(sym, base)

	if ok {
		t.Fatalf("level growth without removals should stay silent, got %+v", m)
	}
}

func BenchmarkTrackerApplyOrder(b *testing.B) {
	tr := NewTracker()
	tr.ObserveMid(sym, pair, 100)

	for b.Loop() {
		tr.ApplyOrder(sym, pair, "add", "wall", 'b', 99.95, 10, base, base)
		tr.ApplyOrder(sym, pair, "delete", "wall", 'b', 99.95, 10, base, base)
	}
}

func BenchmarkTrackerMeasure(b *testing.B) {
	tr := NewTracker()
	tr.ObserveMid(sym, pair, 100)
	tr.ApplyOrder(sym, pair, "add", "b1", 'b', 99.95, 10, base, base)
	tr.ObserveTrade(sym, pair, 99.95, 10, base)
	tr.ApplyOrder(sym, pair, "delete", "b1", 'b', 99.95, 10, base, base)
	tr.ApplyOrder(sym, pair, "add", "a1", 'a', 100.05, 10, base, base)
	tr.ApplyOrder(sym, pair, "delete", "a1", 'a', 100.05, 10, base, base)

	for b.Loop() {
		_, _ = tr.Measure(sym, base)
	}
}
