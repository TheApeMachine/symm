package trader

import (
	"context"
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
)

func TestCryptoSendRequiresUIDestination(testingTB *testing.T) {
	Convey("Given a ui state frame without destination", testingTB, func() {
		ctx, cancel := context.WithCancel(context.Background())

		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		crypto := NewCrypto(ctx, pool, dmt.NewTree(testingTB.TempDir()))

		artifact := datura.Acquire("trader", datura.Artifact_Type_json).
			WithPayload([]byte(`{"type":"state","gauge_readings":[]}`))

		So(artifact, ShouldNotBeNil)
		So(crypto.uiBroadcast.Send(artifact), ShouldNotBeNil)
	})
}

func TestCryptoPublishesStateFrame(testingTB *testing.T) {
	Convey("Given a trader publishing ui state through qpool", testingTB, func() {
		ctx, cancel := context.WithCancel(context.Background())

		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		subscription := pool.Subscribe("ui", nil)
		tree := dmt.NewTree(testingTB.TempDir())

		crypto := NewCrypto(ctx, pool, tree)

		measurement := datura.Acquire("fluid", datura.Artifact_Type_json)
		measurement.WithRole("measurement")
		measurement.WithScope("BTC/USD")
		So(measurement.SetOrigin(string(logic.SourceFluid)), ShouldBeNil)
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
		So(crypto.uiBroadcast.Send(artifact), ShouldBeNil)

		received, waitErr := subscription.Wait(ctx)

		So(waitErr, ShouldBeNil)

		var decoded map[string]any

		So(sonic.Unmarshal(received.DecryptPayload(), &decoded), ShouldBeNil)
		So(decoded["type"], ShouldEqual, "state")

		gaugeReadings, ok := decoded["gauge_readings"].([]any)

		So(ok, ShouldBeTrue)
		So(len(gaugeReadings), ShouldEqual, 1)
	})
}

func TestCryptoRunPublishesOnTicker(testingTB *testing.T) {
	Convey("Given a running trader ui loop", testingTB, func() {
		ctx, cancel := context.WithCancel(context.Background())

		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		subscription := pool.Subscribe("ui", nil)
		tree := dmt.NewTree(testingTB.TempDir())

		crypto := NewCrypto(ctx, pool, tree)

		go func() {
			_ = crypto.Run()
		}()

		received, waitErr := subscription.Wait(ctx)

		cancel()
		_ = crypto.Close()

		So(waitErr, ShouldBeNil)

		var decoded map[string]any

		So(sonic.Unmarshal(received.DecryptPayload(), &decoded), ShouldBeNil)
		So(decoded["type"], ShouldEqual, "state")
	})
}

func BenchmarkCryptoStateFramePublish(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ[any](ctx, 1, 2, nil)
	tree := dmt.NewTree(b.TempDir())
	crypto := NewCrypto(ctx, pool, tree)

	measurement := datura.Acquire("fluid", datura.Artifact_Type_json)
	measurement.WithRole("measurement")
	measurement.WithScope("BTC/USD")
	_ = measurement.SetOrigin(string(logic.SourceFluid))
	measurement.Poke(datura.Map[float64]{
		"value":      float64(logic.CategoryIndex(logic.CategoryLaminar)),
		"confidence": 0.71,
		"strength":   0.36,
	}, "output")

	payload, err := sonic.Marshal(measurement)

	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()

	for b.Loop() {
		artifact := datura.Acquire("trader", datura.Artifact_Type_json).
			WithPayload(payload)
		artifact.WithDestination("ui")

		if err := crypto.uiBroadcast.Send(artifact); err != nil {
			b.Fatal(err)
		}
	}
}
