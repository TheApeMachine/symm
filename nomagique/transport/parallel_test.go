package transport_test

import (
	"iter"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

type dummySource struct {
	*core.PrimitiveError
	values []float64
}

func (source *dummySource) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for i := range source.values {
			if !yield(unsafe.Pointer(&source.values[i])) {
				return
			}
		}
	}
}

type dummyCollector struct {
	*core.PrimitiveError
	collected []float64
}

func (collector *dummyCollector) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving != nil {
				collector.collected = append(collector.collected, *(*float64)(arriving))
			}
		}
	}
}

func TestParallel(t *testing.T) {
	Convey("Parallel provides transparent throughput across branches", t, func() {
		Convey("When in is nil, it drains all sources in order", func() {
			srcA := &dummySource{PrimitiveError: core.NewPrimitiveError(), values: []float64{1.0, 2.0}}
			srcB := &dummySource{PrimitiveError: core.NewPrimitiveError(), values: []float64{3.0, 4.0}}

			parallel := transport.NewParallel(srcA, srcB)
			out := tests.CollectSeq[float64](parallel.Next(nil))

			So(out, ShouldResemble, []float64{1.0, 2.0, 3.0, 4.0})
		})

		Convey("When in is provided, it broadcasts arriving payloads across branches", func() {
			sinkA := &dummyCollector{PrimitiveError: core.NewPrimitiveError()}
			sinkB := &dummyCollector{PrimitiveError: core.NewPrimitiveError()}

			parallel := transport.NewParallel(sinkA, sinkB)
			for range parallel.Next(sequence.NewValue(10.0, 20.0)) {
			}

			So(sinkA.collected, ShouldResemble, []float64{10.0, 20.0})
			So(sinkB.collected, ShouldResemble, []float64{10.0, 20.0})
		})
	})
}
