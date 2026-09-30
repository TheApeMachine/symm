package ui

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
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

	}
}

func TestUITeePush(t *testing.T) {
	Convey("Push queues measurements correctly", t, func() {
		originalRoute, originalFocus := types.Route(), types.Focus()
		defer types.SetRoute(originalRoute)
		defer types.SetFocus(originalFocus)
		types.SetFocus("BTC/USD")
		types.SetRoute("learning")
		tee := NewUITee(t.Context(), "push-test", 32)
		tee.Transition(runtime.READY)
		defer func() { So(tee.Close(), ShouldBeNil) }()

		measurement := data.NewMeasurement[float64]("training", nil)
		measurement.Label, measurement.SeqIdx = "BTC/USD", 1

		Convey("A rejected route drops the measurement immediately", func() {
			types.SetRoute("journal")
			tee.Push(measurement)

			// Next returns a batch of up to tee.batchSize. If empty, it returns nil.
			payload := tee.Next()
			So(payload == nil, ShouldBeTrue)
		})

		Convey("An accepted route queues the cloned measurement for serialization", func() {
			types.SetRoute("learning")
			tee.Push(measurement)

			payload := tee.Next()
			So(payload != nil, ShouldBeTrue)

			decoded := wire.GetRootAsMeasurementsFrame(*(*[]byte)(payload), 0).UnPack()
			So(len(decoded.Rows), ShouldEqual, 1)
			So(decoded.Rows[0].Tick, ShouldEqual, 1)
			So(decoded.Rows[0].Symbol, ShouldEqual, "BTC/USD")
		})
	})
}
