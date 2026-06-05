package cvd

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/numeric/adaptive"
)

func TestNewSignal(t *testing.T) {
	Convey("Given a qpool", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		signal := NewSignal(ctx, pool)
		defer signal.Close()

		Convey("It should wire absorption categories and a pooled calibrator", func() {
			So(signal.categories["hidden_absorption"], ShouldEqual, perspectives.CategoryHiddenAbsorption)
			So(signal.calibrator, ShouldNotBeNil)
			So(signal.classifier, ShouldNotBeNil)
		})
	})
}

func TestPooledPipeClassification(t *testing.T) {
	Convey("Given a shared classifier pipe", t, func() {
		classifier := adaptive.NewClassifier(
			cvdDefaultBandEdges,
			[]float64{0, 1, 2, 3},
			[]string{
				"volume_starvation",
				"stochastic_balance",
				"hidden_absorption",
				"aggressive_drive",
			},
		)
		pipe := numeric.NewClassed(classifier)

		Convey("A strong absorption burst lands in the top band", func() {
			code, err := classifier.Code(100)

			So(err, ShouldBeNil)
			So(pipe.Label(code), ShouldEqual, "aggressive_drive")
		})
	})
}

func TestObserve(t *testing.T) {
	Convey("Given a CVD signal with a measurements subscriber", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		signal := NewSignal(ctx, pool)
		defer signal.Close()

		measurements := signal.broadcasts["measurements"].Subscribe("test:cvd", 64)
		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)

		Convey("When one-sided flow is folded trade by trade", func() {
			for index := range 32 {
				signal.observe(market.TradeUpdate{
					Symbol:    "ALT/EUR",
					Side:      "buy",
					Price:     10 + float64(index)*0.01,
					Qty:       2,
					Timestamp: base.Add(time.Duration(index) * time.Millisecond),
				})
			}

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
					t.Fatal("timed out waiting for CVD measurement")
				}
			}

			Convey("It publishes an absorption reading carrying symbol and price", func() {
				So(measurement.Source, ShouldEqual, perspectives.SourceCVD)
				So(measurement.Symbol, ShouldEqual, "ALT/EUR")
				So(measurement.Last, ShouldBeGreaterThan, 0)
				So(measurement.SNR, ShouldBeGreaterThanOrEqualTo, 0)
			})
		})

		Convey("When a trade has no price or size", func() {
			signal.observe(market.TradeUpdate{Symbol: "ALT/EUR", Side: "buy", Timestamp: base})

			Convey("It is ignored", func() {
				_, ok := signal.symbols.Load("ALT/EUR")
				So(ok, ShouldBeFalse)
			})
		})
	})
}

func BenchmarkObserve(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	defer pool.Close()

	signal := NewSignal(ctx, pool)
	defer signal.Close()

	signal.broadcasts["measurements"].Subscribe("bench:cvd", 1024)
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	trade := market.TradeUpdate{Symbol: "ALT/EUR", Side: "buy", Price: 10, Qty: 2, Timestamp: base}

	b.ReportAllocs()

	for b.Loop() {
		signal.observe(trade)
	}
}
