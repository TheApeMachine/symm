package manifold

import (
	"math"
	"runtime"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	pfluid "github.com/theapemachine/nomagique/physics/fluid"
)

func TestCoordinateEpoch_Map(t *testing.T) {
	Convey("Given an independently scaled two-sided L3 population", t, func() {
		config := pfluid.DefaultConfig()
		at := time.Unix(10, 0)
		orders := []physicalOrder{
			{
				orderID:       "bid",
				side:          book.Bid,
				price:         99,
				quantity:      2,
				priceMoney:    decimal.NewFromInt64(99),
				quantityMoney: decimal.NewFromInt64(2),
				timestamp:     at.Add(-2 * time.Second),
			},
			{
				orderID:       "ask",
				side:          book.Ask,
				price:         101,
				quantity:      4,
				priceMoney:    decimal.NewFromInt64(101),
				quantityMoney: decimal.NewFromInt64(4),
				timestamp:     at.Add(-time.Second),
			},
		}
		candidate := intensityCandidate{
			symbol:   "BTC/USD",
			orders:   orders,
			midPrice: 100,
			outcome:  solverOutcome(at, 4, 2),
		}
		particles, epoch, ready := (*coordinateEpoch)(nil).Map(config, candidate)

		Convey("It should encode geometry, phase opposition, and unit inertia", func() {
			So(ready, ShouldBeTrue)
			So(particles, ShouldHaveLength, 2)
			So(epoch.reference.Cmp(decimal.NewFromInt64(100)), ShouldEqual, 0)
			So(epoch.spread, ShouldAlmostEqual, 0.02)
			So(epoch.buyCapacity.Cmp(decimal.NewFromInt64(404)), ShouldEqual, 0)
			So(epoch.sellCapacity.Cmp(decimal.NewFromInt64(198)), ShouldEqual, 0)
			So(particles[0].Mass, ShouldEqual, 1)
			So(particles[0].Heat, ShouldEqual, float32(2))
			So(particles[1].Heat, ShouldEqual, float32(4))
			So(particles[0].Energy, ShouldEqual, 1)
			So(particles[0].Omega, ShouldBeLessThan, particles[1].Omega)
			phaseDistance := math.Abs(math.Remainder(
				float64(particles[1].Phase-particles[0].Phase),
				2*math.Pi,
			))
			So(phaseDistance, ShouldBeGreaterThan, 0)
		})

		Convey("It should derive periodic velocity from the preceding epoch", func() {
			moved := append([]physicalOrder(nil), orders...)
			moved[0].price = 99.5
			moved[0].priceMoney = decimal.NewFromFloat64(99.5)
			candidate.orders = moved
			candidate.midPrice = 100.25
			candidate.outcome = solverOutcome(at.Add(time.Second), 4, 2)
			next, _, nextReady := epoch.Map(config, candidate)
			So(nextReady, ShouldBeTrue)
			So(next[0].Velocity.X, ShouldNotEqual, 0)
		})
	})
}

func TestMarketTouch_Notional(t *testing.T) {
	Convey("Given price and quantity with independent fixed-point precision", t, func() {
		price, priceErr := decimal.NewFromString("0.00012345")
		quantity, quantityErr := decimal.NewFromString("0.00000067")
		So(priceErr, ShouldBeNil)
		So(quantityErr, ShouldBeNil)
		touch := marketTouch{}

		Convey("It should preserve every finite product digit", func() {
			So(touch.notional(price, quantity).String(), ShouldEqual,
				"0.0000000000827115")
		})
	})
}

/*
TestMarketTouch_reference proves mixed price scales are aligned before addition
and subtraction, so receiver-scale rounding cannot erase finer tick precision.
*/
func TestMarketTouch_reference(t *testing.T) {
	Convey("Given bid and ask prices with different fixed-point scales", t, func() {
		bid, bidErr := decimal.NewFromString("1.2")
		ask, askErr := decimal.NewFromString("1.234")
		So(bidErr, ShouldBeNil)
		So(askErr, ShouldBeNil)
		touch := marketTouch{
			bidPriceMoney: bid,
			askPriceMoney: ask,
		}

		Convey("Then midpoint and spread retain the finer ask scale", func() {
			reference := touch.reference()

			So(reference.String(), ShouldEqual, "1.2170")
			So(touch.spread(reference), ShouldAlmostEqual, 0.034/1.217, 1e-12)
		})
	})
}

/*
TestMarketTouch_observe proves touch aggregation aligns quantity scales before
addition, including precision finer than Decimal's default constructor scale.
*/
func TestMarketTouch_observe(t *testing.T) {
	Convey("Given two fine-grained orders at the same best bid", t, func() {
		price, priceErr := decimal.NewFromString("1.2")
		quantity, quantityErr := decimal.NewFromString("0.00000000000005")
		So(priceErr, ShouldBeNil)
		So(quantityErr, ShouldBeNil)
		touch := marketTouch{
			bidQuantity: decimal.NewFromInt64(0),
			askQuantity: decimal.NewFromInt64(0),
		}
		order := physicalOrder{
			side:          book.Bid,
			price:         1.2,
			quantity:      0.00000000000005,
			priceMoney:    price,
			quantityMoney: quantity,
		}

		touch.observe(order)
		touch.observe(order)

		Convey("Then neither contribution is rounded to the receiver scale", func() {
			So(touch.bidQuantity.String(), ShouldEqual, "0.00000000000010")
		})
	})
}

/*
BenchmarkMarketTouchArithmetic measures the exact fixed-point boundary that is
executed once for every mapped L3 population.
*/
func BenchmarkMarketTouchArithmetic(b *testing.B) {
	bid, _ := decimal.NewFromString("61234.12")
	ask, _ := decimal.NewFromString("61234.123")
	quantity, _ := decimal.NewFromString("0.00006789")
	touch := marketTouch{
		bidPriceMoney: bid,
		askPriceMoney: ask,
	}
	var reference *decimal.Decimal
	var spread float64
	var notional *decimal.Decimal

	b.ReportAllocs()

	for b.Loop() {
		reference = touch.reference()
		spread = touch.spread(reference)
		notional = touch.notional(ask, quantity)
	}

	runtime.KeepAlive(reference)
	runtime.KeepAlive(spread)
	runtime.KeepAlive(notional)
}

func TestStablePhase(t *testing.T) {
	Convey("Given stable symbol and order identity", t, func() {
		Convey("It should be deterministic and symbol-specific", func() {
			So(stablePhase("BTC/USD", "order"), ShouldEqual,
				stablePhase("BTC/USD", "order"))
			So(stablePhase("BTC/USD", "order"), ShouldNotEqual,
				stablePhase("ETH/USD", "order"))
		})
	})
}
