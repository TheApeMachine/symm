package broker

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
)

func priceFor(symbol string, books *BookManager) *Price {
	fee := decimal.RequireFromString("0")
	instrument := &Instrument{cache: &sync.Map{}, takerFees: &sync.Map{}}
	instrument.takerFees.Store(symbol, &fee)

	return &Price{Instrument: instrument, Books: books}
}

func bookWithTouch(symbol, bid, ask, qty string) *BookManager {
	books := NewBookManager()
	b := decimal.RequireFromString(bid)
	a := decimal.RequireFromString(ask)
	q := decimal.RequireFromString(qty)

	books.Update(&market.Level3{
		Type: "snapshot",
		Data: []*market.Level3Data{{
			Symbol: symbol,
			Bids: []*market.Level3Order{{
				OrderID:    "b1",
				LimitPrice: &b,
				OrderQty:   &q,
				Timestamp:  time.Unix(1, 0),
			}},
			Asks: []*market.Level3Order{{
				OrderID:    "a1",
				LimitPrice: &a,
				OrderQty:   &q,
				Timestamp:  time.Unix(1, 0),
			}},
		}},
	})

	return books
}

func tradeFill(execID, side, qty, px string) market.ExecutionData {
	q := decimal.RequireFromString(qty)
	p := decimal.RequireFromString(px)

	return market.ExecutionData{
		ExecID:    execID,
		ExecType:  "trade",
		Side:      side,
		LastQty:   &q,
		LastPrice: &p,
	}
}

func withOpenPosition(
	symbol, tier, qty, entry string,
	books *BookManager,
	f func(*Position),
) func() {
	return func() {
		price := priceFor(symbol, books)
		position := NewPosition(context.Background(), symbol, tier, "path", price, nil, nil)
		position.Apply(tradeFill("seed-buy", "buy", qty, entry))
		f(position)
	}
}

func replayFills(symbol string, fills []market.ExecutionData) *Position {
	position := NewPosition(context.Background(), symbol, "virtual", "path", nil, nil, nil)

	for i := range fills {
		position.Apply(fills[i])
	}

	return position
}

func assertLedgerConsistent(position *Position) {
	if len(position.Fills) == 0 {
		So(position.Qty, ShouldBeNil)
		So(position.Entry, ShouldBeNil)

		return
	}

	wire := make([]market.ExecutionData, 0, len(position.Fills))

	for _, fill := range position.Fills {
		if fill != nil {
			wire = append(wire, *fill)
		}
	}

	replayed := replayFills(position.Symbol, wire)

	lq, le, ls, lv := foldSnapshot(position)
	rq, re, rs, rv := foldSnapshot(replayed)

	So(rq, ShouldEqual, lq)
	So(re, ShouldEqual, le)
	So(rs, ShouldEqual, ls)

	if ls != "" || rs != "" {
		So(rv, ShouldEqual, lv)
	}

	So(len(replayed.Fills), ShouldEqual, len(position.Fills))
}

func foldSnapshot(position *Position) (qty, entry, sold, vwap string) {
	if position.Qty != nil {
		qty = position.Qty.String()
	}

	if position.Entry != nil {
		entry = position.Entry.String()
	}

	if position.Sold != nil {
		sold = position.Sold.String()
	}

	if position.ExitVWAP != nil {
		vwap = position.ExitVWAP.String()
	}

	return qty, entry, sold, vwap
}

