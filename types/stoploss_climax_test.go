package types

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
)

/*
climaxStoploss builds a surging, profitable lot poised for a climax exit.
*/
func climaxStoploss() (*Stoploss, *decimal.Decimal) {
	mark := decimal.NewFromFloat64(150)

	stoploss := &Stoploss{
		SurgeArmed: true,
		ProfitLine: decimal.NewFromFloat64(120),
		Causative: CausativeContext{
			ActivePerspectives: map[string]string{
				"profit_run": "Exhausting",
				"liquidity":  "Depleting",
			},
		},
	}

	return stoploss, mark
}

func TestClimaxExhaustionSellsIntoTheBid(t *testing.T) {
	Convey("Given a surging lot exhausting into a thinning book", t, func() {
		stoploss, mark := climaxStoploss()

		Convey("the climax exit fires while price is still above the profit line", func() {
			So(stoploss.isClimaxExhausted(mark), ShouldBeTrue)
		})
	})
}

func TestClimaxExhaustionRequiresAllThreeConditions(t *testing.T) {
	Convey("Given a climax candidate", t, func() {
		Convey("it does not fire without a surge", func() {
			stoploss, mark := climaxStoploss()
			stoploss.SurgeArmed = false
			So(stoploss.isClimaxExhausted(mark), ShouldBeFalse)
		})

		Convey("it does not fire below the profit line", func() {
			stoploss, _ := climaxStoploss()
			So(stoploss.isClimaxExhausted(decimal.NewFromFloat64(100)), ShouldBeFalse)
		})

		Convey("it does not fire while the run is still extending", func() {
			stoploss, mark := climaxStoploss()
			stoploss.Causative.ActivePerspectives["profit_run"] = "Extending"
			So(stoploss.isClimaxExhausted(mark), ShouldBeFalse)
		})

		Convey("it does not fire while the book still has depth", func() {
			stoploss, mark := climaxStoploss()
			stoploss.Causative.ActivePerspectives["liquidity"] = "Replenishing"
			So(stoploss.isClimaxExhausted(mark), ShouldBeFalse)
		})

		Convey("it does not fire when liquidity has not been read at all", func() {
			stoploss, mark := climaxStoploss()
			delete(stoploss.Causative.ActivePerspectives, "liquidity")
			So(stoploss.isClimaxExhausted(mark), ShouldBeFalse)
		})
	})
}

func TestClimaxExhaustionFiresOnVacuum(t *testing.T) {
	Convey("Given the book vacuuming out beneath an exhausted run", t, func() {
		stoploss, mark := climaxStoploss()
		stoploss.Causative.ActivePerspectives["liquidity"] = "VacuumForming"

		Convey("the exit fires", func() {
			So(stoploss.isClimaxExhausted(mark), ShouldBeTrue)
		})
	})
}

func TestClimaxExhaustionHandlesNilMark(t *testing.T) {
	Convey("Given no mark", t, func() {
		stoploss, _ := climaxStoploss()

		Convey("it declines rather than panicking", func() {
			So(stoploss.isClimaxExhausted(nil), ShouldBeFalse)
		})
	})
}

func TestClimaxExitFiresOnLargePumps(t *testing.T) {
	Convey("Given an exhausting run far past the parabolic threshold", t, func() {
		// A run this size makes isParabolicRun() true. An earlier form of the
		// exit exempted those, which disabled it on exactly the pumps it
		// exists to escape.
		stoploss := &Stoploss{
			SurgeArmed: true,
			ProfitLine: decimal.NewFromFloat64(100),
			Peak:       decimal.NewFromFloat64(300),
			Causative: CausativeContext{
				ActivePerspectives: map[string]string{
					"profit_run": "Exhausting",
					"liquidity":  "VacuumForming",
				},
			},
		}

		Convey("the parabolic run is confirmed", func() {
			So(stoploss.isParabolicRun(), ShouldBeTrue)
		})

		Convey("the climax exit still fires on a 200% run", func() {
			So(stoploss.isClimaxExhausted(decimal.NewFromFloat64(300)), ShouldBeTrue)
		})
	})

	Convey("Given a healthy large runner whose book is intact", t, func() {
		stoploss := &Stoploss{
			SurgeArmed: true,
			ProfitLine: decimal.NewFromFloat64(100),
			Peak:       decimal.NewFromFloat64(300),
			Causative: CausativeContext{
				ActivePerspectives: map[string]string{
					"profit_run": "Extending",
					"liquidity":  "Replenishing",
				},
			},
		}

		Convey("it is left to run", func() {
			So(stoploss.isClimaxExhausted(decimal.NewFromFloat64(300)), ShouldBeFalse)
		})
	})
}

func TestClimaxExitFiresOnGivingBack(t *testing.T) {
	Convey("Given a run giving back into a depleting book", t, func() {
		stoploss, mark := climaxStoploss()
		stoploss.Causative.ActivePerspectives["profit_run"] = "GivingBack"

		Convey("the exit fires", func() {
			So(stoploss.isClimaxExhausted(mark), ShouldBeTrue)
		})
	})
}

func TestClimaxExitFiresOnBidWithdrawal(t *testing.T) {
	Convey("Given market makers pulling bids with no liquidity advisor", t, func() {
		stoploss, mark := climaxStoploss()
		delete(stoploss.Causative.ActivePerspectives, "liquidity")
		stoploss.Causative.NetWithdrawalBid = 0.5

		Convey("the book signal stands on its own", func() {
			So(stoploss.isClimaxExhausted(mark), ShouldBeTrue)
		})
	})

	Convey("Given only mild bid withdrawal", t, func() {
		stoploss, mark := climaxStoploss()
		delete(stoploss.Causative.ActivePerspectives, "liquidity")
		stoploss.Causative.NetWithdrawalBid = 0.1

		Convey("the exit holds", func() {
			So(stoploss.isClimaxExhausted(mark), ShouldBeFalse)
		})
	})
}
