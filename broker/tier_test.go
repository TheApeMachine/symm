package broker

import (
	"context"
	"encoding/json"
	goruntime "runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/symm/venue"
	"github.com/theapemachine/symm/venue/kraken"
)

func newTestTier(t *testing.T, tt TierType, price *Price, host market.Venue) *Tier {
	t.Helper()

	if host == nil {
		host = venueKrakenWithCash("USD", "50000")
	}

	return &Tier{
		ctx:       context.Background(),
		Type:      tt,
		unlocked:  true,
		price:     price,
		venue:     host,
		positions: NewCache(context.Background(), tt.String(), nil, price, nil, nil),
		pending:   &sync.Map{},
	}
}

func TestTierTypeString(t *testing.T) {
	Convey("TierType stringifies known lanes", t, func() {
		So(TierTypeVirtual.String(), ShouldEqual, "virtual")
		So(TierTypePaper.String(), ShouldEqual, "paper")
		So(TierTypeReal.String(), ShouldEqual, "real")
		So(TierType(99).String(), ShouldEqual, "")
	})
}

func TestTierUnlocked(t *testing.T) {
	Convey("virtual and paper stay unlocked", t, func() {
		So((&Tier{Type: TierTypeVirtual}).Unlocked(nil), ShouldBeTrue)
		So((&Tier{Type: TierTypePaper}).Unlocked(nil), ShouldBeTrue)
	})

	Convey("live-only real skips the paper proof", t, func() {
		So((&Tier{Type: TierTypeReal, live: true}).Unlocked(nil), ShouldBeTrue)
	})

	Convey("virgin real unlocks from paper alone", t, func() {
		real := &Tier{Type: TierTypeReal}
		paper := &Tier{Type: TierTypePaper, WinRate: 0.51, Edge: 0.01}

		So(real.Unlocked(nil), ShouldBeFalse)
		So(real.Unlocked(&Tier{Type: TierTypePaper, WinRate: 0.5, Edge: 1}), ShouldBeFalse)
		So(real.Unlocked(paper), ShouldBeTrue)
	})

	Convey("real with history must stay ok even when paper holds", t, func() {
		paper := &Tier{Type: TierTypePaper, WinRate: 0.8, Edge: 1}
		real := &Tier{Type: TierTypeReal, WinRate: 0.8, Edge: 1, unlocked: true}

		So(real.Unlocked(paper), ShouldBeTrue)

		real.WinRate = 0.4
		So(real.Unlocked(paper), ShouldBeFalse)

		real.WinRate = 0.8
		real.Edge = -0.1
		So(real.Unlocked(paper), ShouldBeFalse)
	})

	Convey("live-only Unlocked keeps real open after a losing Recalculate", t, func() {
		loss := decimal.NewFromInt(-2)
		closed := time.Now()
		cache := &Cache{open: &sync.Map{}, closed: &sync.Map{}}
		cache.closed.Store("AAA/USD", &Position{
			Symbol:   "AAA/USD",
			ClosedAt: &closed,
			PnLPct:   &loss,
		})
		real := &Tier{
			Type:      TierTypeReal,
			unlocked:  true,
			live:      true,
			positions: cache,
		}

		real.Recalculate()
		So(real.Unlocked(nil), ShouldBeTrue)
	})
}

func TestSendWriteErrorKeepsPosition(t *testing.T) {
	Convey("Write failure keeps the open position for late fills by cl_ord_id", t, func() {
		tier := &Tier{
			positions: NewCache(context.Background(), "real", nil, nil, nil, nil),
			private:   &writeFailClient{},
		}
		pos := &Position{
			Symbol: "BTC/USD",
			EntryOrder: &spot.OrderRequest{
				ClOrdId:   "enter-1",
				OrderType: "market",
				Type:      "buy",
				Volume:    "0.01",
			},
		}

		So(tier.positions.Add(pos.Symbol, pos, true), ShouldBeFalse)

		err := tier.Send(pos, "enter")
		So(err, ShouldNotBeNil)
		So(tier.positions.Has(pos.Symbol), ShouldBeTrue)
	})
}

type writeFailClient struct{}

func (*writeFailClient) Read(...string) ([]byte, error) { return nil, nil }
func (*writeFailClient) Write([]byte) error {
	return errnie.Err(errnie.IO, "write failed", nil)
}
func (*writeFailClient) Post(string, any) ([]byte, error)    { return nil, nil }
func (*writeFailClient) Watch(string, any, json.Unmarshaler) {}

