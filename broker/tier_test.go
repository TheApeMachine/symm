package broker

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/venue"
	"github.com/theapemachine/symm/venue/kraken"
)

type captureClient struct {
	mu     sync.Mutex
	writes [][]byte
}

func (c *captureClient) Read(...string) ([]byte, error) { return nil, nil }
func (c *captureClient) Write(msg []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.writes = append(c.writes, append([]byte(nil), msg...))
	return nil
}
func (c *captureClient) Post(string, any) ([]byte, error) {
	return []byte(`{"error":[],"result":{"trades":{}}}`), nil
}
func (c *captureClient) Watch(string, any, json.Unmarshaler, func()) {}

func withTier(ctx context.Context, tt TierType, f func(*Tier)) func() {
	return func() {
		withPrice(func(price *Price) {
			private := &historyClient{trades: []byte(`{"error":[],"result":{"trades":{}}}`)}
			tier := NewTier(
				ctx,
				tt,
				nil,
				private,
				price,
				nil,
				nil,
			)
			if price.balance != nil {
				price.balance.ToMarket(tier.balance)
			}
			f(tier)
		})()
	}
}

func seedMarkable(pos *Position) {
	if pos == nil {
		return
	}

	if pos.EntryOrder == nil {
		pos.EntryOrder = &spot.OrderRequest{ClOrdId: "enter-1", Type: "buy", OrderType: "market"}
	}

	if pos.EntryVenueOrderID == "" {
		pos.EntryVenueOrderID = "OID-1"
	}

	if pos.onSettled == nil {
		pos.onSettled = func(*Position) {}
	}
}

func TestTierTypeString(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When each tier constant is stringified", func() {
			So(TierTypeVirtual.String(), ShouldEqual, "virtual")
			So(TierTypePaper.String(), ShouldEqual, "paper")
			So(TierTypeReal.String(), ShouldEqual, "real")
		})

		Convey("When the tier value is invalid", func() {
			So(TierType(99).String(), ShouldEqual, "")
		})
	})
}

func TestNewTier(t *testing.T) {
	Convey("Setup", t, withTier(t.Context(), TierTypePaper, func(tier *Tier) {
		Convey("When a paper tier is constructed", func() {
			So(tier, ShouldNotBeNil)
			So(tier.Type, ShouldEqual, TierTypePaper)
			So(tier.positions, ShouldNotBeNil)
			So(tier.balance, ShouldNotBeNil)
		})
	}))
}

func TestNewTierExecutionsSubscribe(t *testing.T) {
	Convey("Setup", t, func() {
		withPrice(func(price *Price) {
			cap := &captureClient{}
			price.Instrument.auth = kraken.NewAuth()
			price.Instrument.auth.SeedToken("tok")
			host := venue.NewKraken("USD")

			prevModel := system.Cfg.Market.Model
			system.Cfg.Market.Model = "real"
			tier := NewTier(t.Context(), TierTypeReal, nil, cap, price, host, nil)
			system.Cfg.Market.Model = prevModel
			So(tier, ShouldNotBeNil)
			So(tier.Status(), ShouldEqual, runtime.READY)

			var found bool

			for _, raw := range cap.writes {
				var msg map[string]any
				So(sonic.Unmarshal(raw, &msg), ShouldBeNil)

				if msg["method"] != "subscribe" {
					continue
				}

				params, _ := msg["params"].(map[string]any)

				if params["channel"] != "executions" {
					continue
				}

				So(params["token"], ShouldEqual, "tok")
				So(params["snap_orders"], ShouldEqual, true)
				So(params["snap_trades"], ShouldEqual, false)
				found = true
			}

			Convey("Real tier subscribes to executions on the private socket", func() {
				So(found, ShouldBeTrue)
			})
		})()
	})
}

