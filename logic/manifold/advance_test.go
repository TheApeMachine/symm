package manifold

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSlotAdvance(t *testing.T) {
	Convey("Given two SDK book changes observed before any GPU work", t, func() {
		viper.Set("market.l3_depth", 10)
		viper.Set("market.manifold.lifetime_capacity", 256)
		viper.Set("market.forecast.rls.initial_variance", 1.0)
		viper.Set("market.forecast.rls.forgetting_factor", 1.0)
		engine := NewEngine()
		defer engine.Close()
		slot, err := engine.Admit("BTC/USD")
		So(err, ShouldBeNil)
		managed := book.New()
		managed.Name = "BTC/USD"
		managed.Update(&book.UpdateOptions{
			Direction: book.Bid, ID: "bid-1",
			Price: decimal.NewFromFloat64(99.5), Quantity: decimal.NewFromFloat64(2),
			Timestamp: time.Unix(1, 0),
		})
		managed.Update(&book.UpdateOptions{
			Direction: book.Ask, ID: "ask-1",
			Price: decimal.NewFromFloat64(100.5), Quantity: decimal.NewFromFloat64(3),
			Timestamp: time.Unix(1, 0),
		})

		first := slot.ObserveBook(managed)
		managed.Update(&book.UpdateOptions{
			Direction: book.Bid, ID: "bid-1",
			Price: decimal.NewFromFloat64(99.5), Quantity: decimal.NewFromFloat64(4),
			Timestamp: time.Unix(2, 0),
		})
		second := slot.ObserveBook(managed)
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
