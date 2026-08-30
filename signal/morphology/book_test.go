package morphology

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
)

func morphOrder(price, qty float64, at time.Time) kraken.Level3Order {
	return kraken.Level3Order{
		Event:      "add",
		OrderID:    "order",
		LimitPrice: decimal.NewFromFloat64(price),
		OrderQty:   decimal.NewFromFloat64(qty),
		Timestamp:  at,
	}
}

func morphMessage(symbol string, at time.Time, bids, asks []kraken.Level3Order) kraken.Level3Data {
	return kraken.Level3Data{
		Symbol:    symbol,
		Timestamp: at,
		Bids:      bids,
		Asks:      asks,
	}
}

func morphMetric(measurement *data.Measurement[float64], name string) float64 {
	if measurement == nil {
		return 0
	}

	return measurement.Metrics[name].Raw
}

func morphHasMetric(measurement *data.Measurement[float64], name string) bool {
	if measurement == nil {
		return false
	}

	_, found := measurement.Metrics[name]

	return found
}

func TestProjectShape(t *testing.T) {
	Convey("Given a crossed or degenerate message", t, func() {
		message := morphMessage("BTC/USD", time.Now(),
			[]kraken.Level3Order{morphOrder(101, 1, time.Now())},
			[]kraken.Level3Order{morphOrder(99, 1, time.Now())},
		)

		_, _, _, ok := projectShape(message)

		Convey("projectShape reports not-ok, never fabricating a shape", func() {
			So(ok, ShouldBeFalse)
		})
	})

	Convey("Given a single-level symmetric message", t, func() {
		now := time.Now()
		message := morphMessage("BTC/USD", now,
			[]kraken.Level3Order{morphOrder(99, 2, now)},
			[]kraken.Level3Order{morphOrder(101, 2, now)},
		)

		bidFolded, askFolded, whole, ok := projectShape(message)

		Convey("bilateral shapes are folded onto the positive distance axis", func() {
			So(ok, ShouldBeTrue)
			// Bid touch (99, mid 100, spread 2) folds to (100-99)/2 = +0.5,
			// and the ask touch folds to (101-100)/2 = +0.5: one mirrored book.
			So(bidFolded[0].Position, ShouldAlmostEqual, 0.5)
			So(askFolded[0].Position, ShouldAlmostEqual, 0.5)
		})

		Convey("the whole-book shape retains signed positions", func() {
			So(ok, ShouldBeTrue)
			// whole book has two points: bid at -0.5 and ask at +0.5.
			So(len(whole), ShouldEqual, 2)
			So(whole[0].Position, ShouldAlmostEqual, -0.5)
			So(whole[1].Position, ShouldAlmostEqual, 0.5)
		})
	})
}

