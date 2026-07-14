package manifold

import (
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSlotAdvance(t *testing.T) {
	Convey("Given two valid L3 rows observed before any GPU work", t, func() {
		viper.Set("market.l3_depth", 10)
		viper.Set("market.manifold.lifetime_capacity", 256)
		viper.Set("market.forecast.rls.initial_variance", 1.0)
		viper.Set("market.forecast.rls.forgetting_factor", 1.0)
		engine := NewEngine()
		defer engine.Close()
		slot, err := engine.Admit("BTC/USD")
		So(err, ShouldBeNil)
		book := &observationBook{bestBid: 99, bestAsk: 101}
		snapshot := kraken.Level3Data{
			Symbol: "BTC/USD", Type: "snapshot", Timestamp: time.Unix(1, 0),
			Bids: []kraken.Level3Order{{
				OrderID: "bid-1", LimitPrice: 99, OrderQty: 2, Timestamp: time.Unix(1, 0),
			}},
			Asks: []kraken.Level3Order{{
				OrderID: "ask-1", LimitPrice: 101, OrderQty: 3, Timestamp: time.Unix(1, 0),
			}},
		}
		update := kraken.Level3Data{
			Symbol: "BTC/USD", Type: "update", Timestamp: time.Unix(2, 0),
			Bids: []kraken.Level3Order{{
				Event: "modify", OrderID: "bid-1", LimitPrice: 99,
				OrderQty: 4, Timestamp: time.Unix(2, 0),
			}},
		}

		first := slot.Observe(snapshot, 1, 8, book)
		second := slot.Observe(update, 1, 8, book)
		orders := slot.population.Orders()

		Convey("When the field advances once", func() {
			result := slot.Advance()

			Convey("Then it advances the latest accumulated population and consumes readiness", func() {
				So(first.AdvanceReady, ShouldBeTrue)
				So(second.AdvanceReady, ShouldBeTrue)
				So(orders, ShouldHaveLength, 2)
				So(orders[0].Quantity+orders[1].Quantity, ShouldEqual, 7.0)
				So(result.StateProduced, ShouldBeTrue)
				So(result.Observation.At, ShouldEqual, time.Unix(2, 0))
				So(result.Observation.Count, ShouldEqual, 2)
				So(result.OrderCount, ShouldEqual, 2)
				So(result.State.Epoch, ShouldEqual, uint64(1))
				So(slot.advanceReady, ShouldBeFalse)
				So(slot.Advance().StateProduced, ShouldBeFalse)
			})
		})
	})
}