func TestTierHello(t *testing.T) {
	Convey("Setup", t, withTier(t.Context(), TierTypePaper, func(tier *Tier) {
		pos := NewPosition(tier.ctx, testSymbol, "paper", "path", tier.price, nil, nil)
		seedMarkable(pos)
		pos.Apply(market.ExecutionData{
			ExecID: "b1", Side: "buy",
			LastQty: dec("1"), LastPrice: dec("100"),
			Timestamp: time.Unix(1, 0).UTC(),
		})
		tier.positions.Add(testSymbol, pos, true)

		Convey("When Hello refreshes open marks", func() {
			tier.Update(testSymbol, dec("100"))
			tier.Hello()

			Convey("Unrealized PnL matches mark 100 at 0.40% taker", func() {
				So(tier.balance.Unrealized, ShouldNotBeNil)
				So(tier.balance.Unrealized.String(), ShouldEqual, "-0.8")
				So(pos.PnL, ShouldNotBeNil)
				So(pos.PnL.String(), ShouldEqual, "-0.8")
			})
		})
	}))
}

func TestTierUpdate(t *testing.T) {
	Convey("Setup", t, withTier(t.Context(), TierTypePaper, func(tier *Tier) {
		pos := NewPosition(tier.ctx, testSymbol, "paper", "path", tier.price, nil, nil)
		seedMarkable(pos)
		pos.Apply(market.ExecutionData{
			ExecID: "b1", Side: "buy",
			LastQty: dec("1"), LastPrice: dec("100"),
			Timestamp: time.Unix(1, 0).UTC(),
		})
		tier.positions.Add(testSymbol, pos, true)

		Convey("When Update carries a new bid mark", func() {
			tier.price.Books.Update(level3Snapshot(testSymbol, "120", "121"))
			tier.Update(testSymbol, dec("120"))

			Convey("Position PnL matches fee-net 100 → 120 via Hello snapshot", func() {
				So(pos.Mark.String(), ShouldEqual, "120")
				So(pos.PnL.String(), ShouldEqual, "19.12")
				So(pos.PnLPct.String(), ShouldEqual, "19.12")
			})
		})
	}))
}

func TestTierEnter(t *testing.T) {
	Convey("Setup", t, withTier(t.Context(), TierTypePaper, func(tier *Tier) {
		Convey("When Enter opens a new position", func() {
			err := tier.Enter(testSymbol, "R00/M1", true)

			Convey("Orders arm and cache holds the symbol", func() {
				So(err, ShouldBeNil)
				So(tier.positions.Has(testSymbol), ShouldBeTrue)
				open := tier.positions.Open(testSymbol)
				So(len(open), ShouldEqual, 1)
				So(open[0].EntryPath, ShouldEqual, "R00/M1")
				So(open[0].EntryOrder, ShouldNotBeNil)
			})
		})

		Convey("When Enter hits an existing symbol", func() {
			So(tier.Enter(testSymbol, "R00/M1", true), ShouldBeNil)
			err := tier.Enter(testSymbol, "R00/M2", true)

			Convey("It returns too-many-requests", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "[broker.Tier.Enter] position already exists")
			})
		})

		Convey("When Enter cannot size", func() {
			tier.balance = market.NewBalance(map[string]any{
				"quote":  "USD",
				"assets": []*market.Asset{},
			})
			err := tier.Enter(testSymbol, "R00/M1", true)

			Convey("It returns the allocate failure", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "[price] allocate failed")
			})
		})
	}))
}

func TestTierExit(t *testing.T) {
	Convey("Setup", t, withTier(t.Context(), TierTypePaper, func(tier *Tier) {
		pos := NewPosition(tier.ctx, testSymbol, "paper", "path", tier.price, nil, nil)
		seedMarkable(pos)
		pos.ExitPath = "R00/M1"
		pos.Apply(market.ExecutionData{
			ExecID: "b1", Side: "buy",
			LastQty: dec("1"), LastPrice: dec("100"),
			Timestamp: time.Unix(1, 0).UTC(),
		})
		tier.positions.Add(testSymbol, pos, true)

		Convey("When Exit is called on a READY row", func() {
			setBook(tier.price, "120", "121", "30")
			tier.Update(testSymbol, dec("120"))
			err := tier.Exit(pos, false)

			Convey("Closing is armed", func() {
				So(err, ShouldBeNil)
				So(pos.Status() == runtime.BUSY, ShouldBeTrue)
				So(pos.ExitOrder, ShouldNotBeNil)
				So(pos.ExitTrigger, ShouldEqual, "policy exit match R00/M1")
			})
		})

		Convey("When Exit would sell into a fee-net loss", func() {
			setBook(tier.price, "120", "121", "30")
			tier.Update(testSymbol, dec("120"))
			setBook(tier.price, "100", "101", "30")
			err := tier.Exit(pos, false)

			Convey("It aborts and holds READY", func() {
				So(errnie.IsForbidden(err), ShouldBeTrue)
				So(pos.Status(), ShouldEqual, runtime.READY)
				So(pos.ExitOrder, ShouldBeNil)
			})
		})

		Convey("When Exit hits a book that cannot absorb Qty", func() {
			setBook(tier.price, "120", "121", "30")
			tier.Update(testSymbol, dec("120"))
			setBook(tier.price, "120", "121", "0.1")
			err := tier.Exit(pos, false)

			Convey("It returns ErrExitAborted", func() {
				So(errnie.IsForbidden(err), ShouldBeTrue)
				So(pos.Status(), ShouldEqual, runtime.READY)
			})
		})

	}))
}

