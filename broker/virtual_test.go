package broker

import (
	"context"
	"sync"
	"testing"

	"github.com/bytedance/sonic"
	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/venue/kraken"
)

func TestBindVirtualChargesAfterFee(t *testing.T) {
	Convey("Virtual wallet debit matches Price.afterFee", t, func() {
		prevQuote := system.Cfg.Market.QuoteCurrency
		prevBal := system.Cfg.Market.Balance
		system.Cfg.Market.QuoteCurrency = "USD"
		system.Cfg.Market.Balance = 1000
		Reset(func() {
			system.Cfg.Market.QuoteCurrency = prevQuote
			system.Cfg.Market.Balance = prevBal
		})

		fee := decimal.RequireFromString("0.4")
		instrument := &Instrument{cache: &sync.Map{}, takerFees: &sync.Map{}}
		instrument.takerFees.Store("AAA/USD", &fee)

		ask := decimal.NewFromInt(10)
		bid := decimal.NewFromInt(9)
		qty := decimal.NewFromInt(100)
		books := NewBookManager()
		books.Update(&market.Level3{
			Data: []*market.Level3Data{{
				Symbol: "AAA/USD",
				Bids: []*market.Level3Order{{
					LimitPrice: &bid,
					OrderQty:   &qty,
				}},
				Asks: []*market.Level3Order{{
					LimitPrice: &ask,
					OrderQty:   &qty,
				}},
			}},
		})

		price := &Price{Instrument: instrument, Books: books}
		virtual := network.NewVirtual(context.Background())
		BindVirtual(virtual, price)

		gross := ask.Mul(decimal.NewFromInt(2))
		wantCash := price.afterFee(&gross, &fee, BUY)
		So(wantCash, ShouldNotBeNil)
		wantFee := wantCash.Sub(gross)

		buf, err := sonic.Marshal(map[string]any{
			"method": "add_order",
			"params": map[string]any{
				"side":      "buy",
				"symbol":    "AAA/USD",
				"order_qty": "2",
				"cl_ord_id": "fee-1",
			},
		})
		So(err, ShouldBeNil)
		So(virtual.Write(buf), ShouldBeNil)

		frame, err := virtual.Read()
		So(err, ShouldBeNil)

		exec := kraken.NewExecution()
		So(sonic.Unmarshal(frame, exec), ShouldBeNil)
		So(exec.Data[0].FeeUsdEquiv.String(), ShouldEqual, wantFee.String())

		balances, err := virtual.Read()
		So(err, ShouldBeNil)

		venue := kraken.NewBalance()
		So(sonic.Unmarshal(balances, venue), ShouldBeNil)
		So(
			venue.Result["USD"].String(),
			ShouldEqual,
			decimal.NewFromInt(1000).Sub(*wantCash).String(),
		)
	})
}
