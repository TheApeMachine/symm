package broker

import (
	"context"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/broker/position"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/runtime"
)

type orderProbe struct {
	writes int
}

func (o *orderProbe) Write(buf []byte) error {
	o.writes++
	return nil
}

func TestEnterOnPendingBeforeTransportWrite(t *testing.T) {
	Convey("onPending runs before EnterWithRegulator (and any Write)", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		viper.Set("trading.allocation.max_fraction", 0.1)
		viper.Set("market.model", "paper")

		probe := &orderProbe{}
		price := NewPrice(ctx, nil, nil, nil, nil)
		price.SetFee("BTC/USD", kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(0.1)})

		balance := NewBalance(ctx, nil)
		balance.Quote = "USD"
		balance.UpdateWallet(&kraken.Balance{
			Data: []kraken.BalanceData{{
				Asset:   "USD",
				Balance: decimal.NewFromFloat64(1000),
			}},
		})

		price.Transition(runtime.READY)
		exec := NewExecution(ctx, probe, price, balance)
		exec.Transition(runtime.READY)

		order := make([]string, 0, 2)
		reg, err := exec.Enter("BTC/USD", func(r *position.Regulator) {
			order = append(order, "pending:"+r.Symbol)
			So(probe.writes, ShouldEqual, 0)
		})

		// No book → Quantity fails after onPending, before Write.
		So(order, ShouldResemble, []string{"pending:BTC/USD"})
		So(probe.writes, ShouldEqual, 0)
		So(reg, ShouldNotBeNil)
		So(err, ShouldNotBeNil)
	})
}
