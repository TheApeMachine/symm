package logic

import (
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestComposerCompose(t *testing.T) {
	Convey("Given typed measurements at two exact event times", t, func() {
		composer := NewComposer()
		first := composerMeasurement("BTC/USD", time.Unix(2, 0), types.SideBuy)
		second := composerMeasurement("BTC/USD", time.Unix(1, 0), types.SideBuy)
		third := composerMeasurement("BTC/USD", time.Unix(1, 0), types.SideSell)

		Convey("When the batch is composed", func() {
			epochs, err := composer.Compose([]*types.Measurement{first, second, third})

			Convey("Then exact peers stay together and epochs remain chronological", func() {
				So(err, ShouldBeNil)
				So(epochs, ShouldHaveLength, 2)
				So(epochs[0].At, ShouldEqual, time.Unix(1, 0))
				So(epochs[0].Measurements, ShouldHaveLength, 2)
				So(epochs[0].Measurements[0].Side, ShouldEqual, types.SideBuy)
				So(epochs[0].Measurements[1].Side, ShouldEqual, types.SideSell)
				So(epochs[1].At, ShouldEqual, time.Unix(2, 0))
			})
		})
	})

	Convey("Given a legacy category measurement", t, func() {
		composer := NewComposer()
		measurement := &types.Measurement{
			Source: types.SourceCVD,
			Symbol: "BTC/USD",
		}

		Convey("When it enters the typed composer directly", func() {
			_, err := composer.Compose([]*types.Measurement{measurement})

			Convey("Then the migration boundary is rejected rather than inferred", func() {
				So(err, ShouldNotBeNil)
			})
		})
	})
}

func BenchmarkComposerCompose(b *testing.B) {
	const symbols = 1455
	measurements := make([]*types.Measurement, 0, symbols*2)
	at := time.Unix(1, 0)

	for index := range symbols {
		symbol := fmt.Sprintf("ASSET-%04d/USD", index)
		measurements = append(
			measurements,
			composerMeasurement(symbol, at, types.SideBuy),
			composerMeasurement(symbol, at, types.SideSell),
		)
	}

	composer := NewComposer()
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		epochs, err := composer.Compose(measurements)

		if err != nil {
			b.Fatal(err)
		}

		if len(epochs) != symbols {
			b.Fatalf("expected %d epochs, got %d", symbols, len(epochs))
		}
	}
}

func composerMeasurement(
	symbol string,
	at time.Time,
	side types.MeasurementSide,
) *types.Measurement {
	from := at.Add(-time.Second)

	return &types.Measurement{
		Source:       types.SourceHawkes,
		Metric:       types.MetricConditionalIntensity,
		Subject:      types.SubjectHawkesProcess,
		Stream:       "trades",
		Symbol:       symbol,
		Side:         side,
		At:           at,
		ObservedFrom: from,
		Horizon:      at.Sub(from),
		Unit:         types.UnitEventsPerSecond,
		Raw:          2,
		Maturity:     0.5,
		Validity: types.MeasurementValidity{
			State:     types.ValidityValid,
			Readiness: types.ReadinessIntensity,
		},
		Scale: types.ScaleReference{
			Kind:    types.ScaleObservationWindow,
			From:    from,
			Through: at,
		},
	}
}
