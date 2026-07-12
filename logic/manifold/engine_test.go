package manifold

import (
	"testing"
	"time"

	"github.com/spf13/viper"
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/strategy"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSlotProcess(t *testing.T) {
	Convey("Given the synchronous slot process contract", t, func() {
		viper.Set("market.l3_depth", 10)
		viper.Set("market.manifold.lifetime_capacity", 256)
		viper.Set("market.forecast.rls.initial_variance", 1.0)
		viper.Set("market.forecast.rls.forgetting_factor", 1.0)
		engine := NewEngine()
		defer engine.Close()
		slot, err := engine.Admit("ETH/USD", strategy.NewThesis())
		So(err, ShouldBeNil)
		book := &observationBook{bestBid: 1999, bestAsk: 2001}
		row := kraken.Level3Data{
			Symbol: "ETH/USD", Type: "snapshot", Timestamp: time.Unix(1, 0), Checksum: 42,
			Bids: []kraken.Level3Order{{
				OrderID: "bid-1", LimitPrice: 1999, OrderQty: 2, Timestamp: time.Unix(1, 0),
			}},
			Asks: []kraken.Level3Order{{
				OrderID: "ask-1", LimitPrice: 2001, OrderQty: 3, Timestamp: time.Unix(1, 0),
			}},
		}

		Convey("When Process observes and advances the row", func() {
			result := slot.Process(row, 1, 8, book)

			Convey("Then it returns the same completed advance artifact as the original API", func() {
				So(result.StateProduced, ShouldBeTrue)
				So(result.Observation.At, ShouldEqual, row.Timestamp)
				So(result.Observation.FrameType, ShouldEqual, row.Type)
				So(result.Observation.Checksum, ShouldEqual, row.Checksum)
				So(result.Observation.Count, ShouldEqual, 1)
				So(result.OrderCount, ShouldEqual, 2)
				So(result.State.Rho, ShouldNotBeEmpty)
				So(slot.advanceReady, ShouldBeFalse)
			})
		})
	})

	Convey("Given a slot whose authoritative book is unavailable", t, func() {
		config := &pmanifold.Config{
			GridX: 1, GridY: 1, GridZ: 1,
			DomainX: 1, DomainY: 1, DomainZ: 1,
		}
		slot, err := newSlot(
			"BTC/USD",
			strategy.NewThesis(),
			config,
			ForecastConfig{InitialVariance: 1, ForgettingFactor: 1},
			256,
			time.Second,
			1e-9,
		)
		So(err, ShouldBeNil)

		Convey("When Process cannot validate the row", func() {
			result := slot.Process(
				kraken.Level3Data{Symbol: "BTC/USD", Timestamp: time.Unix(1, 0)},
				1,
				1,
				nil,
			)

			Convey("Then it produces an invalid state without trying to advance", func() {
				So(result.StateProduced, ShouldBeTrue)
				So(result.State.Ready, ShouldBeFalse)
				So(result.State.InvalidReason, ShouldEqual, BookInvalid)
				So(result.AdvanceReady, ShouldBeFalse)
				So(slot.advanceReady, ShouldBeFalse)
				_, ok := slot.thesis.Evidence("BTC/USD", "manifold_forecasts")
				So(ok, ShouldBeFalse)
			})
		})
	})
}