func TestUpdate(t *testing.T) {
	Convey("Setup", t, withOpenPosition(
		"BTC/USD", "virtual", "1", "100",
		bookWithTouch("BTC/USD", "100", "101", "1"),
		func(position *Position) {
			Convey("When the book walk marks and print is display-only", func() {
				mark := decimal.RequireFromString("98")
				So(position.Update(&mark), ShouldBeNil)

				So(position.Mark.String(), ShouldEqual, "98")
				So(position.ExitVWAP.String(), ShouldEqual, "100")
				So(position.PnL.IsZero(), ShouldBeTrue)
				So(position.PnLPct.IsZero(), ShouldBeTrue)

				pnlBefore := position.PnL.String()
				pctBefore := position.PnLPct.String()

				wildPrint := decimal.RequireFromString("200")
				So(position.Update(&wildPrint), ShouldBeNil)

				So(position.Mark.String(), ShouldEqual, "200")
				So(position.PnL.String(), ShouldEqual, pnlBefore)
				So(position.PnLPct.String(), ShouldEqual, pctBefore)
				So(position.ExitVWAP.String(), ShouldEqual, "100")
			})
		},
	))

	Convey("Setup", t, withOpenPosition(
		"BTC/USD", "virtual", "1", "100",
		NewBookManager(),
		func(position *Position) {
			Convey("When the book cannot mark Update errors without touching Mark", func() {
				before := ""

				if position.Mark != nil {
					before = position.Mark.String()
				}

				mark := decimal.RequireFromString("98")
				So(position.Update(&mark), ShouldNotBeNil)

				if before == "" {
					So(position.Mark, ShouldBeNil)
				} else {
					So(position.Mark.String(), ShouldEqual, before)
				}
			})
		},
	))

	Convey("Setup", t, func() {
		Convey("When the position was never filled", func() {
			position := NewPosition(context.Background(), "BTC/USD", "virtual", "path", &Price{}, nil, nil)
			So(position.Update(nil), ShouldNotBeNil)
		})
	})

	Convey("Setup", t, func() {
		Convey("When the position is already closed Update errors", func() {
			price := priceFor("BTC/USD", bookWithTouch("BTC/USD", "100", "101", "1"))
			position := NewPosition(
				context.Background(), "BTC/USD", "virtual", "path", price, nil, nil,
			)

			position.Apply(tradeFill("b1", "buy", "1", "100"))
			position.Apply(tradeFill("s1", "sell", "1", "99"))
			So(position.ClosedAt, ShouldNotBeNil)

			mark := decimal.RequireFromString("99")
			So(position.Update(&mark), ShouldNotBeNil)
		})

		Convey("When qty and entry were never filled Update errors", func() {
			position := NewPosition(
				context.Background(), "BTC/USD", "virtual", "path",
				priceFor("BTC/USD", bookWithTouch("BTC/USD", "100", "101", "1")),
				nil, nil,
			)

			mark := decimal.RequireFromString("99")
			So(position.Update(&mark), ShouldNotBeNil)
		})
	})

	Convey("Setup", t, func() {
		Convey("When sell fills exist Update does not rewrite PnL", func() {
			position := NewPosition(
				context.Background(), "BTC/USD", "virtual", "path",
				priceFor("BTC/USD", bookWithTouch("BTC/USD", "100", "101", "2")),
				nil, nil,
			)

			position.Apply(tradeFill("b1", "buy", "2", "100"))

			mark := decimal.RequireFromString("102")
			So(position.Update(&mark), ShouldBeNil)
			So(position.PnL, ShouldNotBeNil)
			pnlBefore := *position.PnL

			position.Apply(tradeFill("s1", "sell", "1", "101"))

			So(position.Qty.String(), ShouldEqual, "1")
			So(position.Sold.String(), ShouldEqual, "1")

			mark = decimal.RequireFromString("103")
			So(position.Update(&mark), ShouldBeNil)
			So(position.Mark.String(), ShouldEqual, "103")
			So(position.PnL.String(), ShouldEqual, pnlBefore.String())
		})
	})

	Convey("Setup", t, withOpenPosition(
		"BTC/USD", "virtual", "1", "100",
		bookWithTouch("BTC/USD", "100", "101", "1"),
		func(position *Position) {
			Convey("When EntryPath is exchange stoploss is not armed", func() {
				position.EntryPath = "exchange"

				mark := decimal.RequireFromString("50")
				So(position.Update(&mark), ShouldBeNil)
				So(position.Stoploss, ShouldBeNil)
				So(position.Closing, ShouldBeFalse)
			})
		},
	))

	Convey("Setup", t, withOpenPosition(
		"BTC/USD", "virtual", "1", "100",
		bookWithTouch("BTC/USD", "100", "101", "1"),
		func(position *Position) {
			Convey("When Closing is already armed Update skips stoploss", func() {
				position.Closing = true
				position.Stoploss = NewStoploss(0)

				mark := decimal.RequireFromString("110")
				So(position.Update(&mark), ShouldBeNil)
				So(position.Stoploss.State, ShouldEqual, StoplossHealthy)
				So(position.Closing, ShouldBeTrue)
			})
		},
	))

	Convey("Setup", t, withOpenPosition(
		"BTC/USD", "virtual", "1", "100",
		bookWithTouch("BTC/USD", "100", "101", "5"),
		func(position *Position) {
			Convey("When book marks round-trip through arm then giveback onSettled fires", func() {
				triggered := false
				position.onSettled = func(*Position) { triggered = true }

				books := position.price.Books
				books.Update(&market.Level3{
					Type: "snapshot",
					Data: []*market.Level3Data{{
						Symbol: "BTC/USD",
						Bids: []*market.Level3Order{{
							OrderID: "b2",
							LimitPrice: decimalPtr("110"),
							OrderQty:   decimalPtr("5"),
							Timestamp:  time.Unix(2, 0),
						}},
						Asks: []*market.Level3Order{{
							OrderID: "a2",
							LimitPrice: decimalPtr("111"),
							OrderQty:   decimalPtr("5"),
							Timestamp:  time.Unix(2, 0),
						}},
					}},
				})

				markUp := decimal.RequireFromString("110")
				So(position.Update(&markUp), ShouldBeNil)
				So(position.Stoploss, ShouldNotBeNil)
				So(position.Stoploss.State, ShouldEqual, StoplossArmed)

				books.Update(&market.Level3{
					Type: "snapshot",
					Data: []*market.Level3Data{{
						Symbol: "BTC/USD",
						Bids: []*market.Level3Order{{
							OrderID: "b3",
							LimitPrice: decimalPtr("90"),
							OrderQty:   decimalPtr("5"),
							Timestamp:  time.Unix(3, 0),
						}},
						Asks: []*market.Level3Order{{
							OrderID: "a3",
							LimitPrice: decimalPtr("91"),
							OrderQty:   decimalPtr("5"),
							Timestamp:  time.Unix(3, 0),
						}},
					}},
				})

				markDown := decimal.RequireFromString("90")
				So(position.Update(&markDown), ShouldBeNil)
				So(position.Closing, ShouldBeTrue)
				So(triggered, ShouldBeTrue)
			})
		},
	))
}

