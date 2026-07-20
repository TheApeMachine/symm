package strategy

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/types"
)

/*
TestArbiterDollarRanking proves the Select sort key stays a single dollar scale.
Fresh enters prequote max_fraction of cash capped by visible buy capacity, so a
thin book cannot ride a sharp per-unit edge above a deployable position; rotation
challengers rank on their stamped notional; and missing wallet cash falls back to
one consistent bare-utility scale rather than collapsing every rank to zero.
*/
func TestArbiterDollarRanking(t *testing.T) {
	Convey("Given an arbiter with a fixed max fraction", t, func() {
		arbiter := &Arbiter{
			price:       &broker.Price{},
			maxFraction: decimal.NewFromFloat64(0.2),
		}
		cash := decimal.NewFromInt64(1000)

		Convey("Fresh enters prequote feasible notional capped by buy capacity", func() {
			forecasts := map[string]types.Forecasts{
				"THIN": {Symbol: "THIN", BuyCapacity: decimal.NewFromInt64(10)},
				"DEEP": {Symbol: "DEEP", BuyCapacity: decimal.NewFromInt64(10_000)},
			}

			thin := arbiter.dollar(
				types.Decision{Symbol: "THIN", Utility: 1.0}, cash, forecasts,
			)
			deep := arbiter.dollar(
				types.Decision{Symbol: "DEEP", Utility: 0.5}, cash, forecasts,
			)

			So(thin.Cmp(decimal.NewFromInt64(10)), ShouldEqual, 0)
			So(deep.Cmp(decimal.NewFromInt64(100)), ShouldEqual, 0)
			So(deep.Cmp(thin), ShouldBeGreaterThan, 0)
		})

		Convey("A missing forecast falls back to max_fraction of cash", func() {
			feasible := arbiter.feasible(cash, types.Forecasts{})

			So(feasible.Cmp(decimal.NewFromInt64(200)), ShouldEqual, 0)
		})

		Convey("Rotation challengers rank on their stamped notional", func() {
			decision := types.Decision{
				Symbol:           "ROT",
				Utility:          2.0,
				ProposedNotional: decimal.NewFromFloat64(50),
			}

			value := arbiter.dollar(decision, cash, map[string]types.Forecasts{})
			So(value.Cmp(decimal.NewFromInt64(100)), ShouldEqual, 0)
		})

		Convey("Unavailable cash ranks by bare utility, never utility times zero", func() {
			forecasts := map[string]types.Forecasts{
				"A": {Symbol: "A", BuyCapacity: decimal.NewFromInt64(5)},
			}

			value := arbiter.dollar(
				types.Decision{Symbol: "A", Utility: 3.0}, nil, forecasts,
			)
			So(value.Cmp(decimal.NewFromInt64(3)), ShouldEqual, 0)
		})
	})
}
