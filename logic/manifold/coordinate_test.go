package manifold

import (
	"math"
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