func TestTierForceExit(t *testing.T) {
	Convey("Setup", t, withTier(t.Context(), TierTypePaper, func(tier *Tier) {
		pos := NewPosition(tier.ctx, testSymbol, "paper", "path", tier.price, nil, nil)
		pos.Apply(market.ExecutionData{
			ExecID: "b1", Side: "buy",
			LastQty: dec("1"), LastPrice: dec("100"),
			Timestamp: time.Unix(1, 0).UTC(),
		})
		tier.positions.Add(testSymbol, pos, true)

		Convey("When ForceExit dumps an underwater row", func() {
			setBook(tier.price, "100", "101", "30")
			err := tier.Exit(pos, true)

			Convey("Manual exit arms BUSY past the profit floor", func() {
				So(err, ShouldBeNil)
				So(pos.Status() == runtime.BUSY, ShouldBeTrue)
				So(pos.ExitOrder, ShouldNotBeNil)
				So(pos.ExitTrigger, ShouldEqual, "manual")
				So(len(tier.positions.Open()), ShouldEqual, 1)
			})
		})

		Convey("When Hello runs while the row is BUSY", func() {
			setBook(tier.price, "100", "101", "30")
			So(tier.Exit(pos, true), ShouldBeNil)
			So(pos.Status(), ShouldEqual, runtime.BUSY)

			tier.Hello()

			Convey("Desk cache keeps BUSY; Hello still ships it while venue-open", func() {
				So(len(tier.positions.Open()), ShouldEqual, 1)
				So(tier.positions.Open()[0].Status(), ShouldEqual, runtime.BUSY)
			})
		})
	}))
}

func TestTierPending(t *testing.T) {
	Convey("Setup", t, withTier(t.Context(), TierTypePaper, func(tier *Tier) {
		pos := NewPosition(tier.ctx, testSymbol, "paper", "path", tier.price, nil, nil)
		tier.pending.Store(testSymbol, pos)

		Convey("When Pending walks the map", func() {
			var seen []*Position
			for p := range tier.Pending() {
				seen = append(seen, p)
			}

			Convey("The pending handle is visible", func() {
				So(len(seen), ShouldEqual, 1)
				So(seen[0].ID, ShouldEqual, pos.ID)
			})
		})
	}))
}

func TestTierClearPending(t *testing.T) {
	Convey("Setup", t, withTier(t.Context(), TierTypePaper, func(tier *Tier) {
		pos := NewPosition(tier.ctx, testSymbol, "paper", "path", tier.price, nil, nil)
		tier.pending.Store(testSymbol, pos)

		Convey("When ClearPending runs", func() {
			tier.ClearPending(testSymbol)

			Convey("Pending is empty", func() {
				n := 0
				for range tier.Pending() {
					n++
				}
				So(n, ShouldEqual, 0)
			})
		})
	}))
}

