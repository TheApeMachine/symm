package broker

import (
	"context"
	"encoding/json"
	goruntime "runtime"
	"sync"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/venue"
	"github.com/theapemachine/symm/venue/kraken"
)

func TestDeskUpdateNotesTape(t *testing.T) {
	Convey("a trade's quote dollars land on the price tape", t, func() {
		price := &Price{Books: NewBookManager(), tape: NewTape()}
		desk := &Desk{price: price}
		at := time.Now()

		desk.Update([]*market.TradeData{{
			Symbol:    "AAA/USD",
			Price:     decimal.RequireFromString("10"),
			Qty:       decimal.RequireFromString("25"),
			Timestamp: at,
		}})

		recent, prior := price.TapeUptake("AAA/USD", at)
		So(recent, ShouldEqual, 250)
		So(prior, ShouldEqual, 0)
	})
}

func TestNewDeskLiveOnly(t *testing.T) {
	Convey("real model arms only the live tier", t, func() {
		// Nil private keeps Live disarmed; we only assert which tiers exist.
		desk := DeskFor(context.Background(), map[string]network.Client{
			"public": &deskTransportStub{},
		}, nil, nil, true)

		So(len(desk.tiers), ShouldEqual, 1)
		So(desk.tiers[TierTypeReal], ShouldNotBeNil)
		So(desk.tiers[TierTypeVirtual], ShouldBeNil)
		So(desk.tiers[TierTypePaper], ShouldBeNil)
	})
}

func TestDeskApplyRoutesExecutions(t *testing.T) {
	Convey("Apply fans fills into every tier", t, func() {
		tier := &Tier{positions: newTestCache("real")}
		desk := &Desk{tiers: map[TierType]*Tier{TierTypeReal: tier}}

		open := NewPosition(context.Background(), "BTC/USD", "real", "path", nil, nil, nil)
		open.EntryOrder = &spot.OrderRequest{ClOrdId: "desk-cl", Type: "buy"}
		open.Apply(tradeFill("b1", "buy", "1", "100"))
		tier.positions.Add("BTC/USD", open, true)

		qty := decimal.RequireFromString("0.5")
		px := decimal.RequireFromString("101")
		desk.Apply(&market.Execution{
			Data: []*market.ExecutionData{{
				ExecID:        "add",
				ExecType:      "trade",
				ClientOrderID: "desk-cl",
				Symbol:        "BTC/USD",
				Side:          "buy",
				LastQty:       &qty,
				LastPrice:     &px,
			}},
		})

		So(open.Qty.String(), ShouldEqual, "1.5")
	})
}

func TestDeskMarkSkipsIdleSymbols(t *testing.T) {
	Convey("trade-driven marks reach tiers only when a lot is open", t, func() {
		fixture := newPriceFixture(t, priceFixture{
			symbol: "DMK/USD", cash: 10_000,
			qtyMinRaw: "0.01", costMinRaw: "1", step: "0.01",
			bidPx: 100, bidQty: 10, askPx: 101, askQty: 10, feePct: 0,
		})

		tier := newTestTier(t, TierTypeVirtual, fixture.price, nil)
		desk := &Desk{price: fixture.price, tiers: map[TierType]*Tier{TierTypeVirtual: tier}}

		at := time.Now().UTC()
		desk.Update([]*market.TradeData{{
			Symbol:    fixture.symbol,
			Price:     decimal.NewFromInt(50),
			Qty:       decimal.NewFromInt(1),
			Timestamp: at,
		}})

		So(desk.Has(fixture.symbol), ShouldBeFalse)

		position := NewPosition(
			context.Background(), fixture.symbol, "virtual", "path",
			fixture.price, nil, nil,
		)
		position.Apply(tradeFill("b1", "buy", "1", "100"))
		tier.positions.Add(fixture.symbol, position, true)

		desk.Update([]*market.TradeData{{
			Symbol:    fixture.symbol,
			Price:     decimal.NewFromInt(105),
			Qty:       decimal.NewFromInt(1),
			Timestamp: at,
		}})

		So(position.Mark, ShouldNotBeNil)
		So(position.Mark.String(), ShouldEqual, "100")
		So(position.PnL, ShouldNotBeNil)
	})
}

