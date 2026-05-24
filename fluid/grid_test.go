package fluid

import (
	"testing"
)

func sampleGridRows(count int) []SymbolSnapshot {
	rows := make([]SymbolSnapshot, count)

	for index := range rows {
		rows[index] = SymbolSnapshot{
			Symbol:    "SYM/EUR",
			ChangePct: float64(index) * 0.1,
			Vol:       float64(index+1) * 10,
			Re:        float64(index+1) * 0.05,
		}
	}

	return rows
}

func TestBuildFluidGridBinsRows(t *testing.T) {
	params := NewDisplayParams()
	builder := NewGridBuilder(params)
	grid := builder.Build(sampleGridRows(100), params.activeGridSize())

	if grid.Size != GridSize {
		t.Fatalf("expected grid size %d, got %d", GridSize, grid.Size)
	}

	if len(grid.Heights) != GridSize || len(grid.Heights[0]) != GridSize {
		t.Fatal("expected square height matrix")
	}

	if grid.FilledCells <= 0 {
		t.Fatal("expected filled cells")
	}

	if grid.Max <= grid.Min {
		t.Fatalf("expected min < max, min=%v max=%v", grid.Min, grid.Max)
	}
}

func TestBuildFluidGridEMASmoothsAcrossTicks(t *testing.T) {
	params := NewDisplayParams()
	builder := NewGridBuilder(params)
	first := builder.Build(sampleGridRows(100), params.activeGridSize())

	shifted := sampleGridRows(100)

	for index := range shifted {
		shifted[index].Re *= 1.5
	}

	second := builder.Build(shifted, params.activeGridSize())

	if first.Heights[0][0] == second.Heights[0][0] {
		t.Fatal("expected EMA smoothing to blend prior and new heights")
	}
}

func TestBuildFluidGridSanitizesNaNHeights(t *testing.T) {
	params := NewDisplayParams()
	builder := NewGridBuilder(params)
	grid := builder.Build(sampleGridRows(100), params.activeGridSize())

	for rowIndex := range grid.Heights {
		for column := range grid.Heights[rowIndex] {
			if !isFinite(grid.Heights[rowIndex][column]) {
				t.Fatalf(
					"expected finite height at [%d][%d], got %v",
					rowIndex,
					column,
					grid.Heights[rowIndex][column],
				)
			}
		}
	}
}

func TestSummarizeFluidScalingClipsOutliers(t *testing.T) {
	rows := []SymbolSnapshot{
		{Symbol: "A/EUR", Re: 1},
		{Symbol: "B/EUR", Re: 2},
		{Symbol: "C/EUR", Re: 100},
	}

	summary := summarizeFluidScaling(rows, gridQuantileClip)

	if summary.ClippedCount == 0 {
		t.Fatal("expected clipped outliers")
	}

	if summary.RawMaxSymbol != "C/EUR" {
		t.Fatalf("expected raw max symbol C/EUR, got %q", summary.RawMaxSymbol)
	}
}

func BenchmarkBuildFluidGrid(b *testing.B) {
	params := NewDisplayParams()
	builder := NewGridBuilder(params)
	rows := sampleGridRows(128)

	b.ReportAllocs()

	for b.Loop() {
		builder.Build(rows, params.activeGridSize())
	}
}
