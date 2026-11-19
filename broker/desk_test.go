package broker

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/system"
)

func fundDeskBalances(desk *Desk, wallet *market.Balance) {
	if desk == nil || wallet == nil {
		return
	}

	if desk.price != nil && desk.price.balance != nil {
		wallet.ToMarket(desk.price.balance)
	}

	for _, tier := range desk.tiers {
		if tier != nil && tier.balance != nil {
			wallet.ToMarket(tier.balance)
		}
	}
}

func withDesk(ctx context.Context, live bool, f func(*Desk)) func() {
	return func() {
		withPrice(func(price *Price) {
			prev := system.Cfg.Market.Model
			if live {
				system.Cfg.Market.Model = "real"
			} else {
				system.Cfg.Market.Model = "paper"
			}
			defer func() { system.Cfg.Market.Model = prev }()

			stub := &historyClient{trades: []byte(`{"error":[],"result":{"trades":{}}}`)}
			wallet := price.balance
			desk := NewDesk(ctx, map[string]network.Client{
				"public":  stub,
				"private": stub,
				"paper":   stub,
				"virtual": stub,
			}, price, nil)
			fundDeskBalances(desk, wallet)
			f(desk)
		})()
	}
}

func TestNewDesk(t *testing.T) {
	Convey("Setup", t, withDesk(t.Context(), false, func(desk *Desk) {
		Convey("When paper mode builds three tiers", func() {
			So(desk, ShouldNotBeNil)
			So(desk.tiers[TierTypeVirtual], ShouldNotBeNil)
			So(desk.tiers[TierTypePaper], ShouldNotBeNil)
			So(desk.tiers[TierTypeReal], ShouldNotBeNil)
			So(desk.live, ShouldBeFalse)
		})
	}))
}

func TestNewDeskLive(t *testing.T) {
	Convey("Setup", t, withDesk(t.Context(), true, func(desk *Desk) {
		Convey("When live mode arms only real", func() {
			So(desk.tiers[TierTypeReal], ShouldNotBeNil)
			So(desk.tiers[TierTypePaper], ShouldBeNil)
			So(desk.tiers[TierTypeVirtual], ShouldBeNil)
			So(desk.live, ShouldBeTrue)
		})
	}))
}

func TestDeskOpenCount(t *testing.T) {
	Convey("Setup", t, withDesk(t.Context(), false, func(desk *Desk) {
		paper := desk.tiers[TierTypePaper]
		pos := NewPosition(t.Context(), testSymbol, "paper", "path", desk.price, nil, nil)
		pos.Apply(market.ExecutionData{
			ExecID: "b1", Side: "buy",
			LastQty: dec("1"), LastPrice: dec("100"),
			Timestamp: time.Unix(1, 0).UTC(),
		})
		paper.positions.Add(testSymbol, pos, true)

		Convey("When one paper position is open", func() {
			So(desk.OpenCount(TierTypePaper), ShouldEqual, 1)
			So(desk.OpenCount(TierTypeReal), ShouldEqual, 0)
		})
	}))
}

func TestDeskHas(t *testing.T) {
	Convey("Setup", t, withDesk(t.Context(), false, func(desk *Desk) {
		Convey("When no tier holds the symbol", func() {
			So(desk.Has(testSymbol), ShouldBeFalse)
		})

		Convey("When paper holds an open row", func() {
			paper := desk.tiers[TierTypePaper]
			pos := NewPosition(t.Context(), testSymbol, "paper", "path", desk.price, nil, nil)
			paper.positions.Add(testSymbol, pos, true)

			So(desk.Has(testSymbol), ShouldBeTrue)
		})
	}))
}

