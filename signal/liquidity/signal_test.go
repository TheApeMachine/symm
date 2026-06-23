package liquidity

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
)

const liquidityCrossSectionWarmupTicks = 4

func categoryResult(result *datura.Artifact) int {
	return int(datura.Peek[float64](result, "output", "category"))
}

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

func newTestPool(testingTB testing.TB) *qpool.Q[any] {
	if testingTB != nil {
		testingTB.Helper()
	}

	pool := qpool.NewQ[any](context.Background(), 2, 4, nil)

	if pool == nil && testingTB != nil {
		testingTB.Fatal("qpool.NewQ returned nil")
	}

	return pool
}

func tickerDatapoint(symbol string, last, volume, changePct float64, timestamp int64) *datura.Artifact {
	payload := fmt.Sprintf(
		`{"channel":"ticker","type":"update","data":[{"symbol":%q,"bid":%g,"bid_qty":740.0,"ask":%g,"ask_qty":740.0,"last":%g,"volume":%g,"change_pct":%g}]}`,
		symbol, last-0.01, last+0.01, last, volume, changePct,
	)
	artifact := datura.Acquire("kraken:public", datura.APPJSON)
	artifact.WithRole("ticker")
	artifact.WithScope("update")
	artifact.WithPayload([]byte(payload))
	artifact.SetTimestamp(timestamp)

	return artifact
}

func warmupCrossSection(signal *Signal, base time.Time) {
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
			datapoint := tickerDatapoint(row.name, last, row.volume, 0.1, at)
			_ = signal.Measure(datapoint)
			datapoint.Release()
		}
	}
}

func refreshMedianCrossSectionPeers(signal *Signal, base time.Time, tick int) {
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
		datapoint := tickerDatapoint(row.name, last, row.volume, 0.1, at)
		_ = signal.Measure(datapoint)
		datapoint.Release()
	}
}

func measureBestMedianVolume(
	signal *Signal,
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
		refreshMedianCrossSectionPeers(signal, base, tick)
		measured := measureSymbolVolume(signal, symbol, volume, base, tick)

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

func refreshCrossSectionPeers(signal *Signal, base time.Time, tick int) {
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
		datapoint := tickerDatapoint(row.name, last, row.volume, 0.1, at)
		_ = signal.Measure(datapoint)
		datapoint.Release()
	}
}

func measureSymbolVolume(
	signal *Signal,
	symbol string,
	volume float64,
	base time.Time,
	tick int,
) *datura.Artifact {
	at := base.Add(time.Duration(tick) * time.Minute).UnixNano()
	datapoint := tickerDatapoint(symbol, 100, volume, 0.1, at)
	measured := signal.Measure(datapoint)
	datapoint.Release()

	return measured
}

func measureBestSymbolVolume(
	signal *Signal,
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
		refreshCrossSectionPeers(signal, base, tick)
		measured := measureSymbolVolume(signal, symbol, volume, base, tick)

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

func measureBestSymbolVolumeForScore(
	signal *Signal,
	symbol string,
	volume float64,
	base time.Time,
	fromTick, toTick int,
	scoreKey string,
) *datura.Artifact {
	var (
		result    *datura.Artifact
		bestScore float64
	)

	for tick := fromTick; tick <= toTick; tick++ {
		refreshCrossSectionPeers(signal, base, tick)
		measured := measureSymbolVolume(signal, symbol, volume, base, tick)

		if measured == nil {
			continue
		}

		score := outputScore(measured, scoreKey)

		if score > bestScore {
			if result != nil {
				result.Release()
			}

			result = measured
			bestScore = score

			continue
		}

		measured.Release()
	}

	return result
}

func depthFeaturesPayload(
	scaledQuoteVol float64,
	peers []float64,
	relativeVolume float64,
	baselineReady bool,
) []float64 {
	samples := []float64{scaledQuoteVol, float64(len(peers))}
	samples = append(samples, peers...)
	samples = append(samples, relativeVolume)

	baselineFlag := 0.0

	if baselineReady {
		baselineFlag = 1
	}

	samples = append(samples, baselineFlag)

	return samples
}

func TestSignalMeasureCategorySemantics(testingTB *testing.T) {
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)

	Convey("Given cross-section warmup and peak scarcity volume", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		warmupCrossSection(signal, base)
		result := measureBestSymbolVolume(
			signal, "SCARCE/USD", 50, base,
			liquidityCrossSectionWarmupTicks, liquidityCrossSectionWarmupTicks+3,
		)

		Convey("It should classify extreme scarcity with scarcityScore winning", func() {
			So(result, ShouldNotBeNil)
			So(outputScore(result, "scarcityScore"), ShouldBeGreaterThan, outputScore(result, "medianScore"))
			So(outputScore(result, "scarcityScore"), ShouldBeGreaterThan, outputScore(result, "depthScore"))
			So(winningClassifierInput(result), ShouldEqual, "scarcityScore")
			So(categoryResult(result), ShouldEqual, 1)

			result.Release()
		})
	})

	Convey("Given cross-section warmup and median-band volume", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		warmupCrossSection(signal, base)
		result := measureBestMedianVolume(
			signal, "ETH/USD", 1020, base,
			liquidityCrossSectionWarmupTicks, liquidityCrossSectionWarmupTicks+12,
		)

		Convey("It should classify median depth with medianScore winning", func() {
			So(result, ShouldNotBeNil)
			So(categoryResult(result), ShouldEqual, 2)
			So(outputScore(result, "medianScore"), ShouldBeGreaterThan, 0)
			So(outputScore(result, "medianScore"), ShouldBeGreaterThan, outputScore(result, "depthScore"))
			So(winningClassifierInput(result), ShouldEqual, "medianScore")
			So(categoryResult(result), ShouldEqual, 2)

			result.Release()
		})
	})

	Convey("Given cross-section warmup and deep volume", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		warmupCrossSection(signal, base)

		for tick := range liquidityCrossSectionWarmupTicks {
			_ = measureSymbolVolume(signal, "DEEP/USD", 1500, base, tick)
		}

		result := measureBestSymbolVolume(
			signal, "DEEP/USD", 2500, base,
			liquidityCrossSectionWarmupTicks, liquidityCrossSectionWarmupTicks+3,
		)

		Convey("It should classify robust liquidity with depthScore winning", func() {
			So(result, ShouldNotBeNil)
			So(outputScore(result, "depthScore"), ShouldBeGreaterThan, outputScore(result, "scarcityScore"))
			So(outputScore(result, "depthScore"), ShouldBeGreaterThan, outputScore(result, "medianScore"))
			So(winningClassifierInput(result), ShouldEqual, "depthScore")
			So(categoryResult(result), ShouldEqual, 3)

			result.Release()
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)

	b.ReportAllocs()

	for b.Loop() {
		signal := NewSignal(context.Background(), newTestPool(b), dmt.NewTree(""))

		if signal == nil {
			b.Fatal("NewSignal returned nil")
		}

		warmupCrossSection(signal, base)
		result := measureSymbolVolume(signal, "DEEP/USD", 2000, base, liquidityCrossSectionWarmupTicks)

		if result == nil {
			b.Fatal("Measure returned nil")
		}

		if categoryResult(result) != 3 {
			b.Fatalf("Measure classified category %d, want robust liquidity (3)", categoryResult(result))
		}

		result.Release()
		_ = signal.Close()
	}
}
