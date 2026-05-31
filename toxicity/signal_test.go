package toxicity

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestNewToxicity(t *testing.T) {
	Convey("Given a qpool", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
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
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		tox := NewToxicity(ctx, pool)
		defer tox.Close()

		measurements := tox.measurements.Subscribe("test:toxicity", 64)
		symbol := "ETH/EUR"
		now := time.Now()

		tox.tracker.ObserveMid(symbol, market.Pair{}, 100)
		state := tox.tracker.stateLocked(symbol, market.Pair{})
		state.toxic[100] = now.Add(time.Minute)

		Convey("When a toxic near-touch level is measured", func() {
			tox.publishMeasurement(symbol, 100)

			var measurement perspectives.Measurement
			received := false
			deadline := time.After(time.Second)

			for !received {
				select {
				case value := <-measurements.Incoming:
					reading, ok := value.Value.(perspectives.Measurement)

					if ok {
						measurement = reading
						received = true
					}
				case <-deadline:
					t.Fatal("timed out waiting for toxicity measurement")
				}
			}

			Convey("It should publish toxic bluff with measurable strength", func() {
				So(measurement.Category, ShouldEqual, perspectives.CategoryToxicBluff)
				So(measurement.Symbol, ShouldEqual, symbol)
				So(measurement.Strength, ShouldBeGreaterThan, 0)
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
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	defer pool.Close()

	tox := NewToxicity(ctx, pool)
	defer tox.Close()

	tox.measurements.Subscribe("bench:toxicity", 1024)

	symbol := "ETH/EUR"
	now := time.Now()
	tox.tracker.ObserveMid(symbol, market.Pair{}, 100)
	state := tox.tracker.stateLocked(symbol, market.Pair{})
	state.toxic[100] = now.Add(time.Minute)

	b.ReportAllocs()

	for b.Loop() {
		tox.publishMeasurement(symbol, 100)
	}
}
