package broker

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/ui"
	"github.com/theapemachine/symm/venue/kraken"
	. "github.com/smartystreets/goconvey/convey"
)

func TestLedgerFold(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("closedLegFills returns nil on empty input", func() {
			So(closedLegFills(nil), ShouldBeNil)
			So(closedLegFills([]*market.ExecutionData{}), ShouldBeNil)
		})

		Convey("closedLegFills ignores sells before any buy", func() {
			q, _ := decimal.NewFromString("1")
			t0 := time.Unix(1, 0).UTC()

			closed := closedLegFills([]*market.ExecutionData{
				{Side: "sell", LastQty: &q, Timestamp: t0},
			})

			So(closed, ShouldBeNil)
		})

		Convey("closedLegFills drops nil and zero-qty rows", func() {
			q, _ := decimal.NewFromString("1")
			z := decimal.Zero
			t0 := time.Unix(1, 0).UTC()
			t1 := time.Unix(2, 0).UTC()

			closed := closedLegFills([]*market.ExecutionData{
				nil,
				{Side: "buy", LastQty: &z, Timestamp: t0},
				{Side: "buy", LastQty: &q, Timestamp: t0},
				{Side: "sell", LastQty: &q, Timestamp: t1},
			})

			So(len(closed), ShouldEqual, 1)
			So(len(closed[0]), ShouldEqual, 2)
			So(closed[0][0].Side, ShouldEqual, "buy")
			So(closed[0][1].Side, ShouldEqual, "sell")
		})

		Convey("closedLegFills leaves open leg unmatched", func() {
			q, _ := decimal.NewFromString("1")
			t0 := time.Unix(1, 0).UTC()
			t1 := time.Unix(2, 0).UTC()
			t2 := time.Unix(3, 0).UTC()

			closed := closedLegFills([]*market.ExecutionData{
				{Side: "buy", LastQty: &q, Timestamp: t0},
				{Side: "sell", LastQty: &q, Timestamp: t1},
				{Side: "buy", LastQty: &q, Timestamp: t2},
			})

			So(len(closed), ShouldEqual, 1)
			So(len(closed[0]), ShouldEqual, 2)
		})

		Convey("closedLegFills splits completed round trips", func() {
			buy := func(t time.Time, qty string) *market.ExecutionData {
				q, _ := decimal.NewFromString(qty)

				return &market.ExecutionData{Side: "buy", LastQty: &q, Timestamp: t}
			}

			sell := func(t time.Time, qty string) *market.ExecutionData {
				q, _ := decimal.NewFromString(qty)

				return &market.ExecutionData{Side: "sell", LastQty: &q, Timestamp: t}
			}

			t0 := time.Unix(1, 0).UTC()
			t1 := time.Unix(2, 0).UTC()
			t2 := time.Unix(3, 0).UTC()
			t3 := time.Unix(4, 0).UTC()

			closed := closedLegFills([]*market.ExecutionData{
				buy(t0, "1"), sell(t1, "1"),
				buy(t2, "2"), sell(t3, "2"),
			})

			So(len(closed), ShouldEqual, 2)
			So(closed[0][0].Side, ShouldEqual, "buy")
			So(closed[0][1].Side, ShouldEqual, "sell")
			So(closed[1][0].Timestamp, ShouldEqual, t2)
			So(closed[1][1].Timestamp, ShouldEqual, t3)
		})

		Convey("closedLotID is stable across history re-folds", func() {
			q, _ := decimal.NewFromString("1")
			t0 := time.Unix(10, 0).UTC()
			t1 := time.Unix(20, 0).UTC()

			fills := []*market.ExecutionData{
				{ExecID: "buy-tx", Side: "buy", LastQty: &q, Timestamp: t0},
				{ExecID: "sell-tx", Side: "sell", LastQty: &q, Timestamp: t1},
			}

			a := closedLotID("XXBTZUSD", fills)
			b := closedLotID("XXBTZUSD", fills)

			So(a, ShouldEqual, "buy-tx:sell-tx")
			So(b, ShouldEqual, a)
		})

		Convey("openLegForHold returns the forward leg when it matches balance", func() {
			qBuy, _ := decimal.NewFromString("100")
			qSell, _ := decimal.NewFromString("50")
			hold, _ := decimal.NewFromString("50")
			t0 := time.Unix(1, 0).UTC()
			t1 := time.Unix(2, 0).UTC()

			all := []*market.ExecutionData{
				{Side: "buy", LastQty: &qBuy, Timestamp: t0},
				{Side: "sell", LastQty: &qSell, Timestamp: t1},
			}

			tier := &Tier{price: &Price{Instrument: &Instrument{cache: &sync.Map{}}}}
			min := decimal.RequireFromString("0.0001")
			tier.price.Instrument.cache.Store("FOO/USD", kraken.InstrumentPair{
				Symbol: "FOO/USD", Base: "FOO", Quote: "USD", Status: "online",
				QtyPrecision: 4, QtyMin: &min,
			})

			leg := tier.openLegForHold(all, &hold, "FOO/USD")

			So(len(leg), ShouldEqual, 2)
			So(netFillQty(leg).String(), ShouldEqual, "50")
		})
	})
}