func TestApplyPartialExitResends(t *testing.T) {
	Convey("a partial exit fill resizes the market sell and kicks a resend", t, func() {
		fee := decimal.Zero
		instrument := &Instrument{cache: &sync.Map{}, takerFees: &sync.Map{}}
		instrument.takerFees.Store("BTC/USD", &fee)
		min := decimal.RequireFromString("0.0001")
		instrument.cache.Store("BTC/USD", kraken.InstrumentPair{
			Symbol: "BTC/USD", Base: "BTC", Quote: "USD", Status: "online",
			QtyPrecision: 8, QtyMin: &min,
		})

		writes := 0
		client := &countingClient{writes: &writes}

		auth := kraken.NewAuth()
		auth.SeedToken("test-ws-token")
		instrument.auth = auth

		price := &Price{Instrument: instrument, Books: NewBookManager()}
		tier := &Tier{
			ctx:       context.Background(),
			Type:      TierTypeReal,
			price:     price,
			private:   client,
			positions: NewCache(context.Background(), "real", client, price, nil, nil),
		}

		position := NewPosition(
			context.Background(), "BTC/USD", "real", "path", price, nil, tier.settle,
		)
		position.Apply(tradeFill("b1", "buy", "1", "100"))
		position.Closing = true
		position.ExitOrder = &spot.OrderRequest{
			ClOrdId:   "exit-live",
			OrderType: "market",
			Type:      "sell",
			Volume:    "1",
		}
		cl := position.ExitOrder.ClOrdId
		tier.positions.Add("BTC/USD", position, true)

		part := decimal.RequireFromString("0.4")
		sellPx := decimal.RequireFromString("99")
		tier.apply(&market.Execution{
			Data: []*market.ExecutionData{{
				ExecID:        "s1",
				ExecType:      "trade",
				ClientOrderID: cl,
				Symbol:        "BTC/USD",
				Side:          "sell",
				LastQty:       &part,
				LastPrice:     &sellPx,
			}},
		})

		So(position.Qty.String(), ShouldEqual, "0.6")
		So(position.ClosedAt, ShouldBeNil)
		So(position.ExitOrder.Volume, ShouldEqual, "0.6")
		So(position.ExitOrder.Type, ShouldEqual, "sell")
		So(writes, ShouldEqual, 1)
		So(string(client.last), ShouldContainSubstring, `"order_qty":0.6`)
		So(string(client.last), ShouldContainSubstring, `"side":"sell"`)
	})
}

func TestKickExitRetriesAfterBlankClOrdID(t *testing.T) {
	Convey("kickExit resends when closing but cl_ord_id was cleared", t, func() {
		position := NewPosition(context.Background(), "BTC/USD", "real", "path", nil, nil, nil)
		position.Apply(tradeFill("b1", "buy", "1", "100"))
		position.Closing = true

		tier := &Tier{private: &writeFailClient{}}

		tier.kickExit(position)
		So(position.ExitOrder.ClOrdId, ShouldNotBeBlank)

		prior := position.ExitOrder.ClOrdId
		position.ExitOrder.ClOrdId = ""
		tier.kickExit(position)
		So(position.ExitOrder.ClOrdId, ShouldNotBeBlank)
		So(position.ExitOrder.ClOrdId, ShouldNotEqual, prior)
	})
}

func TestTierExitSkipsWhenAlreadyClosing(t *testing.T) {
	Convey("policy Exit does not write when stoploss already armed", t, func() {
		writes := 0
		client := &countingClient{writes: &writes}

		position := NewPosition(
			context.Background(), "BTC/USD", "real", "path", nil, client, nil,
		)
		position.Apply(tradeFill("b1", "buy", "1", "100"))
		position.Closing = true
		position.ExitOrder = &spot.OrderRequest{
			ClOrdId:   "exit-live",
			OrderType: "market",
			Type:      "sell",
			Volume:    "1",
		}

		tier := &Tier{
			Type:      TierTypeReal,
			private:   client,
			positions: NewCache(context.Background(), "real", client, nil, nil, nil),
		}

		So(tier.Exit(position), ShouldBeNil)
		So(writes, ShouldEqual, 0)
	})
}

type countingClient struct {
	writes *int
	last   []byte
}

func (c *countingClient) Read(...string) ([]byte, error) { return nil, nil }
func (c *countingClient) Write(buf []byte) error {
	*c.writes++
	c.last = append([]byte(nil), buf...)
	return nil
}
func (*countingClient) Post(string, any) ([]byte, error)    { return nil, nil }
func (*countingClient) Watch(string, any, json.Unmarshaler) {}

type tickerWatchClient struct {
	watches *int
}

func (client *tickerWatchClient) Read(...string) ([]byte, error)   { return nil, nil }
func (client *tickerWatchClient) Write([]byte) error               { return nil }
func (client *tickerWatchClient) Post(string, any) ([]byte, error) { return nil, nil }
func (client *tickerWatchClient) Watch(string, any, json.Unmarshaler) {
	*client.watches++
}

func TestSendExitBlockedBelowQtyMin(t *testing.T) {
	Convey("real Send exit refuses when open qty is below venue min", t, func() {
		min := decimal.RequireFromString("1")
		instrument := &Instrument{cache: &sync.Map{}}
		instrument.cache.Store("SM/USD", kraken.InstrumentPair{
			Symbol: "SM/USD", Status: "online", QtyMin: &min,
		})

		writes := 0
		client := &countingClient{writes: &writes}

		tier := &Tier{
			ctx:       context.Background(),
			Type:      TierTypeReal,
			price:     &Price{Instrument: instrument},
			private:   client,
			positions: NewCache(context.Background(), "real", client, nil, nil, nil),
		}

		pos := NewPosition(context.Background(), "SM/USD", "real", "path", tier.price, client, nil)
		pos.Apply(tradeFill("b1", "buy", "0.5", "10"))
		pos.Closing = true
		pos.ensureExitOrder()
		tier.positions.Add("SM/USD", pos, true)

		So(tier.Send(pos, "exit"), ShouldNotBeNil)
		So(writes, ShouldEqual, 0)
	})
}