func TestTierSend(t *testing.T) {
	Convey("Setup", t, withTier(t.Context(), TierTypePaper, func(tier *Tier) {
		cap := &captureClient{}
		tier.private = cap
		pos := NewPosition(tier.ctx, testSymbol, "paper", "path", tier.price, nil, nil)
		pos.EntryOrder = &spot.OrderRequest{
			ClOrdId: "enter-1", OrderType: "market", Type: "buy", Volume: "1",
		}

		Convey("When Send wires an enter", func() {
			err := tier.Send(pos, "enter")

			Convey("add_order lands on the private client", func() {
				So(err, ShouldBeNil)
				So(len(cap.writes), ShouldEqual, 1)

				var msg map[string]any
				So(sonic.Unmarshal(cap.writes[0], &msg), ShouldBeNil)
				So(msg["method"], ShouldEqual, "add_order")
				params := msg["params"].(map[string]any)
				So(params["side"], ShouldEqual, "buy")
				So(params["cl_ord_id"], ShouldEqual, "enter-1")
				So(params["order_qty"], ShouldAlmostEqual, 1)
			})
		})

		Convey("When order fields are missing", func() {
			pos.EntryOrder = nil
			err := tier.Send(pos, "enter")

			Convey("Send rejects incomplete enter", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "exit/enter order is incomplete")
				So(len(cap.writes), ShouldEqual, 0)
			})
		})
	}))
}

func TestTierApply(t *testing.T) {
	Convey("Setup", t, withTier(t.Context(), TierTypePaper, func(tier *Tier) {
		pos := NewPosition(tier.ctx, testSymbol, "paper", "path", tier.price, nil, nil)
		pos.EntryOrder = &spot.OrderRequest{ClOrdId: "enter-1"}
		tier.positions.Add(testSymbol, pos, true)

		Convey("When a fill arrives for an unknown symbol", func() {
			So(func() {
				tier.apply(&market.Execution{
					Data: []*market.ExecutionData{{
						ExecID: "x1", Side: "buy", Symbol: "MISSING/USD",
						LastQty: dec("1"), LastPrice: dec("1"),
						Timestamp: time.Unix(1, 0).UTC(),
					}},
				})
			}, ShouldNotPanic)
		})

		Convey("When a matching buy fill arrives", func() {
			tier.apply(&market.Execution{
				Data: []*market.ExecutionData{{
					ExecID: "b1", Side: "buy", Symbol: testSymbol,
					ClientOrderID: "enter-1",
					LastQty:       dec("1"), LastPrice: dec("100"),
					Timestamp: time.Unix(1, 0).UTC(),
				}},
			})

			Convey("Qty and Entry stamp; row waits for the book", func() {
				So(pos.Qty.String(), ShouldEqual, "1")
				So(pos.Entry.String(), ShouldEqual, "100")
				So(pos.Status(), ShouldEqual, runtime.WAITING)
			})
		})

		Convey("When a flattening sell arrives", func() {
			pos.Apply(market.ExecutionData{
				ExecID: "b1", Side: "buy",
				LastQty: dec("1"), LastPrice: dec("100"),
				Timestamp: time.Unix(1, 0).UTC(),
			})
			pos.ExitOrder = &spot.OrderRequest{ClOrdId: "exit-1"}

			tier.apply(&market.Execution{
				Data: []*market.ExecutionData{{
					ExecID: "s1", Side: "sell", Symbol: testSymbol,
					ClientOrderID: "exit-1",
					LastQty:       dec("1"), LastPrice: dec("120"),
					Timestamp: time.Unix(2, 0).UTC(),
				}},
			})

			Convey("Open moves to pending", func() {
				So(tier.positions.Has(testSymbol), ShouldBeFalse)
				n := 0
				for p := range tier.Pending() {
					n++
					So(p.ID, ShouldEqual, pos.ID)
				}
				So(n, ShouldEqual, 1)
				So(pos.ClosedAt, ShouldNotBeNil)
			})
		})

		Convey("When an exit cancel arrives with remaining qty", func() {
			pos.Apply(market.ExecutionData{
				ExecID: "b1", Side: "buy",
				LastQty: dec("1"), LastPrice: dec("100"),
				Timestamp: time.Unix(1, 0).UTC(),
			})
			So(pos.exit(false), ShouldBeNil)
			cl := pos.ExitOrder.ClOrdId

			tier.apply(&market.Execution{
				Data: []*market.ExecutionData{{
					ExecID: "c1", ExecType: "canceled", Side: "sell", Symbol: testSymbol,
					ClientOrderID: cl,
					LastQty:       dec("0"), LastPrice: dec("120"),
					Timestamp: time.Unix(2, 0).UTC(),
				}},
			})

			Convey("Position returns to READY", func() {
				So(pos.Status(), ShouldEqual, runtime.READY)
				So(pos.ExitOrder, ShouldBeNil)
			})
		})

	}))
}

