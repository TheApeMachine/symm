package correlation

import (
	"context"
	"fmt"
	"maps"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/geometry"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

var schema = new(Ticker).Register().Metrics

func tick(symbol string, price float64, at time.Time) *data.Measurement[float64] {
	m := data.NewMeasurement("correlation", maps.Clone(schema))
	m.Label, m.At, m.From = symbol, at, at
	m.Metrics["last_price"] = m.Metrics["last_price"].Write(price)

	return m
}

func timestamp(second int64) time.Time {
	return time.Unix(1_700_000_000+second, 0)
}

func drive(entity *Ticker, symbol string, prices []float64) []*data.Measurement[float64] {
	measurements := make([]*data.Measurement[float64], 0, len(prices))

	for index, price := range prices {
		measurements = append(measurements, sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue(tick(
			symbol, price, timestamp(int64(index)+1),
		)))))
	}

	return measurements
}

func TestTickerNext(t *testing.T) {
	Convey("Given a correlation ticker-path instrument", t, func() {
		grid := store.NewGrid[*geometry.Coordinate]()
		entity := NewTicker(t.Context(), grid)

		Convey("the first tick yields one measurement with no warmup", func() {
			measurement := sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue(tick("BTC/USD", 100.0, timestamp(1)))))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["last_price"].Raw, ShouldEqual, 100.0)
			So(measurement.Metrics["observation_count"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["signed_correlation"].Raw, ShouldEqual, 0.0)

			So(measurement.Maturity, ShouldEqual, 0.0)
			So(measurement.SNR, ShouldEqual, 0.0)
		})

		Convey("every metric pipeline registers as an addressable cell in the grid", func() {
			sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue(tick("BTC/USD", 100.0, timestamp(1)))))

			for index := range 31 {
				readAddress := transport.NewAddress[*geometry.Coordinate]()
				readAddress.Identify(geometry.NewCoordinate(index, 0))
				readQuery := core.NewQuery[*geometry.Coordinate, core.Primitive](readAddress, core.Read)
				cell := sequence.Read[core.Primitive](grid.Next(readQuery.Next(nil)))
				So(cell, ShouldNotBeNil)
			}
		})

		Convey("individual metric pipelines compute their expected outputs in isolation", func() {
			lastPricePipeline := entity.Metrics()["last_price"]
			So(lastPricePipeline, ShouldNotBeNil)

			out := sequence.Read[*data.Measurement[float64]](lastPricePipeline.Next(sequence.NewValue(tick("BTC/USD", 150.0, timestamp(1)))))
			So(out, ShouldNotBeNil)
			So(out.Metrics["last_price"].Raw, ShouldEqual, 150.0)

			obsPipeline := entity.Metrics()["observation_count"]
			So(obsPipeline, ShouldNotBeNil)

			outObs := sequence.Read[*data.Measurement[float64]](obsPipeline.Next(sequence.NewValue(tick("ETH/USD", 200.0, timestamp(1)))))
			So(outObs, ShouldNotBeNil)
			So(outObs.Metrics["observation_count"].Raw, ShouldEqual, 1.0)
		})

		Convey("a quoted market with no recent trade does not enter the price path", func() {
			untraded := tick("CORN/USD", 0, timestamp(1))

			measurement := sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue(untraded)))

			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["last_price"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["observation_count"].Raw, ShouldEqual, 0.0)
			So(measurement.Maturity, ShouldEqual, 0.0)
			So(measurement.Provenance["last_trade_price_state"], ShouldEqual, "unobserved")

			observed := sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue(tick("CORN/USD", 0.03, timestamp(2)))))
			unobservedAgain := sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue(tick("CORN/USD", 0, timestamp(3)))))
			observedAgain := sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue(tick("CORN/USD", 0.033, timestamp(4)))))

			So(observed.Err, ShouldBeNil)
			So(observed.Metrics["observation_count"].Raw, ShouldEqual, 1.0)
			So(observed.Metrics["last_price"].Raw, ShouldEqual, 0.03)
			So(unobservedAgain.Err, ShouldBeNil)
			So(unobservedAgain.Metrics["observation_count"].Raw, ShouldEqual, 0.0)
			So(observedAgain.Err, ShouldBeNil)
			So(observedAgain.Metrics["observation_count"].Raw, ShouldEqual, 2.0)
		})

		Convey("a measurement without a price fails the gate", func() {
			measurement := sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue(tick("BTC/USD", -1, timestamp(1)))))

			So(measurement.Err, ShouldNotBeNil)
		})

		Convey("cohort and pair-history facts appear once two symbols share paths", func() {
			drive(entity, "BTC/USD", []float64{100, 101, 102, 103, 104})
			measurements := drive(entity, "ETH/USD", []float64{200, 202, 203, 205, 206})

			last := measurements[len(measurements)-1]

			So(last.Metrics, ShouldContainKey, "signed_correlation")
			So(last.Metrics, ShouldContainKey, "absolute_correlation")

			signed := last.Metrics["signed_correlation"].Raw
			So(signed, ShouldBeGreaterThan, 0.0)
			So(signed, ShouldBeLessThan, 1.0)

			So(last.Metrics["cohort_peer_count"].Raw, ShouldEqual, 1.0)
			So(last.Metrics["overlap_pair_count"].Raw, ShouldEqual, 4.0)
			So(last.Metrics["supported_return_count:measured"].Raw, ShouldEqual, 4.0)
			So(last.Metrics["supported_return_count:reference"].Raw, ShouldEqual, 4.0)
			So(last.Metrics["shared_time"].Raw, ShouldAlmostEqual, 4.0, 1e-3)
			So(last.Metrics["overlap_density"].Raw, ShouldBeGreaterThan, 0.0)
			So(last.Metrics, ShouldContainKey, "covariance")
			So(last.Metrics, ShouldContainKey, "return_energy:reference")
			So(last.Metrics, ShouldContainKey, "return_energy:measured")
			So(last.Metrics["return_energy_rate:reference"].Raw, ShouldBeGreaterThan, 0.0)
			So(last.Metrics["return_energy_rate:measured"].Raw, ShouldBeGreaterThan, 0.0)
			So(last.Metrics["peer_return_energy_rate"].Raw, ShouldBeGreaterThan, 0.0)
			So(last.Metrics, ShouldContainKey, "correlation_p_value")
			So(last.Metrics, ShouldContainKey, "correlation_standard_error_fisher")
			So(last.Metrics, ShouldContainKey, "cohort_correlation_dispersion")
			So(last.Metrics, ShouldContainKey, "cohort_effective_peer_count")
			So(last.Metrics, ShouldContainKey, "relative_return_energy")

			So(last.Metrics, ShouldContainKey, "correlation_baseline")
			So(last.Metrics, ShouldContainKey, "correlation_divergence")
			So(last.Metrics, ShouldContainKey, "correlation_zscore")
			So(last.Metrics, ShouldContainKey, "correlation_velocity")
			So(last.Metrics, ShouldContainKey, "relative_return_energy_baseline")
		})

		Convey("a settled correlation estimator yields a defined SNR", func() {
			drive(entity, "BTC/USD", []float64{100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 111})
			measurements := drive(entity, "ETH/USD", []float64{200, 202, 201, 205, 203, 208, 204, 211, 206, 214, 208, 217})

			last := measurements[len(measurements)-1]

			So(last, ShouldNotBeNil)
			So(last.Err, ShouldBeNil)
			So(last.SNRDefined, ShouldBeTrue)
			So(last.SNR, ShouldBeGreaterThanOrEqualTo, 0.0)
		})

		Convey("time regression surfaces as zero support without error", func() {
			sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue(tick("BTC/USD", 100.0, timestamp(2)))))

			measurement := sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue(tick("BTC/USD", 101.0, timestamp(1)))))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metadata[data.MetadataSupport], ShouldEqual, "0")
			So(measurement.Provenance["event_time_state"], ShouldEqual, "regressed")
		})

		Convey("Register declares the full metric schema without values", func() {
			measurement := entity.Register()

			So(measurement.ID, ShouldEqual, -1)
			So(measurement.Metrics, ShouldContainKey, "last_price")
			So(measurement.Metrics, ShouldContainKey, "signed_correlation")

			for label, metric := range measurement.Metrics {
				So(label, ShouldEqual, metric.Label)
				So(metric.Raw, ShouldEqual, 0.0)
			}
		})
	})
}