func ptrDec(s string) *decimal.Decimal {
	d := decimal.RequireFromString(s)
	return &d
}

func decimalPtr(s string) *decimal.Decimal {
	d := decimal.RequireFromString(s)
	return &d
}

func TestApply(t *testing.T) {
	Convey("Setup", t, func() {
		cases := []struct {
			name    string
			seq     []market.ExecutionData
			wantQty string
			wantEnt string
			wantSol string
			wantVW  string
			flat    bool
		}{
			{
				name:    "single buy",
				seq:     []market.ExecutionData{tradeFill("b1", "buy", "1", "100")},
				wantQty: "1",
				wantEnt: "100",
			},
			{
				name: "scaled buys blend entry vwap",
				seq: []market.ExecutionData{
					tradeFill("b1", "buy", "10", "100"),
					tradeFill("b2", "buy", "5", "120"),
				},
				wantQty: "15",
				wantEnt: "106.666666666666666666",
			},
			{
				name: "partial sell keeps open qty",
				seq: []market.ExecutionData{
					tradeFill("b1", "buy", "3", "100"),
					tradeFill("s1", "sell", "1", "110"),
				},
				wantQty: "2",
				wantEnt: "100",
				wantSol: "1",
				wantVW:  "110",
			},
			{
				name: "oversell clips to open size then closes",
				seq: []market.ExecutionData{
					tradeFill("b1", "buy", "1", "100"),
					tradeFill("s1", "sell", "3", "90"),
				},
				wantQty: "0",
				wantEnt: "100",
				wantSol: "1",
				wantVW:  "90",
				flat:    true,
			},
			{
				name: "full exit flats",
				seq: []market.ExecutionData{
					tradeFill("b1", "buy", "2", "100"),
					tradeFill("s1", "sell", "2", "90"),
				},
				wantQty: "0",
				wantEnt: "100",
				wantSol: "2",
				wantVW:  "90",
				flat:    true,
			},
		}

		eps := decimal.RequireFromString("0.000000000000001")

		for _, tc := range cases {
			Convey("When "+tc.name, func() {
				live := replayFills("BTC/USD", tc.seq)
				again := replayFills("BTC/USD", tc.seq)

				lq, le, ls, lv := foldSnapshot(live)
				aq, ae, as, av := foldSnapshot(again)

				So(aq, ShouldEqual, lq)
				So(ae, ShouldEqual, le)
				So(as, ShouldEqual, ls)
				So(av, ShouldEqual, lv)
				So(len(again.Fills), ShouldEqual, len(live.Fills))
				assertLedgerConsistent(live)

				So(live.Qty.String(), ShouldEqual, tc.wantQty)

				if tc.wantEnt != "" {
					want := decimal.RequireFromString(tc.wantEnt)
					So(live.Entry.Sub(want).Abs().LessThanOrEqual(eps), ShouldBeTrue)
				}

				if tc.wantSol != "" {
					So(live.Sold.String(), ShouldEqual, tc.wantSol)
				}

				if tc.wantVW != "" {
					So(live.ExitVWAP.String(), ShouldEqual, tc.wantVW)
				}

				So(live.Flat(), ShouldEqual, tc.flat)

				if tc.flat {
					So(live.ClosedAt, ShouldNotBeNil)
				}
			})
		}

		Convey("When a fill has no usable price it must not land in Fills", func() {
			position := NewPosition(context.Background(), "BTC/USD", "real", "path", nil, nil, nil)

			q := decimal.RequireFromString("1")
			position.Apply(market.ExecutionData{
				ExecID:   "orphan",
				ExecType: "trade",
				Side:     "buy",
				LastQty:  &q,
			})

			So(position.Fills, ShouldHaveLength, 0)
			So(position.Qty, ShouldBeNil)
		})

		Convey("When closed every later fill is a no-op", func() {
			price := priceFor("BTC/USD", NewBookManager())
			position := NewPosition(context.Background(), "BTC/USD", "real", "path", price, nil, nil)

			position.Apply(tradeFill("b1", "buy", "1", "100"))
			position.Apply(tradeFill("s1", "sell", "1", "105"))

			So(position.ClosedAt, ShouldNotBeNil)

			beforeFills := len(position.Fills)
			beforePnL := position.PnL.String()

			position.Apply(tradeFill("s2", "sell", "1", "50"))
			position.Apply(tradeFill("b2", "buy", "1", "50"))

			So(len(position.Fills), ShouldEqual, beforeFills)
			So(position.PnL.String(), ShouldEqual, beforePnL)
		})

		Convey("When the same exec id arrives twice", func() {
			position := NewPosition(context.Background(), "BTC/USD", "real", "path", nil, nil, nil)

			fill := tradeFill("exec-1", "buy", "1", "100")
			fill.OrderStatus = "filled"
			fill.Symbol = "BTC/USD"

			position.Apply(fill)
			position.Apply(fill)

			So(position.Fills, ShouldHaveLength, 1)
			assertLedgerConsistent(position)
		})

		Convey("When ExecID is empty duplicate wire rows must not double qty", func() {
			position := NewPosition(context.Background(), "BTC/USD", "real", "path", nil, nil, nil)

			fill := tradeFill("", "buy", "1", "100")
			position.Apply(fill)
			position.Apply(fill)

			So(position.Fills, ShouldHaveLength, 1)
			So(position.Qty.String(), ShouldEqual, "1")
		})

		Convey("When fillWireDuplicate matches side qty price and cl_ord_id", func() {
			stored := &market.ExecutionData{
				Side: "buy", LastQty: ptrDec("1"), LastPrice: ptrDec("10"),
				ClientOrderID: "wire-a",
			}
			dup := &market.ExecutionData{
				Side: "buy", LastQty: ptrDec("1"), LastPrice: ptrDec("10"),
				ClientOrderID: "wire-a",
			}
			other := &market.ExecutionData{
				Side: "buy", LastQty: ptrDec("1"), LastPrice: ptrDec("11"),
				ClientOrderID: "wire-a",
			}

			So(fillWireDuplicate(stored, dup), ShouldBeTrue)
			So(fillWireDuplicate(stored, other), ShouldBeFalse)
		})

		Convey("When empty ExecID differs by client order id both rows count", func() {
			position := NewPosition(context.Background(), "BTC/USD", "real", "path", nil, nil, nil)

			first := tradeFill("", "buy", "1", "100")
			first.ClientOrderID = "wire-a"
			second := tradeFill("", "buy", "1", "100")
			second.ClientOrderID = "wire-b"

			position.Apply(first)
			position.Apply(second)

			So(position.Fills, ShouldHaveLength, 2)
			So(position.Qty.String(), ShouldEqual, "2")
			assertLedgerConsistent(position)
		})

		Convey("When CumQty on one row must not replace multi-buy open qty", func() {
			position := NewPosition(context.Background(), "BTC/USD", "real", "path", nil, nil, nil)

			position.Apply(tradeFill("b1", "buy", "10", "100"))
			cum := decimal.RequireFromString("5")
			f := tradeFill("b2", "buy", "5", "120")
			f.CumQty = &cum
			position.Apply(f)

			So(position.Qty.String(), ShouldEqual, "15")

			wantEntry := decimal.RequireFromString("106.666666666666666666")
			eps := decimal.RequireFromString("0.000000000000001")
			So(position.Entry.Sub(wantEntry).Abs().LessThanOrEqual(eps), ShouldBeTrue)
		})

		Convey("When a partial sell while Closing leaves qty and stale exit wire", func() {
			position := NewPosition(context.Background(), "BTC/USD", "real", "path", nil, nil, nil)

			position.Apply(tradeFill("b1", "buy", "1", "100"))
			position.Closing = true
			position.ExitOrder = &spot.OrderRequest{
				ClOrdId: "exit", OrderType: "market", Type: "sell", Volume: "1",
			}
			position.Apply(tradeFill("s1", "sell", "0.4", "99"))

			So(position.Qty.String(), ShouldEqual, "0.6")
			So(position.ExitWireStale, ShouldBeTrue)
			So(position.ExitOrder.Volume, ShouldEqual, "0.6")
		})

		Convey("When a full sell realizes PnL after Closing", func() {
			position := NewPosition(
				context.Background(), "BTC/USD", "real", "path",
				priceFor("BTC/USD", NewBookManager()), nil, nil,
			)

			position.Apply(tradeFill("b1", "buy", "2", "100"))
			position.Closing = true
			position.Apply(tradeFill("s1", "sell", "2", "90"))

			So(position.Flat(), ShouldBeTrue)
			So(position.ClosedAt, ShouldNotBeNil)
			So(position.ExitVWAP.String(), ShouldEqual, "90")
			So(position.PnL, ShouldNotBeNil)
			So(position.PnL.Cmp(decimal.RequireFromString("-20")), ShouldEqual, 0)
			So(position.PnLPct.Sign(), ShouldBeLessThan, 0)
			assertLedgerConsistent(position)
		})

		Convey("When a second sell against a flat position is dropped", func() {
			position := NewPosition(
				context.Background(), "BTC/USD", "virtual", "path",
				priceFor("BTC/USD", NewBookManager()), nil, nil,
			)

			position.Apply(tradeFill("b1", "buy", "237.81801", "0.1456"))
			position.Closing = true
			position.Apply(tradeFill("s1", "sell", "237.81801", "0.1495"))

			So(position.Flat(), ShouldBeTrue)
			So(position.ExitVWAP.String(), ShouldEqual, "0.1495")
			pnlBefore := *position.PnL

			position.Apply(tradeFill("s2", "sell", "237.81801", "0.1334"))

			So(position.Fills, ShouldHaveLength, 2)
			So(position.ExitVWAP.String(), ShouldEqual, "0.1495")
			So(position.PnL.Equal(pnlBefore), ShouldBeTrue)
			So(position.PnL.Sign(), ShouldBeGreaterThan, 0)
		})

		Convey("When a sell flats the position even if Closing was cleared", func() {
			settled := false
			position := NewPosition(
				context.Background(), "FIL/USD", "virtual", "path",
				priceFor("FIL/USD", NewBookManager()), nil,
				func(*Position) { settled = true },
			)

			position.Apply(tradeFill("b1", "buy", "1", "2"))
			So(position.Closing, ShouldBeFalse)
			position.Apply(tradeFill("s1", "sell", "1", "1.9"))

			So(position.Flat(), ShouldBeTrue)
			So(position.ClosedAt, ShouldNotBeNil)
			So(settled, ShouldBeTrue)
			assertLedgerConsistent(position)
		})

		Convey("When a sell exceeds open qty only open qty is folded", func() {
			position := NewPosition(context.Background(), "BTC/USD", "real", "path", nil, nil, nil)

			position.Apply(tradeFill("b1", "buy", "1", "100"))
			position.Apply(tradeFill("s1", "sell", "3", "90"))

			So(position.Qty.Sign(), ShouldBeLessThanOrEqualTo, 0)
			So(position.Sold.String(), ShouldEqual, "1")
			So(position.ExitVWAP.String(), ShouldEqual, "90")
		})

		Convey("When LastPrice is missing AvgPrice is used", func() {
			position := NewPosition(context.Background(), "BTC/USD", "real", "path", nil, nil, nil)

			q := decimal.RequireFromString("1")
			px := decimal.RequireFromString("42")
			position.Apply(market.ExecutionData{
				ExecID:   "b1",
				ExecType: "trade",
				Side:     "buy",
				LastQty:  &q,
				AvgPrice: &px,
			})

			So(position.Entry.String(), ShouldEqual, "42")
		})

		Convey("When Instrument is missing flat still realizes gross pnl", func() {
			position := NewPosition(
				context.Background(), "BTC/USD", "virtual", "path",
				&Price{Books: NewBookManager()}, nil, nil,
			)

			position.Apply(tradeFill("b1", "buy", "1", "100"))
			position.Apply(tradeFill("s1", "sell", "1", "110"))

			So(position.Flat(), ShouldBeTrue)
			So(position.PnL, ShouldNotBeNil)
			So(position.PnL.Cmp(decimal.RequireFromString("10")), ShouldEqual, 0)
		})
	})
}

