package trader

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/focus"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestHandleActionGuards(t *testing.T) {
	Convey("Given an open focus stream", t, func() {
		streams := focus.NewSet()
		streams.Add("BTC/EUR")

		crypto := &Crypto{
			streams: streams,
			desk:    nil,
		}

		viper.Set("trading.paper.wallet_eur", 200.0)
		viper.Set("market.quote_currency", "EUR")
		defer viper.Set("trading.paper.wallet_eur", 0)
		defer viper.Set("market.quote_currency", "")

		entry := perspectives.ActionFromMeasurement(
			perspectives.ActionLimit,
			perspectives.Measurement{Symbol: "BTC/EUR", Last: 50_000},
		)

		Convey("When an entry action arrives while already holding", func() {
			So(crypto.handleAction(entry), ShouldBeNil)
			So(streams.Has("BTC/EUR"), ShouldBeTrue)
		})
	})
}
