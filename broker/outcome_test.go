package broker

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func TestPriceReconcile(t *testing.T) {
	Convey("Given exact partial entry and exit fills with exchange fees", t, func() {
		price := &Price{}
		executions := []*kraken.Execution{{
			Data: []kraken.ExecutionData{
				{
					ExecID: "buy-1", ExecType: "trade", Symbol: "BTC/USD", Side: "buy",
					LastQty: 0.4, LastPrice: *decimal.NewFromInt64(100),
					Cost: *decimal.NewFromInt64(40), FeeUsdEquiv: *decimal.NewFromFloat64(0.4),
				},
				{
					ExecID: "buy-2", ExecType: "trade", Symbol: "BTC/USD", Side: "buy",
					LastQty: 0.6, LastPrice: *decimal.NewFromInt64(100),
					Cost: *decimal.NewFromInt64(60), FeeUsdEquiv: *decimal.NewFromFloat64(0.6),
				},
			},
		}, {
			Data: []kraken.ExecutionData{
				{
					ExecID: "sell-1", ExecType: "trade", Symbol: "BTC/USD", Side: "sell",
					LastQty: 1, LastPrice: *decimal.NewFromInt64(110),
					Cost: *decimal.NewFromInt64(110), FeeUsdEquiv: *decimal.NewFromInt64(1),
				},
			},
		}}

		outcome, err := price.Reconcile("BTC/USD", executions)

		Convey("Then realized PnL uses each unique fill and actual fee exactly once", func() {
			So(err, ShouldBeNil)
			So(outcome.EntryNotional.Float64(), ShouldEqual, 100.0)
			So(outcome.ExitNotional.Float64(), ShouldEqual, 110.0)
			So(outcome.EntryFee.Float64(), ShouldEqual, 1.0)
			So(outcome.ExitFee.Float64(), ShouldEqual, 1.0)
			So(outcome.PnL.Float64(), ShouldEqual, 8.0)
			So(outcome.ReturnPct, ShouldAlmostEqual, 7.9207920792, 0.0000001)
		})

		Convey("Then a duplicate execution identity is not counted twice", func() {
			executions = append(executions, &kraken.Execution{
				Data: []kraken.ExecutionData{executions[1].Data[0]},
			})

			outcome, err = price.Reconcile("BTC/USD", executions)
			So(err, ShouldBeNil)
			So(outcome.PnL.Float64(), ShouldEqual, 8.0)
		})
	})

	Convey("Given a maker-rebate fill with negative USD-equivalent fee", t, func() {
		price := &Price{}
		executions := []*kraken.Execution{{Data: []kraken.ExecutionData{
			{
				ExecID: "buy", ExecType: "trade", Symbol: "BTC/USD", Side: "buy",
				LastQty: 1, LastPrice: *decimal.NewFromInt64(100),
				Cost: *decimal.NewFromInt64(100), FeeUsdEquiv: *decimal.NewFromFloat64(1),
			},
			{
				ExecID: "sell", ExecType: "trade", Symbol: "BTC/USD", Side: "sell",
				LastQty: 1, LastPrice: *decimal.NewFromInt64(110),
				Cost: *decimal.NewFromInt64(110), FeeUsdEquiv: *decimal.NewFromFloat64(-0.5),
			},
		}}}

		outcome, err := price.Reconcile("BTC/USD", executions)

		Convey("Then the rebate reduces total fees and increases realized PnL", func() {
			So(err, ShouldBeNil)
			So(outcome.ExitFee.Float64(), ShouldEqual, -0.5)
			So(outcome.PnL.Float64(), ShouldEqual, 9.5)
		})
	})
}

func BenchmarkPriceReconcile(b *testing.B) {
	price := &Price{}
	executions := []*kraken.Execution{{Data: []kraken.ExecutionData{
		{
			ExecID: "buy", ExecType: "trade", Symbol: "BTC/USD", Side: "buy",
			LastQty: 1, LastPrice: *decimal.NewFromInt64(100),
			Cost: *decimal.NewFromInt64(100), FeeUsdEquiv: *decimal.NewFromInt64(1),
			Timestamp: time.Unix(1, 0),
		},
		{
			ExecID: "sell", ExecType: "trade", Symbol: "BTC/USD", Side: "sell",
			LastQty: 1, LastPrice: *decimal.NewFromInt64(110),
			Cost: *decimal.NewFromInt64(110), FeeUsdEquiv: *decimal.NewFromInt64(1),
			Timestamp: time.Unix(2, 0),
		},
	}}}

	b.ReportAllocs()

	for b.Loop() {
		outcome, err := price.Reconcile("BTC/USD", executions)

		if err != nil || outcome.PnL.Float64() != 8 {
			b.Fatal("realized outcome was not reconciled")
		}
	}
}