func TestDeskPosition(t *testing.T) {
	Convey("Setup", t, withDesk(t.Context(), false, func(desk *Desk) {
		Convey("When the open row lives on virtual", func() {
			pos := NewPosition(t.Context(), testSymbol, "virtual", "path", desk.price, nil, nil)
			desk.tiers[TierTypeVirtual].positions.Add(testSymbol, pos, true)

			Convey("Paper lookup misses while virtual and Has hit", func() {
				So(desk.Position(TierTypePaper, testSymbol), ShouldBeNil)
				So(desk.Position(TierTypeVirtual, testSymbol), ShouldEqual, pos)
				So(desk.Has(testSymbol), ShouldBeTrue)
			})
		})
	}))
}

func TestDeskHello(t *testing.T) {
	Convey("Setup", t, withDesk(t.Context(), false, func(desk *Desk) {
		Convey("When Hello fans out to tiers", func() {
			So(func() { desk.Hello() }, ShouldNotPanic)
		})
	}))
}

func TestDeskUpdate(t *testing.T) {
	Convey("Setup", t, withDesk(t.Context(), false, func(desk *Desk) {
		paper := desk.tiers[TierTypePaper]
		pos := NewPosition(t.Context(), testSymbol, "paper", "path", desk.price, nil, nil)
		seedMarkable(pos)
		pos.Apply(market.ExecutionData{
			ExecID: "b1", Side: "buy",
			LastQty: dec("1"), LastPrice: dec("100"),
			Timestamp: time.Unix(1, 0).UTC(),
		})
		paper.positions.Add(testSymbol, pos, true)

		Convey("When a trade marks open positions", func() {
			desk.Update([]*market.TradeData{{Symbol: testSymbol}})

			Convey("Paper position PnL matches the bid mark", func() {
				So(pos.Mark.String(), ShouldEqual, "100")
				So(pos.PnL.String(), ShouldEqual, "-0.8")
			})
		})
	}))
}

func TestDeskEnter(t *testing.T) {
	Convey("Setup", t, withDesk(t.Context(), false, func(desk *Desk) {
		Convey("When Enter routes to paper", func() {
			err := desk.Enter(testSymbol, "R00/M1", TierTypePaper, true, SizeHint{})

			Convey("Paper cache holds the symbol", func() {
				So(err, ShouldBeNil)
				So(desk.tiers[TierTypePaper].positions.Has(testSymbol), ShouldBeTrue)
			})
		})

		Convey("When Enter targets an unknown tier", func() {
			err := desk.Enter(testSymbol, "R00/M1", TierType(99), true, SizeHint{})

			Convey("Desk returns not-found", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "[desk.Enter] unknown tier")
			})
		})
	}))
}

func TestDeskExit(t *testing.T) {
	Convey("Setup", t, withDesk(t.Context(), false, func(desk *Desk) {
		paper := desk.tiers[TierTypePaper]
		pos := NewPosition(t.Context(), testSymbol, "paper", "path", desk.price, nil, nil)
		seedMarkable(pos)
		pos.ExitPath = "R00/M1"
		pos.Apply(market.ExecutionData{
			ExecID: "b1", Side: "buy",
			LastQty: dec("1"), LastPrice: dec("100"),
			Timestamp: time.Unix(1, 0).UTC(),
		})
		paper.positions.Add(testSymbol, pos, true)

		Convey("When Exit routes through paper", func() {
			setBook(desk.price, "120", "121", "30")
			paper.Update(testSymbol, dec("120"))
			err := desk.Exit(pos, false)

			Convey("Closing is armed", func() {
				So(err, ShouldBeNil)
				So(pos.Status() == runtime.BUSY, ShouldBeTrue)
			})
		})
	}))
}

func TestDeskForceExit(t *testing.T) {
	Convey("Setup", t, withDesk(t.Context(), false, func(desk *Desk) {
		paper := desk.tiers[TierTypePaper]
		pos := NewPosition(t.Context(), testSymbol, "paper", "path", desk.price, nil, nil)
		pos.Apply(market.ExecutionData{
			ExecID: "b1", Side: "buy",
			LastQty: dec("1"), LastPrice: dec("100"),
			Timestamp: time.Unix(1, 0).UTC(),
		})
		paper.positions.Add(testSymbol, pos, true)

		Convey("When Exit force dumps an underwater row", func() {
			setBook(desk.price, "100", "101", "30")
			err := desk.Exit(pos, true)

			Convey("Manual exit arms BUSY", func() {
				So(err, ShouldBeNil)
				So(pos.Status() == runtime.BUSY, ShouldBeTrue)
				So(pos.ExitTrigger, ShouldEqual, "manual")
			})
		})
	}))
}

