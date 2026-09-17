package correlation

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/geometry"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmcorrelation "github.com/theapemachine/symm/nomagique/statistic/correlation"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
)

func timestamp(second int64) time.Time {
	return time.Unix(1_700_000_000+second, 0)
}

func observation(symbol string, price float64, at time.Time) nmcorrelation.PriceObservation {
	return nmcorrelation.PriceObservation{
		Symbol: symbol,
		Price: temporal.Price{
			At:    at.UnixNano(),
			Value: price,
		},
	}
}

func drive(entity *Ticker, symbol string, prices []float64) []map[string]float64 {
	snapshots := make([]map[string]float64, 0, len(prices))

	for index, price := range prices {
		snapshots = append(snapshots, sequence.Read[map[string]float64](entity.Next(sequence.NewValue(observation(
			symbol, price, timestamp(int64(index)+1),
		)))))
	}

	return snapshots
}

func TestTickerNext(t *testing.T) {
	Convey("Given a correlation ticker-path instrument", t, func() {
		grid := store.NewGrid[*geometry.Coordinate]()
		entity := NewTicker(t.Context(), grid)

		Convey("every metric registers as an addressable cell before any observation", func() {
			So(len(entity.Metrics), ShouldEqual, 31)

			for index := range 31 {
				readAddress := transport.NewAddress[*geometry.Coordinate]()
				readAddress.Identify(geometry.NewCoordinate(index, 0))
				readQuery := core.NewQuery[*geometry.Coordinate, core.Primitive](readAddress, core.Read)
				cell := sequence.Read[core.Primitive](grid.Next(readQuery.Next(nil)))
				So(cell, ShouldNotBeNil)
			}
		})

		Convey("the first observation yields last price and count without a pair", func() {
			snapshot := sequence.Read[map[string]float64](entity.Next(sequence.NewValue(observation(
				"BTC/USD", 100.0, timestamp(1),
			))))

			So(snapshot["last_price"], ShouldEqual, 100.0)
			So(snapshot["observation_count"], ShouldEqual, 1.0)
			_, hasSigned := snapshot["signed_correlation"]
			So(hasSigned, ShouldBeFalse)
		})

		Convey("the last-price cell computes in isolation from a price observation", func() {
			out := sequence.Read[float64](entity.Metrics["last_price"].Next(sequence.NewValue(observation(
				"BTC/USD", 150.0, timestamp(1),
			))))
			So(out, ShouldEqual, 150.0)
		})

		Convey("an unobserved zero price does not lengthen the path", func() {
			untraded := sequence.Read[map[string]float64](entity.Next(sequence.NewValue(observation(
				"CORN/USD", 0, timestamp(1),
			))))
			So(untraded["last_price"], ShouldEqual, 0.0)
			So(untraded["observation_count"], ShouldEqual, 0.0)

			observed := sequence.Read[map[string]float64](entity.Next(sequence.NewValue(observation(
				"CORN/USD", 0.03, timestamp(2),
			))))
			unobservedAgain := sequence.Read[map[string]float64](entity.Next(sequence.NewValue(observation(
				"CORN/USD", 0, timestamp(3),
			))))
			observedAgain := sequence.Read[map[string]float64](entity.Next(sequence.NewValue(observation(
				"CORN/USD", 0.033, timestamp(4),
			))))

			So(observed["observation_count"], ShouldEqual, 1.0)
			So(observed["last_price"], ShouldEqual, 0.03)
			So(unobservedAgain["observation_count"], ShouldEqual, 0.0)
			So(observedAgain["observation_count"], ShouldEqual, 2.0)
		})

		Convey("cohort and pair facts appear once two symbols share paths", func() {
			drive(entity, "BTC/USD", []float64{100, 101, 102, 103, 104})
			snapshots := drive(entity, "ETH/USD", []float64{200, 202, 203, 205, 206})
			last := snapshots[len(snapshots)-1]

			signed := last["signed_correlation"]
			So(signed, ShouldBeGreaterThan, 0.0)
			So(signed, ShouldBeLessThan, 1.0)
			So(last["absolute_correlation"], ShouldEqual, signed)
			So(last["cohort_peer_count"], ShouldEqual, 1.0)
			So(last["overlap_pair_count"], ShouldEqual, 4.0)
			So(last["supported_return_count:measured"], ShouldEqual, 4.0)
			So(last["supported_return_count:reference"], ShouldEqual, 4.0)
			So(last["shared_time"], ShouldAlmostEqual, 4.0, 1e-3)
			So(last["overlap_density"], ShouldBeGreaterThan, 0.0)
			So(last["covariance"], ShouldBeGreaterThan, 0.0)
			So(last["return_energy:reference"], ShouldBeGreaterThan, 0.0)
			So(last["return_energy:measured"], ShouldBeGreaterThan, 0.0)
			So(last["return_energy_rate:reference"], ShouldBeGreaterThan, 0.0)
			So(last["return_energy_rate:measured"], ShouldBeGreaterThan, 0.0)
			So(last["peer_return_energy_rate"], ShouldBeGreaterThan, 0.0)
			So(last["relative_return_energy"], ShouldBeGreaterThan, 0.0)
			So(last, ShouldContainKey, "cohort_signed_correlation")
			So(last, ShouldContainKey, "cohort_absolute_correlation")
			So(last, ShouldContainKey, "cohort_effective_peer_count")
			So(last, ShouldContainKey, "cohort_correlation_dispersion")
			So(last, ShouldContainKey, "correlation_baseline")
			So(last, ShouldContainKey, "correlation_divergence")
			So(last, ShouldContainKey, "correlation_zscore")
			So(last, ShouldContainKey, "correlation_velocity")
			So(last, ShouldContainKey, "relative_return_energy_baseline")
		})

		Convey("executing the signed-correlation cell returns the pair correlation", func() {
			drive(entity, "BTC/USD", []float64{100, 101, 102, 103, 104})
			snapshots := drive(entity, "ETH/USD", []float64{200, 202, 203, 205, 206})
			last := snapshots[len(snapshots)-1]

			address := transport.NewAddress[*geometry.Coordinate]()
			address.Identify(entity.Metrics["signed_correlation"].Identity())
			query := core.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
				address, core.Execute,
			)
			reading := nmcorrelation.PairsReading{
				Selected: nmcorrelation.DependenceReading{
					LagEstimate: nmcorrelation.LagEstimate{
						Correlation: last["signed_correlation"],
						Defined:     true,
					},
					Defined: true,
				},
			}
			executed := sequence.Read[float64](grid.Next(query.Next(sequence.NewValue(reading))))
			So(executed, ShouldEqual, last["signed_correlation"])
		})

		Convey("time regression keeps support at the previously accepted path length", func() {
			sequence.Read[map[string]float64](entity.Next(sequence.NewValue(observation(
				"BTC/USD", 100.0, timestamp(2),
			))))
			snapshot := sequence.Read[map[string]float64](entity.Next(sequence.NewValue(observation(
				"BTC/USD", 101.0, timestamp(1),
			))))
			So(snapshot["observation_count"], ShouldEqual, 1.0)
			So(snapshot["last_price"], ShouldEqual, 101.0)
		})
	})
}

