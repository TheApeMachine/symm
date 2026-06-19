package ui

import (
	"context"
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
)

func TestHubReceivesStateFrame(testingTB *testing.T) {
	Convey("Given the ui broadcast path used by trader and hub", testingTB, func() {
		ctx, cancel := context.WithCancel(context.Background())

		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		subscription := pool.Subscribe("ui", nil)
		group := pool.CreateBroadcastGroup("ui")

		measurement := datura.Acquire("fluid", datura.Artifact_Type_json)
		measurement.WithRole("measurement")
		measurement.WithScope("BTC/USD")
		So(measurement.SetOrigin("fluid"), ShouldBeNil)
		measurement.Poke(datura.Map[float64]{
			"value":      float64(logic.CategoryIndex(logic.CategoryLaminar)),
			"confidence": 0.71,
			"strength":   0.36,
			"surprise":   2.4,
			"elapsed":    30.0,
		}, "output")

		payload, err := sonic.Marshal(measurement)

		So(err, ShouldBeNil)

		artifact := datura.Acquire("trader", datura.Artifact_Type_json).
			WithPayload(payload)

		So(artifact, ShouldNotBeNil)
		artifact.WithDestination("ui")

		Convey("When trader publishes through the ui broadcast group", func() {
			So(group.Send(artifact), ShouldBeNil)

			received, waitErr := subscription.Wait(ctx)

			So(waitErr, ShouldBeNil)
			So(received, ShouldNotBeNil)

			wire := received.DecryptPayload()

			So(len(wire), ShouldBeGreaterThan, 0)

			var decoded map[string]any

			So(sonic.Unmarshal(wire, &decoded), ShouldBeNil)
			So(decoded["type"], ShouldEqual, "state")

			gaugeReadings, ok := decoded["gauge_readings"].([]any)

			So(ok, ShouldBeTrue)
			So(len(gaugeReadings), ShouldEqual, 1)

			reading, ok := gaugeReadings[0].(map[string]any)

			So(ok, ShouldBeTrue)
			So(reading["source"], ShouldEqual, "fluid")
			So(reading["symbol"], ShouldEqual, "BTC/USD")
		})
	})
}
