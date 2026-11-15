package broker

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
)

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

func TestTierHello(t *testing.T) {
	Convey("Setup", t, withTier(t.Context(), TierTypePaper, func(tier *Tier) {
		pos := NewPosition(tier.ctx, testSymbol, "paper", "path", tier.price, nil, nil)
		pos.Apply(market.ExecutionData{
			ExecID: "b1", Side: "buy",
			LastQty: dec("1"), LastPrice: dec("100"),
			Timestamp: time.Unix(1, 0).UTC(),
		})
		tier.positions.Add(testSymbol, pos, true)

		Convey("When Hello refreshes open marks", func() {
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
				So(pos.PnLPct.String(), ShouldEqual, "19.04382470119522")
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
	}))
}

func TestTierExit(t *testing.T) {
	Convey("Setup", t, withTier(t.Context(), TierTypePaper, func(tier *Tier) {
		pos := NewPosition(tier.ctx, testSymbol, "paper", "path", tier.price, nil, nil)
		pos.ExitPath = "R00/M1"
		pos.Apply(market.ExecutionData{
			ExecID: "b1", Side: "buy",
			LastQty: dec("1"), LastPrice: dec("100"),
			Timestamp: time.Unix(1, 0).UTC(),
		})
		tier.positions.Add(testSymbol, pos, true)

		Convey("When Exit is called on a READY row", func() {
			setBook(tier.price, "120", "121", "30")
			err := tier.Exit(pos)

			Convey("Closing is armed", func() {
				So(err, ShouldBeNil)
				So(pos.Status() == runtime.BUSY, ShouldBeTrue)
				So(pos.ExitTrigger, ShouldEqual, "policy exit match R00/M1")
			})
		})
	}))
}

func TestTierSend(t *testing.T) {
	Convey("Setup", t, withTier(t.Context(), TierTypePaper, func(tier *Tier) {
		pos := NewPosition(tier.ctx, testSymbol, "paper", "path", tier.price, nil, nil)
		So(pos.Enter("R00/M1", dec("1"), tier.balance), ShouldBeNil)

		Convey("When private transport is nil", func() {
			err := tier.Send(pos, "enter")

			Convey("Send is a no-op success", func() {
				So(err, ShouldBeNil)
			})
		})
	}))
}

func TestTierClearPending(t *testing.T) {
	Convey("Setup", t, withTier(t.Context(), TierTypePaper, func(tier *Tier) {
		pos := NewPosition(tier.ctx, testSymbol, "paper", "path", tier.price, nil, nil)
		tier.pending.Store(testSymbol, pos)

		Convey("When ClearPending drops the symbol", func() {
			tier.ClearPending(testSymbol)

			Convey("Pending iterator is empty", func() {
				n := 0
				for range tier.Pending() {
					n++
				}
				So(n, ShouldEqual, 0)
			})
		})

		Convey("When ClearPending gets an empty symbol", func() {
			tier.ClearPending("")

			Convey("Pending row remains", func() {
				n := 0
				for range tier.Pending() {
					n++
				}
				So(n, ShouldEqual, 1)
			})
		})
	}))
}

func TestTierPending(t *testing.T) {
	Convey("Setup", t, withTier(t.Context(), TierTypePaper, func(tier *Tier) {
		pos := NewPosition(tier.ctx, testSymbol, "paper", "path", tier.price, nil, nil)
		tier.pending.Store(testSymbol, pos)

		Convey("When Pending yields rows", func() {
			var seen []*Position
			for p := range tier.Pending() {
				seen = append(seen, p)
			}

			Convey("The pending position is returned", func() {
				So(len(seen), ShouldEqual, 1)
				So(seen[0].ID, ShouldEqual, pos.ID)
			})
		})
	}))
}

func TestTierSettle(t *testing.T) {
	Convey("Setup", t, withTier(t.Context(), TierTypePaper, func(tier *Tier) {
		pos := NewPosition(tier.ctx, testSymbol, "paper", "path", tier.price, nil, nil)
		tier.positions.Add(testSymbol, pos, true)

		Convey("When settle arms a stoploss position", func() {
			tier.settle(pos)

			Convey("Open stays until the exit fill; pending is armed", func() {
				So(tier.positions.Has(testSymbol), ShouldBeTrue)
				pending := 0
				for range tier.Pending() {
					pending++
				}
				So(pending, ShouldEqual, 1)
			})
		})
	}))
}

func BenchmarkTierTypeString(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = TierTypePaper.String()
	}
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
		pos := NewPosition(t.ctx, testSymbol, "paper", "path", t.price, nil, nil)
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
		pos := NewPosition(t.ctx, testSymbol, "paper", "path", t.price, nil, nil)
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
			pos := NewPosition(tier.ctx, testSymbol, "paper", "path", tier.price, nil, nil)
			pos.ExitPath = "R00/M1"
			tier.positions.Add(testSymbol, pos, true)
			_ = tier.Exit(pos)
		})()
	}
}

func BenchmarkTierSend(b *testing.B) {
	ctx := context.Background()
	var tier *Tier
	var pos *Position
	withTier(ctx, TierTypePaper, func(t *Tier) {
		tier = t
		pos = NewPosition(t.ctx, testSymbol, "paper", "path", t.price, nil, nil)
		_ = pos.Enter("R00/M1", dec("1"), t.balance)
	})()

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = tier.Send(pos, "enter")
	}
}

func BenchmarkTierClearPending(b *testing.B) {
	ctx := context.Background()

	b.ReportAllocs()

	for b.Loop() {
		withTier(ctx, TierTypePaper, func(tier *Tier) {
			pos := NewPosition(tier.ctx, testSymbol, "paper", "path", tier.price, nil, nil)
			tier.pending.Store(testSymbol, pos)
			tier.ClearPending(testSymbol)
		})()
	}
}

func BenchmarkTierPending(b *testing.B) {
	ctx := context.Background()
	var tier *Tier
	withTier(ctx, TierTypePaper, func(t *Tier) {
		tier = t
		pos := NewPosition(t.ctx, testSymbol, "paper", "path", t.price, nil, nil)
		tier.pending.Store(testSymbol, pos)
	})()

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		for range tier.Pending() {
		}
	}
}

func BenchmarkTierSettle(b *testing.B) {
	ctx := context.Background()

	b.ReportAllocs()

	for b.Loop() {
		withTier(ctx, TierTypePaper, func(tier *Tier) {
			pos := NewPosition(tier.ctx, testSymbol, "paper", "path", tier.price, nil, nil)
			tier.positions.Add(testSymbol, pos, true)
			tier.settle(pos)
		})()
	}
}
