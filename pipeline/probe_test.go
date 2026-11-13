package pipeline

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	nmruntime "github.com/theapemachine/nomagique/runtime"
)

type stubNode struct {
	*nmruntime.System
	delay time.Duration
}

func (node *stubNode) Step(envelope *nmruntime.Envelope) *nmruntime.Envelope {
	if node.delay > 0 {
		time.Sleep(node.delay)
	}

	return envelope
}

func TestNewProbe(t *testing.T) {
	Convey("Setup", t, func() {
		node := &stubNode{System: nmruntime.NewSystem(t.Context(), "stub", nil)}

		Convey("When NewProbe wraps the node", func() {
			probe := NewProbe("signal", node)

			Convey("Group and Name come from the ring and node", func() {
				So(probe.Group(), ShouldEqual, "signal")
				So(probe.Name(), ShouldEqual, "stub")
			})
		})
	})
}

func TestStep(t *testing.T) {
	Convey("Setup", t, func() {
		node := &stubNode{
			System: nmruntime.NewSystem(t.Context(), "stub", nil),
			delay:  time.Millisecond,
		}
		probe := NewProbe("signal", node)

		Convey("When Step runs one envelope", func() {
			env := nmruntime.NewEnvelope(nil)
			out := probe.Step(env)

			Convey("work time and sample count are recorded", func() {
				So(out, ShouldEqual, env)
				So(probe.n.Load(), ShouldEqual, 1)
				So(probe.emaNs.Load(), ShouldEqual, probe.lastNs.Load())
			})
		})

		Convey("When record applies an 8ms then 1ms sample", func() {
			probe.record(8 * time.Millisecond)
			probe.record(time.Millisecond)

			Convey("EMA is 1/8 blend toward the later sample", func() {
				So(probe.emaNs.Load(), ShouldEqual, int64(7_125_000))
			})
		})
	})
}

func TestSnapshot(t *testing.T) {
	Convey("Setup", t, func() {
		node := &stubNode{System: nmruntime.NewSystem(t.Context(), "cvd", nil)}
		probe := NewProbe("signal", node)
		probe.record(3 * time.Millisecond)

		Convey("When Snapshot is called", func() {
			row := probe.Snapshot()

			Convey("the dashboard row matches probe state", func() {
				So(row["id"], ShouldEqual, "cvd")
				So(row["group"], ShouldEqual, "signal")
				So(row["n"], ShouldEqual, uint64(1))
				So(row["last_ns"], ShouldEqual, int64(3_000_000))
				So(row["ema_ns"], ShouldEqual, int64(3_000_000))
				So(row["status"], ShouldEqual, "init")
			})
		})
	})
}

func BenchmarkNewProbe(b *testing.B) {
	ctx := context.Background()
	node := &stubNode{System: nmruntime.NewSystem(ctx, "stub", nil)}

	b.ReportAllocs()

	for b.Loop() {
		_ = NewProbe("signal", node)
	}
}

func BenchmarkStep(b *testing.B) {
	ctx := context.Background()
	node := &stubNode{System: nmruntime.NewSystem(ctx, "stub", nil)}
	probe := NewProbe("signal", node)
	env := nmruntime.NewEnvelope(nil)

	b.ReportAllocs()

	for b.Loop() {
		_ = probe.Step(env)
	}
}

func BenchmarkSnapshot(b *testing.B) {
	ctx := context.Background()
	node := &stubNode{System: nmruntime.NewSystem(ctx, "cvd", nil)}
	probe := NewProbe("signal", node)
	probe.record(3 * time.Millisecond)

	b.ReportAllocs()

	for b.Loop() {
		_ = probe.Snapshot()
	}
}
