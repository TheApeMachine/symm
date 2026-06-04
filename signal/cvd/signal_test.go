package cvd

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/numeric/adaptive"
)

func TestNewSignal(t *testing.T) {
	Convey("Given a qpool", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		signal := NewSignal(ctx, pool)
		defer signal.Close()

		Convey("It should wire absorption categories", func() {
			So(signal.categories["hidden_absorption"], ShouldEqual, perspectives.CategoryHiddenAbsorption)
			So(signal.categories["aggressive_drive"], ShouldEqual, perspectives.CategoryAggressiveDrive)
		})
	})
}

func TestMeasureFusedConfidence(t *testing.T) {
	Convey("Given quartile band boundaries", t, func() {
		classifier := adaptive.NewClassifier(
			[]float64{2.75, 5.5, 8.25},
			cvdBandCodes,
			cvdBandLabels,
		)

		Convey("It should read high confidence deep inside a band", func() {
			So(classifier.Confidence(6), ShouldBeGreaterThan, 0.1)
		})

		Convey("It should read a near-floor — not zero — confidence on a quartile boundary", func() {
			near := classifier.Confidence(5.5)
			So(near, ShouldBeGreaterThan, 0) // exact 0 is a clamping artifact
			So(near, ShouldBeLessThan, 0.1)
		})
	})

	Convey("Given a cold fused history", t, func() {
		Convey("It should emit no category before warm-up completes", func() {
			cold := newCVDState()
			category, clarity, _ := cold.measureFused(1)

			So(category, ShouldEqual, perspectives.CategoryTypeNone)
			So(clarity, ShouldEqual, 0)
		})
	})

	Convey("Given a warmed fused history spanning a real range", t, func() {
		state := newCVDState()

		// Warm past minCVDFusedSamples with a spread so the quartile bands mean
		// something. Before the fix, the SigmaClamp was fed 0, so every banded
		// value was 0 and every reading collapsed to volume_starvation regardless.
		for sample := 1; sample <= 20; sample++ {
			state.measureFused(float64(sample))
		}

		Convey("A strong absorption burst lands in an upper band, not the floor", func() {
			category, clarity, standout := state.measureFused(40)

			So(category, ShouldBeIn, []perspectives.CategoryType{
				perspectives.CategoryHiddenAbsorption,
				perspectives.CategoryAggressiveDrive,
			})
			So(clarity, ShouldBeGreaterThan, 0)
			So(standout, ShouldBeGreaterThan, 0.9) // raw strength, uncapped
		})

		Convey("A faint reading lands in the bottom band, and the two differ", func() {
			high, _, highStandout := state.measureFused(40)
			low, _, lowStandout := state.measureFused(0.2)

			So(low, ShouldEqual, perspectives.CategoryVolumeStarvation)
			So(high, ShouldNotEqual, low)              // the old bug made everything equal
			So(highStandout, ShouldBeGreaterThan, lowStandout) // standout tracks raw strength
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
