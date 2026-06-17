package trader

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/market"
)

func TestInstrumentUpdate(t *testing.T) {
	Convey("Given an instrument catalog", t, func() {
		viper.Set("market.quote_currency", "USD")

		pool := qpool.NewQ[any](t.Context(), 1, 2, nil)
		instrument := NewInstrument(t.Context(), pool)

		update := market.InstrumentUpdate{
			Pairs: []market.InstrumentPair{
				{Symbol: "BTC/USD", Quote: "USD", Status: "online"},
				{Symbol: "ETH/USD", Quote: "USD", Status: "online"},
				{Symbol: "ETH/EUR", Quote: "EUR", Status: "online"},
				{Symbol: "XRP/USD", Quote: "USD", Status: "cancel_only"},
			},
		}

		Convey("When Update is called", func() {
			added, err := instrument.Update(update)

			Convey("It should return only new online USD pairs", func() {
				So(err, ShouldBeNil)
				So(added, ShouldResemble, []string{"BTC/USD", "ETH/USD"})
			})

			Convey("When Update receives the same pairs again", func() {
				repeated, repeatErr := instrument.Update(update)

				Convey("It should not return duplicates", func() {
					So(repeatErr, ShouldBeNil)
					So(repeated, ShouldBeEmpty)
				})
			})
		})
	})
}

func TestInstrumentSubscribe(t *testing.T) {
	Convey("Given a new instrument trader", t, func() {
		pool := qpool.NewQ[any](t.Context(), 1, 2, nil)
		instrument := NewInstrument(t.Context(), pool)

		Convey("When Subscribe is called before any catalog update", func() {
			err := instrument.Subscribe()

			Convey("It should subscribe to the instrument channel without panicking", func() {
				So(err, ShouldBeNil)
			})
		})

		Convey("When SubscribeSymbols is called before any catalog update", func() {
			err := instrument.SubscribeSymbols()

			Convey("It should no-op without panicking", func() {
				So(err, ShouldBeNil)
			})
		})
	})
}