func TestTierSettle(t *testing.T) {
	Convey("Setup", t, withTier(t.Context(), TierTypePaper, func(tier *Tier) {
		pos := NewPosition(tier.ctx, testSymbol, "paper", "path", tier.price, nil, nil)
		pos.Apply(market.ExecutionData{
			ExecID: "b1", Side: "buy",
			LastQty: dec("1"), LastPrice: dec("100"),
			Timestamp: time.Unix(1, 0).UTC(),
		})
		tier.positions.Add(testSymbol, pos, true)

		Convey("When settle parks a stoploss trigger", func() {
			tier.settle(pos)

			Convey("Pending holds the open row", func() {
				So(tier.positions.Has(testSymbol), ShouldBeTrue)
				n := 0
				for p := range tier.Pending() {
					n++
					So(p.ID, ShouldEqual, pos.ID)
				}
				So(n, ShouldEqual, 1)
			})
		})
	}))
}

func TestTierAckOrder(t *testing.T) {
	Convey("Setup", t, withTier(t.Context(), TierTypePaper, func(tier *Tier) {
		pos := NewPosition(tier.ctx, testSymbol, "paper", "path", tier.price, nil, nil)
		pos.EntryOrder = &spot.OrderRequest{ClOrdId: "enter-1"}
		tier.positions.Add(testSymbol, pos, true)

		Convey("When add_order succeeds for the entry ClOrdId", func() {
			tier.ackOrder(&kraken.Order{
				Success: true,
				Result:  kraken.OrderResult{ClOrdID: "enter-1", OrderID: "OID-1"},
			})

			Convey("EntryVenueOrderID is stamped", func() {
				So(pos.EntryVenueOrderID, ShouldEqual, "OID-1")
			})
		})
	}))
}

func TestTierRejectOrder(t *testing.T) {
	Convey("Setup", t, withTier(t.Context(), TierTypePaper, func(tier *Tier) {
		Convey("When an unfilled enter is rejected", func() {
			pos := NewPosition(tier.ctx, testSymbol, "paper", "path", tier.price, nil, nil)
			pos.EntryOrder = &spot.OrderRequest{ClOrdId: "enter-1"}
			tier.positions.Add(testSymbol, pos, true)

			tier.rejectOrder(&kraken.Order{
				Success: false,
				Error:   "Insufficient funds",
				Result:  kraken.OrderResult{ClOrdID: "enter-1"},
			})

			Convey("The open row is dropped", func() {
				So(tier.positions.Has(testSymbol), ShouldBeFalse)
			})
		})

		Convey("When an exit add_order is rejected", func() {
			pos := NewPosition(tier.ctx, testSymbol, "paper", "path", tier.price, nil, nil)
			pos.Apply(market.ExecutionData{
				ExecID: "b1", Side: "buy",
				LastQty: dec("1"), LastPrice: dec("100"),
				Timestamp: time.Unix(1, 0).UTC(),
			})
			So(pos.exit(false), ShouldBeNil)
			cl := pos.ExitOrder.ClOrdId
			tier.positions.Add(testSymbol, pos, true)

			tier.rejectOrder(&kraken.Order{
				Success: false,
				Error:   "Order rejected",
				Result:  kraken.OrderResult{ClOrdID: cl},
			})

			Convey("AbortExit returns the row to READY", func() {
				So(pos.Status(), ShouldEqual, runtime.READY)
				So(pos.ExitOrder, ShouldBeNil)
				So(tier.positions.Has(testSymbol), ShouldBeTrue)
			})
		})
	}))
}

