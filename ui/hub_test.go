package ui

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
)

func TestHubRun(t *testing.T) {
	Convey("Given a ui hub", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)

		listener, listenErr := net.Listen("tcp", "127.0.0.1:0")

		So(listenErr, ShouldBeNil)

		addr := listener.Addr().String()
		_ = listener.Close()

		viper.Set("ui.addr", addr)

		hub := NewHub(ctx, pool, nil)

		So(hub, ShouldNotBeNil)

		runErr := make(chan error, 1)

		go func() {
			runErr <- hub.Run()
		}()

		Convey("When Run is serving the dashboard websocket route", func() {
			var resp *http.Response

			for range 50 {
				var getErr error

				resp, getErr = http.Get("http://" + addr + "/ws")

				if getErr == nil {
					break
				}

				time.Sleep(20 * time.Millisecond)
			}

			Convey("It should accept websocket upgrade requests", func() {
				So(resp, ShouldNotBeNil)
				So(resp.StatusCode, ShouldEqual, http.StatusUpgradeRequired)
				_ = resp.Body.Close()
			})
		})

		Reset(func() {
			_ = hub.Close()

			select {
			case <-runErr:
			case <-time.After(2 * time.Second):
				So("hub run shutdown", ShouldEqual, "completed")
			}
		})
	})
}

func TestWireArtifactPayload(t *testing.T) {
	Convey("Given a ui artifact with a JSON payload", t, func() {
		payload := []byte(`{"type":"state","measurements":[]}`)

		artifact := datura.Acquire("ui", datura.Artifact_Type_json).
			WithRole("state")

		So(artifact.WithPayload(payload), ShouldNotBeNil)

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
			err := PublishMeasurements(pool, measurements, 3, 1, logic.WalkTrace{})

			Convey("It should emit one bulk state frame", func() {
				So(err, ShouldBeNil)

				var frame map[string]any

				select {
				case frame = <-received:
				case <-time.After(2 * time.Second):
					So("ui measurement state", ShouldEqual, "received")
				}

				So(frame["type"], ShouldEqual, "state")
				So(frame["story_ticks"], ShouldEqual, 3)

				rawMeasurements, ok := frame["measurements"].([]any)

				So(ok, ShouldBeTrue)
				So(len(rawMeasurements), ShouldEqual, 2)

				gaugeReadings, gaugeOK := frame["gauge_readings"].([]any)

				So(gaugeOK, ShouldBeTrue)
				So(len(gaugeReadings), ShouldEqual, 2)
			})
		})
	})
}

func TestPublishMeasurementsEmpty(t *testing.T) {
	Convey("Given no measurements", t, func() {
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

		Convey("When PublishMeasurements is called", func() {
			err := PublishMeasurements(pool, nil, 1, 0, logic.WalkTrace{})

			Convey("It should still publish the heartbeat state frame", func() {
				So(err, ShouldBeNil)

				var frame map[string]any

				select {
				case frame = <-received:
				case <-time.After(2 * time.Second):
					So("ui heartbeat frame", ShouldEqual, "received")
				}

				So(frame["story_ticks"], ShouldEqual, 1)
			})
		})
	})
}

func TestPublishPayload(t *testing.T) {
	Convey("Given a dashboard payload", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)

		received := make(chan map[string]any, 1)

		pool.Subscribe("ui", func(artifact *datura.Artifact) error {
			payload, decodeErr := qpool.ArtifactValue[map[string]any](artifact)

			if decodeErr != nil || payload["type"] != "fluid" {
				return nil
			}

			received <- payload

			return nil
		})

		Convey("When PublishPayload is called", func() {
			err := PublishPayload(pool, "fluid", map[string]any{
				"type":         "fluid",
				"symbol_count": 1,
				"symbols":      []map[string]any{{"symbol": "ETH/USD"}},
			})

			Convey("It should emit one dashboard frame", func() {
				So(err, ShouldBeNil)

				var frame map[string]any

				select {
				case frame = <-received:
				case <-time.After(2 * time.Second):
					So("ui dashboard payload", ShouldEqual, "received")
				}

				So(frame["type"], ShouldEqual, "fluid")
				So(frame["symbol_count"], ShouldEqual, 1)
			})
		})
	})
}

func TestPublishDecisionTree(t *testing.T) {
	Convey("Given embedded playbook branches", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)

		received := make(chan map[string]any, 1)

		pool.Subscribe("ui", func(artifact *datura.Artifact) error {
			payload, decodeErr := qpool.ArtifactValue[map[string]any](artifact)

			if decodeErr != nil || payload["type"] != "decision_tree" {
				return nil
			}

			received <- payload

			return nil
		})

		Convey("When PublishDecisionTree is called", func() {
			err := PublishDecisionTree(pool, []*logic.Branch{{
				ConditionGroup: &logic.ConditionGroup{
					Boolean: logic.BooleanTypeAnd,
				},
			}})

			Convey("It should emit one decision tree frame", func() {
				So(err, ShouldBeNil)

				var frame map[string]any

				select {
				case frame = <-received:
				case <-time.After(2 * time.Second):
					So("ui decision tree frame", ShouldEqual, "received")
				}

				So(frame["type"], ShouldEqual, "decision_tree")

				branches, ok := frame["branches"].([]any)

				So(ok, ShouldBeTrue)
				So(len(branches), ShouldEqual, 1)
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
		if err := PublishMeasurements(pool, measurements, 1, 1, logic.WalkTrace{}); err != nil {
			b.Fatal(err)
		}
	}
}
