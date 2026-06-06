package toxicity

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/market"
)

func TestPriceKey(t *testing.T) {
	Convey("Given a pair with tick size", t, func() {
		pair := market.Pair{TickSize: "0.1"}

		Convey("It should round prices to tick boundaries", func() {
			So(priceKey(100.01, pair), ShouldEqual, priceKey(100.04, pair))
			So(priceFromKey(priceKey(100.01, pair), pair), ShouldAlmostEqual, 100.0, 1e-9)
		})
	})

	Convey("Given a pair without tick size", t, func() {
		pair := market.Pair{}

		Convey("It should discretize with fixed scale", func() {
			key := priceKey(100.000000001, pair)
			So(priceFromKey(key, pair), ShouldAlmostEqual, 100.0, 1e-4)
		})
	})
}

func TestIsToxicPriceKeyLookup(t *testing.T) {
	Convey("Given a toxic level stored at a rounded price", t, func() {
		tracker := NewTracker()
		symbol := "ETH/EUR"
		now := trackerNow()
		pair := market.Pair{TickSize: "0.01"}

		state := tracker.stateLocked(symbol, pair)
		state.toxic[priceKey(100.0, pair)] = now.Add(toxicCooldown)

		Convey("It should match a slightly perturbed lookup price", func() {
			So(tracker.IsToxic(symbol, 100.0000004, now), ShouldBeTrue)
		})
	})
}

func trackerNow() time.Time {
	return time.Now()
}
