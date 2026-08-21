package sentiment

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/kraken"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

func TestSentimentNumber(t *testing.T) {
	Convey("Given a regressing ticker sequence on the live callback path", t, func() {
		thesis := types.NewThesis(context.Background(), nil)
		market := thesis.Symbol("AAA/USD")
		start := time.Unix(1_700_000_000, 0).UTC()
		signal := NewSignal(context.Background(), thesis)
		defer signal.Close()

		market.AppendTicker(kraken.TickerData{
			Symbol:    "AAA/USD",
			Timestamp: start.Add(time.Second),
		})
		market.AppendTicker(kraken.TickerData{
			Symbol:    "AAA/USD",
			Timestamp: start,
		})

		Convey("It should retain both observations without regressing its model clock", func() {
			deadline := time.Now().Add(time.Second)

			for market.Measurements.Length(
				market.MeasurementConsumers[types.MeasurementConsumerCategory],
			) < 2 && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}

			measurements := make([]*nmtypes.Measurement, 0, 2)

			for measurement := range market.MarketMeasurements(
				market.MeasurementConsumers[types.MeasurementConsumerCategory],
			) {
				measurements = append(measurements, measurement)
			}

			So(signal.Error(), ShouldBeNil)
			So(len(measurements), ShouldEqual, 2)
			So(measurements[0].At, ShouldResemble, start.Add(time.Second))
			So(measurements[1].At, ShouldResemble, start)
			So(signal.cohortAt, ShouldResemble, start.Add(time.Second))
			So(measurements[0].Source, ShouldEqual, string(types.SourceSentiment))
			So(measurements[1].Source, ShouldEqual, string(types.SourceSentiment))
			So(measurements[0].Metric("breadth").Name, ShouldEqual, "breadth")
			So(measurements[1].Metric("breadth").Name, ShouldEqual, "breadth")
		})
	})

	Convey("Given an older observation after a completed direct step", t, func() {
		thesis := types.NewThesis(context.Background(), nil)
		market := thesis.Symbol("AAA/USD")
		start := time.Unix(1_700_000_000, 0).UTC()
		signal := NewSignal(context.Background(), thesis)
		defer signal.Close()

		err := signal.measure(market, kraken.TickerData{
			Symbol: "AAA/USD", Timestamp: start.Add(time.Second),
		})
		So(err, ShouldBeNil)

		err = signal.measure(market, kraken.TickerData{
			Symbol: "AAA/USD", Timestamp: start,
		})

		Convey("It should retain the late observation without regressing the cohort clock", func() {
			measurements := make([]*nmtypes.Measurement, 0, 2)

			for measurement := range market.MarketMeasurements(
				market.MeasurementConsumers[types.MeasurementConsumerCategory],
			) {
				measurements = append(measurements, measurement)
			}

			So(err, ShouldBeNil)
			So(len(measurements), ShouldEqual, 2)
			So(measurements[0].At, ShouldResemble, start.Add(time.Second))
			So(measurements[1].At, ShouldResemble, start)
			So(signal.cohortAt, ShouldResemble, start.Add(time.Second))
		})
	})

	Convey("Given concurrent symbols with adversarially regressing event clocks", t, func() {
		const (
			symbolCount     = 64
			eventsPerSymbol = 8
		)

		thesis := types.NewThesis(context.Background(), nil)
		start := time.Unix(1_700_000_000, 0).UTC()
		signal := NewSignal(context.Background(), thesis)
		defer signal.Close()
		symbols := make([]*types.Symbol, symbolCount)
		var writers sync.WaitGroup

		for symbolIndex := range symbolCount {
			symbols[symbolIndex] = thesis.Symbol(
				strconv.Itoa(symbolIndex) + "/USD",
			)
			writers.Add(1)

			go func(symbolIndex int) {
				defer writers.Done()

				for eventIndex := eventsPerSymbol - 1; eventIndex >= 0; eventIndex-- {
					offset := symbolIndex*eventsPerSymbol + eventIndex
					symbols[symbolIndex].AppendTicker(kraken.TickerData{
						Symbol: symbols[symbolIndex].Symbol,
						Timestamp: start.Add(
							time.Duration(offset) * time.Nanosecond,
						),
					})
				}
			}(symbolIndex)
		}

		writers.Wait()
		deadline := time.Now().Add(5 * time.Second)
		total := uint64(0)

		for time.Now().Before(deadline) {
			total = 0

			for _, symbol := range symbols {
				total += symbol.Measurements.Length(
					symbol.MeasurementConsumers[types.MeasurementConsumerCategory],
				)
			}

			if total == symbolCount*eventsPerSymbol {
				break
			}

			time.Sleep(time.Millisecond)
		}

		Convey("It should process every event exactly once under one monotonic cohort clock", func() {
			So(signal.Error(), ShouldBeNil)
			So(total, ShouldEqual, uint64(symbolCount*eventsPerSymbol))
			So(signal.cohortAt, ShouldResemble,
				start.Add((symbolCount*eventsPerSymbol-1)*time.Nanosecond))

			for symbolIndex, symbol := range symbols {
				measurements := make([]*nmtypes.Measurement, 0, eventsPerSymbol)

				for measurement := range symbol.MarketMeasurements(
					symbol.MeasurementConsumers[types.MeasurementConsumerCategory],
				) {
					measurements = append(measurements, measurement)
				}

				So(len(measurements), ShouldEqual, eventsPerSymbol)

				for eventIndex, measurement := range measurements {
					expectedOffset := symbolIndex*eventsPerSymbol +
						(eventsPerSymbol - 1 - eventIndex)
					So(measurement.At, ShouldResemble,
						start.Add(time.Duration(expectedOffset)*time.Nanosecond))
					So(measurement.Source, ShouldEqual,
						string(types.SourceSentiment))
				}
			}
		})
	})
}

func BenchmarkSentimentNumber(b *testing.B) {
	const cohortSize = 200

	start := time.Unix(1_700_000_000, 0).UTC()
	b.ReportAllocs()

	for range b.N {
		thesis := types.NewThesis(context.Background(), nil)
		signal := NewSignal(context.Background(), thesis)

		for symbolIndex := range cohortSize {
			symbol := thesis.Symbol("AAA/USD")
			err := signal.measure(symbol, kraken.TickerData{
				Symbol:    symbol.Symbol,
				Timestamp: start.Add(time.Duration(symbolIndex) * time.Nanosecond),
			})

			if err != nil {
				b.Fatal(err)
			}
		}

		signal.Close()
	}
}