func TestVenueExitAllowed(t *testing.T) {
	Convey("venueExitAllowed gates real exits on spot status and qty min", t, func() {
		min := decimal.RequireFromString("1")
		instrument := &Instrument{cache: &sync.Map{}}
		instrument.cache.Store("VE/USD", kraken.InstrumentPair{
			Symbol: "VE/USD", Status: "online", QtyMin: &min,
		})
		instrument.cache.Store("OFF/USD", kraken.InstrumentPair{
			Symbol: "OFF/USD", Status: "halt", QtyMin: &min,
		})

		real := &Tier{
			Type:  TierTypeReal,
			price: &Price{Instrument: instrument},
		}

		half := decimal.RequireFromString("0.5")
		one := decimal.RequireFromString("1")

		So(real.venueExitAllowed("VE/USD", &one), ShouldBeTrue)
		So(real.venueExitAllowed("VE/USD", &half), ShouldBeFalse)
		So(real.venueExitAllowed("OFF/USD", &one), ShouldBeFalse)
		So(real.venueExitAllowed("", &one), ShouldBeFalse)
	})
}

func TestTierRecalculateWinRate(t *testing.T) {
	Convey("Recalculate tracks win rate and edge from closed lots", t, func() {
		cache := newTestCache("virtual")
		now := time.Now().UTC()

		win := decimal.RequireFromString("5")
		loss := decimal.RequireFromString("-2")

		cache.closed.Store("W/USD", &Position{Symbol: "W/USD", ClosedAt: &now, PnLPct: &win})
		cache.closed.Store("L/USD", &Position{Symbol: "L/USD", ClosedAt: &now, PnLPct: &loss})

		tier := &Tier{Type: TierTypeVirtual, positions: cache}
		tier.Recalculate()

		So(tier.WinRate, ShouldEqual, 0.5)
		So(tier.Edge, ShouldBeGreaterThan, 0)
	})
}

func TestFillReport(t *testing.T) {
	Convey("fillReport admits trades with positive qty", t, func() {
		tier := &Tier{}
		q := decimal.RequireFromString("1")

		So(tier.fillReport(&market.ExecutionData{
			ExecType: "trade", LastQty: &q,
		}), ShouldBeTrue)
		So(tier.fillReport(&market.ExecutionData{
			ExecType: "status", LastQty: &q,
		}), ShouldBeFalse)
		So(tier.fillReport(&market.ExecutionData{
			ExecType: "trade", LastQty: &decimal.Zero,
		}), ShouldBeFalse)
	})
}

func TestAckOrderBindsVenueID(t *testing.T) {
	Convey("a successful add_order ack stores venue order_id on the position", t, func() {
		tier := &Tier{
			positions: NewCache(context.Background(), "real", nil, nil, nil, nil),
		}

		position := NewPosition(context.Background(), "BTC/USD", "real", "path", nil, nil, nil)
		position.EntryOrder = &spot.OrderRequest{ClOrdId: "client-1", Type: "buy", Volume: "1"}
		tier.positions.Add("BTC/USD", position, true)

		tier.ackOrder(&kraken.Order{
			Success: true,
			Result: kraken.OrderResult{
				OrderID: "OID-1",
				ClOrdID: "client-1",
			},
		})

		So(position.EntryVenueOrderID, ShouldEqual, "OID-1")
	})

	Convey("exit ack binds venue order_id", t, func() {
		tier := &Tier{
			positions: NewCache(context.Background(), "real", nil, nil, nil, nil),
		}

		position := NewPosition(context.Background(), "BTC/USD", "real", "path", nil, nil, nil)
		position.ExitOrder = &spot.OrderRequest{ClOrdId: "exit-client", Type: "sell", Volume: "1"}
		tier.positions.Add("BTC/USD", position, true)

		tier.ackOrder(&kraken.Order{
			Success: true,
			Result: kraken.OrderResult{
				OrderID: "OID-EXIT",
				ClOrdID: "exit-client",
			},
		})

		So(position.ExitVenueOrderID, ShouldEqual, "OID-EXIT")
	})
}

func TestRejectEntryDropsPendingLot(t *testing.T) {
	Convey("an entry reject before fill drops the open key", t, func() {
		tier := &Tier{
			positions: NewCache(context.Background(), "real", nil, nil, nil, nil),
		}

		position := NewPosition(context.Background(), "ETH/USD", "real", "path", nil, nil, nil)
		position.EntryOrder = &spot.OrderRequest{ClOrdId: "entry-dead", Type: "buy", Volume: "1"}
		tier.positions.Add("ETH/USD", position, true)

		tier.rejectOrder(&kraken.Order{
			Result: kraken.OrderResult{ClOrdID: "entry-dead"},
			Error:  "insufficient margin",
		})

		So(tier.positions.Has("ETH/USD"), ShouldBeFalse)
	})
}

