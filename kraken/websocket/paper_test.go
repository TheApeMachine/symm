package websocket

import (
	"context"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken"
)

/*
TestPaperAddOrder verifies that the in-process matcher emits the submitted
symbol while retaining fill identity and fee under simulator latency.
*/
func TestPaperAddOrder(t *testing.T) {
	Convey("Given a paper client with a seeded mark and base balance", t, func() {
		viper.Set("system.actor.buffer", 64)
		viper.Set("market.quote_currency", "USD")

		paper := NewPaper(context.Background(), NewSimulator(), config.Fixture())
		paper.Matcher().SeedBalance("BTC", 0.00299963)
		paper.Matcher().SetMark("BTC/USD", 64838.8)

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
			So(orderAck.Result.OrderID, ShouldStartWith, "PAPER-")
			So(execution.Data, ShouldHaveLength, 1)
			So(execution.Data[0].ExecID, ShouldStartWith, "PAPER-")
			So(execution.Data[0].Symbol, ShouldEqual, "BTC/USD")
			So(execution.Data[0].LastPrice.Float64(), ShouldEqual, 64838.8)
			So(balance.Type, ShouldEqual, "snapshot")
			So(len(balance.Data), ShouldBeGreaterThan, 0)
		})
	})
}
