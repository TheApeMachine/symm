package broker

import (
	"context"
	"encoding/json"
	"sync"
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

	Convey("a failed real stays shut while paper still holds", t, func() {
		prev := system.Cfg.Market.Model
		system.Cfg.Market.Model = "paper"
		Reset(func() { system.Cfg.Market.Model = prev })

		paper := &Tier{Type: TierTypePaper, WinRate: 0.8, Edge: 1}
		real := &Tier{Type: TierTypeReal, WinRate: 0.8, Edge: 1, unlocked: true}

		real.WinRate = 0.4
		So(real.Unlocked(paper), ShouldBeFalse)
		So(real.Unlocked(paper), ShouldBeFalse)
	})

	Convey("live-only Unlocked keeps real open after a losing Recalculate", t, func() {
		prev := system.Cfg.Market.Model
		system.Cfg.Market.Model = "real"
		Reset(func() { system.Cfg.Market.Model = prev })

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
			positions: cache,
		}

		real.Recalculate()
		So(real.Unlocked(nil), ShouldBeTrue)
	})
}

func TestSpotBalanceCopiesVenueLedger(t *testing.T) {
	Convey("spotBalance copies venue Balance into a market.Balance", t, func() {
		host := venue.NewKraken("USD")
		price := &Price{
			balance: market.NewBalance(map[string]any{
				"quote":  "USD",
				"assets": []*market.Asset{},
			}),
		}
		tier := &Tier{Type: TierTypeReal, price: price, venue: host}

		usd := decimal.NewFromInt(200)
		host.Balance().(*kraken.Balance).Result = map[string]*decimal.Decimal{
			"ZUSD": &usd,
		}
		host.Balance().(*kraken.Balance).Normalize()
		host.Balance().ToMarket(price.balance)

		So(price.balance.Cash("USD").String(), ShouldEqual, "200")
		So(tier.spotBalance().Cash("USD").String(), ShouldEqual, "200")
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
	Convey("Write failure keeps the open lot for late fills by cl_ord_id", t, func() {
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