func TestRejectExitStopsAfterMaxAttempts(t *testing.T) {
	Convey("exit reject beyond eight attempts stops resend", t, func() {
		tier := &Tier{
			positions: NewCache(context.Background(), "real", nil, nil, nil, nil),
		}

		position := NewPosition(context.Background(), "BTC/USD", "real", "path", nil, nil, nil)
		position.ExitAttempts = 8
		position.Closing = true
		position.ExitOrder = &spot.OrderRequest{ClOrdId: "maxed", Type: "sell", Volume: "1"}
		tier.positions.Add("BTC/USD", position, true)

		tier.rejectOrder(&kraken.Order{
			Result: kraken.OrderResult{ClOrdID: "maxed"},
			Error:  "blocked",
		})

		So(position.ExitAttempts, ShouldEqual, 9)
		So(position.ExitOrder.ClOrdId, ShouldEqual, "maxed")
	})

	Convey("exit reject below the cap clears cl_ord_id for remint", t, func() {
		tier := &Tier{
			positions: NewCache(context.Background(), "real", nil, nil, nil, nil),
		}

		position := NewPosition(context.Background(), "BTC/USD", "real", "path", nil, nil, nil)
		position.ExitAttempts = 3
		position.Closing = true
		position.ExitOrder = &spot.OrderRequest{ClOrdId: "retry", Type: "sell", Volume: "1"}
		tier.positions.Add("BTC/USD", position, true)

		tier.rejectOrder(&kraken.Order{
			Result: kraken.OrderResult{ClOrdID: "retry"},
			Error:  "temporary",
		})

		So(position.ExitAttempts, ShouldEqual, 4)
		So(position.ExitOrder.ClOrdId, ShouldEqual, "")
	})
}

func TestRejectExitRetriesSettle(t *testing.T) {
	Convey("a rejected exit mints a new cl_ord_id and resizes the sell", t, func() {
		fee := decimal.Zero
		instrument := &Instrument{cache: &sync.Map{}, takerFees: &sync.Map{}}
		instrument.takerFees.Store("BTC/USD", &fee)

		price := &Price{Instrument: instrument, Books: NewBookManager()}
		tier := &Tier{
			Type:      TierTypeReal,
			price:     price,
			positions: NewCache(context.Background(), "real", nil, price, nil, nil),
		}

		position := NewPosition(
			context.Background(), "BTC/USD", "real", "path", price, nil, tier.settle,
		)
		position.Apply(tradeFill("b1", "buy", "1", "100"))
		position.Closing = true
		position.ExitOrder = &spot.OrderRequest{
			ClOrdId:   "rejected-id",
			OrderType: "market",
			Type:      "sell",
			Volume:    "1",
		}
		tier.positions.Add("BTC/USD", position, true)

		tier.rejectOrder(&kraken.Order{
			Result: kraken.OrderResult{ClOrdID: "rejected-id"},
			Error:  "insufficient funds",
		})

		So(position.ExitOrder.ClOrdId, ShouldNotEqual, "rejected-id")
		So(position.ExitOrder.ClOrdId, ShouldNotBeBlank)
		So(position.ExitOrder.Volume, ShouldEqual, "1")
	})
}

