package trader

import (
	"context"
	"iter"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/ui"
)

type meanConfidenceSignal struct {
	source string
	mean   float64
}

var _ engine.MeanConfidenceReader = (*meanConfidenceSignal)(nil)

func (signal *meanConfidenceSignal) Source() string {
	return signal.source
}

func (signal *meanConfidenceSignal) Measure(
	_ context.Context,
	_ time.Time,
) iter.Seq[engine.Measurement] {
	return func(yield func(engine.Measurement) bool) {}
}

func (signal *meanConfidenceSignal) Feedback(_ engine.PredictionFeedback) {}

func (signal *meanConfidenceSignal) MeanConfidence() float64 {
	return signal.mean
}

func TestPublishSignalScoreAfterMeasure(t *testing.T) {
	Convey("Given a crypto trader with a ui broadcast group", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		uiGroup := pool.CreateBroadcastGroup("ui", 10*time.Millisecond)
		subscriber := uiGroup.Subscribe("test", 8)
		stream := ui.NewMarketStream(uiGroup)

		crypto, err := NewCrypto(
			ctx,
			pool,
			uiGroup,
			NewWallet(PaperWallet, "EUR", 200, 0.26),
			stubPrices{"PUMP/EUR": 100},
			&meanConfidenceSignal{source: "hawkes", mean: 0.42},
		)

		So(err, ShouldBeNil)
		crypto.BindUIStream(stream)

		crypto.collectMeasurements(
			&meanConfidenceSignal{source: "hawkes", mean: 0.42},
			time.Now(),
		)

		var payload map[string]any

		select {
		case value := <-subscriber.Incoming:
			payload, _ = value.Value.(map[string]any)
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for signal_score")
		}

		Convey("It should send one signal_score frame to the ui group", func() {
			So(payload["event"], ShouldEqual, "signal_score")
			So(payload["source"], ShouldEqual, "hawkes")
			So(payload["confidence"], ShouldAlmostEqual, 0.42, 0.0001)
		})
	})
}
