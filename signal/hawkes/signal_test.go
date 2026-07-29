package hawkes

import (
	"context"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/nomagique/algorithm/excitation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func newTestSignal() *Signal {
	return &Signal{
		ctx:      context.Background(),
		sample:   excitation.NewSample(),
		process:  excitation.NewProcess(),
		evidence: NewEvidence(),
	}
}

func calc(signal *Signal, rows ...kraken.TradeData) ([]*types.Measurement, error) {
	return signal.Calculate(nil, rows, nil)
}

func tradeRow(symbol, side string, price float64, quantity float64, at time.Time) kraken.TradeData {
	return kraken.TradeData{
		Symbol:    symbol,
		Side:      side,
		Price:     *decimal.NewFromFloat64(price),
		Qty:       quantity,
		Timestamp: at,
	}
}

func TestSignal_Calculate(t *testing.T) {
	Convey("Given a Hawkes signal driven by the central market cut", t, func() {
		signal := newTestSignal()
		at := time.Date(2023, 9, 25, 9, 4, 31, 0, time.UTC)
		row := tradeRow("BTC/USD", "buy", 100.5, 1.25, at)

		Convey("When a trade frame is calculated", func() {
			_, err := calc(signal, row)

			Convey("Then Calculate should accept the row without error", func() {
				So(err, ShouldBeNil)
			})
		})

		Convey("When an empty trade batch arrives", func() {
			measurements, err := calc(signal)

			Convey("Then nothing should be measured", func() {
				So(err, ShouldBeNil)
				So(measurements, ShouldBeEmpty)
			})
		})

		Convey("When a malformed marked arrival is calculated", func() {
			invalid := row
			invalid.Side = "hold"
			measurements, err := calc(signal, invalid)

			Convey("Then the invalid input should be returned to the caller", func() {
				So(err, ShouldNotBeNil)
				So(measurements, ShouldBeEmpty)
			})
		})

		Convey("When ingress overlaps drains", func() {
			previous := viper.GetInt("system.actor.buffer")
			viper.Set("system.actor.buffer", 64)
			Reset(func() { viper.Set("system.actor.buffer", previous) })

			live := types.NewActor(t.Context(), "live", nil)
			root := types.NewSubscription[any]()
			live.AddRoot("trade", root)
			ticker := types.NewSubscription[any]()
			book := types.NewSubscription[any]()
			live.AddRoot("ticker", ticker)
			live.AddRoot("book", book)

			signal := NewSignal(context.Background(), nil)
			thesis := types.NewThesis(nil)
			signal.Initialize(live, thesis)

			for range 32 {
				root.Send(&kraken.Trade{Data: []kraken.TradeData{row}})
			}

			time.Sleep(50 * time.Millisecond)
			So(signal.Close(), ShouldBeNil)
		})
	})
}

func BenchmarkSignal_Calculate(benchmark *testing.B) {
	signal := newTestSignal()
	start := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)

	for index := range 16 {
		side := "buy"

		if index%2 == 0 {
			side = "sell"
		}

		if _, err := calc(signal, tradeRow(
			"MATIC/USD",
			side,
			0.56+float64(index)*0.001,
			1+float64(index),
			start.Add(time.Duration(index)*100*time.Millisecond),
		)); err != nil {
			benchmark.Fatal(err)
		}
	}

	benchmark.ReportAllocs()
	index := 16

	for benchmark.Loop() {
		side := "buy"

		if index%2 == 0 {
			side = "sell"
		}

		if _, err := calc(signal, tradeRow(
			"MATIC/USD",
			side,
			0.56+float64(index)*0.001,
			1+float64(index),
			start.Add(time.Duration(index)*100*time.Millisecond),
		)); err != nil {
			benchmark.Fatal(err)
		}

		index++
	}
}