func TestDeskPositions(t *testing.T) {
	Convey("Setup", t, withDesk(t.Context(), false, func(desk *Desk) {
		paper := desk.tiers[TierTypePaper]
		pos := NewPosition(t.Context(), testSymbol, "paper", "path", desk.price, nil, nil)
		pos.Apply(market.ExecutionData{
			ExecID: "b1", Side: "buy",
			LastQty: dec("1"), LastPrice: dec("100"),
			Timestamp: time.Unix(1, 0).UTC(),
		})
		paper.positions.Add(testSymbol, pos, true)

		Convey("When Positions iterates paper opens", func() {
			var seen []*Position
			for p := range desk.Positions(TierTypePaper, testSymbol) {
				seen = append(seen, p)
			}

			Convey("The filled row is yielded", func() {
				So(len(seen), ShouldEqual, 1)
				So(seen[0].ID, ShouldEqual, pos.ID)
			})
		})
	}))
}

func TestDeskPending(t *testing.T) {
	Convey("Setup", t, withDesk(t.Context(), false, func(desk *Desk) {
		paper := desk.tiers[TierTypePaper]
		pos := NewPosition(t.Context(), testSymbol, "paper", "path", desk.price, nil, nil)
		paper.pending.Store(testSymbol, pos)

		Convey("When Pending walks the tier map", func() {
			var seen []*Position
			for p := range desk.Pending(TierTypePaper) {
				seen = append(seen, p)
			}

			Convey("The pending handle is visible", func() {
				So(len(seen), ShouldEqual, 1)
				So(seen[0].ID, ShouldEqual, pos.ID)
			})
		})
	}))
}

func TestDeskClearPending(t *testing.T) {
	Convey("Setup", t, withDesk(t.Context(), false, func(desk *Desk) {
		paper := desk.tiers[TierTypePaper]
		pos := NewPosition(t.Context(), testSymbol, "paper", "path", desk.price, nil, nil)
		paper.pending.Store(testSymbol, pos)

		Convey("When ClearPending runs at desk scope", func() {
			desk.ClearPending(TierTypePaper, testSymbol)

			Convey("Pending is empty", func() {
				n := 0
				for range desk.Pending(TierTypePaper) {
					n++
				}
				So(n, ShouldEqual, 0)
			})
		})
	}))
}

func TestDeskFlush(t *testing.T) {
	Convey("Setup", t, withDesk(t.Context(), false, func(desk *Desk) {
		paper := desk.tiers[TierTypePaper]
		pos := NewPosition(t.Context(), testSymbol, "paper", "path", desk.price, nil, nil)
		paper.positions.Add(testSymbol, pos, true)

		Convey("When Flush persists open rows", func() {
			err := desk.Flush()

			Convey("Nil store save succeeds", func() {
				So(err, ShouldBeNil)
			})
		})
	}))
}

func TestDeskApply(t *testing.T) {
	Convey("Setup", t, withDesk(t.Context(), false, func(desk *Desk) {
		paper := desk.tiers[TierTypePaper]
		pos := NewPosition(t.Context(), testSymbol, "paper", "path", desk.price, nil, nil)
		paper.positions.Add(testSymbol, pos, true)

		Convey("When execution frames land on every tier", func() {
			pos.EntryOrder = &spot.OrderRequest{ClOrdId: "enter-1"}
			desk.Apply(&market.Execution{
				Data: []*market.ExecutionData{{
					ExecID: "b1", Side: "buy", Symbol: testSymbol,
					ClientOrderID: "enter-1",
					LastQty:       dec("1"), LastPrice: dec("100"),
					Timestamp: time.Unix(1, 0).UTC(),
				}},
			})

			Convey("Paper position records the fill", func() {
				So(pos.Qty.String(), ShouldEqual, "1")
				So(pos.Entry.String(), ShouldEqual, "100")
			})
		})

		Convey("When Apply gets nil", func() {
			So(func() { desk.Apply(nil) }, ShouldNotPanic)
		})
	}))
}

