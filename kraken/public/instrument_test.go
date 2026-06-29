package public

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/qpool"
)

func TestInstrumentUpdate(t *testing.T) {
	Convey("Given a Kraken instrument snapshot with mixed pairs", t, func() {
		previousQuote := viper.GetString("market.quote_currency")
		viper.Set("market.quote_currency", "USD")
		defer viper.Set("market.quote_currency", previousQuote)

		pool := qpool.NewQ[any](t.Context(), 1, 1, nil)
		instrument := NewInstrument(t.Context(), pool)
		artifact := datura.Acquire("kraken:public", datura.APPJSON).WithPayload([]byte(`{
			"channel":"instrument",
			"type":"snapshot",
			"data":{
				"pairs":[
					{"symbol":"DOGE/USD","base":"DOGE","quote":"USD","status":"online"},
					{"symbol":"DOGE/EUR","base":"DOGE","quote":"EUR","status":"online"},
					{"symbol":"ETH/USD","base":"ETH","quote":"USD","status":"cancel_only"}
				]
			}
		}`))
		defer artifact.Release()

		Convey("When Update handles the snapshot", func() {
			consumer := pool.Subscribe("kraken:public", nil)
			instrument.Update(artifact)

			Convey("Then it should cache only online pairs for the configured quote", func() {
				_, dogeUSD := instrument.cache.Load("DOGE/USD")
				_, dogeEUR := instrument.cache.Load("DOGE/EUR")
				_, ethUSD := instrument.cache.Load("ETH/USD")

				So(dogeUSD, ShouldBeTrue)
				So(dogeEUR, ShouldBeFalse)
				So(ethUSD, ShouldBeFalse)
			})

			Convey("And it should subscribe to Kraken's canonical trade channel", func() {
				waitCtx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()

				channels := make(map[string]bool)

				for range 4 {
					message, waitErr := consumer.Wait(waitCtx)

					So(waitErr, ShouldBeNil)
					So(message, ShouldNotBeNil)

					channels[datura.Peek[string](message, "params", "channel")] = true
				}

				So(channels[TradesChannel], ShouldBeTrue)
				So(channels["trades"], ShouldBeFalse)
			})
		})
	})
}