func TestTierIngest(t *testing.T) {
	Convey("Setup", t, withTier(t.Context(), TierTypePaper, func(tier *Tier) {
		pos := NewPosition(tier.ctx, testSymbol, "paper", "path", tier.price, nil, nil)
		pos.EntryOrder = &spot.OrderRequest{ClOrdId: "enter-1"}
		tier.positions.Add(testSymbol, pos, true)

		Convey("When ingest gets a balance frame", func() {
			cash := decimal.RequireFromString("42")
			bal := kraken.NewBalance()
			bal.Result = map[string]*decimal.Decimal{"USD": &cash}
			tier.ingest(bal)

			Convey("Tier balance receives quote cash", func() {
				So(tier.balance.Cash("USD").String(), ShouldEqual, "42")
			})
		})

		Convey("When ingest gets a trade balance frame", func() {
			tb := kraken.NewTradeBalance()
			So(tb.UnmarshalJSON([]byte(`{"eb":"100","tb":"90","m":"0","n":"0","c":"0","v":"0","e":"0","mf":"0"}`)), ShouldBeNil)
			tier.ingest(tb)

			Convey("Hello path runs without panic", func() {
				So(tier.balance, ShouldNotBeNil)
			})
		})

		Convey("When ingest gets an execution fill", func() {
			qty := decimal.RequireFromString("1")
			px := decimal.RequireFromString("100")
			exec := &kraken.Execution{
				Data: []kraken.ExecutionData{{
					ExecID: "b1", Symbol: testSymbol, Side: "buy",
					ExecType:      "trade",
					ClientOrderID: "enter-1",
					LastQty:       &qty,
					LastPrice:     &px,
					Timestamp:     time.Unix(1, 0).UTC(),
				}},
			}
			tier.ingest(exec)

			Convey("Position records the fill", func() {
				So(pos.Qty.String(), ShouldEqual, "1")
			})
		})

		Convey("When ingest gets a successful order ack", func() {
			tier.ingest(&kraken.Order{
				Success: true,
				Result:  kraken.OrderResult{ClOrdID: "enter-1", OrderID: "OID-9"},
			})

			Convey("EntryVenueOrderID stamps", func() {
				So(pos.EntryVenueOrderID, ShouldEqual, "OID-9")
			})
		})

		Convey("When ingest gets a rejected enter ack", func() {
			tier.ingest(&kraken.Order{
				Success: false,
				Error:   "Insufficient funds",
				Result:  kraken.OrderResult{ClOrdID: "enter-1"},
			})

			Convey("Unfilled enter is dropped", func() {
				So(tier.positions.Has(testSymbol), ShouldBeFalse)
			})
		})

		Convey("When ingest gets nil", func() {
			So(func() { tier.ingest(nil) }, ShouldNotPanic)
		})
	}))
}

func TestTierRun(t *testing.T) {
	Convey("Setup", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		withTier(ctx, TierTypePaper, func(tier *Tier) {
			tier.private = &blockUntilDoneClient{done: ctx.Done()}

			Convey("When Run reads until context cancel", func() {
				done := make(chan struct{})
				go func() {
					tier.Run()
					close(done)
				}()
				cancel()

				select {
				case <-done:
				case <-time.After(2 * time.Second):
					So("tier.Run", ShouldEqual, "timed out")
				}
			})
		})()
	})
}

func TestTierSendExitReal(t *testing.T) {
	Convey("Setup", t, withTier(t.Context(), TierTypeReal, func(tier *Tier) {
		cap := &captureClient{}
		tier.private = cap
		if tier.price.Instrument != nil {
			tier.price.Instrument.auth = kraken.NewAuth()
			tier.price.Instrument.auth.SeedToken("tok")
		}

		pos := NewPosition(tier.ctx, testSymbol, "real", "path", tier.price, nil, nil)
		pos.Apply(market.ExecutionData{
			ExecID: "b1", Side: "buy",
			LastQty: dec("1"), LastPrice: dec("100"),
			Timestamp: time.Unix(1, 0).UTC(),
		})
		So(pos.exit(false), ShouldBeNil)

		Convey("When Send wires a real exit", func() {
			err := tier.Send(pos, "exit")

			Convey("Market sell is written with auth token", func() {
				So(err, ShouldBeNil)
				So(len(cap.writes), ShouldEqual, 1)
				var msg map[string]any
				So(sonic.Unmarshal(cap.writes[0], &msg), ShouldBeNil)
				params := msg["params"].(map[string]any)
				So(params["order_type"], ShouldEqual, "market")
				So(params["side"], ShouldEqual, "sell")
				So(params["token"], ShouldEqual, "tok")
			})
		})
	}))
}

func TestTierClearPendingEmpty(t *testing.T) {
	Convey("Setup", t, withTier(t.Context(), TierTypePaper, func(tier *Tier) {
		Convey("When ClearPending gets an empty symbol", func() {
			So(func() { tier.ClearPending("") }, ShouldNotPanic)
		})
	}))
}