func TestDeskFeed(t *testing.T) {
	Convey("Setup", t, withDesk(t.Context(), false, func(desk *Desk) {
		Convey("When Feed gets a level3 frame", func() {
			desk.Feed([]byte(`{
				"channel":"level3",
				"type":"snapshot",
				"data":[{
					"symbol":"BTC/USD",
					"checksum":0,
					"bids":[{"order_id":"b1","limit_price":100,"order_qty":10,"timestamp":"2024-01-02T12:00:00Z"}],
					"asks":[{"order_id":"a1","limit_price":101,"order_qty":10,"timestamp":"2024-01-02T12:00:00Z"}]
				}]
			}`))

			Convey("The book is enqueued and applied", func() {
				deadline := time.Now().Add(2 * time.Second)
				for time.Now().Before(deadline) {
					if desk.price.Books.Get(testSymbol) != nil {
						break
					}
					time.Sleep(5 * time.Millisecond)
				}
				So(desk.price.Books.Get(testSymbol), ShouldNotBeNil)
			})
		})

		Convey("When Feed gets a non-level3 frame", func() {
			So(func() { desk.Feed([]byte(`{"channel":"trade"}`)) }, ShouldNotPanic)
		})

		Convey("When Feed gets invalid JSON", func() {
			So(func() { desk.Feed([]byte(`not-json`)) }, ShouldNotPanic)
		})
	}))
}

func TestDeskOnTrade(t *testing.T) {
	Convey("Setup", t, withDesk(t.Context(), false, func(desk *Desk) {
		seen := make(chan *market.Trade, 1)
		desk.OnTrade(func(trade *market.Trade) { seen <- trade })

		Convey("When a trade is enqueued", func() {
			trade := market.NewTrade("", "", nil)
			trade.Data = []*market.TradeData{{
				Symbol: testSymbol,
				Price:  decimal.RequireFromString("100"),
				Qty:    decimal.RequireFromString("1"),
			}}
			desk.enqueueTrade(trade)

			Convey("OnTrade receives the print", func() {
				select {
				case got := <-seen:
					So(got, ShouldEqual, trade)
				case <-time.After(2 * time.Second):
					So("onTrade", ShouldEqual, "timed out")
				}
			})
		})

		Convey("When enqueueTrade gets nil", func() {
			So(func() { desk.enqueueTrade(nil) }, ShouldNotPanic)
		})

		Convey("When the trade ring is full", func() {
			desk.OnTrade(nil)
			for i := 0; i < tradeQueueFrames+2; i++ {
				desk.enqueueTrade(market.NewTrade("", "", nil))
			}

			Convey("Oldest frames are dropped without blocking", func() {
				So(len(desk.tradeCh) <= tradeQueueFrames, ShouldBeTrue)
			})
		})
	}))
}

func TestDeskIngest(t *testing.T) {
	Convey("Setup", t, withDesk(t.Context(), false, func(desk *Desk) {
		seen := make(chan struct{}, 1)
		desk.OnTrade(func(*market.Trade) { seen <- struct{}{} })

		Convey("When ingest gets a kraken trade frame", func() {
			raw := []byte(`{
				"channel":"trade",
				"type":"update",
				"data":[{
					"symbol":"BTC/USD",
					"side":"buy",
					"price":"100",
					"qty":1,
					"ord_type":"market",
					"trade_id":1,
					"timestamp":"2024-01-02T12:00:00Z"
				}]
			}`)
			desk.ingest(desk.venue.Read(raw))

			Convey("The trade pump fires OnTrade", func() {
				select {
				case <-seen:
				case <-time.After(2 * time.Second):
					So("ingest trade", ShouldEqual, "timed out")
				}
			})
		})

		Convey("When ingest gets nil", func() {
			So(func() { desk.ingest(nil) }, ShouldNotPanic)
		})
	}))
}

