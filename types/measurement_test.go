package types

import (
	"strconv"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestObservationValidity(t *testing.T) {
	Convey("Given observation-window evidence counts", t, func() {
		Convey("Then empty windows are invalid", func() {
			validity := ObservationValidity(0)

			So(validity.State, ShouldEqual, ValidityInvalid)
			So(validity.Readiness, ShouldEqual, ReadinessObservation)
			So(validity.Reason, ShouldNotBeEmpty)
		})

		Convey("Then a single event stays provisional", func() {
			validity := ObservationValidity(1)

			So(validity.State, ShouldEqual, ValidityProvisional)
			So(validity.Readiness, ShouldEqual, ReadinessObservation)
			So(validity.Reason, ShouldNotBeEmpty)
		})

		Convey("Then corroborated windows are valid", func() {
			validity := ObservationValidity(2)

			So(validity.State, ShouldEqual, ValidityValid)
			So(validity.Readiness, ShouldEqual, ReadinessObservation)
			So(validity.Reason, ShouldBeEmpty)
		})
	})
}

func TestMeasurementCanonicalizeProvenance(t *testing.T) {
	Convey("Given mixed provenance timestamps", t, func() {
		at := time.Unix(10, 0)
		measurement := Measurement{
			At:           at,
			ObservedFrom: at.Add(2 * time.Second),
			Scale: ScaleReference{
				Kind:    ScaleObservationWindow,
				From:    at.Add(time.Second),
				Through: at.Add(3 * time.Second),
			},
		}

		Convey("It should fold them into one forward envelope", func() {
			measurement.CanonicalizeProvenance()
			So(measurement.ValidateStruct(), ShouldBeNil)
			So(measurement.ObservedFrom, ShouldEqual, at)
			So(measurement.At, ShouldEqual, at.Add(3*time.Second))
			So(measurement.Scale.From, ShouldEqual, at)
			So(measurement.Scale.Through, ShouldEqual, at.Add(3*time.Second))
		})
	})
}

func TestMeasurementValidateStruct(t *testing.T) {
	Convey("Given forward and backwards evidence intervals", t, func() {
		forward := Measurement{At: time.Unix(2, 0), ObservedFrom: time.Unix(1, 0)}
		backwards := Measurement{At: time.Unix(1, 0), ObservedFrom: time.Unix(2, 0)}

		Convey("Then validation canonicalizes mixed provenance before rejecting", func() {
			So(forward.ValidateStruct(), ShouldBeNil)
			So(backwards.ValidateStruct(), ShouldBeNil)
			So(backwards.ObservedFrom, ShouldEqual, time.Unix(1, 0))
			So(backwards.At, ShouldEqual, time.Unix(2, 0))
		})

		Convey("Then mixed scale provenance resolves to the forward envelope", func() {
			at := time.Unix(2, 0)
			mixed := Measurement{
				At: at,
				Scale: ScaleReference{
					Kind:    ScaleObservationWindow,
					From:    at.Add(3 * time.Second),
					Through: at.Add(time.Second),
				},
			}

			So(mixed.ValidateStruct(), ShouldBeNil)

			from, through := mixed.Interval()
			So(from, ShouldEqual, at)
			So(through, ShouldEqual, at.Add(3*time.Second))
		})
	})
}

func TestMeasurementInterval(t *testing.T) {
	Convey("Given explicit and implicit evidence provenance", t, func() {
		at := time.Unix(3, 0)
		explicit := Measurement{
			At: at, ObservedFrom: time.Unix(1, 0),
			Scale: ScaleReference{From: time.Unix(2, 0), Through: at},
		}
		implicit := Measurement{At: at}

		Convey("Then declared provenance wins before observation time", func() {
			from, through := explicit.Interval()
			implicitFrom, implicitThrough := implicit.Interval()
			So(from, ShouldEqual, time.Unix(1, 0))
			So(through, ShouldEqual, at)
			So(implicitFrom, ShouldEqual, at)
			So(implicitThrough, ShouldEqual, at)
		})
	})
}

func TestFilterLatest(t *testing.T) {
	Convey("Given unsynchronized measurement epochs across symbols", t, func() {
		btcOlder := &Measurement{
			Symbol: "BTC/USD",
			Metric: MetricBreadth,
			At:     time.Unix(1, 0),
		}
		btcBreadth := &Measurement{
			Symbol: "BTC/USD",
			Metric: MetricBreadth,
			At:     time.Unix(2, 0),
		}
		btcStrength := &Measurement{
			Symbol: "BTC/USD",
			Metric: MetricStrength,
			At:     time.Unix(2, 0),
		}
		ethLatest := &Measurement{
			Symbol: "ETH/USD",
			Metric: MetricStrength,
			At:     time.Unix(3, 0),
		}

		Convey("Then every symbol keeps its newest complete epoch", func() {
			filtered := FilterLatest([]*Measurement{
				btcOlder,
				btcBreadth,
				btcStrength,
				ethLatest,
			})

			So(filtered, ShouldResemble, []*Measurement{
				btcBreadth,
				btcStrength,
				ethLatest,
			})
		})
	})
}

func BenchmarkFilterLatest(b *testing.B) {
	const (
		symbolCount = 256
		metricCount = 9
		epochCount  = 3
	)

	measurements := make(
		[]*Measurement,
		0,
		symbolCount*metricCount*epochCount,
	)

	for symbolIndex := 0; symbolIndex < symbolCount; symbolIndex++ {
		symbol := "PAIR-" + strconv.Itoa(symbolIndex)

		for epochIndex := 0; epochIndex < epochCount; epochIndex++ {
			for metricIndex := 0; metricIndex < metricCount; metricIndex++ {
				measurements = append(measurements, &Measurement{
					Symbol: symbol,
					Metric: MetricStrength,
					At:     time.Unix(int64(epochIndex), 0),
				})
			}
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	for iteration := 0; b.Loop(); iteration++ {
		if len(FilterLatest(measurements)) != symbolCount*metricCount {
			b.Fatal("latest measurement epoch lost a symbol")
		}
	}
}
