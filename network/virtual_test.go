package network

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bytedance/sonic"
	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/venue/kraken"
)

type stubFills struct {
	ask decimal.Decimal
	bid decimal.Decimal
}

func (stub stubFills) FillAsk(_ string, _ *decimal.Decimal) *decimal.Decimal {
	out := stub.ask
	return &out
}

func (stub stubFills) FillBid(_ string, _ *decimal.Decimal) *decimal.Decimal {
	out := stub.bid
	return &out
}

// stubSettle mirrors broker.Price.afterFee for tests (rate = feePct/100).
func stubSettle(feePct decimal.Decimal) QuoteSettle {
	return func(_ string, gross decimal.Decimal, buy bool) (cash, feeAmt decimal.Decimal) {
		if feePct.Sign() <= 0 || gross.Sign() <= 0 {
			return gross, decimal.Zero
		}

		feeAmt = gross.Mul(feePct).Div(decimal.NewFromInt(100))

		if buy {
			return gross.Add(feeAmt), feeAmt
		}

		return gross.Sub(feeAmt), feeAmt
	}
}

func withVirtual(f func(*Virtual)) func() {
	return func() {
		prevQuote := system.Cfg.Market.QuoteCurrency
		prevBal := system.Cfg.Market.Balance
		system.Cfg.Market.QuoteCurrency = "USD"
		system.Cfg.Market.Balance = 1000

		defer func() {
			system.Cfg.Market.QuoteCurrency = prevQuote
			system.Cfg.Market.Balance = prevBal
		}()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		virtual := NewVirtual(ctx)
		virtual.UseFills(stubFills{
			ask: decimal.NewFromInt(10),
			bid: decimal.NewFromInt(9),
		})
		f(virtual)
	}
}