func TestTierBalanceReading(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("paper folds open lot PnL into unrealized and equity", func() {
			fee := decimal.Zero
			instrument := &Instrument{cache: &sync.Map{}, takerFees: &sync.Map{}}
			instrument.takerFees.Store("BTC/USD", &fee)

			price := &Price{
				Instrument: instrument,
				Books:      NewBookManager(),
				ticker:     market.NewTicker(nil),
			}
			price.ticker.Set("BTC/USD", &market.TickerData{Symbol: "BTC/USD", Last: 110})

			host := venueKrakenWithCash("USD", "100")
			tier := &Tier{
				Type:      TierTypePaper,
				price:     price,
				venue:     host,
				positions: NewCache(context.Background(), "paper", nil, price, nil, nil),
			}

			pnl := decimal.RequireFromString("9.5")
			position := NewPosition(context.Background(), "BTC/USD", "paper", "exchange", price, nil, nil)
			position.Apply(tradeFill("b1", "buy", "1", "100"))
			position.PnL = &pnl
			tier.positions.Add("BTC/USD", position, true)

			reading := tier.balanceReading()

			So(reading["cash"], ShouldEqual, 100)
			So(reading["unrealized"], ShouldEqual, 9.5)
			So(reading["equity"], ShouldEqual, reading["cash"]+reading["unrealized"])
		})

		Convey("real with open symm lots ignores Kraken n for header unrealized", func() {
			fee := decimal.Zero
			instrument := &Instrument{cache: &sync.Map{}, takerFees: &sync.Map{}}
			instrument.takerFees.Store("BTC/USD", &fee)

			price := &Price{
				Instrument: instrument,
				Books:      NewBookManager(),
				ticker:     market.NewTicker(nil),
			}
			price.ticker.Set("BTC/USD", &market.TickerData{Symbol: "BTC/USD", Last: 99})

			krakenUn, _ := decimal.NewFromString("-10")
			eb, _ := decimal.NewFromString("169.42")

			host := venue.NewKraken("USD")
			tb := host.TradeBalance().(*kraken.TradeBalance)
			tb.Result = kraken.TradeBalanceResult{
				EquivalentBalance: &eb,
				UnrealizedPnL:     &krakenUn,
			}

			tier := &Tier{
				Type:      TierTypeReal,
				price:     price,
				venue:     host,
				positions: NewCache(context.Background(), "real", nil, price, nil, nil),
				exchange: struct {
					equity, unrealized float64
					ok                 bool
				}{ok: true, unrealized: -10, equity: 169.42},
			}

			lotPnL, _ := decimal.NewFromString("-0.15")
			position := NewPosition(context.Background(), "BTC/USD", "real", "R00", price, nil, nil)
			position.Apply(tradeFill("b1", "buy", "1", "100"))
			position.PnL = &lotPnL
			tier.positions.Add("BTC/USD", position, true)

			reading := tier.balanceReading()

			So(reading["unrealized"], ShouldEqual, -0.15)
			So(reading["unrealized"], ShouldNotEqual, -10)
		})

		Convey("real without desk lots uses Kraken TradeBalance", func() {
			price := &Price{ticker: market.NewTicker(nil)}

			usd, _ := decimal.NewFromString("67.37")
			eb, _ := decimal.NewFromString("169.42")
			unrealized, _ := decimal.NewFromString("-1.55")

			host := venue.NewKraken("USD")
			krakenBal := host.Balance().(*kraken.Balance)
			krakenBal.Result = map[string]*decimal.Decimal{"USD": &usd}

			tb := host.TradeBalance().(*kraken.TradeBalance)
			tb.Result = kraken.TradeBalanceResult{
				EquivalentBalance: &eb,
				UnrealizedPnL:     &unrealized,
			}

			tier := &Tier{
				Type:      TierTypeReal,
				price:     price,
				venue:     host,
				positions: NewCache(context.Background(), "real", nil, price, nil, nil),
			}

			reading := tier.balanceReading()

			So(reading["cash"], ShouldEqual, 67.37)
			So(reading["unrealized"], ShouldEqual, -1.55)
			So(reading["equity"], ShouldEqual, 169.42)
		})

		Convey("exchangeBalanceFrom uses valuation minus cost when n is zero", func() {
			n := decimal.Zero
			v, _ := decimal.NewFromString("70.54")
			c, _ := decimal.NewFromString("72.09")
			eb, _ := decimal.NewFromString("168.94")

			host := venue.NewKraken("USD")
			tb := host.TradeBalance().(*kraken.TradeBalance)
			tb.Result = kraken.TradeBalanceResult{
				EquivalentBalance: &eb,
				UnrealizedPnL:     &n,
				Valuation:         &v,
				CostBasis:         &c,
			}

			tier := &Tier{venue: host}

			_, un, ok := tier.exchangeBalanceFrom(tb)

			So(ok, ShouldBeTrue)
			want := decimal.RequireFromString("-1.55")
			got := decimal.NewFromFloat(un)
			eps := decimal.RequireFromString("0.01")
			So(got.Sub(want).Abs().LessThanOrEqual(eps), ShouldBeTrue)
		})
	})
}

func TestTierVirtualRoundTrip(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When virtual tier Enter and Exit run without private wire the lot opens and closes locally", func() {
			fixture := newPriceFixture(t, priceFixture{
				symbol:     "VRT/USD",
				cash:       50_000,
				qtyMinRaw:  "0.01",
				costMinRaw: "1",
				step:       "0.01",
				bidPx:      99,
				bidQty:     1_000,
				askPx:      100,
				askQty:     1_000,
				feePct:     0,
			})

			tier := newTestTier(t, TierTypeVirtual, fixture.price, nil)

			So(tier.Enter(fixture.symbol, "R00", true), ShouldBeNil)
			So(tier.positions.Has(fixture.symbol), ShouldBeTrue)

			open := tier.positions.Open(fixture.symbol)
			So(open, ShouldHaveLength, 1)
			So(open[0].Qty, ShouldNotBeNil)
			So(open[0].Qty.Sign(), ShouldBeGreaterThan, 0)

			So(tier.Exit(open[0]), ShouldBeNil)
			So(open[0].ClosedAt, ShouldNotBeNil)
			So(open[0].Flat(), ShouldBeTrue)
			So(tier.positions.Has(fixture.symbol), ShouldBeFalse)
			So(open[0].PnL, ShouldNotBeNil)
			So(open[0].ExitVWAP, ShouldNotBeNil)
			So(open[0].ExitVWAP.Sign(), ShouldBeGreaterThan, 0)

			_, pending := tier.pending.Load(fixture.symbol)
			So(pending, ShouldBeTrue)
		})
	})
}