func TestDeskRoutingHelpers(t *testing.T) {
	Convey("desk helpers route through tiers", t, func() {
		fixture := newPriceFixture(t, priceFixture{
			symbol: "RT/USD", cash: 10_000,
			qtyMinRaw: "0.01", costMinRaw: "1", step: "0.01",
			bidPx: 100, bidQty: 10, askPx: 101, askQty: 10, feePct: 0,
		})

		virtual := newTestTier(t, TierTypeVirtual, fixture.price, nil)
		desk := &Desk{price: fixture.price, tiers: map[TierType]*Tier{TierTypeVirtual: virtual}}

		awaiting := NewPosition(context.Background(), "RT/USD", "virtual", "path", fixture.price, nil, nil)
		awaiting.EntryOrder = &spot.OrderRequest{ClOrdId: "pend", Type: "buy", Volume: "1"}
		virtual.positions.Add("RT/USD", awaiting, true)

		So(desk.Has("RT/USD"), ShouldBeTrue)
		So(desk.AwaitingFill("RT/USD"), ShouldBeTrue)
		So(desk.CanSize("RT/USD"), ShouldBeTrue)
		So(desk.BookStale("RT/USD"), ShouldBeFalse)

		n := 0
		for range desk.Positions(TierTypeVirtual, "RT/USD") {
			n++
		}
		So(n, ShouldEqual, 0)

		virtual.pending.Store("RT/USD", awaiting)
		pending := 0
		for range desk.Pending(TierTypeVirtual) {
			pending++
		}
		So(pending, ShouldEqual, 1)

		desk.ClearPending(TierTypeVirtual, "RT/USD")
		_, ok := virtual.pending.Load("RT/USD")
		So(ok, ShouldBeFalse)
	})
}

func TestDeskRefreshMarksOpenLots(t *testing.T) {
	Convey("Refresh pushes touch marks through tiers", t, func() {
		fixture := newPriceFixture(t, priceFixture{
			symbol: "REF/USD", cash: 10_000,
			qtyMinRaw: "0.01", costMinRaw: "1", step: "0.01",
			bidPx: 50, bidQty: 10, askPx: 51, askQty: 10, feePct: 0,
		})

		tier := newTestTier(t, TierTypeVirtual, fixture.price, nil)
		desk := &Desk{price: fixture.price, tiers: map[TierType]*Tier{TierTypeVirtual: tier}}

		pos := NewPosition(context.Background(), "REF/USD", "virtual", "path", fixture.price, nil, nil)
		pos.Apply(tradeFill("b1", "buy", "1", "48"))
		tier.positions.Add("REF/USD", pos, true)

		desk.Refresh([]string{"REF/USD"})

		So(pos.Mark.String(), ShouldEqual, "50")
		So(pos.PnL, ShouldNotBeNil)
	})
}

func TestDeskExitDelegates(t *testing.T) {
	Convey("Exit closes a virtual lot through the tier", t, func() {
		fixture := newPriceFixture(t, priceFixture{
			symbol: "EX/USD", cash: 10_000,
			qtyMinRaw: "0.01", costMinRaw: "1", step: "0.01",
			bidPx: 100, bidQty: 10, askPx: 101, askQty: 10, feePct: 0,
		})

		virtual := newTestTier(t, TierTypeVirtual, fixture.price, nil)
		desk := &Desk{price: fixture.price, tiers: map[TierType]*Tier{TierTypeVirtual: virtual}}

		So(desk.Enter("EX/USD", "R00", TierTypeVirtual, true), ShouldBeNil)

		var pos *Position
		for p := range desk.Positions(TierTypeVirtual, "EX/USD") {
			pos = p
		}
		So(pos, ShouldNotBeNil)

		So(desk.Exit(TierTypeVirtual, pos), ShouldBeNil)
		So(pos.Flat(), ShouldBeTrue)
		So(desk.Has("EX/USD"), ShouldBeFalse)
	})
}

func TestDeskEnterRejectsUnknownTier(t *testing.T) {
	Convey("Enter surfaces missing tiers", t, func() {
		desk := &Desk{tiers: map[TierType]*Tier{}}
		So(desk.Enter("BTC/USD", "R00", TierTypePaper, true), ShouldNotBeNil)
	})
}

type deskTransportStub struct{}

func (*deskTransportStub) Read(...string) ([]byte, error) { return nil, nil }
func (*deskTransportStub) Write([]byte) error             { return nil }
func (*deskTransportStub) Post(string, any) ([]byte, error) {
	return nil, nil
}
func (*deskTransportStub) Watch(string, any, json.Unmarshaler) {}