func TestWrite(t *testing.T) {
	Convey("Setup", t, withVirtual(func(virtual *Virtual) {
		cases := []struct {
			name    string
			message map[string]any
			wantErr bool
		}{
			{
				name:    "garbage is rejected",
				message: nil,
				wantErr: true,
			},
			{
				name:    "unknown method is rejected",
				message: map[string]any{"method": "cancel_order"},
				wantErr: true,
			},
			{
				name: "subscribe balances is accepted",
				message: map[string]any{
					"method": "subscribe",
					"params": map[string]any{"channel": "balances"},
				},
				wantErr: false,
			},
			{
				name: "subscribe executions is accepted",
				message: map[string]any{
					"method": "subscribe",
					"params": map[string]any{"channel": "executions"},
				},
				wantErr: false,
			},
			{
				name: "add_order is accepted",
				message: map[string]any{
					"method": "add_order",
					"params": map[string]any{
						"side":      "buy",
						"symbol":    "AAA/USD",
						"order_qty": "1",
						"cl_ord_id": "entry-1",
					},
				},
				wantErr: false,
			},
		}

		for _, tc := range cases {
			Convey("When "+tc.name, func() {
				var (
					buf []byte
					err error
				)

				if tc.message == nil {
					err = virtual.Write([]byte("{"))
				} else {
					buf, err = sonic.Marshal(tc.message)
					So(err, ShouldBeNil)
					err = virtual.Write(buf)
				}

				if tc.wantErr {
					So(err, ShouldNotBeNil)
					return
				}

				So(err, ShouldBeNil)

				frame, readErr := virtual.Read()
				So(readErr, ShouldBeNil)
				So(len(frame), ShouldBeGreaterThan, 0)
			})
		}

		Convey("When subscribe balances is read back as a wallet", func() {
			buf, err := sonic.Marshal(map[string]any{
				"method": "subscribe",
				"params": map[string]any{"channel": "balances"},
			})
			So(err, ShouldBeNil)
			So(virtual.Write(buf), ShouldBeNil)

			frame, err := virtual.Read()
			So(err, ShouldBeNil)

			venue := kraken.NewBalance()
			So(sonic.Unmarshal(frame, venue), ShouldBeNil)
			So(venue.Result["USD"].String(), ShouldEqual, "1000")

			book := market.NewBalance(map[string]any{
				"quote":  "USD",
				"assets": []*market.Asset{},
			})
			venue.ToMarket(book)
			So(book.Cash("USD").String(), ShouldEqual, "1000")
		})

		Convey("When add_order fills against the book with desk fees", func() {
			fresh := NewVirtual(virtual.ctx)
			fresh.UseFills(stubFills{
				ask: decimal.NewFromInt(10),
				bid: decimal.NewFromInt(9),
			})
			fresh.UseQuoteSettle(stubSettle(decimal.RequireFromString("0.4")))

			buf, err := sonic.Marshal(map[string]any{
				"method": "add_order",
				"params": map[string]any{
					"side":      "buy",
					"symbol":    "AAA/USD",
					"order_qty": "2",
					"cl_ord_id": "cl-abc",
				},
			})
			So(err, ShouldBeNil)
			So(fresh.Write(buf), ShouldBeNil)

			frame, err := fresh.Read()
			So(err, ShouldBeNil)

			exec := kraken.NewExecution()
			So(sonic.Unmarshal(frame, exec), ShouldBeNil)
			So(exec.Channel, ShouldEqual, "executions")
			So(len(exec.Data), ShouldEqual, 1)
			So(exec.Data[0].ClientOrderID, ShouldEqual, "cl-abc")
			So(exec.Data[0].Symbol, ShouldEqual, "AAA/USD")
			So(exec.Data[0].Side, ShouldEqual, "buy")
			So(exec.Data[0].ExecType, ShouldEqual, "trade")
			So(exec.Data[0].OrderStatus, ShouldEqual, "filled")
			So(exec.Data[0].LastQty.String(), ShouldEqual, "2")
			So(exec.Data[0].LastPrice.String(), ShouldEqual, "10")
			So(exec.Data[0].AvgPrice.String(), ShouldEqual, "10")
			So(exec.Data[0].FeeUsdEquiv.String(), ShouldEqual, "0.08")
			So(exec.Data[0].Cost.String(), ShouldEqual, "20")

			book := &market.Execution{}
			exec.ToMarket(book)
			So(len(book.Data), ShouldEqual, 1)
			So(book.Data[0].ClientOrderID, ShouldEqual, "cl-abc")
			So(book.Data[0].LastPrice.String(), ShouldEqual, "10")

			balances, err := fresh.Read()
			So(err, ShouldBeNil)

			venue := kraken.NewBalance()
			So(sonic.Unmarshal(balances, venue), ShouldBeNil)
			So(venue.Result["USD"].String(), ShouldEqual, "979.92")
			So(venue.Result["AAA"].String(), ShouldEqual, "2")
		})

		Convey("When add_order has no book price it fails", func() {
			bare := NewVirtual(virtual.ctx)
			bare.UseFills(stubFills{})

			buf, err := sonic.Marshal(map[string]any{
				"method": "add_order",
				"params": map[string]any{
					"side":      "buy",
					"symbol":    "AAA/USD",
					"order_qty": "1",
					"cl_ord_id": "cl-x",
				},
			})
			So(err, ShouldBeNil)
			So(bare.Write(buf), ShouldNotBeNil)
		})

		Convey("When Restock credits base a sell can fill", func() {
			fresh := NewVirtual(virtual.ctx)
			fresh.UseFills(stubFills{
				ask: decimal.NewFromInt(10),
				bid: decimal.NewFromInt(9),
			})
			fresh.UseQuoteSettle(stubSettle(decimal.RequireFromString("0.4")))

			fresh.Restock(
				"AAA/USD",
				decimal.NewFromInt(2),
				decimal.RequireFromString("20.08"),
			)

			buf, err := sonic.Marshal(map[string]any{
				"method": "add_order",
				"params": map[string]any{
					"side":      "sell",
					"symbol":    "AAA/USD",
					"order_qty": "2",
					"cl_ord_id": "exit-1",
				},
			})
			So(err, ShouldBeNil)
			So(fresh.Write(buf), ShouldBeNil)

			frame, err := fresh.Read()
			So(err, ShouldBeNil)

			exec := kraken.NewExecution()
			So(sonic.Unmarshal(frame, exec), ShouldBeNil)
			So(exec.Data[0].Side, ShouldEqual, "sell")
			So(exec.Data[0].LastQty.String(), ShouldEqual, "2")
			So(exec.Data[0].FeeUsdEquiv.String(), ShouldEqual, "0.072")

			balances, err := fresh.Read()
			So(err, ShouldBeNil)

			venue := kraken.NewBalance()
			So(sonic.Unmarshal(balances, venue), ShouldBeNil)
			So(venue.Result["AAA"].String(), ShouldEqual, "0")
			So(venue.Result["USD"].String(), ShouldEqual, "997.848")
		})
	}))
}

