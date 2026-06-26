package liquidity

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/signal/testutil"
)

var liquidityCategories = []logic.CategoryType{
	logic.CategoryExtremeScarcity,
	logic.CategoryMedianDepth,
	logic.CategoryRobustLiquidity,
}

func categoryResult(result *datura.Artifact) int {
	return testutil.DominantCategoryIndex(result, liquidityCategories)
}

const liquidityCrossSectionWarmupTicks = 4

var classifierInputs = []string{"scarcityScore", "medianScore", "depthScore"}

func outputScore(result *datura.Artifact, key string) float64 {
	return datura.Peek[float64](result, "output", key)
}

func winningClassifierInput(result *datura.Artifact) string {
	bestKey := classifierInputs[0]
	bestScore := outputScore(result, bestKey)

	for _, key := range classifierInputs[1:] {
		score := outputScore(result, key)

		if score > bestScore {
			bestScore = score
			bestKey = key
		}
	}

	return bestKey
}

func warmupCrossSection(signal *Signal, crossSection *market.CrossSection, base time.Time) {
	symbols := []struct {
		name   string
		volume float64
	}{
		{"BTC/USD", 1100},
		{"ETH/USD", 950},
		{"SOL/USD", 1000},
	}

	for tick := range liquidityCrossSectionWarmupTicks {
		at := base.Add(time.Duration(tick) * time.Minute).UnixNano()

		for symbolIndex, row := range symbols {
			last := 100 + float64(tick) + float64(symbolIndex)
			datapoint := testutil.TickerDatapointWithVolume(row.name, last, row.volume, 0.1, at)
			testutil.ObservePeers(crossSection, datapoint)
			_ = testutil.FirstMeasured(signal.Measure(datapoint, crossSection))
			datapoint.Release()
		}
	}
}

func refreshMedianCrossSectionPeers(signal *Signal, crossSection *market.CrossSection, base time.Time, tick int) {
	rows := []struct {
		name   string
		volume float64
	}{
		{"BTC/USD", 1200},
		{"ETH/USD", 950},
		{"SOL/USD", 1050},
		{"LOW/USD", 800},
	}

	at := base.Add(time.Duration(tick) * time.Minute).UnixNano()

	for symbolIndex, row := range rows {
		last := 100 + float64(tick) + float64(symbolIndex)
		datapoint := testutil.TickerDatapointWithVolume(row.name, last, row.volume, 0.1, at)
		testutil.ObservePeers(crossSection, datapoint)
		_ = testutil.FirstMeasured(signal.Measure(datapoint, crossSection))
		datapoint.Release()
	}
}

func measureBestMedianVolume(
	signal *Signal,
	crossSection *market.CrossSection,
	symbol string,
	volume float64,
	base time.Time,
	fromTick, toTick int,
) *datura.Artifact {
	var (
		result         *datura.Artifact
		bestConfidence float64
	)

	for tick := fromTick; tick <= toTick; tick++ {
		refreshMedianCrossSectionPeers(signal, crossSection, base, tick)
		measured := measureSymbolVolume(signal, crossSection, symbol, volume, base, tick)

		if measured == nil {
			continue
		}

		confidence := outputScore(measured, "confidence")

		if confidence > bestConfidence {
			if result != nil {
				result.Release()
			}

			result = measured
			bestConfidence = confidence

			continue
		}

		measured.Release()
	}

	return result
}

func refreshCrossSectionPeers(signal *Signal, crossSection *market.CrossSection, base time.Time, tick int) {
	rows := []struct {
		name   string
		volume float64
	}{
		{"BTC/USD", 1100},
		{"ETH/USD", 950},
		{"SOL/USD", 1000},
		{"MEDIAN/USD", 1000},
	}

	at := base.Add(time.Duration(tick) * time.Minute).UnixNano()

	for symbolIndex, row := range rows {
		last := 100 + float64(tick) + float64(symbolIndex)
		datapoint := testutil.TickerDatapointWithVolume(row.name, last, row.volume, 0.1, at)
		testutil.ObservePeers(crossSection, datapoint)
		_ = testutil.FirstMeasured(signal.Measure(datapoint, crossSection))
		datapoint.Release()
	}
}

func measureSymbolVolume(
	signal *Signal,
	crossSection *market.CrossSection,
	symbol string,
	volume float64,
	base time.Time,
	tick int,
) *datura.Artifact {
	at := base.Add(time.Duration(tick) * time.Minute).UnixNano()
	datapoint := testutil.TickerDatapointWithVolume(symbol, 100, volume, 0.1, at)
	testutil.ObservePeers(crossSection, datapoint)
	measured := testutil.FirstMeasured(signal.Measure(datapoint, crossSection))
	signal.tree = testutil.StoreMeasurement(signal.tree, measured)
	datapoint.Release()

	return measured
}