func TestTierUpdate(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When the book moves Tier Update rewrites open PnL not print alone", func() {
			fixture := newPriceFixture(t, priceFixture{
				symbol:     "UPD/USD",
				cash:       10_000,
				qtyMinRaw:  "0.01",
				costMinRaw: "1",
				step:       "0.01",
				bidPx:      100,
				bidQty:     100,
				askPx:      101,
				askQty:     100,
				feePct:     0,
			})

			tier := newTestTier(t, TierTypeVirtual, fixture.price, nil)

			position := NewPosition(
				context.Background(), fixture.symbol, "virtual", "path",
				fixture.price, nil, nil,
			)
			position.Apply(tradeFill("b1", "buy", "1", "100"))
			tier.positions.Add(fixture.symbol, position, true)

			mark := decimal.RequireFromString("102")
			So(position.Update(&mark), ShouldBeNil)
			pnlBefore := position.PnL.String()

			bid := decimal.RequireFromString("106")
			ask := decimal.RequireFromString("107")
			qty := decimal.RequireFromString("100")
			fixture.price.Books.Update(&market.Level3{
				Type: "snapshot",
				Data: []*market.Level3Data{{
					Symbol: fixture.symbol,
					Bids:   []*market.Level3Order{{LimitPrice: &bid, OrderQty: &qty}},
					Asks:   []*market.Level3Order{{LimitPrice: &ask, OrderQty: &qty}},
				}},
			})

			mark = decimal.RequireFromString("106")
			tier.Update(&market.Update{Symbol: fixture.symbol, Price: &mark})

			So(position.Mark.String(), ShouldEqual, "106")
			So(position.PnL, ShouldNotBeNil)
			So(position.PnL.String(), ShouldNotEqual, pnlBefore)
		})
	})
}

func TestTierSettle(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When a position is flat settle removes it from open", func() {
			tier := newTestTier(t, TierTypeVirtual, nil, nil)

			position := NewPosition(
				context.Background(), "SET/USD", "virtual", "path",
				priceFor("SET/USD", bookWithTouch("SET/USD", "10", "11", "1")),
				nil, tier.settle,
			)
			position.Apply(tradeFill("b1", "buy", "1", "10"))
			position.Apply(tradeFill("s1", "sell", "1", "11"))

			So(position.ClosedAt, ShouldNotBeNil)
			tier.positions.Add("SET/USD", position, true)
			So(tier.positions.Has("SET/USD"), ShouldBeTrue)

			tier.settle(position)

			So(tier.positions.Has("SET/USD"), ShouldBeFalse)
			_, pending := tier.pending.Load("SET/USD")
			So(pending, ShouldBeTrue)
		})
	})
}

func TestTierApplyIgnoresClosedLaneMutations(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When only the closed lane matches fills do not mutate ledger", func() {
			tier := &Tier{
				positions: newTestCache("real"),
			}

			now := time.Now().UTC()
			position := NewPosition(context.Background(), "BTC/USD", "real", "path", nil, nil, nil)
			position.EntryOrder = &spot.OrderRequest{ClOrdId: "cl-live", Type: "buy"}
			position.Apply(tradeFill("b1", "buy", "1", "100"))
			position.Apply(tradeFill("s1", "sell", "1", "99"))
			position.ClosedAt = &now

			tier.positions.closed.Store("BTC/USD", position)
			fillCount := len(position.Fills)

			qty := decimal.RequireFromString("1")
			px := decimal.RequireFromString("50")
			tier.apply(&market.Execution{
				Data: []*market.ExecutionData{{
					ExecID:        "late",
					ExecType:      "trade",
					ClientOrderID: "cl-live",
					Symbol:        "BTC/USD",
					Side:          "buy",
					LastQty:       &qty,
					LastPrice:     &px,
				}},
			})

			So(len(position.Fills), ShouldEqual, fillCount)
		})
	})
}

func TestTierEnterForbidden(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When the tier is locked Enter refuses", func() {
			fixture := newPriceFixture(t, priceFixture{
				symbol:     "LCK/USD",
				cash:       10_000,
				qtyMinRaw:  "0.01",
				costMinRaw: "1",
				step:       "0.01",
				bidPx:      100,
				bidQty:     100,
				askPx:      101,
				askQty:     100,
				feePct:     0,
			})

			tier := newTestTier(t, TierTypeReal, fixture.price, nil)
			tier.unlocked = false

			So(tier.Enter(fixture.symbol, "R00", true), ShouldNotBeNil)
			So(tier.positions.Has(fixture.symbol), ShouldBeFalse)
		})
	})
}

