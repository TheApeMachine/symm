package trader

import (
	"context"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
)

func TestCryptoSendRequiresUIDestination(t *testing.T) {
	Convey("Given a ui state frame without destination", t, func() {
		ctx, cancel := context.WithCancel(context.Background())

		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		crypto := NewCrypto(ctx, pool, dmt.NewTree(t.TempDir()))

		artifact := datura.Acquire("trader", datura.Artifact_Type_json).
			WithPayload([]byte(`{"type":"state","measurements":[]}`))

		So(artifact, ShouldNotBeNil)
		So(crypto.uiBroadcast.Send(artifact), ShouldNotBeNil)
	})
}

func TestCryptoPublishesStateFrame(t *testing.T) {
	Convey("Given a trader publishing ui state through qpool", t, func() {
		ctx, cancel := context.WithCancel(context.Background())

		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		subscription := pool.Subscribe("ui", nil)
		tree := dmt.NewTree(t.TempDir())

		crypto := NewCrypto(ctx, pool, tree)

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
		So(crypto.uiBroadcast.Send(artifact), ShouldBeNil)

		received, waitErr := subscription.Wait(ctx)

		So(waitErr, ShouldBeNil)

		var decoded map[string]any

		So(sonic.Unmarshal(received.DecryptPayload(), &decoded), ShouldBeNil)
		So(decoded["type"], ShouldEqual, "state")

		gaugeReadings, ok := decoded["measurements"].([]any)

		So(ok, ShouldBeTrue)
		So(len(gaugeReadings), ShouldEqual, 1)
	})
}

func TestCryptoRunPublishesMeasurementFrames(t *testing.T) {
	Convey("Given ingested ticker frames and a ui subscriber", t, func() {
		ctx, cancel := context.WithCancel(context.Background())

		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		subscription := pool.Subscribe("ui", nil)
		tree := dmt.NewTree(t.TempDir())
		crypto := NewCrypto(ctx, pool, tree)

		defer func() {
			_ = crypto.Close()
		}()

		replayAt := time.Now().UnixNano()
		ingestProgressiveTicker(tree, 59, 100, 10000, &replayAt)
		ingestVerticalTicker(tree, &replayAt)
		insertManifoldFeaturesForScope(tree, "update", []float64{1, 0.9, 10, 2, 50000})

		done := make(chan error, 1)

		go func() {
			done <- crypto.Run()
		}()

		received, waitErr := subscription.Wait(ctx)

		cancel()

		select {
		case runErr := <-done:
			So(runErr, ShouldBeNil)
		case <-time.After(2 * time.Second):
			So("Run did not stop after cancel", ShouldBeBlank)
		}

		So(waitErr, ShouldBeNil)
		So(received, ShouldNotBeNil)

		role, _ := received.Role()

		So(role, ShouldEqual, "measurement")

		destination, _ := received.Destination()

		So(destination, ShouldEqual, "ui")

		origin, _ := received.Origin()

		So(origin, ShouldNotBeBlank)
		So(datura.Peek[float64](received, "output", "confidence"), ShouldBeGreaterThan, 0)
		So(datura.Peek[bool](received, "calibrated"), ShouldBeTrue)
	})
}

func TestCryptoRunStoryTick(testingTB *testing.T) {
	Convey("Given ingested ticker frames", testingTB, func() {
		ctx, cancel := context.WithCancel(context.Background())

		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		tree := dmt.NewTree(testingTB.TempDir())
		crypto := NewCrypto(ctx, pool, tree)

		defer func() {
			_ = crypto.Close()
		}()

		replayAt := time.Now().UnixNano()
		ingestProgressiveTicker(tree, 59, 100, 10000, &replayAt)
		ingestVerticalTicker(tree, &replayAt)
		insertManifoldFeaturesForScope(tree, "update", []float64{1, 0.9, 10, 2, 50000})

		done := make(chan struct{}, 1)

		go func() {
			_ = crypto.Run()
			done <- struct{}{}
		}()

		time.Sleep(1200 * time.Millisecond)
		cancel()

		select {
		case <-done:
		case <-time.After(2 * time.Second):
			So("Run did not stop after cancel", ShouldBeBlank)
		}

		So(crypto.storyTicks.Load(), ShouldBeGreaterThan, 0)
	})
}

func BenchmarkCryptoMeasurementPublish(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ[any](ctx, 1, 2, nil)
	tree := dmt.NewTree(b.TempDir())
	crypto := NewCrypto(ctx, pool, tree)

	measurement := datura.Acquire("fluid", datura.Artifact_Type_json)
	measurement.WithRole("measurement")
	measurement.WithScope("trader")
	_ = measurement.SetOrigin(string(logic.SourceFluid))
	measurement.Poke(datura.Map[float64]{
		"value":      float64(logic.CategoryIndex(logic.CategoryLaminar)),
		"confidence": 0.71,
		"strength":   0.36,
	}, "output")

	b.ResetTimer()

	for b.Loop() {
		measurement.WithDestination("ui")

		if err := crypto.uiBroadcast.Send(measurement); err != nil {
			b.Fatal(err)
		}
	}
}
