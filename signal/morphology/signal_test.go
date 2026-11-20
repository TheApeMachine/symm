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
		Type: "snapshot",
		Data: []*market.Level3Data{{
			Symbol: symbol,
			Bids: []*market.Level3Order{{
				OrderID:    "b1",
				LimitPrice: &bidPx,
				OrderQty:   &bidQty,
			}},
			Asks: []*market.Level3Order{{
				OrderID:    "a1",
				LimitPrice: &askPx,
				OrderQty:   &askQty,
			}},
		}},
	})
}

func TestNewSignal(t *testing.T) {
	Convey("Setup", t, func() {
		books := broker.NewBookManager(nil)

		Convey("When NewSignal is called", func() {
			signal := morphology.NewSignal(context.Background(), books)

			Convey("It returns a usable signal", func() {
				So(signal, ShouldNotBeNil)
			})
		})
	})
}

func TestStep(t *testing.T) {
	Convey("Setup", t, func() {
		symbol := "MOR/USD"
		books := broker.NewBookManager(nil)
		seedBook(books, symbol)

		signal := morphology.NewSignal(context.Background(), books)
		envelope := runtime.NewEnvelope(nil)
		at := time.Unix(1_700_000_000, 0).UTC()

		Convey("When book-shaped trades repeat", func() {
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

			Convey("Shape is stable with unit strength", func() {
				reading := out.Morphology[symbol]
				So(reading, ShouldNotBeNil)
				So(reading.ShapeStable, ShouldEqual, 1)
				So(reading.AskVacuum, ShouldEqual, 0)
				So(reading.BidNotionalLean, ShouldEqual, 0)
				So(reading.Value, ShouldEqual, 1)
				So(reading.Strength, ShouldEqual, 1)
			})
		})
	})
}

func BenchmarkNewSignal(b *testing.B) {
	books := broker.NewBookManager(nil)

	b.ReportAllocs()

	for b.Loop() {
		_ = morphology.NewSignal(context.Background(), books)
	}
}

func BenchmarkStep(b *testing.B) {
	symbol := "MOR/USD"
	books := broker.NewBookManager(nil)
	seedBook(books, symbol)
	signal := morphology.NewSignal(context.Background(), books)
	envelope := runtime.NewEnvelope(nil)
	at := time.Unix(1_700_000_000, 0).UTC()

	for i := range 31 {
		envelope.Trade = &market.Trade{Data: []*market.TradeData{{
			Symbol:    symbol,
			Side:      market.SideBuy,
			Price:     decimal.NewFromInt(31),
			Qty:       decimal.NewFromInt(1),
			Timestamp: at.Add(time.Duration(i) * time.Second),
		}}}
		envelope = signal.Step(envelope)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		envelope.Trade = &market.Trade{Data: []*market.TradeData{{
			Symbol:    symbol,
			Side:      market.SideBuy,
			Price:     decimal.NewFromInt(31),
			Qty:       decimal.NewFromInt(1),
			Timestamp: at.Add(32 * time.Second),
		}}}
		envelope = signal.Step(envelope)
	}
}
