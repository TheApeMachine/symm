package pumpdump_test

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/signal/pumpdump"
)

func seedBook(books *broker.BookManager, symbol string, index int64) {
	bidPx := decimal.NewFromFloat(99.5 + float64(index))
	askPx := decimal.NewFromFloat(100.5 + float64(index))
	bidQty := decimal.NewFromInt(40)
	askQty := decimal.NewFromInt(40)

	books.Update(&market.Level3{
		Type: "snapshot",
		Data: []*market.Level3Data{{
			Symbol: symbol,
			Bids: []*market.Level3Order{{
				LimitPrice: &bidPx,
				OrderQty:   &bidQty,
			}},
			Asks: []*market.Level3Order{{
				LimitPrice: &askPx,
				OrderQty:   &askQty,
			}},
		}},
	})
}

func TestPumpdumpStep(t *testing.T) {
	Convey("Step records pump pressure from rising prints", t, func() {
		symbol := "PD/USD"
		books := broker.NewBookManager()
		at := time.Now().UTC()

		signal := pumpdump.NewSignal(context.Background(), books)
		envelope := runtime.NewEnvelope(nil)

		for i := range 140 {
			seedBook(books, symbol, int64(i))

			envelope.Trade = &market.Trade{Data: []*market.TradeData{{
				Symbol:    symbol,
				Side:      market.SideBuy,
				Price:     decimal.NewFromFloat(100 + float64(i)),
				Qty:       decimal.NewFromInt(20),
				Timestamp: at.Add(time.Duration(i) * time.Second),
			}}}
			envelope = signal.Step(envelope)
		}

		reading := envelope.Pumpdump[symbol]
		So(reading, ShouldNotBeNil)
		So(reading.RVOL, ShouldEqual, 1)
		So(reading.Compression, ShouldEqual, 0)
		So(reading.Ignition, ShouldBeGreaterThan, 0.8)
		So(reading.Trend, ShouldBeGreaterThan, 0.7)
	})
}
