package correlation_test

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/signal/correlation"
)

func TestNewSignal(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When NewSignal is called without a price heartbeat", func() {
			signal := correlation.NewSignal(context.Background(), nil)

			Convey("It returns a usable signal", func() {
				So(signal, ShouldNotBeNil)
			})
		})
	})
}

func TestStep(t *testing.T) {
	Convey("Setup", t, func() {
		signal := correlation.NewSignal(context.Background(), nil)
		envelope := runtime.NewEnvelope(nil)
		at := time.Unix(1_700_000_000, 0).UTC()
		order := []string{"B/USD", "C/USD", "D/USD", "E/USD", "A/USD"}

		Convey("When trades fan through a shared peer cohort", func() {
			for i := range 128 {
				price := decimal.NewFromFloat(100 + float64(i))
				ts := at.Add(time.Duration(i) * time.Second)

				for _, symbol := range order {
					envelope.Trade = &market.Trade{Data: []*market.TradeData{{
						Symbol:    symbol,
						Side:      market.SideBuy,
						Price:     price,
						Qty:       decimal.NewFromInt(2),
						Timestamp: ts,
					}}}
					envelope = signal.Step(envelope)
				}
			}

			Convey("Focus symbol is eligible herd with unit correlation", func() {
				reading := envelope.Correlation["A/USD"]
				So(reading, ShouldNotBeNil)
				So(reading.Eligible, ShouldBeTrue)
				So(reading.Category, ShouldEqual, 1)
				So(reading.HerdScore, ShouldEqual, 1)
				So(reading.AlphaScore, ShouldEqual, 0)
				So(reading.NoiseScore, ShouldEqual, 0)
				So(reading.Correlation, ShouldEqual, 1)
				So(reading.Energy, ShouldEqual, 0.004524917759958988)
				So(reading.Strength, ShouldEqual, 1)
			})
		})
	})
}

func BenchmarkNewSignal(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = correlation.NewSignal(context.Background(), nil)
	}
}

func BenchmarkStep(b *testing.B) {
	signal := correlation.NewSignal(context.Background(), nil)
	envelope := runtime.NewEnvelope(nil)
	at := time.Unix(1_700_000_000, 0).UTC()
	order := []string{"B/USD", "C/USD", "D/USD", "E/USD", "A/USD"}
	trade := &market.Trade{Data: []*market.TradeData{{
		Symbol:    "A/USD",
		Side:      market.SideBuy,
		Price:     decimal.NewFromFloat(200),
		Qty:       decimal.NewFromInt(2),
		Timestamp: at,
	}}}

	for i := range 127 {
		price := decimal.NewFromFloat(100 + float64(i))
		ts := at.Add(time.Duration(i) * time.Second)
		for _, symbol := range order {
			trade.Data[0].Symbol = symbol
			trade.Data[0].Price = price
			trade.Data[0].Timestamp = ts
			envelope.Trade = trade
			envelope = signal.Step(envelope)
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		envelope.Trade = trade
		envelope = signal.Step(envelope)
	}
}
