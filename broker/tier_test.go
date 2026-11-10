package broker

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/venue"
	"github.com/theapemachine/symm/venue/kraken"
)

func TestTierUnlocked(t *testing.T) {
	Convey("virtual and paper stay unlocked", t, func() {
		So((&Tier{Type: TierTypeVirtual}).Unlocked(nil), ShouldBeTrue)
		So((&Tier{Type: TierTypePaper}).Unlocked(nil), ShouldBeTrue)
	})

	Convey("live-only real skips the paper proof", t, func() {
		prev := system.Cfg.Market.Model
		system.Cfg.Market.Model = "real"
		Reset(func() { system.Cfg.Market.Model = prev })

		So((&Tier{Type: TierTypeReal}).Unlocked(nil), ShouldBeTrue)
	})

	Convey("virgin real unlocks from paper alone", t, func() {
		prev := system.Cfg.Market.Model
		system.Cfg.Market.Model = "paper"
		Reset(func() { system.Cfg.Market.Model = prev })

		real := &Tier{Type: TierTypeReal}
		paper := &Tier{Type: TierTypePaper, WinRate: 0.51, Edge: 0.01}

		So(real.Unlocked(nil), ShouldBeFalse)
		So(real.Unlocked(&Tier{Type: TierTypePaper, WinRate: 0.5, Edge: 1}), ShouldBeFalse)
		So(real.Unlocked(paper), ShouldBeTrue)
	})

	Convey("real with history must stay ok even when paper holds", t, func() {
		prev := system.Cfg.Market.Model
		system.Cfg.Market.Model = "paper"
		Reset(func() { system.Cfg.Market.Model = prev })

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
		prev := system.Cfg.Market.Model
		system.Cfg.Market.Model = "paper"
		Reset(func() { system.Cfg.Market.Model = prev })

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

func TestWalletReadingUsesSpotCash(t *testing.T) {
	Convey("Wallet cash is spot Balance USD; flat equity equals cash", t, func() {
		host := venue.NewKraken("USD")
		spot := decimal.NewFromInt(200)
		stale := decimal.NewFromInt(1000)

		host.Balance().(*kraken.Balance).Result = map[string]*decimal.Decimal{
			"USD": &spot,
		}
		// Stale TradeBalance must not drive the strip.
		host.TradeBalance().FromMarket(market.NewTradeBalance(map[string]any{
			"equity":         &stale,
			"unrealized_pnl": &stale,
		}))

		cache := NewCache(context.Background(), "real", nil, nil, nil, nil)
		reading := NewWallet(host, cache).Reading()
		So(reading["cash"], ShouldEqual, 200)
		So(reading["unrealized"], ShouldEqual, 0)
		So(reading["equity"], ShouldEqual, 200)
	})
}

func TestWalletReadingSumsOpenLotPnL(t *testing.T) {
	Convey("unrealized and equity track open lots, not TradeBalance", t, func() {
		host := venue.NewKraken("USD")
		cash := decimal.NewFromInt(130)
		host.Balance().(*kraken.Balance).Result = map[string]*decimal.Decimal{
			"USD": &cash,
		}

		cache := NewCache(context.Background(), "real", nil, nil, nil, nil)
		loss := decimal.NewFromFloat(-0.68)
		qty := decimal.NewFromInt(1)
		entry := decimal.NewFromInt(33)
		cache.Add("FIL/USD", &Position{
			Symbol: "FIL/USD",
			Qty:    &qty,
			Entry:  &entry,
			PnL:    &loss,
		}, true)

		reading := NewWallet(host, cache).Reading()
		So(reading["cash"], ShouldEqual, 130)
		So(reading["unrealized"], ShouldAlmostEqual, -0.68, 1e-9)
		// cash + entry*qty + pnl
		So(reading["equity"], ShouldAlmostEqual, 162.32, 1e-9)
	})
}

func TestSpotWalletRefusesEmptyLedger(t *testing.T) {
	Convey("spotWallet errors when USD is missing", t, func() {
		tier := &Tier{venue: venue.NewKraken("USD")}
		_, err := tier.spotWallet()
		So(err, ShouldNotBeNil)
	})
}

func TestRealBalanceSyncsDeskWallet(t *testing.T) {
	Convey("real spot Balance fills price.balance for BookIntact sizing", t, func() {
		host := venue.NewKraken("USD")
		price := &Price{
			balance: market.NewBalance(map[string]any{
				"quote":  "USD",
				"assets": []*market.Asset{},
			}),
		}
		tier := &Tier{Type: TierTypeReal, price: price, venue: host}

		host.Balance().OnChange(func(frame market.MarketType) {
			if tier.price != nil && tier.price.balance != nil {
				frame.ToMarket(tier.price.balance)
			}
		})

		usd := decimal.NewFromInt(200)
		host.Balance().(*kraken.Balance).Result = map[string]*decimal.Decimal{
			"ZUSD": &usd,
		}
		host.Balance().(*kraken.Balance).Normalize()
		host.Balance().ToMarket(price.balance)

		So(price.balance.Cash("USD").String(), ShouldEqual, "200")

		wallet, err := tier.spotWallet()
		So(err, ShouldBeNil)
		So(wallet.Cash("USD").String(), ShouldEqual, "200")
	})
}

func TestWalletReadingKeepsZeroCash(t *testing.T) {
	Convey("fully deployed cash stays at zero", t, func() {
		prevBal := system.Cfg.Market.Balance
		system.Cfg.Market.Balance = 200
		Reset(func() { system.Cfg.Market.Balance = prevBal })

		host := venue.NewKraken("USD")
		cache := NewCache(context.Background(), "real", nil, nil, nil, nil)
		reading := NewWallet(host, cache).Reading()
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

func TestSendWriteErrorKeepsLot(t *testing.T) {
	Convey("Write failure keeps the open lot and inflight for late fills", t, func() {
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

		pending := 0
		tier.inflight.Range(func(_, _ any) bool {
			pending++
			return true
		})
		So(pending, ShouldEqual, 1)
	})
}

type writeFailClient struct{}

func (*writeFailClient) Read(...string) ([]byte, error) { return nil, nil }
func (*writeFailClient) Write([]byte) error {
	return errnie.Err(errnie.IO, "write failed", nil)
}
func (*writeFailClient) Post(string, any) ([]byte, error) { return nil, nil }
func (*writeFailClient) Watch(string, any, json.Unmarshaler) {}
