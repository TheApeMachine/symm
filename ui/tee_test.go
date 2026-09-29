package ui

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	grid "github.com/theapemachine/symm/nomagique/learning/associative/grid"
	"github.com/theapemachine/symm/nomagique/runtime"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/types"
)

func TestUITeeNext(t *testing.T) {
	Convey("Queued websocket metrics respect the current route and focus", t, func() {
		originalRoute, originalFocus := types.Route(), types.Focus()
		defer types.SetRoute(originalRoute)
		defer types.SetFocus(originalFocus)
		types.SetRoute("dashboard")
		types.SetFocus("BTC/USD")
		tee := NewUITee(t.Context(), "route-test", 32)
		tee.Transition(runtime.READY)
		defer func() { So(tee.Close(), ShouldBeNil) }()
		measurement := data.NewMeasurement[float64]("hawkes:trade", nil)
		measurement.Label = "BTC/USD"
		tee.Push(measurement)

		Convey("Changing route drops queued packets before encoding", func() {
			types.SetRoute("journal")
			So(tee.Next() == nil, ShouldBeTrue)
		})

		Convey("Changing focus drops queued packets for the old symbol", func() {
			types.SetFocus("ETH/USD")
			So(tee.Next() == nil, ShouldBeTrue)
		})

		Convey("The matching route and focus still receive their packets", func() {
			So(tee.Next() == nil, ShouldBeFalse)
			So(tee.Next() == nil, ShouldBeTrue)
		})
	})
}

func BenchmarkUITeeNext(b *testing.B) {
	originalRoute, originalFocus := types.Route(), types.Focus()
	defer types.SetRoute(originalRoute)
	defer types.SetFocus(originalFocus)
	types.SetRoute("xray")
	types.SetFocus("BTC/USD")
	tee := NewUITee(b.Context(), "benchmark", 32)
	tee.Transition(runtime.READY)
	defer func() {
		if err := tee.Close(); err != nil {
			b.Fatal(err)
		}
	}()
	measurement := data.NewMeasurement[float64]("hawkes:trade", nil)
	measurement.Label = "BTC/USD"
	measurement.Metrics["conditional_intensity"] = data.Metric[float64]{Label: "conditional_intensity", Raw: 1.2}
	measurement.Metrics["background_rate"] = data.Metric[float64]{Label: "background_rate", Raw: 0.5}

	for b.Loop() {
		tee.Push(measurement)

		if tee.Next() == nil {
			b.Fatal("matching measurement was dropped")
		}
	}
}

func TestUITeePush(t *testing.T) {
	Convey("A grid publication is materialized only for an accepted route and focus", t, func() {
		originalRoute, originalFocus := types.Route(), types.Focus()
		defer types.SetRoute(originalRoute)
		defer types.SetFocus(originalFocus)
		types.SetFocus("BTC/USD")
		types.SetRoute("learning")
		tee := NewUITee(t.Context(), "grid-test", 32)
		tee.Transition(runtime.READY)
		defer func() { So(tee.Close(), ShouldBeNil) }()
		snapshot := &grid.Snapshot{
			Label:  "BTC/USD",
			Volume: "42",
			Cells: []grid.Quantity{
				{ID: 101, Source: "sensorium", Label: "vol", X: 1.5, Y: 2.5, Value: 3.5, Activity: 1.0, Quality: 0.95, Present: true, Basin: 1},
			},
			Regions: []grid.Region{
				{ID: 1, Condition: 1, Level: 1.0, Change: 0.1, Strength: 0.8, Authority: 0.9, Members: 4},
			},
		}
		measurement := data.NewMeasurement[float64]("training", nil)
		measurement.Label, measurement.SeqIdx, measurement.Result = "BTC/USD", 1, snapshot

		Convey("a rejected route leaves the projection cadence available", func() {
			types.SetRoute("journal")
			tee.Push(measurement)
			So(tee.lastProjection.Load(), ShouldEqual, 0)
			So(tee.queue.Length(), ShouldEqual, 0)
		})

		Convey("queued bytes retain the accepted boundary after owners advance", func() {
			expected := snapshot
			tee.Push(measurement)

			payload := tee.Next()
			So(payload, ShouldNotBeNil)
			decoded := wire.GetRootAsMeasurementsFrame(*(*[]byte)(payload), 0).UnPack()
			So(len(decoded.Rows), ShouldEqual, 1)
			So(decoded.Rows[0].Tick, ShouldEqual, 1)
			So(decoded.Rows[0].Grid.Volume, ShouldEqual, expected.Volume)
			So(decoded.Rows[0].Grid.Quantities[0].Value, ShouldEqual, expected.Cells[0].Value)
			So(decoded.Rows[0].Grid.Quantities[0].X, ShouldEqual, expected.Cells[0].X)
			So(decoded.Rows[0].Grid.Quantities[0].Id, ShouldEqual, expected.Cells[0].ID)
		})
	})
}