func newPumpDesk(t *testing.T, fixture priceFixture) (*Desk, context.CancelFunc) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	tier := newTestTier(t, TierTypeVirtual, fixture.price, nil)

	desk := &Desk{
		System:   runtime.NewSystem(ctx, "desk"),
		price:    fixture.price,
		tiers:    map[TierType]*Tier{TierTypeVirtual: tier},
		level3Ch: make(chan *market.Level3, 4),
		tradeCh:  make(chan *market.Trade, 4),
	}

	go desk.pumpLevel3()
	go desk.pumpTrade()

	return desk, cancel
}

func newManualPumpDesk(t *testing.T, fixture priceFixture) *Desk {
	t.Helper()

	ctx := context.Background()
	tier := newTestTier(t, TierTypeVirtual, fixture.price, nil)

	return &Desk{
		System:   runtime.NewSystem(ctx, "desk"),
		price:    fixture.price,
		tiers:    map[TierType]*Tier{TierTypeVirtual: tier},
		level3Ch: make(chan *market.Level3, 4),
		tradeCh:  make(chan *market.Trade, 4),
	}
}

func drainDeskLevel3(desk *Desk) {
	book := <-desk.level3Ch

	if desk.price != nil && desk.price.Books != nil {
		desk.price.Books.Update(book)
	}

	symbols := make([]string, 0, len(book.Data))
	for _, row := range book.Data {
		if row != nil && row.Symbol != "" {
			symbols = append(symbols, row.Symbol)
		}
	}

	desk.Refresh(symbols)
}

func drainDeskTrade(desk *Desk) {
	trade := <-desk.tradeCh
	desk.Update(trade.Data)

	if desk.onTrade != nil {
		desk.onTrade(trade)
	}
}

func TestDeskPumpAndFeed(t *testing.T) {
	Convey("pumpLevel3 applies books and Refresh marks", t, func() {
		fixture := newPriceFixture(t, priceFixture{
			symbol: "PMP/USD", cash: 10_000,
			qtyMinRaw: "0.01", costMinRaw: "1", step: "0.01",
			bidPx: 40, bidQty: 10, askPx: 41, askQty: 10, feePct: 0,
		})

		desk := newManualPumpDesk(t, fixture)

		pos := NewPosition(context.Background(), "PMP/USD", "virtual", "path", fixture.price, nil, nil)
		pos.Apply(tradeFill("b1", "buy", "1", "38"))
		desk.tiers[TierTypeVirtual].positions.Add("PMP/USD", pos, true)

		bidPx := decimal.NewFromInt(55)
		bidQty := decimal.NewFromInt(5)
		askPx := decimal.NewFromInt(56)
		askQty := decimal.NewFromInt(5)

		book := market.NewLevel3()
		book.Data = []*market.Level3Data{{
			Symbol: "PMP/USD",
			Bids: []*market.Level3Order{{
				LimitPrice: &bidPx,
				OrderQty:   &bidQty,
			}},
			Asks: []*market.Level3Order{{
				LimitPrice: &askPx,
				OrderQty:   &askQty,
			}},
		}}

		book.Type = "snapshot"
		desk.enqueueLevel3(book)
		drainDeskLevel3(desk)

		So(pos.Mark, ShouldNotBeNil)
		So(pos.Mark.String(), ShouldEqual, "55")
	})

	Convey("Feed decodes level3 frames into the pump", t, func() {
		fixture := newPriceFixture(t, priceFixture{
			symbol: "FD/USD", cash: 10_000,
			qtyMinRaw: "0.01", costMinRaw: "1", step: "0.01",
			bidPx: 1, bidQty: 1, askPx: 2, askQty: 1, feePct: 0,
		})

		desk := newManualPumpDesk(t, fixture)

		raw := []byte(`{
			"channel":"level3",
			"type":"snapshot",
			"data":[{
				"symbol":"FD/USD",
				"bids":[{"order_id":"1","limit_price":70,"order_qty":3}],
				"asks":[{"order_id":"2","limit_price":71,"order_qty":3}]
			}]
		}`)

		desk.Feed(raw)
		drainDeskLevel3(desk)

		touch := fixture.price.TouchBid("FD/USD")
		So(touch, ShouldNotBeNil)
		So(touch.String(), ShouldEqual, "70")
	})

	Convey("pumpTrade fans prints and OnTrade", t, func() {
		fixture := newPriceFixture(t, priceFixture{
			symbol: "TR/USD", cash: 10_000,
			qtyMinRaw: "0.01", costMinRaw: "1", step: "0.01",
			bidPx: 10, bidQty: 10, askPx: 11, askQty: 10, feePct: 0,
		})

		desk := newManualPumpDesk(t, fixture)

		var seen *market.Trade
		desk.OnTrade(func(trade *market.Trade) { seen = trade })

		px := decimal.NewFromInt(12)
		qty := decimal.NewFromInt(2)
		at := time.Now().UTC()

		desk.enqueueTrade(&market.Trade{Data: []*market.TradeData{{
			Symbol: "TR/USD", Price: px, Qty: qty, Timestamp: at,
		}}})
		drainDeskTrade(desk)

		So(seen, ShouldNotBeNil)

		recent, prior := fixture.price.TapeUptake("TR/USD", at)
		So(recent, ShouldEqual, 24)
		So(prior, ShouldEqual, 0)
	})

	Convey("enqueueTrade drops oldest when the ring is full", t, func() {
		desk := &Desk{tradeCh: make(chan *market.Trade, 2)}

		first := time.Unix(1, 0).UTC()
		second := time.Unix(2, 0).UTC()
		third := time.Unix(3, 0).UTC()

		desk.tradeCh <- &market.Trade{Data: []*market.TradeData{{
			Symbol: "DR/USD", Price: decimal.NewFromInt(1), Qty: decimal.NewFromInt(1), Timestamp: first,
		}}}
		desk.tradeCh <- &market.Trade{Data: []*market.TradeData{{
			Symbol: "DR/USD", Price: decimal.NewFromInt(2), Qty: decimal.NewFromInt(1), Timestamp: second,
		}}}

		desk.enqueueTrade(&market.Trade{Data: []*market.TradeData{{
			Symbol: "DR/USD", Price: decimal.NewFromInt(3), Qty: decimal.NewFromInt(1), Timestamp: third,
		}}})

		So(len(desk.tradeCh), ShouldEqual, 2)

		older := <-desk.tradeCh
		newest := <-desk.tradeCh

		So(older.Data[0].Timestamp, ShouldEqual, second)
		So(newest.Data[0].Timestamp, ShouldEqual, third)
	})
}