func TestExit(t *testing.T) {
	Convey("Setup", t, func() {
		position := NewPosition(context.Background(), "BTC/USD", "real", "path", nil, nil, nil)
		position.ExitPath = "exit/path"

		Convey("When already Closing Exit is a no-op", func() {
			position.Closing = true
			trigger := position.ExitTrigger

			So(position.Exit(), ShouldBeNil)
			So(position.ExitTrigger, ShouldEqual, trigger)
		})

		Convey("When ExitTrigger is preset it wins over policy text", func() {
			position.ExitTrigger = "stoploss"
			So(position.Exit(), ShouldBeNil)
			So(position.Closing, ShouldBeTrue)
			So(position.ExitTrigger, ShouldEqual, "stoploss")
		})
	})
}

func TestEnter(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When path is empty Enter rejects", func() {
			position := NewPosition(
				context.Background(), "BTC/USD", "real", "path",
				&Price{}, nil, nil,
			)

			So(position.Enter("", nil, nil), ShouldNotBeNil)
		})

		Convey("When qty and path are valid Enter records market orders", func() {
			books := bookWithTouch("BTC/USD", "100", "101", "10")
			price := priceFor("BTC/USD", books)
			position := NewPosition(
				context.Background(), "BTC/USD", "virtual", "path", price, nil, nil,
			)

			qty := decimal.RequireFromString("1")
			wallet := market.NewBalance(map[string]any{
				"quote": "USD",
				"assets": []*market.Asset{
					market.NewAsset(nil, "USD", decimal.NewFromInt(10_000), decimal.Zero, decimal.NewFromInt(10_000)),
				},
			})

			So(position.Enter("R00/R01", &qty, wallet), ShouldBeNil)
			So(position.EntryOrder, ShouldNotBeNil)
			So(position.ExitOrder, ShouldNotBeNil)
			So(position.EntryOrder.Volume, ShouldEqual, "1")
			So(position.OpenedAt.IsZero(), ShouldBeFalse)
		})
	})
}

