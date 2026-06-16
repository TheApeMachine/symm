package ui

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
)

func TestWireArtifactPayload(t *testing.T) {
	Convey("Given a ui artifact with a JSON payload", t, func() {
		payload := []byte(`{"type":"state","measurements":[]}`)

		artifact := datura.Acquire("ui", datura.Artifact_Type_json).
			WithRole("state")

		So(artifact.SetPayload(payload), ShouldBeNil)

		Convey("When wireArtifactPayload is called", func() {
			wirePayload, err := wireArtifactPayload(artifact)

			Convey("It should return the raw JSON bytes", func() {
				So(err, ShouldBeNil)
				So(string(wirePayload), ShouldEqual, string(payload))
			})
		})
	})
}

func TestPublishMeasurements(t *testing.T) {
	Convey("Given live measurements", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)

		received := make(chan map[string]any, 1)

		pool.Subscribe("ui", func(artifact *datura.Artifact) error {
			payload, decodeErr := qpool.ArtifactValue[map[string]any](artifact)

			if decodeErr != nil || payload["type"] != "state" {
				return nil
			}

			received <- payload

			return nil
		})

		measurements := []logic.Measurement{
			{
				Source:     logic.SourceFluid,
				Symbol:     "ETH/USD",
				Confidence: 0.72,
				Surprise:   1.1,
			},
			{
				Source:     logic.SourceHawkes,
				Symbol:     "ETH/USD",
				Confidence: 0.55,
				Surprise:   2.4,
			},
		}

		Convey("When PublishMeasurements is called", func() {
			err := PublishMeasurements(pool, measurements)

			Convey("It should emit one bulk state frame", func() {
				So(err, ShouldBeNil)

				var frame map[string]any

				select {
				case frame = <-received:
				case <-time.After(2 * time.Second):
					So("ui measurement state", ShouldEqual, "received")
				}

				So(frame["type"], ShouldEqual, "state")

				rawMeasurements, ok := frame["measurements"].([]any)

				So(ok, ShouldBeTrue)
				So(len(rawMeasurements), ShouldEqual, 2)
			})
		})
	})
}

func TestPublishMeasurementsEmpty(t *testing.T) {
	Convey("Given no measurements", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)

		received := make(chan struct{}, 1)

		pool.Subscribe("ui", func(artifact *datura.Artifact) error {
			received <- struct{}{}

			return nil
		})

		Convey("When PublishMeasurements is called", func() {
			err := PublishMeasurements(pool, nil)

			Convey("It should not publish", func() {
				So(err, ShouldBeNil)

				select {
				case <-received:
					So("ui frame", ShouldEqual, "absent")
				case <-time.After(50 * time.Millisecond):
				}
			})
		})
	})
}

func BenchmarkPublishMeasurements(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ[any](ctx, 1, 2, nil)

	pool.Subscribe("ui", func(artifact *datura.Artifact) error {
		_, _ = qpool.ArtifactValue[map[string]any](artifact)

		return nil
	})

	measurements := make([]logic.Measurement, 0, logic.SourceCount)

	for _, source := range logic.SpectrumSources {
		measurements = append(measurements, logic.Measurement{
			Source:     source,
			Symbol:     "ETH/USD",
			Confidence: 0.6,
			Surprise:   1.2,
			Strength:   0.4,
		})
	}

	b.ReportAllocs()

	for b.Loop() {
		if err := PublishMeasurements(pool, measurements); err != nil {
			b.Fatal(err)
		}
	}
}
