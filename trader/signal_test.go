package trader

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/tests"
)

func TestSignalMeasureReplayIngest(t *testing.T) {
	Convey("Given many ticker rows in the tree", t, func() {
		pool := qpool.NewQ[any](t.Context(), 1, 2, nil)
		tree := dmt.NewTree(t.TempDir())
		signals := NewSignal(t.Context(), pool, tree)

		defer func() {
			_ = signals.Close()
		}()

		So(len(signals.signals), ShouldEqual, 13)

		replayAt := time.Now().UnixNano()

		const replayTicks = 60

		for tick := range replayTicks {
			tests.NewFixture(tests.FixtureTypeTicker).Ingest(
				tree,
				replayAt+int64(tick),
			)
		}

		measurements := signals.Measure()

		So(len(measurements), ShouldBeGreaterThan, 0)
		So(len(measurements), ShouldBeGreaterThan, len(signals.signals))
	})
}

func TestSignalMeasurePumpdumpConfidence(t *testing.T) {
	Convey("Given ingested ticker rows", t, func() {
		pool := qpool.NewQ[any](t.Context(), 1, 2, nil)
		tree := dmt.NewTree(t.TempDir())
		signals := NewSignal(t.Context(), pool, tree)

		defer func() {
			_ = signals.Close()
		}()

		replayAt := time.Now().UnixNano()
		ingestProgressiveTicker(tree, 59, 100, 10000, &replayAt)
		ingestVerticalTicker(tree, &replayAt)
		insertManifoldFeaturesForScope(tree, "update", []float64{1, 0.9, 10, 2, 50000})

		measurements := signals.Measure()

		So(len(measurements), ShouldBeGreaterThan, 0)

		Convey("It should expose pumpdump classifier output", func() {
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
			So(datura.Peek[float64](pumpdumpMeasurement, "output", "confidence"), ShouldBeGreaterThan, 0)
		})
	})
}

func BenchmarkSignalMeasureReplayIngest(b *testing.B) {
	ctx := b.Context()
	pool := qpool.NewQ[any](ctx, 1, 2, nil)
	tree := dmt.NewTree(b.TempDir())
	signals := NewSignal(ctx, pool, tree)

	defer func() {
		_ = signals.Close()
	}()

	replayAt := time.Now().UnixNano()

	for tick := range 60 {
		tests.NewFixture(tests.FixtureTypeTicker).Ingest(
			tree,
			replayAt+int64(tick),
		)
	}

	b.ResetTimer()

	for b.Loop() {
		measurements := signals.Measure()

		for _, measurement := range measurements {
			if measurement != nil {
				measurement.Release()
			}
		}
	}
}