func TestEnsureExitOrder(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When a partial exit fill runs under Closing", func() {
			cases := []struct {
				name        string
				seed        func(*Position)
				sellQty     string
				wantVolume  string
				wantClOrdID string
				remint      bool
			}{
				{
					name: "exit volume tracks open Qty not stale enter size",
					seed: func(position *Position) {
						position.Closing = true
						position.ExitOrder = &spot.OrderRequest{
							ClOrdId:   "old",
							OrderType: "market",
							Type:      "sell",
							Volume:    "1.0",
						}
					},
					sellQty:     "0.5",
					wantVolume:  "0.5",
					wantClOrdID: "old",
				},
				{
					name: "blank ClOrdId is reminted",
					seed: func(position *Position) {
						position.Closing = true
						position.ExitOrder = &spot.OrderRequest{
							OrderType: "market",
							Type:      "sell",
							Volume:    "1",
						}
					},
					sellQty:    "0.5",
					wantVolume: "0.5",
					remint:     true,
				},
				{
					name: "missing ExitOrder is created",
					seed: func(position *Position) {
						position.Apply(tradeFill("b2", "buy", "1", "100"))
						position.Closing = true
						position.ExitOrder = nil
					},
					sellQty:    "1",
					wantVolume: "1",
					remint:     true,
				},
			}

			for _, tc := range cases {
				Convey("And "+tc.name, func() {
					position := NewPosition(
						context.Background(), "BTC/USD", "real", "path", nil, nil, nil,
					)
					position.Apply(tradeFill("b1", "buy", "1", "100"))
					tc.seed(position)
					position.Apply(tradeFill("s1", "sell", tc.sellQty, "100"))

					So(position.ExitOrder, ShouldNotBeNil)
					So(position.ExitOrder.Volume, ShouldEqual, tc.wantVolume)

					if tc.remint {
						So(position.ExitOrder.ClOrdId, ShouldNotBeEmpty)
					} else {
						So(position.ExitOrder.ClOrdId, ShouldEqual, tc.wantClOrdID)
					}
				})
			}
		})
	})
}

