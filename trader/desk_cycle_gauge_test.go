package trader

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
	. "github.com/theapemachine/symm/signal"
)

func TestCryptoMeasurePublishesGaugeReadings(testingTB *testing.T) {
	Convey("Given tree classifier measurements", testingTB, func() {
		pool := productionPool(testingTB)

		defer pool.Close()

		received := make(chan map[string]any, 1)

		pool.Subscribe("ui", func(artifact *datura.Artifact) error {
			payload, decodeErr := qpool.ArtifactValue[map[string]any](artifact)

			if decodeErr != nil || payload["type"] != "state" {
				return nil
			}

			received <- payload

			return nil
		})

		tree := NewTestTree()
		crypto := NewCrypto(context.Background(), pool, tree)

		defer crypto.Close()

		insertClassifierMeasurement(tree, "fluid", "BTC/USD", 1, 0.82)
		insertClassifierMeasurement(tree, "hawkes", "BTC/USD", 2, 0.76)

		crypto.measure()

		Convey("It should publish state frames with gauge readings", func() {
			var frame map[string]any

			select {
			case frame = <-received:
			case <-time.After(2 * time.Second):
				So("ui state frame", ShouldEqual, "received")
			}

			So(frame["type"], ShouldEqual, "state")
			So(frame["story_ticks"], ShouldEqual, 1)

			gaugeReadings, ok := frame["gauge_readings"].([]any)

			So(ok, ShouldBeTrue)
			So(len(gaugeReadings), ShouldEqual, 2)

			sources := make(map[string]bool, len(gaugeReadings))

			for _, raw := range gaugeReadings {
				reading, readingOK := raw.(map[string]any)

				So(readingOK, ShouldBeTrue)
				sources[reading["source"].(string)] = true
			}

			So(sources["fluid"], ShouldBeTrue)
			So(sources["hawkes"], ShouldBeTrue)
		})
	})
}

func BenchmarkCryptoDeskCycle(b *testing.B) {
	pool := productionPool(b)

	defer pool.Close()

	tree := NewTestTree()

	artifact := datura.Acquire("fluid", datura.Artifact_Type_json).
		WithRole("measurement").
		WithScope("BTC/USD")
	artifact.WithAttribute("classifier.category", 1)
	artifact.WithAttribute("classifier.confidence", 0.72)
	artifact.WithAttribute("classifier.strength", 0.5)

	InsertMeasurement(tree, artifact)

	pool.Subscribe("kraken:private", func(artifact *datura.Artifact) error {
		return nil
	})

	b.ReportAllocs()

	for b.Loop() {
		crypto := NewCrypto(context.Background(), pool, tree)

		if crypto == nil {
			b.Fatal("NewCrypto returned nil")
		}

		if updateErr := crypto.story.Update(storyBalanceArtifact(logic.Balances{
			Inventory: map[string]float64{"BTC/USD": 1},
		})); updateErr != nil {
			b.Fatal(updateErr)
		}

		crypto.measure()
		crypto.applyPlaybookActions()
		_ = crypto.Close()
	}
}