func TestDeskStartRun(t *testing.T) {
	Convey("Setup", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		withDesk(ctx, false, func(desk *Desk) {
			block := &blockUntilDoneClient{done: ctx.Done()}
			desk.public = block
			for _, tier := range desk.tiers {
				if tier != nil {
					tier.private = block
				}
			}

			Convey("When Start launches Run and context cancels", func() {
				desk.Start()
				cancel()
				time.Sleep(50 * time.Millisecond)

				Convey("Run exits without panic", func() {
					So(true, ShouldBeTrue)
				})
			})
		})()
	})
}

type blockUntilDoneClient struct {
	done <-chan struct{}
}

func (c *blockUntilDoneClient) Read(...string) ([]byte, error) {
	<-c.done
	return nil, context.Canceled
}
func (c *blockUntilDoneClient) Write([]byte) error { return nil }
func (c *blockUntilDoneClient) Post(string, any) ([]byte, error) {
	return []byte(`{"error":[],"result":{"trades":{}}}`), nil
}
func (c *blockUntilDoneClient) Watch(string, any, json.Unmarshaler, func()) {}

func BenchmarkNewDesk(b *testing.B) {
	ctx := context.Background()
	var price *Price
	withPrice(func(p *Price) { price = p })()
	prev := system.Cfg.Market.Model
	system.Cfg.Market.Model = "paper"
	defer func() { system.Cfg.Market.Model = prev }()

	b.ReportAllocs()

	for b.Loop() {
		_ = NewDesk(ctx, map[string]network.Client{}, price, nil)
	}
}

func BenchmarkNewDeskLive(b *testing.B) {
	ctx := context.Background()
	var price *Price
	withPrice(func(p *Price) { price = p })()
	prev := system.Cfg.Market.Model
	system.Cfg.Market.Model = "real"
	defer func() { system.Cfg.Market.Model = prev }()

	b.ReportAllocs()

	for b.Loop() {
		_ = NewDesk(ctx, map[string]network.Client{}, price, nil)
	}
}

func BenchmarkDeskOpenCount(b *testing.B) {
	ctx := context.Background()
	var desk *Desk
	withDesk(ctx, false, func(d *Desk) {
		desk = d
		paper := d.tiers[TierTypePaper]
		pos := NewPosition(ctx, testSymbol, "paper", "path", d.price, nil, nil)
		pos.Apply(market.ExecutionData{
			ExecID: "b1", Side: "buy",
			LastQty: dec("1"), LastPrice: dec("100"),
			Timestamp: time.Unix(1, 0).UTC(),
		})
		paper.positions.Add(testSymbol, pos, true)
	})()

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = desk.OpenCount(TierTypePaper)
	}
}

func BenchmarkDeskHas(b *testing.B) {
	ctx := context.Background()
	var desk *Desk
	withDesk(ctx, false, func(d *Desk) {
		desk = d
		paper := d.tiers[TierTypePaper]
		pos := NewPosition(ctx, testSymbol, "paper", "path", d.price, nil, nil)
		paper.positions.Add(testSymbol, pos, true)
	})()

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = desk.Has(testSymbol)
	}
}

func BenchmarkDeskHello(b *testing.B) {
	ctx := context.Background()
	var desk *Desk
	withDesk(ctx, false, func(d *Desk) { desk = d })()

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		desk.Hello()
	}
}