func TestTierAckExitOrder(t *testing.T) {
	Convey("Setup", t, withTier(t.Context(), TierTypePaper, func(tier *Tier) {
		pos := NewPosition(tier.ctx, testSymbol, "paper", "path", tier.price, nil, nil)
		pos.ExitOrder = &spot.OrderRequest{ClOrdId: "exit-1"}
		tier.positions.Add(testSymbol, pos, true)

		Convey("When ackOrder matches the exit ClOrdId", func() {
			tier.ackOrder(&kraken.Order{
				Success: true,
				Result:  kraken.OrderResult{ClOrdID: "exit-1", OrderID: "OID-X"},
			})

			Convey("ExitVenueOrderID stamps", func() {
				So(pos.ExitVenueOrderID, ShouldEqual, "OID-X")
			})
		})
	}))
}

func TestTierFoldEntryCancel(t *testing.T) {
	Convey("Setup", t, withTier(t.Context(), TierTypePaper, func(tier *Tier) {
		pos := NewPosition(tier.ctx, testSymbol, "paper", "path", tier.price, nil, nil)
		pos.EntryOrder = &spot.OrderRequest{ClOrdId: "enter-1"}
		tier.positions.Add(testSymbol, pos, true)

		Convey("When an unfilled enter is canceled", func() {
			tier.apply(&market.Execution{
				Data: []*market.ExecutionData{{
					ExecID: "c1", ExecType: "canceled", Side: "buy", Symbol: testSymbol,
					ClientOrderID: "enter-1",
					LastQty:       dec("0"), LastPrice: dec("100"),
					Timestamp: time.Unix(1, 0).UTC(),
				}},
			})

			Convey("The open row is dropped", func() {
				So(tier.positions.Has(testSymbol), ShouldBeFalse)
			})
		})
	}))
}

func BenchmarkNewTier(b *testing.B) {
	ctx := context.Background()

	b.ReportAllocs()

	for b.Loop() {
		withTier(ctx, TierTypePaper, func(tier *Tier) {
			_ = tier
		})()
	}
}

func BenchmarkTierHello(b *testing.B) {
	ctx := context.Background()
	var tier *Tier
	withTier(ctx, TierTypePaper, func(t *Tier) {
		tier = t
		pos := NewPosition(ctx, testSymbol, "paper", "path", t.price, nil, nil)
		pos.Apply(market.ExecutionData{
			ExecID: "b1", Side: "buy",
			LastQty: dec("1"), LastPrice: dec("100"),
			Timestamp: time.Unix(1, 0).UTC(),
		})
		t.positions.Add(testSymbol, pos, true)
	})()

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		tier.Hello()
	}
}

func BenchmarkTierUpdate(b *testing.B) {
	ctx := context.Background()
	var tier *Tier
	withTier(ctx, TierTypePaper, func(t *Tier) {
		tier = t
		pos := NewPosition(ctx, testSymbol, "paper", "path", t.price, nil, nil)
		pos.Apply(market.ExecutionData{
			ExecID: "b1", Side: "buy",
			LastQty: dec("1"), LastPrice: dec("100"),
			Timestamp: time.Unix(1, 0).UTC(),
		})
		t.positions.Add(testSymbol, pos, true)
	})()
	mark := dec("120")

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		tier.Update(testSymbol, mark)
	}
}

func BenchmarkTierEnter(b *testing.B) {
	ctx := context.Background()

	b.ReportAllocs()

	for b.Loop() {
		withTier(ctx, TierTypePaper, func(tier *Tier) {
			_ = tier.Enter(testSymbol, "R00/M1", true)
		})()
	}
}

func BenchmarkTierExit(b *testing.B) {
	ctx := context.Background()

	b.ReportAllocs()

	for b.Loop() {
		withTier(ctx, TierTypePaper, func(tier *Tier) {
			pos := NewPosition(ctx, testSymbol, "paper", "path", tier.price, nil, nil)
			pos.ExitPath = "R00/M1"
			pos.Apply(market.ExecutionData{
				ExecID: "b1", Side: "buy",
				LastQty: dec("1"), LastPrice: dec("100"),
				Timestamp: time.Unix(1, 0).UTC(),
			})
			tier.positions.Add(testSymbol, pos, true)
			setBook(tier.price, "120", "121", "30")
			_ = tier.Exit(pos, false)
		})()
	}
}