func TestTierHistoryReconcile(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("historyMatchesSpot tolerates desk-precision rounding", func() {
			tier := &Tier{price: &Price{Instrument: &Instrument{cache: &sync.Map{}}}}
			min := decimal.RequireFromString("0.0001")
			tier.price.Instrument.cache.Store("FOO/USD", kraken.InstrumentPair{
				Symbol:       "FOO/USD",
				Base:         "FOO",
				Quote:        "USD",
				Status:       "online",
				QtyPrecision: 4,
				QtyMin:       &min,
			})

			deskQty := decimal.RequireFromString("1.2345")
			hold := decimal.RequireFromString("1.2346")

			So(tier.historyMatchesSpot("FOO/USD", &deskQty, &hold), ShouldBeTrue)

			flat := decimal.RequireFromString("0")
			So(tier.historyMatchesSpot("FOO/USD", &deskQty, &flat), ShouldBeFalse)
		})

		Convey("historyMatchesSpot aligns position qty with coarser Balance decimals", func() {
			tier := &Tier{price: &Price{Instrument: &Instrument{cache: &sync.Map{}}}}
			min := decimal.RequireFromString("0.0001")
			tier.price.Instrument.cache.Store("CAP/USD", kraken.InstrumentPair{
				Symbol: "CAP/USD", Base: "CAP", Quote: "USD", Status: "online",
				QtyPrecision: 8, QtyMin: &min,
			})

			deskQty := decimal.RequireFromString("83.45475")
			hold := decimal.RequireFromString("83.455")

			So(tier.historyMatchesSpot("CAP/USD", &deskQty, &hold), ShouldBeTrue)
		})

		Convey("historyMatchesSpot rejects a hold that does not match desk qty", func() {
			tier := &Tier{price: &Price{Instrument: &Instrument{cache: &sync.Map{}}}}
			min := decimal.RequireFromString("0.0001")
			tier.price.Instrument.cache.Store("GAP/USD", kraken.InstrumentPair{
				Symbol: "GAP/USD", Base: "GAP", Quote: "USD", Status: "online",
				QtyPrecision: 4, QtyMin: &min,
			})

			deskQty := decimal.RequireFromString("10")
			hold := decimal.RequireFromString("8")

			So(tier.historyMatchesSpot("GAP/USD", &deskQty, &hold), ShouldBeFalse)
		})

		Convey("wireExchangeLot sets orders and opened_at from execution fills", func() {
			ctx := context.Background()
			tier := &Tier{ctx: ctx, Type: TierTypeReal}
			position := NewPosition(ctx, "BTC/USD", "real", "exchange", nil, nil, nil)

			qty := decimal.RequireFromString("0.02")
			price := decimal.RequireFromString("30000")
			t0 := time.Unix(100, 0).UTC()
			t1 := time.Unix(200, 0).UTC()

			position.Fills = []*market.ExecutionData{
				{
					OrderID: "ENTRY-TX", ExecID: "T1", Side: "buy", OrderType: "limit",
					LastQty: &qty, LastPrice: &price, AvgPrice: &price,
					Timestamp: t0, OrderStatus: "filled",
				},
				{
					OrderID: "EXIT-TX", ExecID: "T2", Side: "sell", OrderType: "market",
					LastQty: &qty, LastPrice: &price, Timestamp: t1,
				},
			}

			qtyOpen := qty
			position.Qty = &qtyOpen
			position.Entry = &price

			tier.wireExchangeLot(position)

			So(position.OpenedAt, ShouldEqual, t0)
			So(position.EntryOrder.ClOrdId, ShouldEqual, "ENTRY-TX")
			So(position.ExitOrder.ClOrdId, ShouldEqual, "EXIT-TX")
		})

		Convey("markHistoryLot marks exchange positions without stoploss", func() {
			ctx := context.Background()
			bid := decimal.RequireFromString("100")
			ask := decimal.RequireFromString("101")
			bookQty := decimal.RequireFromString("1")

			books := NewBookManager()
			books.Update(&market.Level3{
				Type: "snapshot",
				Data: []*market.Level3Data{{
					Symbol: "BTC/USD",
					Bids: []*market.Level3Order{{
						OrderID: "b1", LimitPrice: &bid, OrderQty: &bookQty,
						Timestamp: time.Unix(1, 0),
					}},
					Asks: []*market.Level3Order{{
						OrderID: "a1", LimitPrice: &ask, OrderQty: &bookQty,
						Timestamp: time.Unix(1, 0),
					}},
				}},
			})

			fee := decimal.Zero
			instrument := &Instrument{cache: &sync.Map{}, takerFees: &sync.Map{}}
			instrument.takerFees.Store("BTC/USD", &fee)

			price := &Price{Instrument: instrument, Books: books}
			tier := &Tier{ctx: ctx, price: price, Type: TierTypeReal}
			position := NewPosition(ctx, "BTC/USD", "real", "exchange", price, nil, nil)
			position.Apply(tradeFill("b1", "buy", "0.01", "90"))

			tier.markHistoryLot(position)

			So(position.Stoploss, ShouldBeNil)
			So(position.PnLPct, ShouldNotBeNil)
		})
	})
}

