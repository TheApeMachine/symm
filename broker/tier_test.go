package broker

import (
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

func TestWalletReadingFromTradeBalance(t *testing.T) {
	Convey("Wallet.Reading mirrors venue TradeBalance", t, func() {
		host := venue.NewKraken("USD")
		cash := decimal.NewFromInt(900)
		equity := decimal.NewFromInt(1000)
		pnl := decimal.Zero

		host.TradeBalance().FromMarket(market.NewTradeBalance(map[string]any{
			"equity":         &equity,
			"unrealized_pnl": &pnl,
			"available_cash": &cash,
			"free_margin":    &cash,
		}))

		reading := NewWallet(host).Reading()
		So(reading["cash"], ShouldEqual, 900)
		So(reading["unrealized"], ShouldEqual, 0)
		So(reading["equity"], ShouldEqual, 1000)
	})
}

func TestWalletReadingKeepsZeroCash(t *testing.T) {
	Convey("fully deployed cash stays at zero", t, func() {
		prevBal := system.Cfg.Market.Balance
		system.Cfg.Market.Balance = 200
		Reset(func() { system.Cfg.Market.Balance = prevBal })

		host := venue.NewKraken("USD")
		zero := decimal.Zero
		host.TradeBalance().FromMarket(market.NewTradeBalance(map[string]any{
			"equity":         &zero,
			"unrealized_pnl": &zero,
			"available_cash": &zero,
			"free_margin":    &zero,
		}))

		reading := NewWallet(host).Reading()
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
