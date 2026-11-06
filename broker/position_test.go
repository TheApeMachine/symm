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
	Convey("Update marks from the book walk; print is display-only", t, func() {
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
		pos.Qty = &qty
		pos.Entry = &entry

		print := decimal.RequireFromString("98")
		So(pos.Update(&print), ShouldBeNil)

		So(pos.Mark.String(), ShouldEqual, "98")
		So(pos.ExitVWAP.String(), ShouldEqual, "100")
		So(pos.PnL.IsZero(), ShouldBeTrue)
		So(pos.PnLPct.IsZero(), ShouldBeTrue)
	})

	Convey("Update prices the lot when the book walk fails", t, func() {
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
		pos.Qty = &qty
		pos.Entry = &entry

		mark := decimal.RequireFromString("98")
		So(pos.Update(&mark), ShouldBeNil)

		So(pos.Mark.String(), ShouldEqual, "98")
		So(pos.PnL, ShouldNotBeNil)
		pnl, _ := pos.PnL.Float64()
		So(pnl, ShouldBeLessThan, 0)
	})

	Convey("Update errors when the lot was never filled", t, func() {
		pos := NewPosition(context.Background(), "BTC/USD", "virtual", "path", &Price{}, nil, nil)
		So(pos.Update(nil), ShouldNotBeNil)
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

func TestApplyVWAPEntry(t *testing.T) {
	Convey("partial buys accumulate a volume-weighted entry", t, func() {
		pos := NewPosition(context.Background(), "BTC/USD", "real", "path", nil, nil, nil)
		q1 := decimal.RequireFromString("10")
		p1 := decimal.RequireFromString("100")
		q2 := decimal.RequireFromString("10")
		p2 := decimal.RequireFromString("200")

		pos.Apply(market.ExecutionData{
			ExecID: "b1", Side: "buy", LastQty: &q1, LastPrice: &p1, ExecType: "trade",
		})
		pos.Apply(market.ExecutionData{
			ExecID: "b2", Side: "buy", LastQty: &q2, LastPrice: &p2, ExecType: "trade",
		})

		So(pos.Qty.String(), ShouldEqual, "20")
		So(pos.Entry.String(), ShouldEqual, "150")
	})
}

func TestApplyExitRealizesFillPnL(t *testing.T) {
	Convey("a full sell realizes PnL from exit fills after Closing", t, func() {
		fee := decimal.Zero
		instrument := &Instrument{cache: &sync.Map{}, takerFees: &sync.Map{}}
		instrument.takerFees.Store("BTC/USD", &fee)
		price := &Price{Instrument: instrument, Books: NewBookManager()}

		pos := NewPosition(context.Background(), "BTC/USD", "real", "path", price, nil, nil)
		buyQty := decimal.RequireFromString("2")
		buyPx := decimal.RequireFromString("100")
		pos.Apply(market.ExecutionData{
			ExecID: "b1", Side: "buy", LastQty: &buyQty, LastPrice: &buyPx, ExecType: "trade",
		})

		pos.Closing = true

		sellQty := decimal.RequireFromString("2")
		sellPx := decimal.RequireFromString("90")
		pos.Apply(market.ExecutionData{
			ExecID: "s1", Side: "sell", LastQty: &sellQty, LastPrice: &sellPx, ExecType: "trade",
		})

		So(pos.Flat(), ShouldBeTrue)
		So(pos.ClosedAt, ShouldNotBeNil)
		So(pos.ExitVWAP.String(), ShouldEqual, "90")
		So(pos.PnL, ShouldNotBeNil)
		pnl, _ := pos.PnL.Float64()
		So(pnl, ShouldAlmostEqual, -20.0, 1e-9)
	})
}

func TestOversellDoesNotDiluteExit(t *testing.T) {
	Convey("a second sell against a flat lot is dropped and PnL stays on the first exit", t, func() {
		fee := decimal.Zero
		instrument := &Instrument{cache: &sync.Map{}, takerFees: &sync.Map{}}
		instrument.takerFees.Store("BTC/USD", &fee)
		price := &Price{Instrument: instrument, Books: NewBookManager()}

		pos := NewPosition(context.Background(), "BTC/USD", "virtual", "path", price, nil, nil)
		buyQty := decimal.RequireFromString("237.81801")
		buyPx := decimal.RequireFromString("0.1456")
		pos.Apply(market.ExecutionData{
			ExecID: "b1", Side: "buy", LastQty: &buyQty, LastPrice: &buyPx, ExecType: "trade",
		})

		pos.Closing = true

		sellQty := decimal.RequireFromString("237.81801")
		sellPx := decimal.RequireFromString("0.1495")
		pos.Apply(market.ExecutionData{
			ExecID: "s1", Side: "sell", LastQty: &sellQty, LastPrice: &sellPx, ExecType: "trade",
		})

		So(pos.Flat(), ShouldBeTrue)
		So(pos.ExitVWAP.String(), ShouldEqual, "0.1495")
		first, _ := pos.PnL.Float64()

		badPx := decimal.RequireFromString("0.1334")
		pos.Apply(market.ExecutionData{
			ExecID: "s2", Side: "sell", LastQty: &sellQty, LastPrice: &badPx, ExecType: "trade",
		})

		So(pos.Fills, ShouldHaveLength, 2)
		So(pos.ExitVWAP.String(), ShouldEqual, "0.1495")
		second, _ := pos.PnL.Float64()
		So(second, ShouldAlmostEqual, first, 1e-12)
		So(second, ShouldBeGreaterThan, 0)
	})
}