func TestTierApplyPrefersOpenLane(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When open and closed both match the open leg receives the fill", func() {
			tier := &Tier{positions: newTestCache("real")}

			cl := "shared-cl"
			closed := NewPosition(context.Background(), "BTC/USD", "real", "path", nil, nil, nil)
			closed.EntryOrder = &spot.OrderRequest{ClOrdId: cl, Type: "buy"}
			closed.Apply(tradeFill("b1", "buy", "1", "100"))
			closed.Apply(tradeFill("s1", "sell", "1", "99"))
			So(closed.ClosedAt, ShouldNotBeNil)

			open := NewPosition(context.Background(), "BTC/USD", "real", "path", nil, nil, nil)
			open.EntryOrder = &spot.OrderRequest{ClOrdId: cl, Type: "buy"}
			open.Apply(tradeFill("b2", "buy", "2", "100"))

			tier.positions.closed.Store("BTC/USD", closed)
			tier.positions.Add("BTC/USD", open, true)

			qty := decimal.RequireFromString("1")
			px := decimal.RequireFromString("101")
			tier.apply(&market.Execution{
				Data: []*market.ExecutionData{{
					ExecID:        "live-fill",
					ExecType:      "trade",
					ClientOrderID: cl,
					Symbol:        "BTC/USD",
					Side:          "buy",
					LastQty:       &qty,
					LastPrice:     &px,
				}},
			})

			So(open.Qty.String(), ShouldEqual, "3")
			So(len(closed.Fills), ShouldEqual, 2)
		})
	})
}

func venueKrakenWithCash(quote string, cash string) *venue.Kraken { //nolint:unparam
	host := venue.NewKraken(quote)
	krakenBal := host.Balance().(*kraken.Balance)
	amount, _ := decimal.NewFromString(cash)
	krakenBal.Result = map[string]*decimal.Decimal{
		quote: &amount,
	}
	return host
}

type ledgerPrimeClient struct {
	posts   []string
	watches []string
}

func (client *ledgerPrimeClient) Read(...string) ([]byte, error) { return nil, nil }

func (client *ledgerPrimeClient) Write([]byte) error { return nil }

func (client *ledgerPrimeClient) Post(url string, body any) ([]byte, error) {
	client.posts = append(client.posts, url)

	switch {
	case strings.Contains(url, "Balance") && !strings.Contains(url, "TradeBalance"):
		return []byte(`{"error":[],"result":{"USD":"250"}}`), nil
	case strings.Contains(url, "TradeBalance"):
		return []byte(`{"error":[],"result":{"eb":"250","tb":"240","m":"0","n":"10","c":"0","v":"0","e":"250"}}`), nil
	default:
		return nil, nil
	}
}

func (client *ledgerPrimeClient) Watch(url string, req any, target json.Unmarshaler) {
	client.watches = append(client.watches, url)
}

func TestTierWirePrivateREST(t *testing.T) {
	Convey("syncExchangeBalances and wirePrivateREST prime venue ledgers", t, func() {
		client := &ledgerPrimeClient{}
		host := venueKrakenWithCash("USD", "100")
		tier := &Tier{Type: TierTypeReal, private: client, venue: host}

		tier.syncExchangeBalances()

		krakenBal := host.Balance().(*kraken.Balance)
		So(krakenBal.Result["USD"].String(), ShouldEqual, "250")

		client.posts = nil
		tier.wirePrivateREST()

		So(len(client.posts), ShouldBeGreaterThanOrEqualTo, 2)
		So(len(client.watches), ShouldBeGreaterThanOrEqualTo, 2)
	})
}

func TestKrakenUnrealized(t *testing.T) {
	Convey("krakenUnrealized prefers non-zero venue fields", t, func() {
		ten := decimal.RequireFromString("10")
		zero := decimal.Zero

		So(krakenUnrealized(kraken.TradeBalanceResult{UnrealizedPnL: &ten}), ShouldEqual, 10)

		val := decimal.RequireFromString("110")
		cost := decimal.RequireFromString("100")
		So(krakenUnrealized(kraken.TradeBalanceResult{
			Valuation: &val, CostBasis: &cost,
		}), ShouldEqual, 10)

		eq := decimal.RequireFromString("250")
		tb := decimal.RequireFromString("240")
		So(krakenUnrealized(kraken.TradeBalanceResult{
			Equity: &eq, TradeBalance: &tb,
		}), ShouldEqual, 10)

		So(krakenUnrealized(kraken.TradeBalanceResult{UnrealizedPnL: &zero}), ShouldEqual, 0)
	})
}

func TestTierBalanceReadingReal(t *testing.T) {
	Convey("balanceReading on real uses trade balance unrealized", t, func() {
		host := venueKrakenWithCash("USD", "500")
		tb := host.TradeBalance().(*kraken.TradeBalance)
		eq := decimal.RequireFromString("600")
		trade := decimal.RequireFromString("580")
		un := decimal.RequireFromString("20")
		tb.Result = kraken.TradeBalanceResult{
			Equity:        &eq,
			TradeBalance:  &trade,
			UnrealizedPnL: &un,
		}

		tier := &Tier{Type: TierTypeReal, venue: host, positions: newTestCache("real")}
		reading := tier.balanceReading()

		So(reading["cash"], ShouldEqual, 500)
		So(reading["equity"], ShouldEqual, 600)
		So(reading["unrealized"], ShouldEqual, 20)
	})
}

