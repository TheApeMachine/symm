package broker

import (
	"testing"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/symm/kraken"
)

func TestBalance(t *testing.T) {
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

		tradeBalance := kraken.TradeBalanceResult{
			TradeBalance:  usdAmount,
			UnrealizedPnL: unrealizedAmount,
			Equity:        equityAmount,
		}

		Convey("When metrics are generated from balance state", func() {
			buf, err := sonic.Marshal(tradeBalance)
			So(err, ShouldBeNil)
			value := data.NewValue("broker", "balance", balance.Quote).Append(buf)

			balance.value.Store(value)
		})
	})
}
