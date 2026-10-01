package store_test

import (
	"fmt"
	"testing"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
)

func TestRegionVocabularyAntiCorrelatedGroups(t *testing.T) {
	grid := store.NewGrid()
	const n = 48
	values := make([]float64, n)
	for tick := 0; tick < 100; tick++ {
		m := data.NewMeasurement[float64]("BTC/USD", nil)
		for i := 0; i < n; i++ {
			group := i / 16 // 3 groups
			switch group {
			case 0:
				values[i] += 0.5
			case 1:
				values[i] -= 0.5
			default:
				values[i] += 0.1 * float64((tick%2)*2-1)
			}
			label := fmt.Sprintf("m%02d", i)
			val := values[i]
			m.SetMetric(label, data.Metric[float64]{Label: label, Raw: val, Standardized: &val})
		}
		grid.Update(m)
	}
	grid.ForceSettle()
	counts := map[uint8]int{}
	for _, metric := range grid.Metrics {
		if metric != nil && metric.Region > 0 {
			counts[metric.Region]++
		}
	}
	t.Logf("metrics=%d regions=%d", len(grid.Metrics), len(counts))
	if len(counts) < 2 {
		t.Fatalf("expected multiple regions for anti-correlated groups, got %d", len(counts))
	}
	if len(counts) >= n {
		t.Fatalf("no dim reduction: %d regions for %d metrics", len(counts), n)
	}
	multi := 0
	for _, c := range counts {
		if c > 1 {
			multi++
		}
	}
	if multi == 0 {
		t.Fatal("expected at least one multi-member region")
	}
}