func TestDeskHelloAndFlush(t *testing.T) {
	Convey("helloFrame aggregates open lots and tier balances", t, func() {
		fixture := newPriceFixture(t, priceFixture{
			symbol: "HL/USD", cash: 10_000,
			qtyMinRaw: "0.01", costMinRaw: "1", step: "0.01",
			bidPx: 55, bidQty: 10, askPx: 56, askQty: 10, feePct: 0,
		})

		virtual := newTestTier(t, TierTypeVirtual, fixture.price, venueKrakenWithCash("USD", "10000"))
		desk := &Desk{price: fixture.price, tiers: map[TierType]*Tier{TierTypeVirtual: virtual}}

		pos := NewPosition(context.Background(), "HL/USD", "virtual", "path", fixture.price, nil, nil)
		pos.Apply(tradeFill("b1", "buy", "1", "10"))
		virtual.positions.Add("HL/USD", pos, true)

		touch := fixture.price.TouchBid("HL/USD")
		So(touch, ShouldNotBeNil)
		So(pos.Update(touch), ShouldBeNil)

		frame := desk.helloFrame()
		positions := frame["positions"].(map[string]*Position)
		balances := frame["balances"].(map[string]any)

		So(positions[pos.ID], ShouldEqual, pos)
		So(pos.Qty.String(), ShouldEqual, "1")

		openPnL, _ := pos.PnL.Float64()
		So(openPnL, ShouldBeGreaterThan, 40)

		virtualReading := balances["virtual"].(map[string]float64)
		So(virtualReading["cash"], ShouldEqual, 10_000)
		So(virtualReading["unrealized"], ShouldAlmostEqual, openPnL, 0.001)
		So(virtualReading["equity"], ShouldAlmostEqual, virtualReading["cash"]+openPnL, 0.01)
	})

	Convey("Flush persists submitted lots", t, func() {
		backing := &memCacheStore{}
		fixture := newPriceFixture(t, priceFixture{
			symbol: "FL/USD", cash: 10_000,
			qtyMinRaw: "0.01", costMinRaw: "1", step: "0.01",
			bidPx: 10, bidQty: 10, askPx: 11, askQty: 10, feePct: 0,
		})

		cache := &Cache{
			System:  runtime.NewSystem(context.Background(), "cache"),
			tierKey: "virtual",
			store:   backing,
			open:    &sync.Map{},
			closed:  &sync.Map{},
			archive: &sync.Map{},
		}

		virtual := &Tier{Type: TierTypeVirtual, price: fixture.price, positions: cache}
		desk := &Desk{price: fixture.price, tiers: map[TierType]*Tier{TierTypeVirtual: virtual}}

		pos := NewPosition(context.Background(), "FL/USD", "virtual", "path", fixture.price, nil, nil)
		pos.ID = "flush-me"
		pos.EntryOrder = &spot.OrderRequest{ClOrdId: "pend", Type: "buy", Volume: "1"}
		cache.open.Store("FL/USD", pos)

		So(desk.Flush(), ShouldBeNil)
		_, ok := backing.blob["virtual/open/FL/USD/flush-me"]
		So(ok, ShouldBeTrue)
	})
}