func BenchmarkDeskUpdate(b *testing.B) {
	ctx := context.Background()
	var desk *Desk
	withDesk(ctx, false, func(d *Desk) {
		desk = d
		paper := d.tiers[TierTypePaper]
		pos := NewPosition(ctx, testSymbol, "paper", "path", d.price, nil, nil)
		pos.Apply(market.ExecutionData{
			ExecID: "b1", Side: "buy",
			LastQty: dec("1"), LastPrice: dec("100"),
			Timestamp: time.Unix(1, 0).UTC(),
		})
		paper.positions.Add(testSymbol, pos, true)
	})()
	trades := []*market.TradeData{{Symbol: testSymbol}}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		desk.Update(trades)
	}
}

func BenchmarkDeskEnter(b *testing.B) {
	ctx := context.Background()

	b.ReportAllocs()

	for b.Loop() {
		withDesk(ctx, false, func(desk *Desk) {
			_ = desk.Enter(testSymbol, "R00/M1", TierTypePaper, true, SizeHint{})
		})()
	}
}

func BenchmarkDeskExit(b *testing.B) {
	ctx := context.Background()

	b.ReportAllocs()

	for b.Loop() {
		withDesk(ctx, false, func(desk *Desk) {
			paper := desk.tiers[TierTypePaper]
			pos := NewPosition(ctx, testSymbol, "paper", "path", desk.price, nil, nil)
			pos.ExitPath = "R00/M1"
			paper.positions.Add(testSymbol, pos, true)
			_ = desk.Exit(pos, false)
		})()
	}
}

func BenchmarkDeskPositions(b *testing.B) {
	ctx := context.Background()
	var desk *Desk
	withDesk(ctx, false, func(d *Desk) {
		desk = d
		paper := d.tiers[TierTypePaper]
		pos := NewPosition(ctx, testSymbol, "paper", "path", d.price, nil, nil)
		pos.Apply(market.ExecutionData{
			ExecID: "b1", Side: "buy",
			LastQty: dec("1"), LastPrice: dec("100"),
			Timestamp: time.Unix(1, 0).UTC(),
		})
		paper.positions.Add(testSymbol, pos, true)
	})()

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		for range desk.Positions(TierTypePaper, testSymbol) {
		}
	}
}

func BenchmarkDeskPending(b *testing.B) {
	ctx := context.Background()
	var desk *Desk
	withDesk(ctx, false, func(d *Desk) {
		desk = d
		paper := d.tiers[TierTypePaper]
		pos := NewPosition(ctx, testSymbol, "paper", "path", d.price, nil, nil)
		paper.pending.Store(testSymbol, pos)
	})()

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		for range desk.Pending(TierTypePaper) {
		}
	}
}

func BenchmarkDeskClearPending(b *testing.B) {
	ctx := context.Background()

	b.ReportAllocs()

	for b.Loop() {
		withDesk(ctx, false, func(desk *Desk) {
			paper := desk.tiers[TierTypePaper]
			pos := NewPosition(ctx, testSymbol, "paper", "path", desk.price, nil, nil)
			paper.pending.Store(testSymbol, pos)
			desk.ClearPending(TierTypePaper, testSymbol)
		})()
	}
}

func BenchmarkDeskFlush(b *testing.B) {
	ctx := context.Background()
	var desk *Desk
	withDesk(ctx, false, func(d *Desk) {
		desk = d
		paper := d.tiers[TierTypePaper]
		pos := NewPosition(ctx, testSymbol, "paper", "path", d.price, nil, nil)
		paper.positions.Add(testSymbol, pos, true)
	})()

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = desk.Flush()
	}
}

func BenchmarkDeskApply(b *testing.B) {
	ctx := context.Background()
	var desk *Desk
	withDesk(ctx, false, func(d *Desk) {
		desk = d
		paper := d.tiers[TierTypePaper]
		pos := NewPosition(ctx, testSymbol, "paper", "path", d.price, nil, nil)
		paper.positions.Add(testSymbol, pos, true)
	})()
	exec := &market.Execution{
		Data: []*market.ExecutionData{{
			ExecID: "b1", Side: "buy", Symbol: testSymbol,
			LastQty: dec("1"), LastPrice: dec("100"),
			Timestamp: time.Unix(1, 0).UTC(),
		}},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		desk.Apply(exec)
	}
}
