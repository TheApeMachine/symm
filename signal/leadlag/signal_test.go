package leadlag

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/nomagique/correlation"
	"github.com/theapemachine/qpool"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	feed "github.com/theapemachine/symm/signal"
)

func newTestPool(testingTB testing.TB) *qpool.Q[any] {
	testingTB.Helper()

	pool := qpool.NewQ[any](context.Background(), 2, 4, nil)

	if pool == nil {
		testingTB.Fatal("qpool.NewQ returned nil")
	}

	return pool
}

func measurementQuery(scope string) datura.Artifact {
	acquired := datura.Acquire("trader", datura.Artifact_Type_json)
	acquired.WithRole("measurement")
	acquired.WithScope(scope)

	return *acquired
}

func seedTickers(signal *Signal, symbol string, base time.Time, count int, startPrice float64) {
	updates := make(krakenmarket.TickerUpdates, count)

	for index := range count {
		price := startPrice + float64(index)*0.01
		updates[index] = &krakenmarket.TickerUpdate{
			Symbol:    symbol,
			Last:      price,
			Bid:       price - 0.01,
			Ask:       price + 0.01,
			BidQty:    1,
			AskQty:    1,
			Timestamp: base.Add(time.Duration(index) * time.Millisecond),
		}
	}

	signal.Update(feed.TickerFeedArtifact(updates))
}

func TestSignalMeasureTickFollowerColdStart(testingTB *testing.T) {
	Convey("Given aligned anchor and follower paths before the move gate warms", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB))
		start := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)

		for index := range minLagSamples {
			at := start.Add(time.Duration(index) * ringSampleSpacing)
			signal.Section.ObservePrice("BTC/USD", 50000+float64(index), at)
			signal.Section.ObservePrice("ETH/USD", 100+float64(index)*2, at)
		}

		eventAt := start.Add(time.Duration(minLagSamples) * ringSampleSpacing)
		seedTickers(signal, "ETH/USD", eventAt, 4, 100+float64(minLagSamples)*2)

		result := signal.Measure(measurementQuery("ETH/USD"))

		Convey("It should publish a contemporaneous follower reading", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "ETH/USD")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 2)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
		})
	})
}

func TestSignalMeasureTickAnchorColdStart(testingTB *testing.T) {
	Convey("Given an anchor before the move baseline warms", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB))
		start := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)

		for index := range minLagSamples {
			signal.Section.ObservePrice(
				"BTC/USD",
				50000,
				start.Add(time.Duration(index)*ringSampleSpacing),
			)
		}

		eventAt := start.Add(time.Duration(minLagSamples) * ringSampleSpacing)
		seedTickers(signal, "BTC/USD", eventAt, 4, 50000)

		result := signal.Measure(measurementQuery("BTC/USD"))

		Convey("It should withhold until the move baseline warms", func() {
			So(result, ShouldBeNil)
		})
	})
}

func TestSignalMeasureTickAnchorStall(testingTB *testing.T) {
	Convey("Given a flat anchor ticker path", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB))
		start := time.Now().Add(-time.Duration(maxLagBars) * barInterval)

		for index := range anchorMoveMinObs + minLagSamples {
			signal.Section.ObservePrice(
				"BTC/USD",
				50000,
				start.Add(time.Duration(index)*2*time.Minute),
			)
			signal.Section.anchorMove()
		}

		eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		seedTickers(signal, "BTC/USD", eventAt, 4, 50000)

		result := signal.Measure(measurementQuery("BTC/USD"))

		Convey("It should publish anchor stall on the anchor symbol", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 4)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "BTC/USD")
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
		})
	})
}

func TestSectionPriceSamples(testingTB *testing.T) {
	Convey("Given ticker observations", testingTB, func() {
		section := NewSection("BTC/EUR")
		start := time.Now()

		for index := range 20 {
			section.ObservePrice(
				"BTC/EUR",
				100+float64(index),
				start.Add(time.Duration(index)*ringSampleSpacing),
			)
		}

		Convey("It should retain enough samples for correlation", func() {
			So(len(section.PriceSamples("BTC/EUR")), ShouldBeGreaterThanOrEqualTo, minLagSamples)
		})
	})
}

func TestSectionCrossLagInsufficientData(testingTB *testing.T) {
	Convey("Given sparse histories", testingTB, func() {
		section := NewSection("BTC/EUR")
		now := time.Now()

		section.ObservePrice("BTC/EUR", 100, now)
		section.ObservePrice("ETH/EUR", 200, now)

		features := section.Features("ETH/EUR")

		Convey("It should refuse to score lag without enough samples", func() {
			So(features.LagOK, ShouldBeFalse)
		})
	})
}

func TestRecentPathMove(testingTB *testing.T) {
	Convey("Given a flat anchor path across the lag window", testingTB, func() {
		start := time.Now()
		samples := make([]correlation.Sample, minLagSamples)

		for index := range minLagSamples {
			samples[index] = correlation.Sample{
				At:    start.Add(time.Duration(index) * 2 * time.Minute),
				Value: 50000,
			}
		}

		move, ok := recentPathMove(samples, time.Duration(maxLagBars)*barInterval)

		Convey("It should report a near-zero move", func() {
			So(ok, ShouldBeTrue)
			So(move, ShouldBeLessThan, 1e-6)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	pool := qpool.NewQ[any](context.Background(), 2, 4, nil)
	query := measurementQuery("ETH/EUR")
	eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)

	b.ReportAllocs()

	for b.Loop() {
		signal := NewSignal(context.Background(), pool)

		for index := range minLagSamples {
			at := base.Add(time.Duration(index) * time.Minute)
			signal.Section.ObservePrice("BTC/EUR", 50000+float64(index), at)
			signal.Section.ObservePrice("ETH/EUR", 100+float64(index), at.Add(2*time.Minute))
		}

		for range anchorMoveMinObs {
			signal.Section.anchorMove()
		}

		seedTickers(signal, "ETH/EUR", eventAt, 4, 100)

		result := signal.Measure(query)

		if result == nil {
			b.Fatal("Measure returned nil")
		}
	}
}
