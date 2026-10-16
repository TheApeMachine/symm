package broker

import (
	"testing"
	"unsafe"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/kraken"
)

type testTee struct {
	pushed []*data.Value
}

func (tee *testTee) Push(value *data.Value) {
	tee.pushed = append(tee.pushed, value)
}

func (tee *testTee) Next() unsafe.Pointer {
	return nil
}

func (tee *testTee) Close() error {
	return nil
}

func (tee *testTee) Run() {}

func (tee *testTee) Pending() int64 {
	return int64(len(tee.pushed))
}

func TestBalance(t *testing.T) {
	Convey("Given a Balance instance with a multi-asset wallet", t, func() {
		tee := &testTee{}
		usdAmount, errUSD := decimal.NewFromString("160.14")
		So(errUSD, ShouldBeNil)
		zamaAmount, errZAMA := decimal.NewFromString("517.71")
		So(errZAMA, ShouldBeNil)
		unrealizedAmount, errUnrealized := decimal.NewFromString("-0.32")
		So(errUnrealized, ShouldBeNil)
		equityAmount, errEquity := decimal.NewFromString("199.68")
		So(errEquity, ShouldBeNil)

		balance := &Balance{
			Quote: "USD",
			uiTee: tee,
			wallet: &kraken.Balance{
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
			},
		}

		tradeBalance := kraken.TradeBalanceResult{
			TradeBalance:  usdAmount,
			UnrealizedPnL: unrealizedAmount,
			Equity:        equityAmount,
		}

		Convey("When metrics are generated from balance state", func() {
			buf, err := sonic.Marshal(tradeBalance)
			So(err, ShouldBeNil)
			value := data.NewValue("broker", "balance", balance.Quote).Append(buf)

			balance.value = value
			balance.uiTee.Push(balance.value)

			So(len(tee.pushed), ShouldEqual, 1)
			pushed := tee.pushed[0]
			So(pushed.Origin, ShouldEqual, "broker")

			tb := data.Attribute[string](pushed.Layers[0], "tb")
			So(tb, ShouldEqual, "160.14")
			equity := data.Attribute[string](pushed.Layers[0], "e")
			So(equity, ShouldEqual, "199.68")
			unrealized := data.Attribute[string](pushed.Layers[0], "n")
			So(unrealized, ShouldEqual, "-0.32")
		})
	})
}
