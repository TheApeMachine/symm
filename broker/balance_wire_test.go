package broker

import (
	"encoding/json"
	"math"
	"sync"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestBalanceFrameArmedStop(t *testing.T) {
	Convey("Given an open lot with non-finite stop geometry", t, func() {
		messages := make(chan []byte, 1)
		qty := decimal.NewFromFloat64(0.01)
		entry := decimal.NewFromFloat64(100)
		mark := decimal.NewFromFloat64(101)
		pnl := decimal.NewFromFloat64(0.5)
		pct := 0.005
		stop := types.NewStoploss(t.Context())
		stop.Bind(100, 0.01)
		stop.LockedFloor = math.Inf(-1)
		stop.Weight = math.NaN()
		stop.PeakReturn = math.Inf(1)

		holdings := &sync.Map{}
		holdings.Store("BTC/USD", &types.Holding{
			Symbol:     "BTC/USD",
			Qty:        qty,
			EntryPrice: entry,
			Mark:       mark,
			PnL:        pnl,
			ReturnPct:  &pct,
			Status:     types.OPEN,
			Stoploss:   stop,
		})

		cash := decimal.NewFromFloat64(100)
		balance := &Balance{
			status:   types.READY,
			quote:    "USD",
			ui:       messages,
			holdings: holdings,
			model: &kraken.Balance{Data: []kraken.BalanceData{{
				Asset: "USD", Balance: cash, Available: cash,
			}}},
		}

		Convey("When Frame marshals the desk snapshot", func() {
			frame := balance.Frame()

			Convey("Then Holding.MarshalJSON keeps the payload valid", func() {
				So(len(frame), ShouldBeGreaterThan, 0)
				So(json.Valid(frame), ShouldBeTrue)
				So(string(frame), ShouldContainSubstring, `"holdings"`)
				So(string(frame), ShouldContainSubstring, `"BTC/USD"`)
				So(string(frame), ShouldContainSubstring, `"pnl":0.5`)
				So(string(frame), ShouldContainSubstring, `"stops"`)
			})
		})
	})
}

func BenchmarkBalanceFrame(b *testing.B) {
	qty := decimal.NewFromFloat64(0.01)
	holdings := &sync.Map{}
	holdings.Store("BTC/USD", &types.Holding{
		Symbol: "BTC/USD", Qty: qty, Status: types.OPEN,
		EntryPrice: decimal.NewFromFloat64(100),
		Mark:       decimal.NewFromFloat64(101),
		PnL:        decimal.NewFromFloat64(0.1),
		Stoploss:   types.NewStoploss(b.Context()),
	})
	cash := decimal.NewFromFloat64(100)
	balance := &Balance{
		status: types.READY, quote: "USD", holdings: holdings,
		model: &kraken.Balance{Data: []kraken.BalanceData{{
			Asset: "USD", Balance: cash, Available: cash,
		}}},
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = balance.Frame()
	}
}