func BenchmarkTickerCrossSectionNext(b *testing.B) {
	grid := store.NewGrid[*geometry.Coordinate]()
	entity := NewTicker(context.Background(), grid)

	for symbolIndex := 0; symbolIndex < benchmarkSymbols; symbolIndex++ {
		symbol := benchmarkSymbol(symbolIndex)

		for index := 0; index < benchmarkWarmup; index++ {
			sequence.Read[map[string]float64](entity.Next(
				sequence.NewValue(observation(
					symbol, 100.0+float64(index), timestamp(int64(index)+1),
				)),
			))
		}
	}

	focal := observation(benchmarkSymbol(0), 0, timestamp(benchmarkWarmup))
	index := 0
	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		focal.Value = 100.0 + float64(index)
		focal.At = timestamp(int64(benchmarkWarmup + index + 1)).UnixNano()
		sequence.Read[map[string]float64](entity.Next(sequence.NewValue(focal)))
		index++
	}
}

func benchmarkSymbol(symbolIndex int) string {
	return fmt.Sprintf("S%02d/USD", symbolIndex)
}

const (
	benchmarkSymbols = 32
	benchmarkWarmup  = 64
)

func TestTickerStepReadiness(t *testing.T) {
	Convey("An inactive pipeline node drops input before touching processing state", t, func() {
		node := &Ticker{System: runtime.NewSystem(t.Context(), "readiness-test")}
		sample := observation("BTC/USD", 100, timestamp(7))

		for _, stage := range []runtime.Stage{runtime.INIT, runtime.WAITING, runtime.ERROR, runtime.FATAL} {
			node.Transition(stage)
			So(sequence.Read[nmcorrelation.PriceObservation](node.Next(sequence.NewValue(sample))), ShouldEqual, sample)
			So(node.Status(), ShouldEqual, stage)
		}
	})
}
