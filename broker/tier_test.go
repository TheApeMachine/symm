package broker

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/venue"
)

func TestTierUnlocked(t *testing.T) {
	Convey("virtual and paper stay unlocked", t, func() {
		So((&Tier{Type: TierTypeVirtual}).Unlocked(nil), ShouldBeTrue)
		So((&Tier{Type: TierTypePaper}).Unlocked(nil), ShouldBeTrue)
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

	Convey("a failed real stays shut until paper loses the proof and earns it again", t, func() {
		paper := &Tier{Type: TierTypePaper, WinRate: 0.8, Edge: 1}
		real := &Tier{Type: TierTypeReal, WinRate: 0.8, Edge: 1, unlocked: true}

		real.WinRate = 0.4
		So(real.Unlocked(paper), ShouldBeFalse)
		So(real.Unlocked(paper), ShouldBeFalse)

		paper.Edge = -1
		So(real.Unlocked(paper), ShouldBeFalse)

		paper.Edge = 1
		So(real.Unlocked(paper), ShouldBeTrue)
		So(real.Unlocked(paper), ShouldBeTrue)

		loss := decimal.NewFromInt(-2)
		closed := time.Now()
		real.positions = &Cache{
			positions: map[string]map[string]*Position{
				"open": {},
				"closed": {
					"AAA/USD": {
						Symbol:   "AAA/USD",
						ClosedAt: &closed,
						PnLPct:   &loss,
					},
				},
			},
		}

		real.Recalculate()
		So(real.unlocked, ShouldBeFalse)
		So(real.halted, ShouldBeTrue)
		So(real.Unlocked(paper), ShouldBeFalse)
	})
}

func TestWalletReadingEquityIncludesOpenMarketValue(t *testing.T) {
	Convey("Equity is free cash plus close-now proceeds, not cash plus PnL", t, func() {
		books := NewBookManager()
		bid := decimal.RequireFromString("100")
		ask := decimal.RequireFromString("101")
		qty := decimal.RequireFromString("1")

		books.Update(&market.Level3{
			Type: "snapshot",
			Data: []*market.Level3Data{{
				Symbol: "AAA/USD",
				Bids: []*market.Level3Order{{
					OrderID:    "b1",
					LimitPrice: &bid,
					OrderQty:   &qty,
					Timestamp:  time.Unix(1, 0),
				}},
				Asks: []*market.Level3Order{{
					OrderID:    "a1",
					LimitPrice: &ask,
					OrderQty:   &qty,
					Timestamp:  time.Unix(1, 0),
				}},
			}},
		})

		fee := decimal.Zero
		instrument := &Instrument{cache: &sync.Map{}, takerFees: &sync.Map{}}
		instrument.takerFees.Store("AAA/USD", &fee)

		price := &Price{
			Instrument: instrument,
			Books:      books,
		}

		host := venue.NewKraken("USD")
		cash := decimal.NewFromInt(900)
		host.Balance().FromMarket(market.NewBalance(map[string]any{
			"quote": "USD",
			"assets": []*market.Asset{
				market.NewAsset(nil, "USD", cash, decimal.Zero, cash),
			},
		}))

		// Empty in-memory cache — avoid S3 restore colliding on symbol.
		tier := &Tier{
			Type:  TierTypeVirtual,
			price: price,
			venue: host,
			positions: &Cache{
				positions: map[string]map[string]*Position{
					"open":   {},
					"closed": {},
				},
			},
		}

		pos := NewPosition(context.Background(), "AAA/USD", "virtual", "path", price, nil, nil)
		entry := decimal.RequireFromString("100")
		pos.Qty = &qty
		pos.Entry = &entry
		mark := decimal.RequireFromString("100")
		So(pos.Update(&mark), ShouldBeNil)
		So(tier.positions.Add("AAA/USD", pos, true), ShouldBeFalse)

		reading := tier.walletReading()
		So(reading["cash"], ShouldEqual, 900)
		So(reading["unrealized"], ShouldEqual, 0)
		// Free cash 900 + flat 100 notional lot ≈ starting 1000, not 900+0.
		So(reading["equity"], ShouldEqual, 1000)
	})
}

func TestWalletReadingKeepsZeroCash(t *testing.T) {
	Convey("fully deployed virtual cash stays at zero", t, func() {
		prevBal := system.Cfg.Market.Balance
		system.Cfg.Market.Balance = 200
		Reset(func() { system.Cfg.Market.Balance = prevBal })

		host := venue.NewKraken("USD")
		zero := decimal.Zero
		host.Balance().FromMarket(market.NewBalance(map[string]any{
			"quote": "USD",
			"assets": []*market.Asset{
				market.NewAsset(nil, "USD", zero, decimal.Zero, zero),
			},
		}))

		tier := &Tier{
			Type:  TierTypeVirtual,
			venue: host,
			positions: &Cache{
				positions: map[string]map[string]*Position{
					"open":   {},
					"closed": {},
				},
			},
		}

		reading := tier.walletReading()
		So(reading["cash"], ShouldEqual, 0)
		So(reading["equity"], ShouldEqual, 0)
	})
}

func TestFillReportKeepsTrades(t *testing.T) {
	Convey("status events are not fills and trade prints are", t, func() {
		tier := &Tier{}
		qty := decimal.NewFromInt(1)

		So(tier.fillReport(&market.ExecutionData{
			ExecType: "new",
			LastQty:  &qty,
		}), ShouldBeFalse)
		So(tier.fillReport(&market.ExecutionData{
			ExecType: "trade",
			LastQty:  &qty,
		}), ShouldBeTrue)
		So(tier.fillReport(&market.ExecutionData{
			ExecType: "trade",
		}), ShouldBeFalse)
	})
}