func TestOwns(t *testing.T) {
	Convey("Setup", t, func() {
		position := NewPosition(context.Background(), "BTC/USD", "real", "path", nil, nil, nil)
		position.EntryOrder = &spot.OrderRequest{ClOrdId: "cl-entry"}
		position.ExitOrder = &spot.OrderRequest{ClOrdId: "cl-exit"}
		position.EntryVenueOrderID = "venue-entry-1"
		position.ExitVenueOrderID = "venue-exit-1"
		position.Fills = []*market.ExecutionData{{OrderID: "prior-venue"}}

		Convey("When ClientOrderID or OrderID matches entry, exit, or prior fills", func() {
			So(position.Owns(market.ExecutionData{ClientOrderID: "cl-entry"}), ShouldBeTrue)
			So(position.Owns(market.ExecutionData{ClientOrderID: "cl-exit"}), ShouldBeTrue)
			So(position.Owns(market.ExecutionData{OrderID: "venue-entry-1"}), ShouldBeTrue)
			So(position.Owns(market.ExecutionData{OrderID: "venue-exit-1"}), ShouldBeTrue)
			So(position.Owns(market.ExecutionData{OrderID: "prior-venue"}), ShouldBeTrue)
		})

		Convey("When identifiers are missing or foreign", func() {
			So(position.Owns(market.ExecutionData{ExecType: "trade"}), ShouldBeFalse)
			So(position.Owns(market.ExecutionData{OrderID: "other"}), ShouldBeFalse)
			So(position.Owns(market.ExecutionData{ClientOrderID: "stranger"}), ShouldBeFalse)
		})
	})
}

