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
		tee := hindsight.NewStoreTee(t.Context(), "drain-test")
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
		tee := hindsight.NewStoreTee(t.Context(), "shutdown")
		tee.Transition(runtime.READY)
		defer func() { So(tee.Close(), ShouldBeNil) }()
		for sequence := int64(1); sequence <= 8; sequence++ {
			measurement := data.NewMeasurement[float64]("signal", nil)
			measurement.Label = "BTC/USD"
			measurement.At = time.Unix(sequence, 0)
			measurement.SeqIdx = sequence
			tee.Push(data.Publication{Measurement: measurement})
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
	tee := hindsight.NewStoreTee(b.Context(), "drain-benchmark")
	tee.Transition(runtime.READY)
	b.Cleanup(func() {
		if err := tee.Close(); err != nil {
			b.Fatal(err)
		}
	})
	ctx, cancel := context.WithCancel(b.Context())
	cancel()

	for batch := 0; b.Loop(); batch++ {
		for sequence := int64(1); sequence <= 2048; sequence++ {
			measurement := data.NewMeasurement[float64]("signal", nil)
			measurement.WriteMetric("value", float64(sequence))
			measurement.Label = "BTC/USD"
			measurement.At = time.Unix(sequence, 0)
			measurement.SeqIdx = sequence
			tee.Push(data.Publication{Measurement: measurement})
		}
		if err := catalog.Drain(ctx, int64(batch+1), tee); err != nil {
			b.Fatal(err)
		}
	}
}

func TestDeriveChannelRoutesVenueTape(t *testing.T) {
	Convey("Provenance channel routes ticker/trade without venue=true", t, func() {
		catalog := tablestest.New(t)
		tee := hindsight.NewStoreTee(t.Context(), "channel-route")
		tee.Transition(runtime.READY)
		defer func() { So(tee.Close(), ShouldBeNil) }()

		ticker := data.NewMeasurement[float64]("websocket", nil)
		ticker.WriteMetric("bid", 100)
		ticker.WriteMetric("ask", 101)
		ticker.Label = "BTC/USD"
		ticker.SeqIdx = 1
		ticker.At = time.Unix(1, 0)
		ticker.SetProvenance("channel", "ticker")
		ticker.SetMetadata("type", "ticker")
		tee.Push(data.Publication{Measurement: ticker})

		trade := data.NewMeasurement[float64]("websocket", nil)
		trade.WriteMetric("price", 100.5)
		trade.WriteMetric("qty", 0.2)
		trade.Label = "BTC/USD"
		trade.SeqIdx = 1
		trade.At = time.Unix(1, 0)
		trade.SetProvenance("channel", "trade")
		trade.SetMetadata("type", "trade")
		tee.Push(data.Publication{Measurement: trade})

		signal := data.NewMeasurement[float64]("cvd", nil)
		signal.WriteMetric("cvd", 1.5)
		signal.Label = "BTC/USD"
		signal.SeqIdx = 1
		signal.At = time.Unix(1, 0)
		tee.Push(data.Publication{Measurement: signal})

		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		So(catalog.Drain(ctx, 200, tee), ShouldBeNil)

		tickers := 0
		for range catalog.Scan(t.Context(), tables.SpotTicker, 200, nil, 0) {
			tickers++
		}
		trades := 0
		for range catalog.Scan(t.Context(), tables.SpotTrade, 200, nil, 0) {
			trades++
		}
		measured := 0
		for range catalog.Scan(t.Context(), tables.Measurements, 200, nil, 0) {
			measured++
		}

		So(tickers, ShouldEqual, 1)
		So(trades, ShouldEqual, 1)
		So(measured, ShouldEqual, 1)

		frames := 0
		for frame := range catalog.Timeline(t.Context(), 200, "BTC/USD", 0, 0) {
			frames++
			So(len(frame.Peers), ShouldBeGreaterThanOrEqualTo, 1)
		}
		So(frames, ShouldEqual, 1)
	})
}

func TestDrainDoesNotPolluteVenueTapeWithSignals(t *testing.T) {
	Convey("Workspace-style signal publications with inherited channel go to Measurements", t, func() {
		catalog := tablestest.New(t)
		tee := hindsight.NewStoreTee(t.Context(), "signal-isolation")
		tee.Transition(runtime.READY)
		defer func() { So(tee.Close(), ShouldBeNil) }()

		raw := data.NewMeasurement[float64]("websocket", nil)
		raw.WriteMetric("bid", 100)
		raw.WriteMetric("ask", 101)
		raw.Label = "BTC/USD"
		raw.SeqIdx = 1
		raw.At = time.Unix(1, 0)
		raw.SetProvenance("ingress_channel", "ticker")
		raw.SetProvenance("channel", "ticker")
		raw.SetMetadata("type", "ticker")
		tee.Push(data.Publication{Measurement: raw})

		// Mimic liquidity:ticker producing fresh WORM measurement while referencing raw as peer
		signal := data.NewMeasurement[float64]("liquidity:ticker", nil)
		signal.Label = raw.Label
		signal.At = raw.At
		signal.SeqIdx = 1
		signal.SetProvenance("channel", "ticker")
		signal.WriteMetric("relative_spread", 0.01)
		signal.Peers = []*data.Measurement[float64]{raw}
		tee.Push(data.Publication{Measurement: signal})

		toxicity := data.NewMeasurement[float64]("toxicity:level3", nil)
		toxicity.Label = raw.Label
		toxicity.At = raw.At
		toxicity.SeqIdx = 1
		toxicity.SetProvenance("channel", "ticker")
		toxicity.WriteMetric("retreat_fraction:bid", 0.2)
		toxicity.Peers = []*data.Measurement[float64]{raw}
		tee.Push(data.Publication{Measurement: toxicity})

		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		So(catalog.Drain(ctx, 201, tee), ShouldBeNil)

		tickers := 0
		for range catalog.Scan(t.Context(), tables.SpotTicker, 201, nil, 0) {
			tickers++
		}
		measured := 0
		sources := map[string]int{}
		for m := range catalog.Scan(t.Context(), tables.Measurements, 201, nil, 0) {
			measured++
			sources[m.Source]++
		}

		So(tickers, ShouldEqual, 1)
		So(measured, ShouldEqual, 2)
		So(sources["liquidity:ticker"], ShouldEqual, 1)
		So(sources["toxicity:level3"], ShouldEqual, 1)
	})
}
