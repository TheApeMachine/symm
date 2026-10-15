package broker

import (
	"testing"
	"unsafe"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
)

type testTee struct {
	pushed []*data.Measurement
}

func (tee *testTee) Push(measurement *data.Measurement) {
	tee.pushed = append(tee.pushed, measurement)
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
			UnrealizedPnL: unrealizedAmount,
			Equity:        equityAmount,
		}

		Convey("When metrics are generated from balance state", func() {
			measurement := tradeBalance.ToMeasurement(balance.wallet, balance.Quote, 1)

			So(measurement.Error(), ShouldBeNil)

			balance.measurement = measurement
			balance.uiTee.Push(balance.measurement)

			So(len(tee.pushed), ShouldEqual, 1)
			pushed := tee.pushed[0]
			So(pushed.Source, ShouldEqual, "balance")

			var cash, equity, unrealized, zama float64
			var cashExact string

			for entry := range pushed.Read("cash") {
				if entry.Metric != nil {
					cash = entry.Metric.Raw()
					if entry.Metric.Exact() != nil {
						cashExact = entry.Metric.Exact().String()
					}
				}
			}
			for entry := range pushed.Read("equity") {
				if entry.Metric != nil {
					equity = entry.Metric.Raw()
				}
			}
			for entry := range pushed.Read("unrealized") {
				if entry.Metric != nil {
					unrealized = entry.Metric.Raw()
				}
			}
			for entry := range pushed.Read("balance:ZAMA") {
				if entry.Metric != nil {
					zama = entry.Metric.Raw()
				}
			}

			So(cash, ShouldEqual, 160.14)
			So(cashExact, ShouldEqual, "160.14")
			So(zama, ShouldEqual, 517.71)
			So(equity, ShouldEqual, 199.68)
			So(unrealized, ShouldEqual, -0.32)
		})
	})
}
