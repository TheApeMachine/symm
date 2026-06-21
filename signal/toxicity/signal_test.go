package toxicity

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/tests"
)

func newTestPool(testingTB testing.TB) *qpool.Q[any] {
	if testingTB != nil {
		testingTB.Helper()
	}

	pool := qpool.NewQ[any](context.Background(), 2, 4, nil)

	if pool == nil && testingTB != nil {
		testingTB.Fatal("qpool.NewQ returned nil")
	}

	return pool
}

/*
insertFrame mirrors live ingest: a packed artifact keyed by role=channel,
scope=type, carrying the raw Kraken JSON in the payload.
*/
func insertFrame(signal *Signal, role, scope, payload string) {
	artifact := datura.Acquire("kraken:public", datura.APPJSON)
	artifact.WithRole(role)
	artifact.WithScope(scope)
	artifact.WithPayload([]byte(payload))

	signal.tree.Insert(artifact.Prefix(), artifact.Pack())
}

func measurementQuery(role, scope string) *datura.Artifact {
	acquired := datura.Acquire("trader", datura.Artifact_Type_json)
	acquired.WithRole(role)
	acquired.WithScope(scope)
	acquired.WithPayload([]byte("{}"))

	return acquired
}

const bookFramePayload = `{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":1.0,"qty":2.0}],"asks":[]}]}`

func TestSignalMeasure(testingTB *testing.T) {
	Convey("Given an ingested book frame at the queried prefix", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		insertFrame(signal, "book", "update", bookFramePayload)

		result := signal.Measure(measurementQuery("book", "update"))

		Convey("It returns a writable, addressed result carrying the pipeline output", func() {
			So(result, ShouldNotBeNil)

			role, _ := result.Role()
			scope, _ := result.Scope()
			origin, _ := result.Origin()

			So(origin, ShouldEqual, "toxicity")
			So(role, ShouldEqual, "measurement")
			So(scope, ShouldEqual, "toxicity")

			payload := result.DecryptPayload()
			So(len(payload), ShouldBeGreaterThan, 0)

			category := datura.Peek[float64](result, "output", "category")
			So(category, ShouldBeGreaterThan, 0)

			result.Release()
		})
	})

	Convey("Given a sparse tree at startup", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		// dmt.NewTree is a process singleton, so query a prefix no other test
		// inserts to guarantee an empty seek.
		result := signal.Measure(measurementQuery("sparse-role", "sparse-scope"))

		Convey("It returns an addressed result with an empty payload, not nil", func() {
			So(result, ShouldNotBeNil)

			origin, _ := result.Origin()
			scope, _ := result.Scope()
			So(origin, ShouldEqual, "toxicity")
			So(scope, ShouldEqual, "toxicity")
			So(string(result.DecryptPayload()), ShouldEqual, "{}")

			result.Release()
		})
	})
}

func TestMeasureBookFrames(testingTB *testing.T) {
	Convey("Given live-shaped kraken book frames in the tree", testingTB, func() {
		ctx, cancel := context.WithCancel(context.Background())

		defer cancel()

		tree := dmt.NewTree(testingTB.TempDir())
		signal := NewSignal(ctx, newTestPool(testingTB), tree)

		defer func() {
			_ = signal.Close()
		}()

		query := measurementQuery("measurement", "update")

		replayAt := time.Now().UnixNano()

		for tick := range 20 {
			tests.NewFixture(tests.FixtureTypeBook).Ingest(
				tree,
				replayAt+int64(tick),
			)
		}

		result := signal.Measure(query)
		query.Release()

		Convey("It should emit classifier output on a writable measurement artifact", func() {
			So(result, ShouldNotBeNil)
			So(len(result.DecryptPayload()), ShouldBeGreaterThan, 2)
			So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 0)
			So(datura.Peek[bool](result, "calibrated"), ShouldBeTrue)
			So(datura.Peek[float64](result, "samples"), ShouldBeBetweenOrEqual, 20, 21)

			result.Release()
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	query := measurementQuery("book", "update")

	b.ReportAllocs()

	for b.Loop() {
		signal := NewSignal(context.Background(), newTestPool(b), dmt.NewTree(""))

		if signal == nil {
			b.Fatal("NewSignal returned nil")
		}

		insertFrame(signal, "book", "update", bookFramePayload)

		result := signal.Measure(query)

		if result == nil {
			b.Fatal("Measure returned nil")
		}

		result.Release()
		_ = signal.Close()
	}
}
