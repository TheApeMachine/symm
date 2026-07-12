package trader

import (
	"context"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestTradeBatchMeasure(t *testing.T) {
	Convey("Given ordered trades and independent recording signals", t, func() {
		first := &recordingSignal{}
		second := &recordingSignal{}
		events := tradeBatchEvents(4)
		batch := NewTradeBatch(
			[]types.Signal[any]{first, second},
			events,
			&types.CrossSection{},
		)

		measurements, err := batch.Measure()

		Convey("It should preserve per-signal event order and deterministic merge order", func() {
			So(err, ShouldBeNil)
			So(first.rows, ShouldHaveLength, len(events))
			So(second.rows, ShouldHaveLength, len(events))
			So(measurements, ShouldHaveLength, len(events)*2)
			So(measurements[0].Metrics["price"], ShouldEqual, events[0].Price)
		})
	})
}

func TestTradeBatchStamp(t *testing.T) {
	Convey("Given an immutable typed measurement", t, func() {
		batch := NewTradeBatch(nil, nil, nil)
		measurement := &types.Measurement{
			Metric: types.MetricConditionalIntensity,
			Raw:    2,
		}

		Convey("When the legacy price adapter sees it", func() {
			batch.stamp([]*types.Measurement{measurement}, 100)

			Convey("Then numerical evidence is not mutated after emission", func() {
				So(measurement.Raw, ShouldEqual, 2.0)
				So(measurement.Metrics, ShouldBeNil)
			})
		})
	})
}

func BenchmarkTradeBatchMeasure(b *testing.B) {
	signals := make([]types.Signal[any], 7)

	for index := range signals {
		signals[index] = &benchmarkSignal{}
	}

	events := tradeBatchEvents(128)
	batch := NewTradeBatch(signals, events, &types.CrossSection{})
	b.ReportAllocs()

	for b.Loop() {
		measurements, err := batch.Measure()

		if err != nil || len(measurements) != len(events)*len(signals) {
			b.Fatal("incomplete trade batch")
		}
	}
}

func BenchmarkTradeBatchLiveSignals(b *testing.B) {
	registry := NewSignal(context.Background())
	events := tradeBatchEvents(1024)
	batch := NewTradeBatch(registry.Trade, events, registry.CrossSection)
	b.ReportAllocs()

	for b.Loop() {
		measurements, err := batch.Measure()

		if err != nil {
			b.Fatal(err)
		}

		_ = measurements
	}
}

func tradeBatchEvents(count int) []types.Event {
	events := make([]types.Event, count)
	at := time.Date(2026, 7, 12, 2, 0, 0, 0, time.UTC)

	for index := range events {
		price := float64(index + 1)
		side := "buy"
		eventAt := at.Add(time.Duration(index) * time.Millisecond)

		if index%2 == 1 {
			side = "sell"
		}

		events[index] = types.Event{
			Stream: "trade", Sequence: uint64(index + 1), At: eventAt,
			Symbol: "BTC/USD", Price: price,
			Row: kraken.TradeData{
				Symbol: "BTC/USD", Side: side, Price: *decimal.NewFromFloat64(price),
				Qty: 1, OrderType: "market", TradeID: int64(index + 1), Timestamp: eventAt,
			},
		}
	}

	return events
}