func measureBestSymbolVolume(
	signal *Signal,
	crossSection *market.CrossSection,
	symbol string,
	volume float64,
	base time.Time,
	fromTick, toTick int,
) *datura.Artifact {
	var (
		result         *datura.Artifact
		bestConfidence float64
	)

	for tick := fromTick; tick <= toTick; tick++ {
		refreshCrossSectionPeers(signal, crossSection, base, tick)
		measured := measureSymbolVolume(signal, crossSection, symbol, volume, base, tick)

		if measured == nil {
			continue
		}

		confidence := datura.Peek[float64](measured, "output", "confidence")

		if confidence > bestConfidence {
			if result != nil {
				result.Release()
			}

			result = measured
			bestConfidence = confidence

			continue
		}

		measured.Release()
	}

	return result
}

func TestSignalMeasureCategorySemantics(testingTB *testing.T) {
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)

	Convey("Given cross-section warmup and peak scarcity volume", testingTB, func() {
		crossSection := testutil.NewTestCrossSection(testingTB)
		signal := NewSignal(context.Background(), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		warmupCrossSection(signal, crossSection, base)
		result := measureBestSymbolVolume(
			signal, crossSection, "SCARCE/USD", 50, base,
			liquidityCrossSectionWarmupTicks, liquidityCrossSectionWarmupTicks+3,
		)

		Convey("It should classify extreme scarcity with scarcityScore winning", func() {
			So(result, ShouldNotBeNil)
			So(outputScore(result, "scarcityScore"), ShouldBeGreaterThan, outputScore(result, "medianScore"))
			So(outputScore(result, "scarcityScore"), ShouldBeGreaterThan, outputScore(result, "depthScore"))
			So(winningClassifierInput(result), ShouldEqual, "scarcityScore")
			So(categoryResult(result), ShouldEqual, logic.CategoryIndex(logic.CategoryExtremeScarcity))

			result.Release()
		})
	})

	Convey("Given cross-section warmup and median-band volume", testingTB, func() {
		crossSection := testutil.NewTestCrossSection(testingTB)
		signal := NewSignal(context.Background(), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		warmupCrossSection(signal, crossSection, base)
		result := measureBestMedianVolume(
			signal, crossSection, "ETH/USD", 1020, base,
			liquidityCrossSectionWarmupTicks, liquidityCrossSectionWarmupTicks+12,
		)

		Convey("It should classify median depth with medianScore winning", func() {
			So(result, ShouldNotBeNil)
			So(categoryResult(result), ShouldEqual, logic.CategoryIndex(logic.CategoryMedianDepth))
			So(outputScore(result, "medianScore"), ShouldBeGreaterThan, 0)
			So(outputScore(result, "medianScore"), ShouldBeGreaterThan, outputScore(result, "depthScore"))
			So(winningClassifierInput(result), ShouldEqual, "medianScore")
			So(categoryResult(result), ShouldEqual, logic.CategoryIndex(logic.CategoryMedianDepth))

			result.Release()
		})
	})

	Convey("Given cross-section warmup and deep volume", testingTB, func() {
		crossSection := testutil.NewTestCrossSection(testingTB)
		signal := NewSignal(context.Background(), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		warmupCrossSection(signal, crossSection, base)

		for tick := range liquidityCrossSectionWarmupTicks {
			_ = measureSymbolVolume(signal, crossSection, "DEEP/USD", 1500, base, tick)
		}

		result := measureBestSymbolVolume(
			signal, crossSection, "DEEP/USD", 2500, base,
			liquidityCrossSectionWarmupTicks, liquidityCrossSectionWarmupTicks+3,
		)

		Convey("It should classify robust liquidity with depthScore winning", func() {
			So(result, ShouldNotBeNil)
			So(outputScore(result, "depthScore"), ShouldBeGreaterThan, outputScore(result, "scarcityScore"))
			So(outputScore(result, "depthScore"), ShouldBeGreaterThan, outputScore(result, "medianScore"))
			So(winningClassifierInput(result), ShouldEqual, "depthScore")
			So(categoryResult(result), ShouldEqual, logic.CategoryIndex(logic.CategoryRobustLiquidity))

			result.Release()
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)

	b.ReportAllocs()

	for b.Loop() {
		crossSection := testutil.NewTestCrossSection(b)
		signal := NewSignal(context.Background(), dmt.NewTree(""))

		if signal == nil {
			b.Fatal("NewSignal returned nil")
		}

		warmupCrossSection(signal, crossSection, base)
		result := measureSymbolVolume(signal, crossSection, "DEEP/USD", 2000, base, liquidityCrossSectionWarmupTicks)

		if result == nil {
			b.Fatal("Measure returned nil")
		}

		if categoryResult(result) != logic.CategoryIndex(logic.CategoryRobustLiquidity) {
			b.Fatalf("Measure classified category %d, want robust liquidity (%d)",
				categoryResult(result), logic.CategoryIndex(logic.CategoryRobustLiquidity))
		}

		result.Release()
		_ = signal.Close()
	}
}