func TestApplyFillEquityUsesQuoteBasis(t *testing.T) {
	Convey("Base quantity is not added into quote equity", t, withVirtual(func(virtual *Virtual) {
		virtual.UseFills(stubFills{
			ask: decimal.NewFromInt(10),
			bid: decimal.NewFromInt(10),
		})
		virtual.UseQuoteSettle(stubSettle(decimal.Zero))

		err := virtual.applyFill(
			"buy", "PEPE/USD",
			decimal.NewFromInt(1),
			decimal.NewFromInt(10),
		)
		So(err, ShouldBeNil)

		raw, err := virtual.Post("/0/private/TradeBalance", nil)
		So(err, ShouldBeNil)

		var venue kraken.TradeBalance
		So(json.Unmarshal(raw, &venue), ShouldBeNil)

		book := market.NewTradeBalance(nil)
		venue.ToMarket(book)
		// Cash 990 + PEPE mark 10 = 1000, not 990 + base qty.
		So(book.Equity.String(), ShouldEqual, "1000")
		So(book.AvailableCash.String(), ShouldEqual, "990")
		So(book.UnrealizedPnL.String(), ShouldEqual, "0")
	}))
}

func TestTradeBalanceMarksOpenLots(t *testing.T) {
	Convey("Open lots mark at fee-net bid so unrealized moves equity", t, withVirtual(func(virtual *Virtual) {
		virtual.UseFills(stubFills{
			ask: decimal.NewFromInt(10),
			bid: decimal.NewFromInt(9),
		})
		virtual.UseQuoteSettle(stubSettle(decimal.Zero))

		err := virtual.applyFill(
			"buy", "DOGE/USD",
			decimal.NewFromInt(1),
			decimal.NewFromInt(10),
		)
		So(err, ShouldBeNil)

		raw, err := virtual.Post("/0/private/TradeBalance", nil)
		So(err, ShouldBeNil)

		var venue kraken.TradeBalance
		So(json.Unmarshal(raw, &venue), ShouldBeNil)

		book := market.NewTradeBalance(nil)
		venue.ToMarket(book)
		// Cash 990 + mark 9 = 999; unrealized = mark − cost = −1.
		So(book.AvailableCash.String(), ShouldEqual, "990")
		So(book.UnrealizedPnL.String(), ShouldEqual, "-1")
		So(book.Equity.String(), ShouldEqual, "999")
	}))
}

func TestPost(t *testing.T) {
	Convey("Setup", t, withVirtual(func(virtual *Virtual) {
		cases := []struct {
			name    string
			url     string
			wantErr bool
		}{
			{"Balance path", "/0/private/Balance", false},
			{"TradeBalance path", "/0/private/TradeBalance", false},
			{"unknown path", "/0/private/OpenOrders", true},
		}

		for _, tc := range cases {
			Convey("When posting "+tc.name, func() {
				raw, err := virtual.Post(tc.url, nil)

				if tc.wantErr {
					So(err, ShouldNotBeNil)
					So(raw, ShouldBeNil)
					return
				}

				So(err, ShouldBeNil)

				if tc.url == "/0/private/TradeBalance" {
					var venue kraken.TradeBalance
					So(json.Unmarshal(raw, &venue), ShouldBeNil)

					book := market.NewTradeBalance(nil)
					venue.ToMarket(book)
					So(book.Equity.String(), ShouldEqual, "1000")
					So(book.AvailableCash.String(), ShouldEqual, "1000")
					So(book.FreeMargin.String(), ShouldEqual, "1000")
					return
				}

				venue := kraken.NewBalance()
				So(json.Unmarshal(raw, venue), ShouldBeNil)
				So(venue.Result["USD"].String(), ShouldEqual, "1000")
			})
		}
	}))
}

func TestRead(t *testing.T) {
	Convey("Setup", t, withVirtual(func(virtual *Virtual) {
		Convey("When the context is cancelled before a frame arrives", func() {
			ctx, cancel := context.WithCancel(context.Background())
			virtual = NewVirtual(ctx)
			cancel()

			_, err := virtual.Read()
			So(err, ShouldNotBeNil)
		})
	}))
}
