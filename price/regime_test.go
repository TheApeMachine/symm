package price

import (
	"context"
	"testing"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPredictionFeedbackUsesPerspectiveRegime(t *testing.T) {
	Convey("Given an open prediction with a market regime", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		t.Cleanup(func() { pool.Close() })

		prediction := NewPrediction(ctx, pool)
		feedback := pool.CreateBroadcastGroup("feedback", 10*time.Millisecond)
		subscriber := feedback.Subscribe("test:feedback-regime", 8)
		source := engine.PerspectiveSource(engine.PerspectiveMicrostructure)
		now := time.Now()
		prediction.prices["BTC/EUR"] = 101
		prediction.open["BTC/EUR"] = map[string]openPrediction{
			source: {
				perspective: engine.Perspective{
					Type:   engine.PerspectiveMicrostructure,
					Regime: engine.RegimeChoppy,
				},
				measurement: engine.Measurement{
					Last:   100,
					Source: "pumpdump",
					Type:   engine.Pump,
					Regime: "microstructure",
					Pairs:  []asset.Pair{{Wsname: "BTC/EUR"}},
				},
				source:          source,
				sources:         []string{"pumpdump"},
				predictedReturn: 0.01,
				confidence:      0.8,
				anchorPrice:     100,
				direction:       1,
				runway:          time.Second,
				dueAt:           now.Add(-time.Millisecond),
				predictedAt:     now.Add(-time.Second),
			},
		}

		prediction.settleDueAt(now)

		Convey("It should settle feedback into the market-regime bucket", func() {
			select {
			case value := <-subscriber.Incoming:
				payload := value.Value.(engine.PredictionFeedback)

				So(payload.Regime, ShouldEqual, "choppy")
			case <-time.After(time.Second):
				t.Fatal("expected settled feedback")
			}
		})
	})
}