func TestHistoryPrimitives(t *testing.T) {
	Convey("ledger helpers", t, func() {
		Convey("historyFill derives price from cost over volume", func() {
			vol := decimal.RequireFromString("2")
			cost := decimal.RequireFromString("100")
			fill := historyFill("tx-1", kraken.HistoryTrade{
				Pair: "BTC/USD", Type: "buy", Volume: &vol, Cost: &cost, Time: 10,
			})

			So(fill, ShouldNotBeNil)
			So(fill.ExecID, ShouldEqual, "tx-1")
			So(fill.LastPrice.String(), ShouldEqual, "50")
			So(fill.Side, ShouldEqual, "buy")
		})

		Convey("historyFill drops zero volume", func() {
			z := decimal.Zero
			So(historyFill("x", kraken.HistoryTrade{Volume: &z}), ShouldBeNil)
		})

		Convey("openLegFills skips closed prefixes", func() {
			q := decimal.RequireFromString("1")
			t0 := time.Unix(1, 0).UTC()
			t1 := time.Unix(2, 0).UTC()
			t2 := time.Unix(3, 0).UTC()

			open := openLegFills([]*market.ExecutionData{
				{Side: "buy", LastQty: &q, Timestamp: t0},
				{Side: "sell", LastQty: &q, Timestamp: t1},
				{Side: "buy", LastQty: &q, Timestamp: t2},
			})

			So(len(open), ShouldEqual, 1)
			So(open[0].Side, ShouldEqual, "buy")
		})

		Convey("historyPairForDesk filters quote suffix", func() {
			So(historyPairForDesk("ETH/USD", "USD"), ShouldBeTrue)
			So(historyPairForDesk("ETHUSD", "USD"), ShouldBeTrue)
			So(historyPairForDesk("ETH/EUR", "USD"), ShouldBeFalse)
			So(historyPairForDesk("", "USD"), ShouldBeFalse)
		})

		Convey("closedLotID falls back when exec ids missing", func() {
			q := decimal.RequireFromString("1")
			t0 := time.Unix(99, 0).UTC()

			id := closedLotID("", []*market.ExecutionData{
				{Side: "buy", LastQty: &q, Timestamp: t0},
			})

			So(id, ShouldEqual, "closed-99")
		})

		Convey("historyLotRejectReason enumerates blockers", func() {
			tier := &Tier{price: &Price{Instrument: &Instrument{cache: &sync.Map{}}}}
			min := decimal.RequireFromString("1")
			tier.price.Instrument.cache.Store("BIG/USD", kraken.InstrumentPair{
				Symbol: "BIG/USD", Status: "online", QtyMin: &min,
			})
			tier.price.Instrument.cache.Store("OFF/USD", kraken.InstrumentPair{
				Symbol: "OFF/USD", Status: "cancel_only", QtyMin: &min,
			})

			tiny := decimal.RequireFromString("0.1")
			So(tier.historyLotRejectReason("", &tiny), ShouldEqual, "zero_qty")
			So(tier.historyLotRejectReason("NOPE/USD", &tiny), ShouldEqual, "not_in_instrument_cache")
			So(tier.historyLotRejectReason("OFF/USD", &tiny), ShouldEqual, "not_spot_online")
			So(tier.historyLotRejectReason("BIG/USD", &tiny), ShouldEqual, "below_qty_min")
		})

		Convey("exchangeLotViable enforces real venue mins", func() {
			min := decimal.RequireFromString("1")
			instrument := &Instrument{cache: &sync.Map{}}
			instrument.cache.Store("R/USD", kraken.InstrumentPair{
				Symbol: "R/USD", Status: "online", QtyMin: &min,
			})
			price := &Price{Instrument: instrument}

			real := &Tier{Type: TierTypeReal, price: price}
			paper := &Tier{Type: TierTypePaper, price: price}

			half := decimal.RequireFromString("0.5")
			one := decimal.RequireFromString("1")

			So(real.exchangeLotViable("R/USD", &half), ShouldBeFalse)
			So(real.exchangeLotViable("R/USD", &one), ShouldBeTrue)
			So(paper.exchangeLotViable("R/USD", &half), ShouldBeTrue)
		})

		Convey("finalizeClosedLot backfills fee-net pnl", func() {
			fixture := newPriceFixture(t, priceFixture{
				symbol: "FIN/USD", cash: 1000,
				qtyMinRaw: "0.01", costMinRaw: "1", step: "0.01",
				bidPx: 110, bidQty: 10, askPx: 111, askQty: 10, feePct: 0,
			})

			tier := &Tier{price: fixture.price}
			closed := time.Now().UTC()
			sold := decimal.RequireFromString("1")
			entry := decimal.RequireFromString("100")
			exit := decimal.RequireFromString("110")

			lot := &Position{
				Symbol: fixture.symbol, EntryPath: "exchange",
				Entry: &entry, Sold: &sold, ExitVWAP: &exit, ClosedAt: &closed,
			}

			tier.finalizeClosedLot(lot)

			So(lot.PnL, ShouldNotBeNil)
			So(lot.PnLPct, ShouldNotBeNil)
			So(lot.PnL.Sign(), ShouldBeGreaterThan, 0)
		})
	})
}

