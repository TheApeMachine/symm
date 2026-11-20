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

func TestNewSignal(t *testing.T) {
	Convey("Setup", t, func() {
		books := broker.NewBookManager(nil)

		Convey("When NewSignal is called without price heartbeat", func() {
			signal := pumpdump.NewSignal(context.Background(), books, nil)

			Convey("It returns a usable signal", func() {
				So(signal, ShouldNotBeNil)
			})
		})
	})
}

func TestStep(t *testing.T) {
	Convey("Setup", t, func() {
		symbol := "PD/USD"
		books := broker.NewBookManager(nil)
		signal := pumpdump.NewSignal(context.Background(), books, nil)
		envelope := runtime.NewEnvelope(nil)
		at := time.Unix(1_700_000_000, 0).UTC()

		Convey("When rising prints hit a climbing book", func() {
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

			Convey("Ignition and trend match seeded climb with RVOL one", func() {
				reading := envelope.Pumpdump[symbol]
				So(reading, ShouldNotBeNil)
				So(reading.RVOL, ShouldEqual, 1)
				So(reading.Compression, ShouldEqual, 0)
				// Squashed rvol/precursor geomean — raw 1.0/1.0 no longer claims ~0.85.
				So(reading.Ignition, ShouldEqual, 0.45924713059653055)
				So(reading.Trend, ShouldEqual, 0.7145109815211617)
			})
		})
	})
}

func BenchmarkNewSignal(b *testing.B) {
	books := broker.NewBookManager(nil)

	b.ReportAllocs()

	for b.Loop() {
		_ = pumpdump.NewSignal(context.Background(), books, nil)
	}
}

func BenchmarkStep(b *testing.B) {
	symbol := "PD/USD"
	books := broker.NewBookManager(nil)
	signal := pumpdump.NewSignal(context.Background(), books, nil)
	envelope := runtime.NewEnvelope(nil)
	at := time.Unix(1_700_000_000, 0).UTC()

	for i := range 139 {
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
	seedBook(books, symbol, 139)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		envelope.Trade = &market.Trade{Data: []*market.TradeData{{
			Symbol:    symbol,
			Side:      market.SideBuy,
			Price:     decimal.NewFromFloat(239),
			Qty:       decimal.NewFromInt(20),
			Timestamp: at.Add(139 * time.Second),
		}}}
		envelope = signal.Step(envelope)
	}
}
