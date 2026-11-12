package morphology_test

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/signal/morphology"
)

func seedBook(books *broker.BookManager, symbol string) {
	bidPx := decimal.NewFromInt(30)
	bidQty := decimal.NewFromInt(12)
	askPx := decimal.NewFromInt(31)
	askQty := decimal.NewFromInt(6)

	books.Update(&market.Level3{
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

func TestMorphologyStep(t *testing.T) {
	Convey("Step records morphology from book-shaped trades", t, func() {
		symbol := "MOR/USD"
		books := broker.NewBookManager()
		seedBook(books, symbol)

		signal := morphology.NewSignal(context.Background(), books)
		envelope := runtime.NewEnvelope(nil)
		at := time.Now().UTC()

		var out *runtime.Envelope
		for i := range 32 {
			envelope.Trade = &market.Trade{Data: []*market.TradeData{{
				Symbol:    symbol,
				Side:      market.SideBuy,
				Price:     decimal.NewFromInt(31),
				Qty:       decimal.NewFromInt(1),
				Timestamp: at.Add(time.Duration(i) * time.Second),
			}}}
			out = signal.Step(envelope)
		}

		reading := out.Morphology[symbol]
		So(reading, ShouldNotBeNil)
		So(reading.ShapeStable, ShouldEqual, 1)
		So(reading.AskVacuum, ShouldEqual, 0)
		So(reading.BidNotionalLean, ShouldEqual, 0)
		So(reading.Value, ShouldEqual, 1)
		So(reading.Strength, ShouldEqual, 1)
	})
}
