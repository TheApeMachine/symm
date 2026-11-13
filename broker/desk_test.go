package broker

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/symm/network"
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
			wallet := price.balance
			desk := DeskFor(ctx, map[string]network.Client{}, price, nil, live)
			fundDeskBalances(desk, wallet)
			f(desk)
		})()
	}
}

func TestDeskFor(t *testing.T) {
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

func TestDeskForLive(t *testing.T) {
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

func TestDeskCanSize(t *testing.T) {
	Convey("Setup", t, withDesk(t.Context(), false, func(desk *Desk) {
		Convey("When the book and balance can allocate", func() {
			So(desk.CanSize(testSymbol), ShouldBeTrue)
		})

		Convey("When the symbol is empty", func() {
			So(desk.CanSize(""), ShouldBeFalse)
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
			err := desk.Enter(testSymbol, "R00/M1", TierTypePaper, true)

			Convey("Paper cache holds the symbol", func() {
				So(err, ShouldBeNil)
				So(desk.tiers[TierTypePaper].positions.Has(testSymbol), ShouldBeTrue)
			})
		})

		Convey("When Enter targets an unknown tier", func() {
			err := desk.Enter(testSymbol, "R00/M1", TierType(99), true)

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
		pos.ExitPath = "R00/M1"
		paper.positions.Add(testSymbol, pos, true)

		Convey("When Exit routes through paper", func() {
			err := desk.Exit(TierTypePaper, pos)

			Convey("Closing is armed", func() {
				So(err, ShouldBeNil)
				So(pos.Closing, ShouldBeTrue)
			})
		})

		Convey("When Exit targets an unknown tier", func() {
			err := desk.Exit(TierType(99), pos)

			Convey("Desk returns not-found", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "[desk.Exit] unknown tier")
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
			desk.Apply(&market.Execution{
				Data: []*market.ExecutionData{{
					ExecID: "b1", Side: "buy", Symbol: testSymbol,
					LastQty: dec("1"), LastPrice: dec("100"),
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

func TestDeskBookStale(t *testing.T) {
	Convey("Setup", t, withDesk(t.Context(), false, func(desk *Desk) {
		Convey("When the symbol has a live book", func() {
			So(desk.BookStale(testSymbol), ShouldBeFalse)
		})
	}))
}

func BenchmarkDeskFor(b *testing.B) {
	ctx := context.Background()
	var price *Price
	withPrice(func(p *Price) { price = p })()

	b.ReportAllocs()

	for b.Loop() {
		_ = DeskFor(ctx, map[string]network.Client{}, price, nil, false)
	}
}

func BenchmarkDeskForLive(b *testing.B) {
	ctx := context.Background()
	var price *Price
	withPrice(func(p *Price) { price = p })()

	b.ReportAllocs()

	for b.Loop() {
		_ = DeskFor(ctx, map[string]network.Client{}, price, nil, true)
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

func BenchmarkDeskCanSize(b *testing.B) {
	ctx := context.Background()
	var desk *Desk
	withDesk(ctx, false, func(d *Desk) { desk = d })()

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = desk.CanSize(testSymbol)
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
			_ = desk.Enter(testSymbol, "R00/M1", TierTypePaper, true)
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
			_ = desk.Exit(TierTypePaper, pos)
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

func BenchmarkDeskBookStale(b *testing.B) {
	ctx := context.Background()
	var desk *Desk
	withDesk(ctx, false, func(d *Desk) { desk = d })()

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = desk.BookStale(testSymbol)
	}
}