func TestFoldHistoryAdmitsExchangeOpen(t *testing.T) {
	Convey("foldHistory rebuilds an exchange open from balance and buys", t, func() {
		symbol := "HIST/USD"
		min := decimal.RequireFromString("0.01")
		instrument := &Instrument{cache: &sync.Map{}, quote: "USD"}
		instrument.cache.Store(symbol, kraken.InstrumentPair{
			Symbol: symbol, Base: "HIST", Quote: "USD", Status: "online",
			QtyPrecision: 4, QtyMin: &min,
		})

		fixture := newPriceFixture(t, priceFixture{
			symbol: symbol, cash: 1000,
			qtyMinRaw: "0.01", costMinRaw: "1", step: "0.01",
			bidPx: 10, bidQty: 100, askPx: 11, askQty: 100, feePct: 0,
		})
		fixture.price.Instrument = instrument

		host := venueKrakenWithCash("USD", "100")
		hold := decimal.RequireFromString("3")
		krakenBal := host.Balance().(*kraken.Balance)
		krakenBal.Result["HIST"] = &hold

		tier := &Tier{
			ctx:       context.Background(),
			Type:      TierTypeReal,
			price:     fixture.price,
			venue:     host,
			positions: NewCache(context.Background(), "real", nil, fixture.price, nil, nil),
		}

		vol := decimal.RequireFromString("3")
		cost := decimal.RequireFromString("30")
		px := decimal.RequireFromString("10")

		tier.foldHistory(map[string]kraken.HistoryTrade{
			"buy-1": {
				Pair: symbol, Type: "buy", Volume: &vol, Cost: &cost, Price: &px, Time: 100,
			},
		})

		So(tier.positions.Has(symbol), ShouldBeTrue)
		open := tier.positions.Open(symbol)
		So(open, ShouldHaveLength, 1)
		So(open[0].EntryPath, ShouldEqual, "exchange")
		So(open[0].Qty.String(), ShouldEqual, "3")
	})
}

