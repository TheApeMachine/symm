package ui

import (
	"context"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/qpool"
)

func TestHubReceivesStateFrame(testingTB *testing.T) {
	Convey("Given the ui broadcast path used by trader and hub", testingTB, func() {
		ctx, cancel := context.WithCancel(context.Background())

		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		subscription := pool.Subscribe("ui", nil)
		group := pool.CreateBroadcastGroup("ui")

		payload, err := sonic.Marshal(map[string]any{
			"type": "state",
			"measurements": []map[string]any{
				{
					"origin": "fluid",
					"scope":  "BTC/USD",
				},
			},
		})

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

			gaugeReadings, ok := decoded["measurements"].([]any)

			So(ok, ShouldBeTrue)
			So(len(gaugeReadings), ShouldEqual, 1)

			reading, ok := gaugeReadings[0].(map[string]any)

			So(ok, ShouldBeTrue)
			So(reading["origin"], ShouldEqual, "fluid")
			So(reading["scope"], ShouldEqual, "BTC/USD")
		})
	})
}

func TestHubRelayCachesStateFrame(testingTB *testing.T) {
	Convey("Given a hub relay consuming the ui broadcast group", testingTB, func() {
		ctx, cancel := context.WithCancel(context.Background())

		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		hub := NewHub(ctx, pool)
		group := pool.CreateBroadcastGroup("ui")

		payload, err := sonic.Marshal(map[string]any{
			"type": "state",
			"measurements": []map[string]any{
				{
					"origin": "hawkes",
					"scope":  "BTC/USD",
				},
			},
		})

		So(err, ShouldBeNil)

		artifact := datura.Acquire("trader", datura.Artifact_Type_json).
			WithPayload(payload).
			WithDestination("ui").
			WithRole("state")

		Convey("When trader publishes a state frame", func() {
			So(group.Send(artifact), ShouldBeNil)

			deadline := timeAfter(ctx, 2*time.Second)

			for {
				if _, ok := hub.cachedWire.Load("state"); ok {
					break
				}

				if deadline() {
					So("relay cache", ShouldEqual, "populated")
				}
			}

			cached, ok := hub.cachedWire.Load("state")

			So(ok, ShouldBeTrue)
			So(len(cached.([]byte)), ShouldBeGreaterThan, 0)
		})
	})
}

func timeAfter(ctx context.Context, duration time.Duration) func() bool {
	deadline := time.Now().Add(duration)

	return func() bool {
		if ctx.Err() != nil {
			return true
		}

		return time.Now().After(deadline)
	}
}
