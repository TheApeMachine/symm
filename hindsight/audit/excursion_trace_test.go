package audit

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
)

func TestExtractExcursionTraces(t *testing.T) {
	Convey("Given excursion trace extraction parameters", t, func() {
		ctx := t.Context()
		grid := store.NewGrid(ctx)

		Convey("when detections are empty, it returns nil", func() {
			traces := ExtractExcursionTraces(ctx, nil, 1, grid, nil, nil, 5)
			So(traces, ShouldBeNil)
		})

		Convey("when grid is nil, it returns nil", func() {
			at := time.Unix(100, 0).UTC()
			det := data.NewMeasurement(1, "BTC/USD", "detector", at, at, &data.StringEntry{Key: "type", Value: "up"})
			det.Write(
				data.NewMetric("start_tick", 10, data.UnitDimensionless, data.TimescaleTick),
				data.NewMetric("b_tick", 20, data.UnitDimensionless, data.TimescaleTick),
				data.NewMetric("c_tick", 30, data.UnitDimensionless, data.TimescaleTick),
				data.NewMetric("b_price", 100, data.UnitPrice, data.TimescaleTick),
				data.NewMetric("c_price", 105, data.UnitPrice, data.TimescaleTick),
			)
			traces := ExtractExcursionTraces(ctx, nil, 1, nil, []*data.Measurement{det}, nil, 5)
			So(traces, ShouldBeNil)
		})
	})
}

func TestFormatTransitionSequence(t *testing.T) {
	Convey("Given an excursion transition trace", t, func() {
		trace := ExcursionTransitionTrace{
			Transitions: []ExcursionTransitionFrame{
				{Token: "R03", DwellTicks: 5},
				{Token: "R05", DwellTicks: 2},
				{Token: "R04", DwellTicks: 1},
			},
		}

		Convey("it formats the transitions with dwell counts", func() {
			formatted := FormatTransitionSequence(trace)
			So(formatted, ShouldEqual, "R03(5) -> R05(2) -> R04(1)")
		})
	})
}
