package trader

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/tests"
)

func TestSignalMeasureUsesLatestIngest(testingTB *testing.T) {
	Convey("Given many ticker rows in the tree", testingTB, func() {
		ctx, cancel := context.WithCancel(context.Background())

		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		tree := dmt.NewTree(testingTB.TempDir())
		signals := NewSignal(ctx, pool, tree)

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

		measurements := signals.Measure()

		So(len(measurements), ShouldBeGreaterThan, 0)
		So(len(measurements), ShouldBeLessThanOrEqualTo, len(signals.sources))
		So(len(measurements), ShouldBeLessThan, len(signals.sources)*replayTicks/2)
	})
}

func TestSignalMeasureTagsUIWire(testingTB *testing.T) {
	Convey("Given ingested ticker rows", testingTB, func() {
		ctx, cancel := context.WithCancel(context.Background())

		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		tree := dmt.NewTree(testingTB.TempDir())
		signals := NewSignal(ctx, pool, tree)

		defer func() {
			_ = signals.Close()
		}()

		tests.NewFixture(tests.FixtureTypeTicker).Ingest(tree, time.Now().UnixNano())
		insertManifoldFeaturesForScope(tree, "update", []float64{1, 0.9, 10, 2, 50000})

		measurements := signals.Measure()

		So(len(measurements), ShouldBeGreaterThan, 0)

		for _, measurement := range measurements {
			role, roleErr := measurement.Role()
			origin, originErr := measurement.Origin()

			So(roleErr, ShouldBeNil)
			So(originErr, ShouldBeNil)
			So(role, ShouldEqual, "measurement")
			So(origin, ShouldNotBeBlank)
			So(origin, ShouldNotEqual, "kraken:public")
		}

		Convey("It should expose pumpdump classifier output on the ui wire", func() {
			var pumpdumpMeasurement *datura.Artifact

			for _, measurement := range measurements {
				origin, _ := measurement.Origin()

				if origin != string(logic.SourcePumpDump) {
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

func BenchmarkSignalMeasureLatestIngest(benchmarkTB *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())

	defer cancel()

	pool := qpool.NewQ[any](ctx, 1, 2, nil)
	tree := dmt.NewTree(benchmarkTB.TempDir())
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

	benchmarkTB.ResetTimer()

	for benchmarkTB.Loop() {
		measurements := signals.Measure()

		for _, measurement := range measurements {
			if measurement != nil {
				measurement.Release()
			}
		}
	}
}
