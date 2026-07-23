package websocket

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken"
)

/*
TestPaperAddOrder verifies that the paper adapter emits the canonical symbol
from the submitted order while retaining the client fill's identity and fee.
*/
func TestPaperAddOrder(t *testing.T) {
	Convey("Given a paper client returning a compact Kraken pair", t, func() {
		viper.Set("system.actor.buffer", 64)
		path := filepath.Join(t.TempDir(), "kraken")
		script := `#!/bin/sh
case "$2" in
sell)
  printf '%s\n' '{"action":"market_order_filled","order_id":"PAPER-00041","trade_id":"PAPER-00042","pair":"BTCUSD","side":"sell","volume":0.00299963,"price":64838.8,"cost":194.492409644,"fee":0.5056802650744,"status":"filled","time":"2026-07-14T21:42:10Z"}'
  ;;
balance)
  printf '%s\n' '{"balances":{"USD":{"available":100,"reserved":0,"total":100}},"mode":"paper"}'
  ;;
*)
  exit 1
  ;;
esac
`
		So(os.WriteFile(path, []byte(script), 0o755), ShouldBeNil)
		t.Setenv("PATH", filepath.Dir(path)+string(os.PathListSeparator)+os.Getenv("PATH"))

		paper := NewPaper(context.Background(), NewSimulator())
		ackSub := paper.Subscribe("add_order")
		execSub := paper.Subscribe("executions")
		balSub := paper.Subscribe("balances")

		order := kraken.NewMarketOrder(
			"sell", decimal.NewFromFloat64(0.00299963), "BTC/USD",
		)
		err := paper.AddOrder(order)

		Convey("Then the emitted fill reconciles with the internal symbol", func() {
			So(err, ShouldBeNil)

			orderAck := kraken.NewOrderResponse((<-ackSub.Channel).([]byte))
			execution := kraken.NewExecution((<-execSub.Channel).([]byte))
			balance := kraken.NewBalance((<-balSub.Channel).([]byte))

			So(orderAck.ReqID, ShouldEqual, order.ReqID)
			So(orderAck.Result.OrderID, ShouldEqual, "PAPER-00041")
			So(execution.Data, ShouldHaveLength, 1)
			So(execution.Data[0].ExecID, ShouldEqual, "PAPER-00042")
			So(execution.Data[0].Symbol, ShouldEqual, "BTC/USD")
			So(execution.Data[0].FeeUsdEquiv.Float64(), ShouldAlmostEqual, 0.5056802650744, 1e-8)
			So(balance.Type, ShouldEqual, "snapshot")
			So(balance.Data, ShouldHaveLength, 1)
			So(balance.Data[0].Asset, ShouldEqual, "USD")
			So(balance.Data[0].Balance.Float64(), ShouldEqual, 100.0)
		})
	})
}
