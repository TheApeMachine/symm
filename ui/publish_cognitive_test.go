package ui

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/qpool"
)

func TestCognitiveFrame(t *testing.T) {
	Convey("Given one sealed cognitive reading", t, func() {
		frame := CognitiveFrame(CognitiveReadingWire{
			Scope:           "BTC/USD",
			Sequence:        "measurement/BTC/USD/fluid",
			RegimePrefix:    "regime/BTC",
			RegimeCohort:    2,
			Ambiguous:       true,
			Sideline:        true,
			EntropyBits:     1.5,
			ClassConfidence: 0.82,
			LookaheadPaths:  3,
			WinnerClass:     "laminar",
		})

		Convey("It should expose dashboard cognitive fields", func() {
			So(frame["type"], ShouldEqual, "cognitive")
			So(frame["scope"], ShouldEqual, "BTC/USD")
			So(frame["sequence"], ShouldEqual, "measurement/BTC/USD/fluid")
			So(frame["regime_prefix"], ShouldEqual, "regime/BTC")
			So(frame["regime_cohort"], ShouldEqual, 2)
			So(frame["ambiguous"], ShouldBeTrue)
			So(frame["sideline"], ShouldBeTrue)
			So(frame["lookahead_paths"], ShouldEqual, 3)
			So(frame["winner_class"], ShouldEqual, "laminar")
		})
	})
}

func TestPublishCognitive(t *testing.T) {
	Convey("Given one cognitive reading", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)

		received := make(chan map[string]any, 1)

		pool.Subscribe("ui", func(artifact *datura.Artifact) error {
			payload, decodeErr := qpool.ArtifactValue[map[string]any](artifact)

			if decodeErr != nil || payload["type"] != "cognitive" {
				return nil
			}

			received <- payload

			return nil
		})

		Convey("When PublishCognitive is called", func() {
			err := PublishCognitive(pool, CognitiveReadingWire{
				Scope:           "BTC/USD",
				Sequence:        "measurement/BTC/USD/fluid",
				ClassConfidence: 0.82,
			})

			Convey("It should emit one cognitive frame", func() {
				So(err, ShouldBeNil)

				var frame map[string]any

				select {
				case frame = <-received:
				case <-time.After(2 * time.Second):
					So("ui cognitive frame", ShouldEqual, "received")
				}

				So(frame["scope"], ShouldEqual, "BTC/USD")
				So(frame["class_confidence"], ShouldEqual, 0.82)
			})
		})
	})
}