func TestPositionUpdateDuringApply(t *testing.T) {
	Convey("Apply during Update keeps ledger and marks consistent", t, func() {
		fixture := newPriceFixture(t, priceFixture{
			symbol: "RC/USD", cash: 5000,
			qtyMinRaw: "0.01", costMinRaw: "1", step: "0.01",
			bidPx: 100, bidQty: 50, askPx: 101, askQty: 50, feePct: 0.26,
		})

		position := NewPosition(context.Background(), fixture.symbol, "virtual", "path", fixture.price, nil, nil)
		position.Apply(tradeFill("b1", "buy", "2", "100"))

		mark := decimal.NewFromInt(101)
		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()

			for range 100 {
				_ = position.Update(&mark)
			}
		}()

		go func() {
			defer wg.Done()

			for i := range 100 {
				position.Apply(tradeFill("fill-"+strconv.Itoa(i), "buy", "0.1", "101"))
			}
		}()

		wg.Wait()

		assertLedgerConsistent(position)

		wantQty := decimal.RequireFromString("12")
		So(position.Qty.Cmp(wantQty), ShouldEqual, 0)
		So(len(position.Fills), ShouldEqual, 101)
		So(position.Entry.String(), ShouldStartWith, "100.8333333333")
		So(position.PnL, ShouldNotBeNil)
	})
}
