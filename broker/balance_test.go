package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/decimal"
	"github.com/theapemachine/symm/kraken"
)

func TestBalanceReading(t *testing.T) {
	Convey("Given a Balance instance with a multi-asset wallet", t, func() {
		usdAmount, errUSD := decimal.NewFromString("160.14")
		So(errUSD, ShouldBeNil)
		zamaAmount, errZAMA := decimal.NewFromString("517.71")
		So(errZAMA, ShouldBeNil)
		unrealizedAmount, errUnrealized := decimal.NewFromString("-0.32")
		So(errUnrealized, ShouldBeNil)
		equityAmount, errEquity := decimal.NewFromString("199.68")
		So(errEquity, ShouldBeNil)

		balance := &Balance{Quote: "USD"}
		balance.wallet.Store(&kraken.Balance{
			Data: []kraken.BalanceData{
				{
					Asset:     "USD",
					Balance:   usdAmount,
					Available: usdAmount,
				},
				{
					Asset:     "ZAMA",
					Balance:   zamaAmount,
					Available: zamaAmount,
				},
			},
		})

		reading := map[string]any{
			"cash":       balance.cash(balance.Quote),
			"equity":     equityAmount,
			"unrealized": unrealizedAmount,
		}
		balance.reading.Store(&reading)

		Convey("It exposes the dashboard reading map", func() {
			So(balance.Reading()["cash"], ShouldEqual, usdAmount)
			So(balance.Reading()["equity"], ShouldEqual, equityAmount)
			So(balance.Reading()["unrealized"], ShouldEqual, unrealizedAmount)
		})
	})
}