type privateWireClient struct {
	writes int
	last   []byte
	pages  map[string][]byte
	posts  []string
}

func (client *privateWireClient) Read(...string) ([]byte, error) { return nil, nil }

func (client *privateWireClient) Write(buf []byte) error {
	client.writes++
	client.last = append([]byte(nil), buf...)
	return nil
}

func (client *privateWireClient) Post(url string, body any) ([]byte, error) {
	client.posts = append(client.posts, url)

	if client.pages != nil {
		if raw, ok := client.pages[url]; ok {
			return raw, nil
		}
	}

	return []byte(`{"error":[],"result":{"count":"0","trades":{},"cursor":{"next":""}}}`), nil
}

func (client *privateWireClient) Watch(string, any, json.Unmarshaler) {}

func TestDeskRunStartOpenPrivate(t *testing.T) {
	Convey("Run and Start exit when the desk context is canceled", t, func() {
		ctx, cancel := context.WithCancel(context.Background())

		reads := &cancelReadClient{
			ctx: ctx,
			frames: [][]byte{
				[]byte(`{"channel":"heartbeat"}`),
			},
		}

		desk := &Desk{
			System: runtime.NewSystem(ctx, "desk"),
			public: reads,
			venue:  venue.NewKraken("USD"),
		}

		done := make(chan struct{})
		go func() {
			desk.Run()
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
			t.Fatal("desk.Run did not exit")
		}

		ctx2, cancel2 := context.WithCancel(context.Background())
		desk2 := &Desk{
			System: runtime.NewSystem(ctx2, "desk"),
			public: &cancelReadClient{ctx: ctx2},
			venue:  venue.NewKraken("USD"),
		}

		desk2.Start()
		cancel2()

		select {
		case <-ctx2.Done():
			So(ctx2.Err(), ShouldEqual, context.Canceled)
		case <-time.After(2 * time.Second):
			t.Fatal("desk context did not cancel")
		}
	})

	Convey("OpenPrivate fans into armed tiers", t, func() {
		balURL := system.Cfg.WebSocket.Endpoints.Balance
		tradeURL := system.Cfg.WebSocket.Endpoints.TradeBalance
		histURL := system.Cfg.WebSocket.Endpoints.TradesHistory

		client := &privateWireClient{pages: map[string][]byte{
			balURL:   []byte(`{"error":[],"result":{"USD":"100"}}`),
			tradeURL: []byte(`{"error":[],"result":{"eb":"100","tb":"100","m":"0","n":"0","c":"0","v":"0","e":"100"}}`),
			histURL:  []byte(`{"error":[],"result":{"count":"0","trades":{},"cursor":{"next":""}}}`),
		}}

		auth := kraken.NewAuth()
		auth.SeedToken("desk-token")

		instrument := &Instrument{cache: &sync.Map{}, takerFees: &sync.Map{}, auth: auth}
		host := venueKrakenWithCash("USD", "100")
		tier := &Tier{
			ctx:       context.Background(),
			Type:      TierTypeReal,
			private:   client,
			venue:     host,
			price:     &Price{Instrument: instrument},
			positions: NewCache(context.Background(), "real", client, nil, nil, nil),
		}

		desk := &Desk{tiers: map[TierType]*Tier{TierTypeReal: tier}}

		So(desk.OpenPrivate(), ShouldBeNil)
		So(client.writes, ShouldBeGreaterThanOrEqualTo, 2)
	})
}