func TestBookStep(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	later := time.Unix(1_700_000_001, 0)

	Convey("Given an exactly mirrored multi-level message", t, func() {
		// A mirrored book by notional: each level carries the same notional C
		// (quantity = C/price), so the bid side's mass profile is identical to
		// the ask side's when reflected — identical folded positions with
		// identical mass at each.
		const commonNotional = 100000.0
		message := morphMessage("BTC/USD", now,
			[]kraken.Level3Order{
				morphOrder(98, commonNotional/98, now),
				morphOrder(99, commonNotional/99, now),
			},
			[]kraken.Level3Order{
				morphOrder(101, commonNotional/101, now),
				morphOrder(102, commonNotional/102, now),
			},
		)

		entity := NewBook()

		Convey("bilateral distance and KS are exactly zero", func() {
			measurement := entity.Step(message)

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(morphMetric(measurement, "book_shape_distance"), ShouldAlmostEqual, 0)
			So(morphMetric(measurement, "book_shape_ks"), ShouldAlmostEqual, 0)
		})
	})

	Convey("Given a symbol observed only once", t, func() {
		entity := NewBook()

		Convey("the measurement reports no SNR, having no structural change yet", func() {
			measurement := entity.Step(morphMessage("BTC/USD", now,
				[]kraken.Level3Order{morphOrder(99, 2, now)},
				[]kraken.Level3Order{morphOrder(101, 2, now)},
			))

			So(measurement, ShouldNotBeNil)
			So(measurement.SNRDefined, ShouldBeFalse)
		})
	})

	Convey("Given a symbol whose shape keeps moving", t, func() {
		entity := NewBook()

		// Walk the book so every step carries a genuine structural change, which
		// is what the change estimator needs before it can report a noise model.
		var measurement *data.Measurement[float64]

		for step := range 12 {
			at := now.Add(time.Duration(step) * time.Second)
			offset := float64(step)

			measurement = entity.Step(morphMessage("BTC/USD", at,
				[]kraken.Level3Order{morphOrder(99-offset, 2, at)},
				[]kraken.Level3Order{morphOrder(101+offset, 2, at)},
			))
		}

		Convey("the measurement reports a defined, finite SNR", func() {
			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.SNRDefined, ShouldBeTrue)
			So(measurement.SNR, ShouldBeGreaterThanOrEqualTo, 0)
		})

		Convey("the change estimator's own baseline and z-score are projected", func() {
			So(morphHasMetric(measurement, "morphology_change_baseline"), ShouldBeTrue)
			So(morphHasMetric(measurement, "morphology_change_zscore"), ShouldBeTrue)
		})
	})

	Convey("Given an asymmetric message", t, func() {
		message := morphMessage("BTC/USD", now,
			[]kraken.Level3Order{
				morphOrder(99, 2, now),
				morphOrder(97, 1, now),
			},
			[]kraken.Level3Order{morphOrder(101, 2, now)},
		)

		entity := NewBook()

		Convey("bilateral distance and KS are positive", func() {
			measurement := entity.Step(message)

			So(measurement, ShouldNotBeNil)
			So(morphMetric(measurement, "book_shape_distance"), ShouldBeGreaterThan, 0)
			So(morphMetric(measurement, "book_shape_ks"), ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given scale-equivalent messages", t, func() {
		Convey("rescaling all quantities yields equivalent normalized morphology", func() {
			first := morphMessage("BTC/USD", now,
				[]kraken.Level3Order{
					morphOrder(98, 1, now),
					morphOrder(99, 2, now),
				},
				[]kraken.Level3Order{
					morphOrder(101, 2, now),
					morphOrder(102, 1, now),
				},
			)

			second := morphMessage("ETH/USD", now,
				[]kraken.Level3Order{
					morphOrder(98, 10, now),
					morphOrder(99, 20, now),
				},
				[]kraken.Level3Order{
					morphOrder(101, 20, now),
					morphOrder(102, 10, now),
				},
			)

			firstMeasurement := NewBook().Step(first)
			secondMeasurement := NewBook().Step(second)

			So(firstMeasurement, ShouldNotBeNil)
			So(secondMeasurement, ShouldNotBeNil)

			So(morphMetric(firstMeasurement, "book_shape_distance"), ShouldAlmostEqual, morphMetric(secondMeasurement, "book_shape_distance"))
			So(morphMetric(firstMeasurement, "book_shape_ks"), ShouldAlmostEqual, morphMetric(secondMeasurement, "book_shape_ks"))
			So(morphMetric(firstMeasurement, "concentration:bid"), ShouldAlmostEqual, morphMetric(secondMeasurement, "concentration:bid"))
			So(morphMetric(firstMeasurement, "entropy:bid"), ShouldAlmostEqual, morphMetric(secondMeasurement, "entropy:bid"))
		})
	})

	Convey("Given the first observation of a symbol", t, func() {
		message := morphMessage("BTC/USD", now,
			[]kraken.Level3Order{morphOrder(99, 2, now)},
			[]kraken.Level3Order{morphOrder(101, 2, now)},
		)

		entity := NewBook()

		Convey("structural change is undefined, never fabricated as zero", func() {
			measurement := entity.Step(message)

			So(measurement, ShouldNotBeNil)
			So(morphHasMetric(measurement, "morphology_change"), ShouldBeFalse)
		})

		Convey("structural change appears once a prior shape exists", func() {
			first := entity.Step(message)
			So(first.Err, ShouldBeNil)

			second := entity.Step(morphMessage("BTC/USD", later,
				[]kraken.Level3Order{
					morphOrder(99, 2, later),
					morphOrder(98, 4, later),
				},
				[]kraken.Level3Order{morphOrder(101, 2, later)},
			))

			So(second, ShouldNotBeNil)
			So(second.Err, ShouldBeNil)
			So(morphHasMetric(second, "morphology_change"), ShouldBeTrue)
			So(morphMetric(second, "morphology_change"), ShouldBeGreaterThan, 0.0)
		})
	})

	Convey("Given a crossed message", t, func() {
		message := morphMessage("BTC/USD", now,
			[]kraken.Level3Order{morphOrder(101, 1, now)},
			[]kraken.Level3Order{morphOrder(99, 1, now)},
		)

		entity := NewBook()

		Convey("Step returns nil for a message with no shape", func() {
			So(entity.Step(message), ShouldBeNil)
		})
	})
}

/*
BenchmarkStep measures the steady-state cost and allocation count of one
morphology Step against a realistic ten-level message, once warm.
*/
func BenchmarkStep(benchmark *testing.B) {
	now := time.Unix(1_700_000_000, 0)

	bids := make([]kraken.Level3Order, 0, 10)
	asks := make([]kraken.Level3Order, 0, 10)

	for level := 0; level < 10; level++ {
		bids = append(bids, morphOrder(99-float64(level), 2, now))
		asks = append(asks, morphOrder(101+float64(level), 2, now))
	}

	message := morphMessage("BTC/USD", now, bids, asks)
	entity := NewBook()

	benchmark.ReportAllocs()
	benchmark.ResetTimer()

	for index := 0; index < benchmark.N; index++ {
		_ = entity.Step(message)
	}
}
