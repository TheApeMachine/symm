package tables_test

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/tests/tablestest"
)

func TestCatalog_Drain(t *testing.T) {
	Convey("An idle or empty storage queue can be drained without dereferencing nil", t, func() {
		tee := hindsight.NewStoreTee(t.Context(), "drain-test", 4)
		defer func() { So(tee.Close(), ShouldBeNil) }()
		catalog := tables.Wrap(nil)

		Convey("Cancellation before activation finishes safely", func() {
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			So(catalog.Drain(ctx, 1, tee), ShouldBeNil)
		})

		Convey("An activated empty queue survives periodic polling and cancellation", func() {
			tee.Transition(runtime.READY)
			// Allow multiple 50ms flush ticks before exercising the final drain.
			ctx, cancel := context.WithTimeout(t.Context(), 150*time.Millisecond)
			defer cancel()
			So(catalog.Drain(ctx, 1, tee), ShouldBeNil)
			So(ctx.Err(), ShouldEqual, context.DeadlineExceeded)
			So(tee.Error(), ShouldBeNil)
		})
	})

	Convey("Cancellation commits every already accepted observation", t, func() {
		catalog := tablestest.New(t)
		tee := hindsight.NewStoreTee(t.Context(), "shutdown", 8)
		tee.Transition(runtime.READY)
		defer func() { So(tee.Close(), ShouldBeNil) }()
		for sequence := int64(1); sequence <= 8; sequence++ {
			measurement := data.NewMeasurement[float64]("signal", nil)
			measurement.Label = "BTC/USD"
			measurement.At = time.Unix(sequence, 0)
			measurement.SeqIdx = sequence
			tee.Push(measurement)
		}
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		So(catalog.Drain(ctx, 100, tee), ShouldBeNil)
		So(tee.Pending(), ShouldEqual, 0)
		count := 0
		for measurement := range catalog.Scan(t.Context(), tables.Measurements, 100, nil, 0) {
			So(measurement.Err, ShouldBeNil)
			count++
			So(measurement.SeqIdx, ShouldEqual, count)
		}
		So(count, ShouldEqual, 8)
	})
}

func BenchmarkCatalog_Drain(b *testing.B) {
	catalog := tablestest.New(b)
	tee := hindsight.NewStoreTee(b.Context(), "drain-benchmark", 2048)
	tee.Transition(runtime.READY)
	b.Cleanup(func() {
		if err := tee.Close(); err != nil {
			b.Fatal(err)
		}
	})
	ctx, cancel := context.WithCancel(b.Context())
	cancel()
	b.ResetTimer()
	for batch := 0; batch < b.N; batch++ {
		for sequence := int64(1); sequence <= 2048; sequence++ {
			measurement := data.NewMeasurement[float64]("signal", map[string]data.Metric[float64]{"value": {Raw: float64(sequence)}})
			measurement.Label = "BTC/USD"
			measurement.At = time.Unix(sequence, 0)
			measurement.SeqIdx = sequence
			tee.Push(measurement)
		}
		if err := catalog.Drain(ctx, int64(batch+1), tee); err != nil {
			b.Fatal(err)
		}
	}
}
