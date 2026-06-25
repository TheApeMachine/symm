package sentiment

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/signal/testutil"
)

var sentimentCategories = []logic.CategoryType{
	logic.CategoryRiskOnSurge,
	logic.CategoryDivergentMove,
	logic.CategorySystemicSlump,
}

func TestSignalMeasureCategorySemantics(testingTB *testing.T) {
	Convey("Given broad positive market breadth with a leading symbol", testingTB, func() {
		crossSection := testutil.NewTestCrossSection(testingTB)
		signal := NewSignal(context.Background(), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		symbols := []string{"BTC/USD", "ETH/USD", "SOL/USD", "ADA/USD"}
		var result *datura.Artifact

		for tick := range 20 {
			at := base.Add(time.Duration(tick) * time.Minute).UnixNano()

			for symbolIndex, symbol := range symbols {
				changePct := 1.0 + float64(tick)*0.05 + float64(symbolIndex)*0.1
				last := 100 + float64(tick) + float64(symbolIndex)
				datapoint := testutil.TickerDatapoint(symbol, last, changePct, at)
				measured := signal.Measure(datapoint, crossSection)

				if measured != nil {
					signal.tree = testutil.StoreMeasurement(signal.tree, measured)
					result = measured
				}

				datapoint.Release()
			}
		}

		Convey("It should emit risk-on surge only with leadership confirmation", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 1.0/3.0)
			So(testutil.DominantCategoryIndex(result, sentimentCategories),
				ShouldEqual, logic.CategoryIndex(logic.CategoryRiskOnSurge))
			So(datura.Peek[float64](result, "output", "surgeScore"), ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given a local leader in a weak cross-section", testingTB, func() {
		crossSection := testutil.NewTestCrossSection(testingTB)
		signal := NewSignal(context.Background(), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		flatSymbols := []string{"BTC/USD", "ETH/USD", "SOL/USD", "ADA/USD"}
		var result *datura.Artifact

		for tick := range 16 {
			at := base.Add(time.Duration(tick) * time.Minute).UnixNano()

			for symbolIndex, symbol := range flatSymbols {
				changePct := 1.5 + float64(tick)*0.05 + float64(symbolIndex)*0.1
				datapoint := testutil.TickerDatapoint(symbol, 100+float64(tick), changePct, at)
				_ = signal.Measure(datapoint, crossSection)
				datapoint.Release()
			}
		}

		at := base.Add(16 * time.Minute).UnixNano()

		for symbolIndex, symbol := range flatSymbols {
			datapoint := testutil.TickerDatapoint(symbol, 100, -1-float64(symbolIndex)*0.2, at)
			_ = signal.Measure(datapoint, crossSection)
			datapoint.Release()
		}

		leader := testutil.TickerDatapoint("LEAD/USD", 120, 6, at)
		result = signal.Measure(leader, crossSection)
		leader.Release()

		Convey("It should classify divergent move with divergentScore winning", func() {
			So(result, ShouldNotBeNil)
			So(testutil.DominantCategoryIndex(result, sentimentCategories),
				ShouldEqual, logic.CategoryIndex(logic.CategoryDivergentMove))
			So(datura.Peek[float64](result, "output", "divergentScore"), ShouldBeGreaterThan, 0)
			So(datura.Peek[float64](result, "output", "divergentScore"), ShouldBeGreaterThan,
				datura.Peek[float64](result, "output", "surgeScore"))
			So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 1.0/3.0)
		})
	})

	Convey("Given weak breadth without leadership", testingTB, func() {
		crossSection := testutil.NewTestCrossSection(testingTB)
		signal := NewSignal(context.Background(), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		symbols := []string{"BTC/USD", "ETH/USD", "SOL/USD", "ADA/USD"}
		var result *datura.Artifact

		for tick := range 4 {
			at := base.Add(time.Duration(tick) * time.Minute).UnixNano()

			for symbolIndex, symbol := range symbols {
				datapoint := testutil.TickerDatapoint(symbol, 100, 0.01+float64(symbolIndex)*0.001, at)
				_ = signal.Measure(datapoint, crossSection)
				datapoint.Release()
			}
		}

		for tick := range 20 {
			at := base.Add(time.Duration(tick+4) * time.Minute).UnixNano()

			for symbolIndex, symbol := range symbols {
				changePct := -1.0 - float64(tick)*0.05 - float64(symbolIndex)*0.1
				datapoint := testutil.TickerDatapoint(symbol, 100-float64(tick), changePct, at)
				_ = signal.Measure(datapoint, crossSection)
				datapoint.Release()
			}

			laggard := testutil.TickerDatapoint("FLAT/USD", 100, -0.4, at)
			measured := signal.Measure(laggard, crossSection)

			if measured != nil {
				result = measured
			}

			laggard.Release()
		}

		Convey("It should classify systemic slump with slumpScore winning", func() {
			So(result, ShouldNotBeNil)
			So(testutil.DominantCategoryIndex(result, sentimentCategories),
				ShouldEqual, logic.CategoryIndex(logic.CategorySystemicSlump))
			So(datura.Peek[float64](result, "output", "slumpScore"), ShouldBeGreaterThan, 0)
			So(datura.Peek[float64](result, "output", "slumpScore"), ShouldBeGreaterThan,
				datura.Peek[float64](result, "output", "surgeScore"))
			So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 1.0/3.0)
		})
	})

	Convey("Given flat market breadth with zero majority threshold", testingTB, func() {
		crossSection := testutil.NewTestCrossSection(testingTB)
		signal := NewSignal(context.Background(), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		first := testutil.TickerDatapoint("BTC/USD", 100, 0, base.UnixNano())
		_ = signal.Measure(first, crossSection)
		first.Release()

		second := testutil.TickerDatapoint("ETH/USD", 100, 0, base.Add(time.Minute).UnixNano())
		result := signal.Measure(second, crossSection)
		second.Release()

		Convey("It should publish conviction on the first positive breadth frame", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 0)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	symbols := []string{"BTC/USD", "ETH/USD", "SOL/USD", "ADA/USD"}

	b.ReportAllocs()

	for b.Loop() {
		crossSection := testutil.NewTestCrossSection(b)
		signal := NewSignal(context.Background(), dmt.NewTree(""))

		for tick := range 20 {
			at := base.Add(time.Duration(tick) * time.Minute).UnixNano()

			for symbolIndex, symbol := range symbols {
				datapoint := testutil.TickerDatapoint(symbol, 100+float64(tick), 1+float64(symbolIndex), at)
				_ = signal.Measure(datapoint, crossSection)
				datapoint.Release()
			}
		}

		_ = signal.Close()
	}
}
