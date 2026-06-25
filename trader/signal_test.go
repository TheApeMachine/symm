package trader

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/signal/testutil"
)

func TestSignalMeasureSeekPrefix(t *testing.T) {
	Convey("Given ingest keyed by timestamp like the websocket", t, func() {
		crossSection := testutil.NewTestCrossSection(t)
		pool := qpool.NewQ[any](context.Background(), 2, 4, nil)
		tree := dmt.NewTree("")
		runner := NewSignal(context.Background(), pool, tree)

		So(runner, ShouldNotBeNil)

		defer runner.Close()

		at := time.Now().UTC().Truncate(time.Second)
		artifact := datura.Acquire("kraken:public", datura.APPJSON)
		artifact.WithRole("ticker")
		artifact.WithScope("update")
		artifact.WithPayload([]byte(`{"channel":"ticker","type":"update","data":[{"symbol":"BTC/USD","last":100,"volume":1,"change_pct":0.5,"bid":99.5,"ask":100.5}]}`))
		artifact.SetTimestamp(at.UnixNano())

		tree.Insert(artifact.Prefix("role", "timestamp"), artifact.Pack())
		artifact.Release()

		measurements := runner.Measure(crossSection)

		Convey("It should replay the ticker frame", func() {
			So(len(measurements), ShouldBeGreaterThan, 0)
		})
	})
}

func TestSignalMeasureIncrementalSeek(t *testing.T) {
	Convey("Given two ticker ingest rows", t, func() {
		crossSection := testutil.NewTestCrossSection(t)
		pool := qpool.NewQ[any](context.Background(), 2, 4, nil)
		tree := dmt.NewTree("")
		runner := NewSignal(context.Background(), pool, tree)

		So(runner, ShouldNotBeNil)

		defer runner.Close()

		firstAt := time.Now().UTC().Add(-time.Second).Truncate(time.Second)
		secondAt := firstAt.Add(time.Second)

		for index, at := range []time.Time{firstAt, secondAt} {
			artifact := datura.Acquire("kraken:public", datura.APPJSON)
			artifact.WithRole("ticker")
			artifact.WithScope("update")
			artifact.WithPayload([]byte(`{"channel":"ticker","type":"update","data":[{"symbol":"BTC/USD","last":100,"volume":1,"change_pct":0.5,"bid":99.5,"ask":100.5}]}`))
			artifact.SetTimestamp(at.UnixNano() + int64(index))

			tree.Insert(artifact.Prefix("role", "timestamp"), artifact.Pack())
			artifact.Release()
		}

		first := runner.Measure(crossSection)
		second := runner.Measure(crossSection)

		Convey("It should only replay unseen rows on the second pass", func() {
			So(len(first), ShouldBeGreaterThan, 0)
			So(len(second), ShouldEqual, 0)
		})
	})
}

func BenchmarkSignalMeasureIncrementalSeek(b *testing.B) {
	crossSection := testutil.NewTestCrossSection(b)
	pool := qpool.NewQ[any](context.Background(), 2, 4, nil)
	tree := dmt.NewTree("")
	runner := NewSignal(context.Background(), pool, tree)

	defer runner.Close()

	at := time.Now().UTC().Truncate(time.Second)

	for index := range 128 {
		artifact := datura.Acquire("kraken:public", datura.APPJSON)
		artifact.WithRole("ticker")
		artifact.WithScope("update")
		artifact.WithPayload([]byte(`{"channel":"ticker","type":"update","data":[{"symbol":"BTC/USD","last":100,"volume":1,"change_pct":0.5,"bid":99.5,"ask":100.5}]}`))
		artifact.SetTimestamp(at.Add(time.Duration(index) * time.Millisecond).UnixNano())

		tree.Insert(artifact.Prefix("role", "timestamp"), artifact.Pack())
		artifact.Release()
	}

	runner.Measure(crossSection)

	b.ReportAllocs()

	for b.Loop() {
		artifact := datura.Acquire("kraken:public", datura.APPJSON)
		artifact.WithRole("ticker")
		artifact.WithScope("update")
		artifact.WithPayload([]byte(`{"channel":"ticker","type":"update","data":[{"symbol":"BTC/USD","last":100,"volume":1,"change_pct":0.5,"bid":99.5,"ask":100.5}]}`))
		artifact.SetTimestamp(at.Add(time.Duration(b.N) * time.Millisecond).UnixNano())

		tree.Insert(artifact.Prefix("role", "timestamp"), artifact.Pack())
		artifact.Release()

		runner.Measure(crossSection)
	}
}