func BenchmarkTierPending(b *testing.B) {
	ctx := context.Background()
	var tier *Tier
	withTier(ctx, TierTypePaper, func(t *Tier) {
		tier = t
		pos := NewPosition(ctx, testSymbol, "paper", "path", t.price, nil, nil)
		t.pending.Store(testSymbol, pos)
	})()

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		for range tier.Pending() {
		}
	}
}

func BenchmarkTierClearPending(b *testing.B) {
	ctx := context.Background()

	b.ReportAllocs()

	for b.Loop() {
		withTier(ctx, TierTypePaper, func(tier *Tier) {
			pos := NewPosition(ctx, testSymbol, "paper", "path", tier.price, nil, nil)
			tier.pending.Store(testSymbol, pos)
			tier.ClearPending(testSymbol)
		})()
	}
}

func BenchmarkTierSend(b *testing.B) {
	ctx := context.Background()
	var tier *Tier
	var pos *Position
	withTier(ctx, TierTypePaper, func(t *Tier) {
		tier = t
		cap := &captureClient{}
		t.private = cap
		pos = NewPosition(ctx, testSymbol, "paper", "path", t.price, nil, nil)
		pos.EntryOrder = &spot.OrderRequest{
			ClOrdId: "enter-1", OrderType: "market", Type: "buy", Volume: "1",
		}
	})()

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = tier.Send(pos, "enter")
	}
}

func BenchmarkTierApply(b *testing.B) {
	ctx := context.Background()
	var tier *Tier
	withTier(ctx, TierTypePaper, func(t *Tier) {
		tier = t
		pos := NewPosition(ctx, testSymbol, "paper", "path", t.price, nil, nil)
		pos.EntryOrder = &spot.OrderRequest{ClOrdId: "enter-1"}
		t.positions.Add(testSymbol, pos, true)
	})()
	exec := &market.Execution{
		Data: []*market.ExecutionData{{
			ExecID: "b1", Side: "buy", Symbol: testSymbol,
			ClientOrderID: "enter-1",
			LastQty:       dec("1"), LastPrice: dec("100"),
			Timestamp: time.Unix(1, 0).UTC(),
		}},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		tier.apply(exec)
	}
}

func BenchmarkTierSettle(b *testing.B) {
	ctx := context.Background()
	var tier *Tier
	var pos *Position
	withTier(ctx, TierTypePaper, func(t *Tier) {
		tier = t
		pos = NewPosition(ctx, testSymbol, "paper", "path", t.price, nil, nil)
		pos.Apply(market.ExecutionData{
			ExecID: "b1", Side: "buy",
			LastQty: dec("1"), LastPrice: dec("100"),
			Timestamp: time.Unix(1, 0).UTC(),
		})
		t.positions.Add(testSymbol, pos, true)
	})()

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		tier.settle(pos)
	}
}

func BenchmarkTierAckOrder(b *testing.B) {
	ctx := context.Background()
	var tier *Tier
	withTier(ctx, TierTypePaper, func(t *Tier) {
		tier = t
		pos := NewPosition(ctx, testSymbol, "paper", "path", t.price, nil, nil)
		pos.EntryOrder = &spot.OrderRequest{ClOrdId: "enter-1"}
		t.positions.Add(testSymbol, pos, true)
	})()
	order := &kraken.Order{
		Success: true,
		Result:  kraken.OrderResult{ClOrdID: "enter-1", OrderID: "OID-1"},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		tier.ackOrder(order)
	}
}

func BenchmarkTierRejectOrder(b *testing.B) {
	ctx := context.Background()

	b.ReportAllocs()

	for b.Loop() {
		withTier(ctx, TierTypePaper, func(tier *Tier) {
			pos := NewPosition(ctx, testSymbol, "paper", "path", tier.price, nil, nil)
			pos.EntryOrder = &spot.OrderRequest{ClOrdId: "enter-1"}
			tier.positions.Add(testSymbol, pos, true)
			tier.rejectOrder(&kraken.Order{
				Success: false,
				Error:   "Insufficient funds",
				Result:  kraken.OrderResult{ClOrdID: "enter-1"},
			})
		})()
	}
}
