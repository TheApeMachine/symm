package trader

import (
	"context"
	"encoding/binary"
	"math"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/signal/pumpdump"
	"github.com/theapemachine/symm/tests"
)

func insertManifoldFeaturesForScope(tree *dmt.Tree, scope string, samples []float64) {
	payload := make([]byte, 8*len(samples))

	for index, sample := range samples {
		offset := index * 8
		binary.BigEndian.PutUint64(payload[offset:offset+8], math.Float64bits(sample))
	}

	artifact := datura.Acquire("manifold-features", datura.APPJSON)
	artifact.WithRole("features")
	artifact.WithScope(scope)
	artifact.WithPayload(payload)

	if wire := artifact.Pack(); len(wire) > 0 {
		tree.Insert(artifact.Prefix(), wire)
	}

	artifact.Release()
}

func TestSignalMeasureStateFrame(testingTB *testing.T) {
	Convey("Given the trader measure loop", testingTB, func() {
		ctx, cancel := context.WithCancel(context.Background())

		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		tree := dmt.NewTree(testingTB.TempDir())
		signals := NewSignal(ctx, pool, tree)

		defer func() {
			_ = signals.Close()
		}()

		So(len(signals.sources), ShouldEqual, 13)
		So(len(signals.signals), ShouldEqual, len(signals.sources))
		So(len(signals.Measure()), ShouldEqual, 0)

		tests.NewFixture(tests.FixtureTypeTicker).Ingest(tree, time.Now().UnixNano())
		insertManifoldFeaturesForScope(tree, "update", []float64{1, 0.9, 10, 2, 50000})

		measurements := signals.Measure()

		So(len(measurements), ShouldEqual, len(signals.sources))

		Convey("It should return non-empty artifacts for each wired signal", func() {
			for _, measurement := range measurements {
				So(measurement, ShouldNotBeNil)
				So(len(measurement.Marshal()), ShouldBeGreaterThan, 0)
			}
		})
	})
}

func TestCryptoRunNilSafe(testingTB *testing.T) {
	Convey("Given live measure ticks with partial signal output", testingTB, func() {
		ctx, cancel := context.WithCancel(context.Background())

		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		tree := dmt.NewTree(testingTB.TempDir())
		crypto := NewCrypto(ctx, pool, tree)

		defer func() {
			_ = crypto.Close()
		}()

		tests.NewFixture(tests.FixtureTypeTicker).Ingest(tree, time.Now().UnixNano())

		done := make(chan error, 1)

		go func() {
			done <- crypto.Run()
		}()

		time.Sleep(1100 * time.Millisecond)
		cancel()

		select {
		case runErr := <-done:
			So(runErr, ShouldBeNil)
		case <-time.After(2 * time.Second):
			So("Run did not stop after cancel", ShouldBeBlank)
		}
	})
}

func TestCryptoMeasureWithIngestedFixtures(testingTB *testing.T) {
	Convey("Given ingested ticker frames on the shared tree", testingTB, func() {
		ctx, cancel := context.WithCancel(context.Background())

		defer cancel()

		tree := dmt.NewTree(testingTB.TempDir())
		signals := NewSignal(ctx, qpool.NewQ[any](ctx, 1, 2, nil), tree)

		defer func() {
			_ = signals.Close()
		}()

		replayAt := time.Now().UnixNano()

		const replayTicks = 60

		for tick := range replayTicks {
			tests.NewFixture(tests.FixtureTypeTicker).Ingest(
				tree,
				replayAt+int64(tick),
			)
		}

		ingestRows := 0

		for _, role := range []string{"ticker", "book", "trade", "ohlc"} {
			for range tree.Seek([]byte(role + "/update")) {
				ingestRows++
			}
		}

		measurements := signals.Measure()

		So(len(measurements), ShouldBeGreaterThan, 0)
		So(len(measurements), ShouldBeLessThanOrEqualTo, len(signals.sources)*ingestRows)

		for _, measurement := range measurements {
			So(measurement, ShouldNotBeNil)
		}

		Convey("It should ship classifier output on pumpdump", func() {
			var pumpdumpMeasurement *datura.Artifact

			for _, measurement := range measurements {
				confidence := datura.Peek[float64](measurement, "output", "confidence")

				if confidence <= 0 {
					continue
				}

				pumpdumpMeasurement = measurement
				break
			}

			So(pumpdumpMeasurement, ShouldNotBeNil)

			plain := pumpdumpMeasurement.DecryptPayload()

			So(len(plain), ShouldBeGreaterThan, 2)

			var body map[string]any

			So(sonic.Unmarshal(plain, &body), ShouldBeNil)

			output, ok := body["output"].(map[string]any)

			So(ok, ShouldBeTrue)
			So(output["confidence"], ShouldNotBeNil)
			So(datura.Peek[float64](pumpdumpMeasurement, "output", "confidence"), ShouldBeGreaterThan, 0)
		})
	})
}

func TestSignalMeasureIngestedFixtures(testingTB *testing.T) {
	Convey("Given kraken ticker fixtures ingested like the public websocket", testingTB, func() {
		ctx, cancel := context.WithCancel(context.Background())

		defer cancel()

		tree := dmt.NewTree(testingTB.TempDir())
		signal := pumpdump.NewSignal(ctx, qpool.NewQ[any](ctx, 1, 2, nil), tree)

		defer func() {
			_ = signal.Close()
		}()

		replayAt := time.Now().UnixNano()

		for tick := range 60 {
			tests.NewFixture(tests.FixtureTypeTicker).Ingest(
				tree,
				replayAt+int64(tick),
			)
		}

		var measurement *datura.Artifact

		for stored := range tree.Seek([]byte("ticker/update")) {
			measurement = signal.Measure(stored)
		}

		Convey("It should ship classifier output through the hub wire shape", func() {
			So(measurement, ShouldNotBeNil)

			plain := measurement.DecryptPayload()

			So(len(plain), ShouldBeGreaterThan, 2)

			var body map[string]any

			So(sonic.Unmarshal(plain, &body), ShouldBeNil)

			output, ok := body["output"].(map[string]any)

			So(ok, ShouldBeTrue)
			So(output["confidence"], ShouldNotBeNil)

			measurement.WithDestination("ui")

			wire := measurement.Pack()

			So(len(wire), ShouldBeGreaterThan, 0)

			inbound := datura.Acquire("ui", datura.APPJSON)
			_, unpackErr := inbound.Unpack(wire)

			So(unpackErr, ShouldBeNil)
			So(datura.Peek[float64](inbound, "output", "confidence"), ShouldBeGreaterThan, 0)
		})
	})
}
