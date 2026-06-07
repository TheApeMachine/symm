package toxicity

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/bus"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func TestNewToxicity(t *testing.T) {
	Convey("Given a qpool", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		tox := NewToxicity(ctx, pool)
		defer tox.Close()

		Convey("It should wire the measurements broadcast", func() {
			So(tox.measurements, ShouldNotBeNil)
		})
	})
}

func TestPublishMeasurement(t *testing.T) {
	Convey("Given a toxicity service with a measurements subscriber", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		tox := NewToxicity(ctx, pool)
		defer tox.Close()

		measurements := tox.measurements.Subscribe("test:toxicity", 64)
		symbol := "ETH/EUR"
		now := time.Now()

		warmTrackerSNR(t, tox.tracker, symbol)

		tox.tracker.ObserveMid(symbol, market.Pair{}, 100)
		tox.tracker.ObserveLast(symbol, market.Pair{}, 100)
		tox.tracker.ApplyOrder(symbol, market.Pair{}, "add", "order-1", SideBid, 100, 15, now, now)
		tox.tracker.ApplyOrder(symbol, market.Pair{}, "delete", "order-1", SideBid, 100, 15, now, now)

		Convey("When a toxic near-touch level is measured", func() {
			tox.publishMeasurement(symbol)

			waitCtx, waitCancel := context.WithTimeout(ctx, time.Second)
			defer waitCancel()

			value, err := bus.PollFor(waitCtx, measurements)
			if err != nil {
				t.Fatal("timed out waiting for toxicity measurement")
			}

			measurement, _ := value.Value.(types.Measurement)

			Convey("It should publish toxic bluff with measurable strength", func() {
				So(measurement.Category, ShouldEqual, types.CategoryToxicBluff)
				So(measurement.Symbol, ShouldEqual, symbol)
				So(measurement.Strength, ShouldBeGreaterThan, 0)
				So(measurement.Confidence, ShouldBeGreaterThan, 0)
				So(measurement.SNR, ShouldBeGreaterThan, 0)
				So(measurement.Last, ShouldEqual, 100)
			})
		})
	})
}

func TestMidOf(t *testing.T) {
	Convey("Given a ticker row", t, func() {
		Convey("It should prefer bid/ask mid when both are present", func() {
			So(midOf(market.TickerUpdate{Bid: 99, Ask: 101, Last: 50}), ShouldEqual, 100)
		})

		Convey("It should fall back to last when the touch is missing", func() {
			So(midOf(market.TickerUpdate{Last: 50}), ShouldEqual, 50)
		})
	})
}

func BenchmarkPublishMeasurement(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ[any](ctx, 2, 4, qpool.NewConfig())
	defer pool.Close()

	tox := NewToxicity(ctx, pool)
	defer tox.Close()

	tox.measurements.Subscribe("bench:toxicity", 1024)

	symbol := "ETH/EUR"
	now := time.Now()
	tox.tracker.ObserveMid(symbol, market.Pair{}, 100)
	tox.tracker.ObserveLast(symbol, market.Pair{}, 100)
	tox.tracker.ApplyOrder(symbol, market.Pair{}, "add", "order-1", SideBid, 100, 15, now, now)
	tox.tracker.ApplyOrder(symbol, market.Pair{}, "delete", "order-1", SideBid, 100, 15, now, now)

	b.ReportAllocs()

	for b.Loop() {
		tox.publishMeasurement(symbol)
	}
}
