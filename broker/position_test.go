package broker

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
)

func TestPositionUpdatePrefersBookPnL(t *testing.T) {
	Convey("Stop/settle PnL follows the book walk; mark is display-only", t, func() {
		books := NewBookManager()
		bid := decimal.RequireFromString("100")
		ask := decimal.RequireFromString("101")
		qty := decimal.RequireFromString("1")

		books.Update(&market.Level3{
			Type: "snapshot",
			Data: []*market.Level3Data{{
				Symbol: "BTC/USD",
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
		instrument.takerFees.Store("BTC/USD", &fee)

		price := &Price{
			Instrument: instrument,
			Books:      books,
		}

		pos := NewPosition(context.Background(), "BTC/USD", "virtual", "path", price, nil, nil)
		entry := decimal.RequireFromString("100")
		pos.qty = &qty
		pos.entry = &entry

		print := decimal.RequireFromString("98")
		pos.Update(&print)

		So(pos.Mark.String(), ShouldEqual, "98")
		So(pos.ExitVWAP.String(), ShouldEqual, "100")
		So(pos.PnL.IsZero(), ShouldBeTrue)
		So(pos.PnLPct.IsZero(), ShouldBeTrue)
	})

	Convey("Mark prices the lot when the book walk fails", t, func() {
		books := NewBookManager()
		fee := decimal.Zero
		instrument := &Instrument{cache: &sync.Map{}, takerFees: &sync.Map{}}
		instrument.takerFees.Store("BTC/USD", &fee)

		price := &Price{
			Instrument: instrument,
			Books:      books,
		}

		pos := NewPosition(context.Background(), "BTC/USD", "virtual", "path", price, nil, nil)
		qty := decimal.RequireFromString("1")
		entry := decimal.RequireFromString("100")
		pos.qty = &qty
		pos.entry = &entry

		mark := decimal.RequireFromString("98")
		pos.Update(&mark)

		So(pos.Mark.String(), ShouldEqual, "98")
		So(pos.PnL, ShouldNotBeNil)
		pnl, _ := pos.PnL.Float64()
		So(pnl, ShouldBeLessThan, 0)
	})
}

func TestApplyIgnoresDuplicateExec(t *testing.T) {
	Convey("the same exec id folds once", t, func() {
		pos := NewPosition(context.Background(), "BTC/USD", "real", "path", nil, nil, nil)
		qty := decimal.RequireFromString("1")
		px := decimal.RequireFromString("100")

		fill := market.ExecutionData{
			ExecID:      "exec-1",
			ExecType:    "trade",
			Symbol:      "BTC/USD",
			Side:        "buy",
			LastQty:     &qty,
			LastPrice:   &px,
			OrderStatus: "filled",
		}

		pos.Apply(fill)
		pos.Apply(fill)

		So(pos.Fills, ShouldHaveLength, 1)
	})
}
