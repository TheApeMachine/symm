package websocket

import (
	"testing"

	"github.com/bytedance/sonic"

	. "github.com/smartystreets/goconvey/convey"
)

// sampleTradeVolume mirrors a real Kraken /0/private/TradeVolume response:
// account-wide volume plus per-altname taker (fees) and maker (fees_maker)
// schedules. Note the keys are altnames (XXBTZUSD), not websocket symbols.
const sampleTradeVolume = `{
  "error": [],
  "result": {
    "currency": "ZUSD",
    "volume": "200709587.4223",
    "fees": {
      "XXBTZUSD": {"fee": "0.1000", "minfee": "0.1000", "maxfee": "0.2600"}
    },
    "fees_maker": {
      "XXBTZUSD": {"fee": "0.0000", "minfee": "0.0000", "maxfee": "0.1600"}
    }
  }
}`

func TestTradeVolumeSchedule(testingTB *testing.T) {
	Convey("Given a real TradeVolume response", testingTB, func() {
		var envelope struct {
			Result tradeVolumeResult `json:"result"`
		}

		So(sonic.Unmarshal([]byte(sampleTradeVolume), &envelope), ShouldBeNil)

		Convey("When resolved with an altname->wsname map", func() {
			schedule, err := envelope.Result.schedule(map[string]string{
				"XXBTZUSD": "BTC/USD",
			})

			So(err, ShouldBeNil)

			Convey("The fallback tier carries taker and maker fractions", func() {
				So(schedule.Fallback.Taker, ShouldAlmostEqual, 0.001)
				So(schedule.Fallback.Maker, ShouldAlmostEqual, 0.0)
			})

			Convey("The itemized pair is keyed by websocket symbol", func() {
				So(schedule.Rates("BTC/USD").Taker, ShouldAlmostEqual, 0.001)
			})

			Convey("An unlisted symbol falls back to the account tier", func() {
				So(schedule.Rates("ZEC/USD").Taker, ShouldAlmostEqual, 0.001)
			})
		})

		Convey("When the taker schedule is empty it errors", func() {
			empty := tradeVolumeResult{}
			_, err := empty.schedule(map[string]string{})
			So(err, ShouldNotBeNil)
		})
	})
}
