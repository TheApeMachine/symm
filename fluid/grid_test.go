package fluid

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
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

func TestBuildFluidGridWarmingWhenSparse(t *testing.T) {
	params := NewDisplayParams()
	builder := NewGridBuilder(params)
	grid := builder.Build(sampleGridRows(3), params.activeGridSize())

	if grid.Max > warmingGridHeight+1e-9 {
		t.Fatalf("expected warming grid max %v, got %v", warmingGridHeight, grid.Max)
	}

	if grid.Heights[0][0] != warmingGridHeight {
		t.Fatalf("expected flat warming height, got %v", grid.Heights[0][0])
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

func TestBuildFluidGridUsesFieldActivityWhenReynoldsZero(t *testing.T) {
	Convey("Given sampled rows with divergence but zero Reynolds", t, func() {
		params := NewDisplayParams()
		builder := NewGridBuilder(params)
		rows := sampleGridRows(64)

		for index := range rows {
			rows[index].Re = 0
			rows[index].Div = float64(index+1) * -0.1
		}

		grid := builder.Build(rows, params.activeGridSize())

		Convey("It should display actual fluid field activity", func() {
			peak := 0.0

			for rowIndex := range grid.Heights {
				for column := range grid.Heights[rowIndex] {
					if grid.Heights[rowIndex][column] > peak {
						peak = grid.Heights[rowIndex][column]
					}
				}
			}

			So(peak, ShouldBeGreaterThan, 0)
			So(grid.Outliers.RawMax, ShouldBeGreaterThan, 0)
		})
	})
}

func TestBuildFluidGridDoesNotUseVolumeWhenFieldActivityZero(t *testing.T) {
	Convey("Given sampled rows with volume but no field activity", t, func() {
		params := NewDisplayParams()
		builder := NewGridBuilder(params)
		rows := sampleGridRows(64)

		for index := range rows {
			rows[index].Re = 0
			rows[index].Div = 0
			rows[index].Vort = 0
			rows[index].Turb = 0
		}

		grid := builder.Build(rows, params.activeGridSize())

		Convey("It should keep the displayed terrain flat", func() {
			for rowIndex := range grid.Heights {
				for column := range grid.Heights[rowIndex] {
					So(grid.Heights[rowIndex][column], ShouldEqual, 0)
				}
			}
		})
	})
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

func TestSummarizeFluidScalingIgnoresZeroActivityForClip(t *testing.T) {
	Convey("Given many flat rows and one active fluid row", t, func() {
		rows := make([]SymbolSnapshot, 64)

		for index := range rows {
			rows[index] = SymbolSnapshot{Symbol: "FLAT/EUR"}
		}

		rows[len(rows)-1] = SymbolSnapshot{
			Symbol: "ACTIVE/EUR",
			Div:    -12,
		}

		summary := summarizeFluidScaling(rows, gridQuantileClip)

		Convey("It should preserve a positive display scale", func() {
			So(summary.ClippedAt, ShouldBeGreaterThan, 0)
			So(summary.DisplayMax, ShouldBeGreaterThan, 0)
			So(summary.RawMaxSymbol, ShouldEqual, "ACTIVE/EUR")
		})
	})
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
