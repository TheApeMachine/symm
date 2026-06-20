package trader

import (
	"context"
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

		measurements := signals.Measure()

		So(len(measurements), ShouldEqual, len(signals.sources))

		Convey("It should return measurement artifacts with capnp identity", func() {
			for _, measurement := range measurements {
				origin, originErr := measurement.Origin()
				scope, scopeErr := measurement.Scope()
				role, roleErr := measurement.Role()

				So(originErr, ShouldBeNil)
				So(scopeErr, ShouldBeNil)
				So(roleErr, ShouldBeNil)
				// Origin is the signal name: the frontend keys gauge readings
				// by source = origin (frontend signals.ts SIGNAL_SOURCES).
				So(origin, ShouldNotBeBlank)
				So(origin, ShouldNotEqual, "trader")
				So(scope, ShouldEqual, origin)
				So(role, ShouldEqual, "measurement")
				So(len(measurement.Marshal()), ShouldBeGreaterThan, 0)
			}
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

		query := datura.Acquire("trader", datura.APPJSON)
		query.WithRole("measurement")
		query.WithScope("update")

		replayAt := time.Now().UnixNano()

		for tick := range 60 {
			tests.NewFixture(tests.FixtureTypeTicker).Ingest(
				tree,
				replayAt+int64(tick),
			)
		}

		measurement := signal.Measure(query)
		query.Release()

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
