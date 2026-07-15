package fluid

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFluidDynamicsEarliestStamp(t *testing.T) {
	Convey("Given an unsorted dynamics stamp trail", t, func() {
		latest := time.Date(2026, 7, 10, 4, 0, 2, 0, time.UTC)
		earliest := time.Date(2026, 7, 10, 4, 0, 0, 0, time.UTC)
		middle := time.Date(2026, 7, 10, 4, 0, 1, 0, time.UTC)
		dynamics := fluidDynamics{
			stamps: []float64{
				float64(latest.UnixNano()),
				float64(earliest.UnixNano()),
				float64(middle.UnixNano()),
			},
		}

		Convey("It should resolve the minimum retained event time", func() {
			So(dynamics.earliestStamp(), ShouldResemble, earliest)
		})
	})
}

func BenchmarkMeasurementsFromReading(b *testing.B) {
	book := NewBook(NewSyncRegistry())
	eventAt := time.Date(2026, 7, 10, 4, 0, 0, 0, time.UTC)
	observedFrom := eventAt.Add(-time.Second)
	reading := fluidReading{
		symbol:            "BTC/USD",
		reynolds:          0.5,
		divergence:        0.1,
		viscosity:         645450.5,
		velocityCurvature: 0.2,
		turbulence:        0.3,
		sourceBalance:     2,
		memory:            0.4,
		midAddRate:        3,
		midExecuteRate:    4,
		gridSteps:         2,
		dynamics: fluidDynamics{
			stamps: []float64{
				float64(eventAt.Add(time.Second).UnixNano()),
				float64(observedFrom.UnixNano()),
				float64(eventAt.UnixNano()),
			},
			reynoldsHistory:          []float64{0.5, 0.5, 0.5},
			divergenceHistory:        []float64{0.1, 0.1, 0.1},
			viscosityHistory:         []float64{645450.5, 645450.5, 645450.5},
			velocityCurvatureHistory: []float64{0.2, 0.2, 0.2},
			turbulenceHistory:        []float64{0.3, 0.3, 0.3},
		},
	}

	b.ReportAllocs()

	for b.Loop() {
		_, err := book.measurementsFromReading(reading, eventAt)

		if err != nil {
			b.Fatal(err)
		}
	}
}