func TestUpsertClosedHistoryLot(t *testing.T) {
	Convey("upsertClosedHistoryLot archives a closed exchange leg once", t, func() {
		symbol := "CL/USD"
		min := decimal.RequireFromString("0.01")
		instrument := &Instrument{cache: &sync.Map{}}
		instrument.cache.Store(symbol, kraken.InstrumentPair{
			Symbol: symbol, Base: "CL", Quote: "USD", Status: "online", QtyMin: &min,
		})

		price := &Price{Instrument: instrument}
		cache := NewCache(context.Background(), "real", nil, price, nil, nil)

		tier := &Tier{
			ctx: context.Background(), Type: TierTypeReal,
			price: price, positions: cache,
		}

		q := decimal.RequireFromString("1")
		t0 := time.Unix(1, 0).UTC()
		t1 := time.Unix(2, 0).UTC()

		fills := []*market.ExecutionData{
			{ExecID: "b", Symbol: symbol, Side: "buy", LastQty: &q, LastPrice: &q, Timestamp: t0},
			{ExecID: "s", Symbol: symbol, Side: "sell", LastQty: &q, LastPrice: &q, Timestamp: t1},
		}

		tier.upsertClosedHistoryLot(symbol, fills)
		tier.upsertClosedHistoryLot(symbol, fills)

		_, loaded := tier.exchangeClosed.Load("b:s")
		So(loaded, ShouldBeTrue)
	})
}

func TestPurgeSyntheticBalanceLots(t *testing.T) {
	Convey("purgeSyntheticBalanceLots drops balance-prefixed opens on real", t, func() {
		cache := NewCache(context.Background(), "real", nil, nil, nil, nil)
		cache.open.Store("SYN/USD", &Position{ID: "balance:usd", Symbol: "SYN/USD"})

		tier := &Tier{Type: TierTypeReal, positions: cache}
		tier.purgeSyntheticBalanceLots()

		So(cache.Has("SYN/USD"), ShouldBeFalse)
	})
}

func TestLotByIDAndDeskSymbol(t *testing.T) {
	Convey("lotByID finds open or closed lanes", t, func() {
		cache := newTestCache("virtual")
		open := &Position{ID: "find-me", Symbol: "F/USD"}
		cache.open.Store("F/USD", open)

		tier := &Tier{positions: cache}
		So(tier.lotByID("find-me"), ShouldEqual, open)
		So(tier.lotByID("nope"), ShouldBeNil)
	})

	Convey("deskSymbol maps through price", t, func() {
		instrument := &Instrument{cache: &sync.Map{}, quote: "USD"}
		min := decimal.RequireFromString("0.01")
		instrument.cache.Store("MAP/USD", kraken.InstrumentPair{
			Symbol: "MAP/USD", Base: "MAP", Quote: "USD", Status: "online", QtyMin: &min,
		})

		tier := &Tier{price: &Price{Instrument: instrument}}
		So(tier.deskSymbol("MAP/USD"), ShouldEqual, "MAP/USD")
	})
}

type historyPostClient struct {
	pages map[string][]byte
	posts []string
}

func (client *historyPostClient) Read(...string) ([]byte, error) { return nil, nil }

func (client *historyPostClient) Write([]byte) error { return nil }

func (client *historyPostClient) Post(url string, body any) ([]byte, error) {
	client.posts = append(client.posts, url)

	if client.pages != nil {
		if raw, ok := client.pages[url]; ok {
			return raw, nil
		}
	}

	return []byte(`{"error":[],"result":{"count":"0","trades":{},"cursor":{"next":""}}}`), nil
}

func (client *historyPostClient) Watch(string, any, json.Unmarshaler) {}

type pagingHistoryClient struct {
	posts []string
}

func (client *pagingHistoryClient) Read(...string) ([]byte, error) { return nil, nil }

func (client *pagingHistoryClient) Write([]byte) error { return nil }

func (client *pagingHistoryClient) Watch(string, any, json.Unmarshaler) {}

func (client *pagingHistoryClient) Post(url string, body any) ([]byte, error) {
	client.posts = append(client.posts, url)

	req, _ := body.(map[string]any)
	cursor, _ := req["cursor"].(string)

	if cursor == "page2" {
		return []byte(`{
			"error":[],
			"result":{
				"count":"1",
				"trades":{"TX2":{"pair":"XBTUSD","type":"sell","ordertype":"market","price":"101","vol":"0.5","time":2}},
				"cursor":{"next":""}
			}
		}`), nil
	}

	return []byte(`{
		"error":[],
		"result":{
			"count":"1",
			"trades":{"TX1":{"pair":"XBTUSD","type":"buy","ordertype":"market","price":"100","vol":"0.5","time":1}},
			"cursor":{"next":"page2"}
		}
	}`), nil
}

