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

func TestMeasurementValidateStruct(t *testing.T) {
	Convey("Given forward and backwards evidence intervals", t, func() {
		forward := Measurement{At: time.Unix(2, 0), ObservedFrom: time.Unix(1, 0)}
		backwards := Measurement{At: time.Unix(1, 0), ObservedFrom: time.Unix(2, 0)}

		Convey("Then forward observation provenance is accepted unchanged", func() {
			So(forward.ValidateStruct(), ShouldBeNil)
			So(forward.ObservedFrom, ShouldEqual, time.Unix(1, 0))
			So(forward.At, ShouldEqual, time.Unix(2, 0))
		})

		Convey("Then ObservedFrom after At is rejected without mutation", func() {
			So(backwards.ValidateStruct(), ShouldNotBeNil)
			So(backwards.ObservedFrom, ShouldEqual, time.Unix(2, 0))
			So(backwards.At, ShouldEqual, time.Unix(1, 0))
		})

		Convey("Then a backwards scale interval is rejected without mutation", func() {
			at := time.Unix(2, 0)
			mixed := Measurement{
				At: at,
				Scale: ScaleReference{
					Kind:    ScaleObservationWindow,
					From:    at.Add(3 * time.Second),
					Through: at.Add(time.Second),
				},
			}

			So(mixed.ValidateStruct(), ShouldNotBeNil)
			So(mixed.Scale.From, ShouldEqual, at.Add(3*time.Second))
			So(mixed.Scale.Through, ShouldEqual, at.Add(time.Second))
		})
	})
}

func TestMeasurementInterval(t *testing.T) {
	Convey("Given observation provenance separate from scale", t, func() {
		at := time.Unix(3, 0)
		fitFrom := time.Unix(1, 0)
		explicit := Measurement{
			At: at, ObservedFrom: time.Unix(2, 0),
			Scale: ScaleReference{From: fitFrom, Through: at},
		}
		implicit := Measurement{At: at}
		scaleOnly := Measurement{
			At: at,
			Scale: ScaleReference{
				Kind: ScaleObservationWindow, From: fitFrom, Through: at,
			},
		}

		Convey("Then Interval is ObservedFrom→At and ignores Scale", func() {
			from, through := explicit.Interval()
			implicitFrom, implicitThrough := implicit.Interval()
			scaleFrom, scaleThrough := scaleOnly.Interval()

			So(from, ShouldEqual, time.Unix(2, 0))
			So(through, ShouldEqual, at)
			So(implicitFrom, ShouldEqual, at)
			So(implicitThrough, ShouldEqual, at)
			So(scaleFrom, ShouldEqual, at)
			So(scaleThrough, ShouldEqual, at)
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
