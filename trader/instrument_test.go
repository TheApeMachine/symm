package trader

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken/market"
)

func TestInstrumentUpdate(t *testing.T) {
	Convey("Given an instrument catalog", t, func() {
		viper.Set("market.quote_currency", "USD")

		instrument := NewInstrument(t.Context())

		update := market.InstrumentUpdate{
			Pairs: []market.InstrumentPair{
				{Symbol: "BTC/USD", Quote: "USD", Status: "online"},
				{Symbol: "ETH/USD", Quote: "USD", Status: "online"},
				{Symbol: "ETH/EUR", Quote: "EUR", Status: "online"},
				{Symbol: "XRP/USD", Quote: "USD", Status: "cancel_only"},
			},
		}

		Convey("When Update is called", func() {
			added := instrument.Update(update)

			Convey("It should return only new online USD pairs", func() {
				So(added, ShouldResemble, []string{"BTC/USD", "ETH/USD"})
			})

			Convey("When Update receives the same pairs again", func() {
				repeated := instrument.Update(update)

				Convey("It should not return duplicates", func() {
					So(repeated, ShouldBeEmpty)
				})
			})
		})
	})
}