func TestFetchHistory(t *testing.T) {
	Convey("fetchHistory paginates until the cursor is empty", t, func() {
		url := system.Cfg.WebSocket.Endpoints.TradesHistory
		client := &pagingHistoryClient{}

		tier := &Tier{Type: TierTypeReal, private: client}

		merged, ok, limited := tier.fetchHistory(url)
		So(limited, ShouldBeFalse)
		So(ok, ShouldBeTrue)
		So(len(merged), ShouldEqual, 2)
		So(merged["TX1"].Pair, ShouldEqual, "XBTUSD")
		So(merged["TX2"].Type, ShouldEqual, "sell")
		So(len(client.posts), ShouldEqual, 2)
	})

	Convey("fetchHistory surfaces rate limits for retry", t, func() {
		url := system.Cfg.WebSocket.Endpoints.TradesHistory
		client := &historyPostClient{pages: map[string][]byte{
			url: []byte(`{"error":["EAPI:Rate limit exceeded"],"result":{}}`),
		}}

		tier := &Tier{Type: TierTypeReal, private: client}

		_, ok, limited := tier.fetchHistory(url)
		So(ok, ShouldBeFalse)
		So(limited, ShouldBeTrue)
	})
}

func TestPublishHistorySync(t *testing.T) {
	Convey("publishHistorySync enqueues a history_sync frame while the hub is in grace", t, func() {
		restore := ui.UI.PrimeGrace()
		defer restore()

		for {
			if _, ok := ui.UI.NextFrame(); !ok {
				break
			}
		}

		tier := &Tier{Type: TierTypeReal}
		tier.publishHistorySync("", map[string]any{"url": "ignored"})
		_, published := ui.UI.NextFrame()
		So(published, ShouldBeFalse)

		tier.publishHistorySync("empty", map[string]any{"url": "stub"})

		raw, ok := ui.UI.NextFrame()
		So(ok, ShouldBeTrue)

		var frame map[string]any
		So(json.Unmarshal(raw, &frame), ShouldBeNil)
		sync := frame["history_sync"].(map[string]any)
		So(sync["status"], ShouldEqual, "empty")
		So(sync["url"], ShouldEqual, "stub")
		So(sync["at"], ShouldNotBeEmpty)
	})
}

func TestPullHistorySkipsWhenFresh(t *testing.T) {
	Convey("non-urgent pullHistory respects the min gap", t, func() {
		client := &historyPostClient{}
		tier := &Tier{
			Type:            TierTypeReal,
			private:         client,
			venue:           venueKrakenWithCash("USD", "1"),
			positions:       NewCache(context.Background(), "real", client, nil, nil, nil),
			historySyncedAt: time.Now(),
		}

		tier.pullHistory(false)

		So(len(client.posts), ShouldEqual, 0)
	})
}

func TestPullHistoryUrgent(t *testing.T) {
	Convey("pullHistory primes balances then folds an empty history", t, func() {
		balURL := system.Cfg.WebSocket.Endpoints.Balance
		tradeURL := system.Cfg.WebSocket.Endpoints.TradeBalance
		histURL := system.Cfg.WebSocket.Endpoints.TradesHistory

		client := &historyPostClient{pages: map[string][]byte{
			balURL:   []byte(`{"error":[],"result":{"USD":"100"}}`),
			tradeURL: []byte(`{"error":[],"result":{"eb":"100","tb":"100","m":"0","n":"0","c":"0","v":"0","e":"100"}}`),
			histURL:  []byte(`{"error":[],"result":{"count":"0","trades":{},"cursor":{"next":""}}}`),
		}}

		host := venueKrakenWithCash("USD", "100")
		tier := &Tier{
			Type:      TierTypeReal,
			private:   client,
			venue:     host,
			positions: NewCache(context.Background(), "real", client, nil, nil, nil),
		}

		tier.pullHistory(true)

		So(len(client.posts), ShouldBeGreaterThanOrEqualTo, 3)

		krakenBal := host.Balance().(*kraken.Balance)
		So(krakenBal.Result["USD"].String(), ShouldEqual, "100")

		tier.historyMu.Lock()
		synced := tier.historySyncedAt
		tier.historyMu.Unlock()

		So(synced.IsZero(), ShouldBeFalse)
		So(time.Since(synced), ShouldBeLessThan, time.Minute)
	})
}
