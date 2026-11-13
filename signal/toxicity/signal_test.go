package toxicity_test

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/signal/toxicity"
)

func pushBook(books *broker.BookManager, symbol string, bidQty int64) {
	bidPx := decimal.NewFromInt(90)
	bQty := decimal.NewFromInt(bidQty)
	askPx := decimal.NewFromInt(91)
	aQty := decimal.NewFromInt(80)

	books.Update(&market.Level3{
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

func TestNewSignal(t *testing.T) {
	Convey("Setup", t, func() {
		books := broker.NewBookManager(nil)

		Convey("When NewSignal is called", func() {
			signal := toxicity.NewSignal(context.Background(), books)

			Convey("It returns a usable signal", func() {
				So(signal, ShouldNotBeNil)
			})
		})
	})
}

func TestStep(t *testing.T) {
	Convey("Setup", t, func() {
		symbol := "TOX/USD"
		books := broker.NewBookManager(nil)
		pushBook(books, symbol, 20)

		signal := toxicity.NewSignal(context.Background(), books)
		envelope := runtime.NewEnvelope(nil)
		at := time.Unix(1_700_000_000, 0).UTC()

		Convey("When touch refills deepen bid support", func() {
			var out *runtime.Envelope
			for i := range 48 {
				pushBook(books, symbol, 20+int64(i)*5)

				envelope.Trade = &market.Trade{Data: []*market.TradeData{{
					Symbol:    symbol,
					Side:      market.SideBuy,
					Price:     decimal.NewFromFloat(90.5),
					Qty:       decimal.NewFromInt(2),
					Timestamp: at.Add(time.Duration(i) * time.Second),
				}}}
				out = signal.Step(envelope)
			}

			Convey("Category is hard support without vacuum or bluff", func() {
				reading := out.Toxicity[symbol]
				So(reading, ShouldNotBeNil)
				So(int(reading.Category), ShouldEqual, 3)
				So(reading.BluffScore, ShouldEqual, 0)
				So(reading.VacuumScore, ShouldEqual, 0)
				So(reading.SupportScore, ShouldEqual, 0.35902039183346857)
				So(reading.Value, ShouldEqual, 0.3137254901960784)
			})
		})
	})
}

func BenchmarkNewSignal(b *testing.B) {
	books := broker.NewBookManager(nil)

	b.ReportAllocs()

	for b.Loop() {
		_ = toxicity.NewSignal(context.Background(), books)
	}
}

func BenchmarkStep(b *testing.B) {
	symbol := "TOX/USD"
	books := broker.NewBookManager(nil)
	pushBook(books, symbol, 20)
	signal := toxicity.NewSignal(context.Background(), books)
	envelope := runtime.NewEnvelope(nil)
	at := time.Unix(1_700_000_000, 0).UTC()

	for i := range 47 {
		pushBook(books, symbol, 20+int64(i)*5)
		envelope.Trade = &market.Trade{Data: []*market.TradeData{{
			Symbol:    symbol,
			Side:      market.SideBuy,
			Price:     decimal.NewFromFloat(90.5),
			Qty:       decimal.NewFromInt(2),
			Timestamp: at.Add(time.Duration(i) * time.Second),
		}}}
		envelope = signal.Step(envelope)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		pushBook(books, symbol, 260)
		envelope.Trade = &market.Trade{Data: []*market.TradeData{{
			Symbol:    symbol,
			Side:      market.SideBuy,
			Price:     decimal.NewFromFloat(90.5),
			Qty:       decimal.NewFromInt(2),
			Timestamp: at.Add(48 * time.Second),
		}}}
		envelope = signal.Step(envelope)
	}
}
