package trader

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/ui"
	. "github.com/theapemachine/symm/signal"
)

func TestCryptoMeasurePublishesCognitiveReadings(testingTB *testing.T) {
	Convey("Given tree classifier measurements", testingTB, func() {
		pool := productionPool(testingTB)

		defer pool.Close()

		published := make([]map[string]any, 0, 1)

		pool.Subscribe("ui", func(artifact *datura.Artifact) error {
			payload, decodeErr := qpool.ArtifactValue[map[string]any](artifact)

			if decodeErr != nil || payload["type"] != "cognitive" {
				return nil
			}

			published = append(published, payload)

			return nil
		})

		tree := NewTestTree()
		crypto := NewCrypto(context.Background(), pool, tree)

		defer crypto.Close()

		viper.Set("market.default_symbols", []string{"BTC/USD"})
		defer viper.Reset()

		insertClassifierMeasurement(tree, "fluid", "BTC/USD", 1, 0.82)
		insertClassifierMeasurement(tree, "hawkes", "BTC/USD", 2, 0.76)

		crypto.measure()

		Convey("It should publish cognitive frames for sealed scopes", func() {
			frame, ok := cognitivePublishedFrame(published, "BTC/USD")

			So(ok, ShouldBeTrue)
			So(frame["type"], ShouldEqual, "cognitive")
			So(frame["scope"], ShouldEqual, "BTC/USD")
			So(frame["sequence"], ShouldContainSubstring, "BTC/USD")
		})
	})
}

func TestCryptoCognitivePublishMatchesConnectSnapshot(testingTB *testing.T) {
	Convey("Given sealed cognitive readings", testingTB, func() {
		pool := productionPool(testingTB)

		defer pool.Close()

		published := make([]map[string]any, 0, 1)

		pool.Subscribe("ui", func(artifact *datura.Artifact) error {
			payload, decodeErr := qpool.ArtifactValue[map[string]any](artifact)

			if decodeErr != nil || payload["type"] != "cognitive" {
				return nil
			}

			published = append(published, payload)

			return nil
		})

		tree := NewTestTree()
		crypto := NewCrypto(context.Background(), pool, tree)

		defer crypto.Close()

		viper.Set("market.default_symbols", []string{"BTC/USD"})
		defer viper.Reset()

		insertClassifierMeasurement(tree, "fluid", "BTC/USD", 1, 0.82)
		insertClassifierMeasurement(tree, "hawkes", "BTC/USD", 2, 0.76)

		crypto.measure()

		Convey("It should match connect snapshot cognitive frames", func() {
			publishedFrame, ok := cognitivePublishedFrame(published, "BTC/USD")

			So(ok, ShouldBeTrue)

			snapshotFrame, ok := cognitiveConnectFrame(crypto, "BTC/USD")

			So(ok, ShouldBeTrue)
			So(snapshotFrame["scope"], ShouldEqual, publishedFrame["scope"])
			So(snapshotFrame["sequence"], ShouldEqual, publishedFrame["sequence"])
		})
	})
}

func cognitivePublishedFrame(
	frames []map[string]any,
	scope string,
) (map[string]any, bool) {
	for _, frame := range frames {
		if frame["scope"] == scope {
			return frame, true
		}
	}

	return nil, false
}

func cognitiveConnectFrame(crypto *Crypto, scope string) (map[string]any, bool) {
	for _, frame := range crypto.ConnectSnapshotFrames() {
		if frame["type"] == "cognitive" && frame["scope"] == scope {
			return frame, true
		}
	}

	return nil, false
}

func BenchmarkPublishCognitiveReadings(b *testing.B) {
	pool := productionPool(b)

	defer pool.Close()

	pool.Subscribe("ui", func(artifact *datura.Artifact) error {
		return nil
	})

	reading := ui.CognitiveReadingWire{
		Scope:           "BTC/USD",
		Sequence:        "measurement/BTC/USD/fluid",
		ClassConfidence: 0.82,
	}

	b.ReportAllocs()

	for b.Loop() {
		if err := ui.PublishCognitiveReadings(pool, []ui.CognitiveReadingWire{reading}); err != nil {
			b.Fatal(err)
		}
	}
}
