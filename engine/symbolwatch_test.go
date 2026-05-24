package engine

import (
	"fmt"
	"testing"
	"time"

	"github.com/theapemachine/symm/config"
)

func TestSymbolWatchPrioritizesDirtySymbols(t *testing.T) {
	config.System.MaxScanSymbols = 2

	watch := NewSymbolWatch([]string{"AAA/EUR", "BBB/EUR", "CCC/EUR"})
	watch.NoteTrade("BBB/EUR", 10)
	watch.NoteBook("CCC/EUR")

	set := watch.ScanSet(2)

	if len(set) != 2 {
		t.Fatalf("expected scan budget of 2, got %d", len(set))
	}

	if set[0] != "BBB/EUR" {
		t.Fatalf("expected hottest dirty symbol first, got %q", set[0])
	}
}

func TestSymbolWatchRotatesColdSymbols(t *testing.T) {
	config.System.MaxScanSymbols = 1

	watch := NewSymbolWatch([]string{"AAA/EUR", "BBB/EUR", "CCC/EUR"})

	first := watch.ScanSet(1)
	watch.AdvanceRotation(1)
	watch.Decay(time.Now())
	second := watch.ScanSet(1)

	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("expected one symbol per scan, got %v and %v", first, second)
	}

	if first[0] == second[0] {
		t.Fatalf("expected rotation to change cold symbol, got %q twice", first[0])
	}
}

func BenchmarkSymbolWatchScanSet(b *testing.B) {
	symbols := make([]string, 0, 128)

	for index := 0; index < 128; index++ {
		symbols = append(symbols, fmt.Sprintf("SYM%d/EUR", index))
	}

	config.System.MaxScanSymbols = 64
	watch := NewSymbolWatch(symbols)
	watch.NoteTrade("SYM0/EUR", 5)

	b.ReportAllocs()

	for b.Loop() {
		watch.ScanSet(64)
	}
}