func BenchmarkTickerCrossSectionNext(b *testing.B) {
	grid := store.NewGrid[*geometry.Coordinate]()
	entity := NewTicker(context.Background(), grid)

	for s := 0; s < benchmarkSymbols; s++ {
		symbol := benchmarkSymbol(s)
		for i := 0; i < benchmarkWarmup; i++ {
			sequence.Read[*data.Measurement[float64]](entity.Next(
				sequence.NewValue(
					tick(symbol, 100.0+float64(i), timestamp(int64(i)+1)),
				),
			))
		}
	}

	focal := benchmarkSymbol(0)
	measurement := tick(focal, 0, timestamp(benchmarkWarmup))
	i := 0
	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		measurement.Metrics["last_price"] = measurement.Metrics["last_price"].Write(100.0 + float64(i))
		measurement.At = timestamp(int64(benchmarkWarmup + i + 1))
		sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue(measurement)))
		i++
	}
}

func benchmarkSymbol(s int) string {
	return fmt.Sprintf("S%02d/USD", s)
}

const (
	benchmarkSymbols = 32
	benchmarkWarmup  = 64
)

func TestTickerStepReadiness(t *testing.T) {
	Convey("An inactive pipeline node drops input before touching processing state", t, func() {
		node := &Ticker{System: runtime.NewSystem(t.Context(), "readiness-test")}
		measurement := &data.Measurement[float64]{Label: "BTC/USD", SeqIdx: 7}
		for _, stage := range []runtime.Stage{runtime.INIT, runtime.WAITING, runtime.ERROR, runtime.FATAL} {
			node.Transition(stage)
			So(sequence.Read[*data.Measurement[float64]](node.Next(sequence.NewValue(measurement))), ShouldEqual, measurement)
			So(node.Status(), ShouldEqual, stage)
			So(measurement.SeqIdx, ShouldEqual, 7)
		}
	})
}
