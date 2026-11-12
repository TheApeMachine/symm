package liquidity_test

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/signal/liquidity"
)

func seedBook(books *broker.BookManager, symbol string, bid, ask, bidQty, askQty int64) {
	bidPx := decimal.NewFromInt(bid)
	bQty := decimal.NewFromInt(bidQty)
	askPx := decimal.NewFromInt(ask)
	aQty := decimal.NewFromInt(askQty)

	books.Update(&market.Level3{
		Type: "snapshot",
		Data: []*market.Level3Data{{
			Symbol: symbol,
			Bids: []*market.Level3Order{{
				LimitPrice: &bidPx,
				OrderQty:   &bQty,
			}},
			Asks: []*market.Level3Order{{
				LimitPrice: &askPx,
				OrderQty:   &aQty,
			}},
		}},
	})
}

func TestLiquidityStep(t *testing.T) {
	Convey("Step records liquidity once depth peers exist", t, func() {
		books := broker.NewBookManager()
		symbols := []string{"L1/USD", "L2/USD", "L3/USD", "L4/USD"}
		seedBook(books, symbols[0], 20, 21, 200, 20)
		seedBook(books, symbols[1], 30, 31, 40, 8)
		seedBook(books, symbols[2], 40, 41, 35, 8)
		seedBook(books, symbols[3], 50, 51, 30, 8)

		signal := liquidity.NewSignal(context.Background(), books)
		envelope := runtime.NewEnvelope(nil)
		at := time.Now().UTC()

		for i := range 64 {
			trades := make([]*market.TradeData, 0, len(symbols))
			for j, symbol := range symbols {
				trades = append(trades, &market.TradeData{
					Symbol:    symbol,
					Side:      market.SideBuy,
					Price:     decimal.NewFromInt(20 + int64(j*10)),
					Qty:       decimal.NewFromInt(2),
					Timestamp: at.Add(time.Duration(i) * time.Second),
				})
			}

			envelope.Trade = &market.Trade{Data: trades}
			envelope = signal.Step(envelope)
		}

		reading := envelope.Liquidity["L1/USD"]
		So(reading, ShouldNotBeNil)
		So(int(reading.Category), ShouldEqual, 3)
		So(reading.ScarcityScore, ShouldEqual, 0)
		So(reading.DepthScore, ShouldBeGreaterThan, 0.4)
	})
}