func TestTierHello(t *testing.T) {
	Convey("Hello marks open lots and publishes a snapshot", t, func() {
		fixture := newPriceFixture(t, priceFixture{
			symbol: "HI/USD", cash: 10_000,
			qtyMinRaw: "0.01", costMinRaw: "1", step: "0.01",
			bidPx: 50, bidQty: 10, askPx: 51, askQty: 10, feePct: 0,
		})

		tier := newTestTier(t, TierTypeVirtual, fixture.price, nil)
		pos := NewPosition(context.Background(), "HI/USD", "virtual", "path", fixture.price, nil, nil)
		pos.Apply(tradeFill("b1", "buy", "1", "48"))
		pos.Mark = nil
		tier.positions.Add("HI/USD", pos, true)

		tier.Hello()

		So(pos.Mark, ShouldNotBeNil)
		So(pos.Mark.String(), ShouldEqual, "50")
	})
}

type cancelReadClient struct {
	ctx    context.Context
	frames [][]byte
	idx    atomic.Int32
}

func (client *cancelReadClient) Read(...string) ([]byte, error) {
	select {
	case <-client.ctx.Done():
		return nil, client.ctx.Err()
	default:
	}

	i := int(client.idx.Add(1)) - 1
	if i < len(client.frames) {
		return client.frames[i], nil
	}

	<-client.ctx.Done()

	return nil, client.ctx.Err()
}

func (client *cancelReadClient) Write([]byte) error { return nil }

func (client *cancelReadClient) Post(string, any) ([]byte, error) { return nil, nil }

func (client *cancelReadClient) Watch(string, any, json.Unmarshaler) {}

func TestTierRunAndOpenPrivate(t *testing.T) {
	Convey("Run ingests private frames until context cancel", t, func() {
		ctx, cancel := context.WithCancel(context.Background())

		host := venue.NewKraken("USD")
		reads := &cancelReadClient{
			ctx: ctx,
			frames: [][]byte{
				[]byte(`{"channel":"heartbeat"}`),
			},
		}

		tier := &Tier{
			ctx:       ctx,
			private:   reads,
			venue:     host,
			positions: NewCache(ctx, "virtual", nil, nil, nil, nil),
		}

		done := make(chan struct{})
		go func() {
			tier.Run()
			close(done)
		}()

		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) && reads.idx.Load() == 0 {
			goruntime.Gosched()
		}

		cancel()

		select {
		case <-done:
			So(ctx.Err(), ShouldEqual, context.Canceled)
		case <-time.After(2 * time.Second):
			t.Fatal("tier.Run did not exit")
		}
	})

	Convey("Run resends closing lots on startup", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		writes := 0
		client := &countingClient{writes: &writes}
		price := &Price{Books: NewBookManager(), Instrument: &Instrument{cache: &sync.Map{}, takerFees: &sync.Map{}}}

		tier := &Tier{
			ctx:     ctx,
			Type:    TierTypeVirtual,
			private: client,
			venue:   venue.NewKraken("USD"),
			price:   price,
		}
		tier.positions = NewCache(ctx, "virtual", client, price, tier.settle, nil)

		pos := NewPosition(ctx, "RUN/USD", "virtual", "path", price, client, tier.settle)
		pos.Apply(tradeFill("b1", "buy", "1", "10"))
		pos.Closing = true
		pos.ensureExitOrder()
		tier.positions.Add("RUN/USD", pos, true)

		done := make(chan struct{})
		go func() {
			tier.Run()
			close(done)
		}()

		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) && writes == 0 {
			goruntime.Gosched()
		}

		So(writes, ShouldBeGreaterThan, 0)
		cancel()

		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("tier.Run did not exit")
		}
	})

	Convey("OpenPrivate wires execution and balance subscriptions", t, func() {
		writes := 0
		client := &countingClient{writes: &writes}

		auth := kraken.NewAuth()
		auth.SeedToken("private-ws-token")

		instrument := &Instrument{
			cache:     &sync.Map{},
			takerFees: &sync.Map{},
			auth:      auth,
		}

		tier := &Tier{
			ctx:       context.Background(),
			Type:      TierTypePaper,
			private:   client,
			venue:     venue.NewKraken("USD"),
			price:     &Price{Instrument: instrument},
			positions: NewCache(context.Background(), "paper", client, nil, nil, nil),
		}

		So(tier.OpenPrivate(), ShouldBeNil)
		So(writes, ShouldBeGreaterThanOrEqualTo, 2)
	})

	Convey("subscribePrivate marshals the channel request", t, func() {
		writes := 0
		client := &countingClient{writes: &writes}

		tier := &Tier{
			private: client,
			venue:   venue.NewKraken("USD"),
			Type:    TierTypePaper,
		}

		So(tier.subscribePrivate("balances"), ShouldBeNil)
		So(writes, ShouldEqual, 1)
		So(string(client.last), ShouldContainSubstring, `"channel":"balances"`)
	})
}
