package sentiment

import (
	"context"
	"iter"
	"strconv"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

var sentimentCategories = []logic.CategoryType{
	logic.CategoryRiskOnSurge,
	logic.CategoryDivergentMove,
	logic.CategorySystemicSlump,
}

func TestSignalMeasureCategorySemantics(testingTB *testing.T) {
	Convey("Given broad positive market breadth with a leading symbol", testingTB, func() {
		crossSection := newTestCrossSection(testingTB)
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
				datapoint := tickerDatapoint(symbol, last, changePct, at)
				observePeers(crossSection, datapoint)
				measured := firstMeasured(signal.Measure(datapoint, crossSection))

				if measured != nil {
					signal.tree = storeMeasurement(signal.tree, measured)
					result = measured
				}

				datapoint.Release()
			}
		}

		Convey("It should emit risk-on surge only with leadership confirmation", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 1.0/3.0)
			So(dominantCategoryIndex(result, sentimentCategories),
				ShouldEqual, logic.CategoryIndex(logic.CategoryRiskOnSurge))
			So(datura.Peek[float64](result, "output", "surgeScore"), ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given a local leader in a weak cross-section", testingTB, func() {
		crossSection := newTestCrossSection(testingTB)
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
				datapoint := tickerDatapoint(symbol, 100+float64(tick), changePct, at)
				observePeers(crossSection, datapoint)
				_ = firstMeasured(signal.Measure(datapoint, crossSection))
				datapoint.Release()
			}
		}

		at := base.Add(16 * time.Minute).UnixNano()

		for symbolIndex, symbol := range flatSymbols {
			datapoint := tickerDatapoint(symbol, 100, -1-float64(symbolIndex)*0.2, at)
			observePeers(crossSection, datapoint)
			_ = firstMeasured(signal.Measure(datapoint, crossSection))
			datapoint.Release()
		}

		leader := tickerDatapoint("LEAD/USD", 120, 6, at)
		observePeers(crossSection, leader)
		result = firstMeasured(signal.Measure(leader, crossSection))
		leader.Release()

		Convey("It should classify divergent move with divergentScore winning", func() {
			So(result, ShouldNotBeNil)
			So(dominantCategoryIndex(result, sentimentCategories),
				ShouldEqual, logic.CategoryIndex(logic.CategoryDivergentMove))
			So(datura.Peek[float64](result, "output", "divergentScore"), ShouldBeGreaterThan, 0)
			So(datura.Peek[float64](result, "output", "divergentScore"), ShouldBeGreaterThan,
				datura.Peek[float64](result, "output", "surgeScore"))
			So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 1.0/3.0)
		})
	})

	Convey("Given weak breadth without leadership", testingTB, func() {
		crossSection := newTestCrossSection(testingTB)
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
				datapoint := tickerDatapoint(symbol, 100, 0.01+float64(symbolIndex)*0.001, at)
				observePeers(crossSection, datapoint)
				_ = firstMeasured(signal.Measure(datapoint, crossSection))
				datapoint.Release()
			}
		}

		for tick := range 20 {
			at := base.Add(time.Duration(tick+4) * time.Minute).UnixNano()

			for symbolIndex, symbol := range symbols {
				changePct := -1.0 - float64(tick)*0.05 - float64(symbolIndex)*0.1
				datapoint := tickerDatapoint(symbol, 100-float64(tick), changePct, at)
				observePeers(crossSection, datapoint)
				_ = firstMeasured(signal.Measure(datapoint, crossSection))
				datapoint.Release()
			}

			laggard := tickerDatapoint("FLAT/USD", 100, -0.4, at)
			measured := firstMeasured(signal.Measure(laggard, crossSection))

			if measured != nil {
				result = measured
			}

			laggard.Release()
		}

		Convey("It should classify systemic slump with slumpScore winning", func() {
			So(result, ShouldNotBeNil)
			So(dominantCategoryIndex(result, sentimentCategories),
				ShouldEqual, logic.CategoryIndex(logic.CategorySystemicSlump))
			So(datura.Peek[float64](result, "output", "slumpScore"), ShouldBeGreaterThan, 0)
			So(datura.Peek[float64](result, "output", "slumpScore"), ShouldBeGreaterThan,
				datura.Peek[float64](result, "output", "surgeScore"))
			So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 1.0/3.0)
		})
	})

	Convey("Given leadership before a threshold exists", testingTB, func() {
		crossSection := newTestCrossSection(testingTB)
		signal := NewSignal(context.Background(), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		leader := tickerDatapoint("LEAD/USD", 100, 5, base.UnixNano())
		observePeers(crossSection, leader)
		result := firstMeasured(signal.Measure(leader, crossSection))
		leader.Release()

		Convey("It should abstain instead of inventing unit leader evidence", func() {
			So(result, ShouldBeNil)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	symbols := []string{"BTC/USD", "ETH/USD", "SOL/USD", "ADA/USD"}

	b.ReportAllocs()

	for b.Loop() {
		crossSection := newTestCrossSection(b)
		signal := NewSignal(context.Background(), dmt.NewTree(""))

		for tick := range 20 {
			at := base.Add(time.Duration(tick) * time.Minute).UnixNano()

			for symbolIndex, symbol := range symbols {
				datapoint := tickerDatapoint(symbol, 100+float64(tick), 1+float64(symbolIndex), at)
				observePeers(crossSection, datapoint)
				_ = firstMeasured(signal.Measure(datapoint, crossSection))
				datapoint.Release()
			}
		}

		_ = signal.Close()
	}
}

func newTestCrossSection(testingTB testing.TB) *market.CrossSection {
	if testingTB != nil {
		testingTB.Helper()
	}

	crossSection, err := market.NewCrossSection(
		market.CrossSectionConfig{
			ReturnCap:  16,
			MinBars:    6,
			BreadthCap: 16,
		},
	)

	if err != nil && testingTB != nil {
		testingTB.Fatal(err)
	}

	return crossSection
}

func tickerDatapoint(symbol string, last float64, changePct float64, timestamp int64) *datura.Artifact {
	artifact := datura.Acquire("kraken:public", datura.APPJSON)
	artifact.WithRole("ticker")
	artifact.WithScope(symbol)
	artifact.WithPayload(datura.Map[any]{
		"channel": "ticker",
		"type":    "update",
		"data": []datura.Map[any]{
			{
				"symbol":     symbol,
				"last":       last,
				"volume":     1000.0,
				"change_pct": changePct,
			},
		},
	}.Marshal())
	artifact.SetTimestamp(timestamp)

	return artifact
}

func observePeers(crossSection *market.CrossSection, datapoint *datura.Artifact) {
	_ = crossSection.Observe(kraken.TickerDataSlice{{
		Symbol:    datura.Peek[string](datapoint, "data", 0, "symbol"),
		Last:      datura.Peek[float64](datapoint, "data", 0, "last"),
		Volume:    datura.Peek[float64](datapoint, "data", 0, "volume"),
		ChangePct: datura.Peek[float64](datapoint, "data", 0, "change_pct"),
		Timestamp: time.Unix(0, datapoint.Timestamp()).UTC(),
	}})
}

func firstMeasured(measurements iter.Seq[*datura.Artifact]) *datura.Artifact {
	for measurement := range measurements {
		return measurement
	}

	return nil
}

func storeMeasurement(tree *dmt.Tree, measurement *datura.Artifact) *dmt.Tree {
	if measurement == nil {
		return tree
	}

	updated, _, _ := tree.InsertArtifact(measurement.Prefix(), measurement)

	if updated == nil {
		return tree
	}

	return updated
}

func categoryMass(result *datura.Artifact, category logic.CategoryType) float64 {
	distribution := datura.Peek[map[string]any](result, "output", "distribution")
	mass, _ := distribution[strconv.Itoa(logic.CategoryIndex(category))].(float64)

	return mass
}

func dominantCategoryIndex(result *datura.Artifact, categories []logic.CategoryType) int {
	best := categories[0]
	bestMass := categoryMass(result, best)

	for _, category := range categories[1:] {
		mass := categoryMass(result, category)

		if mass > bestMass {
			best = category
			bestMass = mass
		}
	}

	return logic.CategoryIndex(best)
}
