package depthflow

import (
	"context"
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	spotbook "github.com/theapemachine/api-go/v2/pkg/book"
	"github.com/theapemachine/api-go/v2/pkg/decimal"
	"github.com/theapemachine/nomagique/algorithm/book/flow"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestMeasure(t *testing.T) {
	Convey("Given independent depth-flow symbols without a ready book", t, func() {
		signal := &Signal{ctx: context.Background(), books: emptyBookSource{}}
		thesis := types.NewThesis(t.Context(), nil)
		thesis.Tickers.Store("AAA/USD", []kraken.TickerData{{Symbol: "AAA/USD"}})
		thesis.Tickers.Store("BBB/USD", []kraken.TickerData{{Symbol: "BBB/USD"}})

		Reset(func() {
			signal.Close()
		})

		Convey("It completes each independent symbol pass without measurements", func() {
			So(signal.Measure(thesis), ShouldBeEmpty)
			So(signal.Measure(thesis), ShouldBeEmpty)
		})
	})
}

type emptyBookSource struct{}

func (emptyBookSource) Book(string) *spotbook.Book {
	return nil
}

func TestSeenTrade(t *testing.T) {
	Convey("Given an exact-once cursor for one depth-flow symbol", t, func() {
		signal := &Signal{lastTrade: &sync.Map{}}
		at := time.Unix(1_700_000_000, 0).UTC()
		first := kraken.TradeData{Symbol: "ALT/USD", TradeID: 11, Timestamp: at}
		secondSameTime := kraken.TradeData{Symbol: "ALT/USD", TradeID: 12, Timestamp: at}
		regressed := kraken.TradeData{
			Symbol: "ALT/USD", TradeID: 13, Timestamp: at.Add(-time.Nanosecond),
		}

		Convey("It should accept distinct same-time IDs and reject replay or regression", func() {
			So(signal.seenTrade(first), ShouldBeFalse)
			signal.commitTrade(first)
			So(signal.seenTrade(first), ShouldBeTrue)
			So(signal.seenTrade(secondSameTime), ShouldBeFalse)
			signal.commitTrade(secondSameTime)
			So(signal.seenTrade(secondSameTime), ShouldBeTrue)
			So(signal.seenTrade(regressed), ShouldBeTrue)
		})
	})

	Convey("Given same-time depth-flow trades without exchange IDs", t, func() {
		signal := &Signal{lastTrade: &sync.Map{}}
		at := time.Unix(1_700_000_100, 0).UTC()
		unidentified := kraken.TradeData{Symbol: "ALT/USD", Timestamp: at}

		signal.commitTrade(unidentified)

		Convey("It should document intrinsic indistinguishability by rejecting the second zero-ID event", func() {
			So(signal.seenTrade(unidentified), ShouldBeTrue)
		})
	})
}

func TestSnapshotDelta(t *testing.T) {
	Convey("Given a full snapshot where one bid vanished", t, func() {
		previous := map[int64]flow.BookLevel{
			100: {Price: 100, Ticks: 100, Quantity: 10},
			99:  {Price: 99, Ticks: 99, Quantity: 8},
		}
		current := map[int64]flow.BookLevel{
			100: {Price: 100, Ticks: 100, Quantity: 7},
		}
		delta := snapshotDelta(current, previous)

		Convey("It should update retained depth and explicitly delete the vanished level", func() {
			So(levelQuantity(delta, 100), ShouldEqual, 7.0)
			So(levelQuantity(delta, 99), ShouldEqual, 0.0)
		})
	})
}

func TestMeasureTrade(t *testing.T) {
	Convey("Given a multi-leg book baseline before aligned aggressive flow", t, func() {
		sample, err := flow.NewSample(8)
		So(err, ShouldBeNil)
		signal := &Signal{
			sample:   sample,
			bookflow: equation.NewBookflow(),
		}

		for range 3 {
			_, _, _, err = sample.MeasureBook(depthflowBookInput(10, 10))
			So(err, ShouldBeNil)
		}

		_, _, _, err = sample.MeasureBook(depthflowBookInput(20, 8))
		So(err, ShouldBeNil)

		at := time.Unix(1_700_000_200, 0).UTC()
		measurements, err := signal.measureTrade(kraken.TradeData{
			Symbol: "BTC/USD", Side: "buy", Price: *decimal.NewFromInt64(100),
			Qty: 5, TradeID: 21, Timestamp: at,
		})

		Convey("It should emit the preserved dimensionless metric contract at the trade time", func() {
			So(err, ShouldBeNil)
			So(measurements, ShouldHaveLength, 1)
			measurement := measurements[0]
			So(measurement.At, ShouldResemble, at)
			So(measurement.Metrics, ShouldHaveLength, 6)
			So(measurement.Sample(types.MetricLoadedScore, types.SideNone).Raw,
				ShouldBeGreaterThan, 0)

			for _, sample := range measurement.Metrics {
				So(sample.Unit, ShouldEqual, types.UnitDimensionless)
			}
		})
	})
}

func TestFrame(t *testing.T) {
	Convey("Given a depth depletion score computed against prior quote notional", t, func() {
		signal := &Signal{}
		at := time.Unix(1_700_000_300, 0).UTC()
		measurements := signal.frame("BTC/USD", at, equation.BookflowOutput{
			Value: 0.3, Strength: 0.3, ThinScore: 0.3, Category: 3, Ready: true,
		}, 0.8)

		Convey("It should preserve the dimensionless depletion fraction and provenance", func() {
			So(measurements, ShouldHaveLength, 1)
			measurement := measurements[0]
			So(measurement.At, ShouldResemble, at)
			So(measurement.Maturity, ShouldEqual, 0.8)
			So(measurement.Sample(types.MetricThinScore, types.SideNone).Raw,
				ShouldAlmostEqual, 0.3, 1e-12)
			So(*measurement.Sample(types.MetricThinScore, types.SideNone).Normalized,
				ShouldAlmostEqual, 0.3, 1e-12)
		})
	})

	Convey("Given the full domain contrast between opposite book imbalances", t, func() {
		measurement := (&Signal{}).frame(
			"BTC/USD",
			time.Unix(1_700_000_400, 0).UTC(),
			equation.BookflowOutput{
				Value: 1.5, Strength: 1.5, SpoofScore: 1.5, Category: 2, Ready: true,
			},
			1,
		)[0]

		Convey("It should scale spoof evidence by the maximum possible contrast", func() {
			So(*measurement.Sample(types.MetricSpoofScore, types.SideNone).Normalized,
				ShouldAlmostEqual, 0.75, 1e-12)
			So(*measurement.Sample(types.MetricStrength, types.SideNone).Normalized,
				ShouldAlmostEqual, 0.75, 1e-12)
		})
	})
}

func levelQuantity(levels []flow.BookLevel, ticks int64) float64 {
	for _, level := range levels {
		if level.Ticks == ticks {
			return level.Quantity
		}
	}

	return -1
}

func depthflowBookInput(bidQuantity, askQuantity float64) flow.BookInput {
	return flow.BookInput{
		Symbol:   "BTC/USD",
		TickSize: 1,
		Bids: []flow.BookLevel{
			{Price: 100, Ticks: 100, Quantity: bidQuantity},
			{Price: 99, Ticks: 99, Quantity: bidQuantity},
		},
		Asks: []flow.BookLevel{
			{Price: 101, Ticks: 101, Quantity: askQuantity},
			{Price: 102, Ticks: 102, Quantity: askQuantity},
		},
	}
}

func BenchmarkFrame(b *testing.B) {
	signal := &Signal{}
	at := time.Unix(1_700_000_500, 0).UTC()
	output := equation.BookflowOutput{
		Value: 0.7, Strength: 0.7, LoadedScore: 0.7, Category: 1, Ready: true,
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = signal.frame("BTC/USD", at, output, 1)
	}
}
