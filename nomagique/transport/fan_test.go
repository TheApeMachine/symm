package transport_test

import (
	"iter"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/transport"
)

type counter struct {
	*core.PrimitiveError
	count float64
	out   float64
}

func (counter *counter) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for range in {
			counter.count++
			counter.out = counter.count

			if !yield(unsafe.Pointer(&counter.out)) {
				return
			}
		}
	}
}

func TestFanNext(t *testing.T) {
	Convey("Fan presents each arrival once to every branch without replaying upstream", t, func() {
		upstream := &counter{PrimitiveError: core.NewPrimitiveError()}
		pipeline := nomagique.NewNumber(
			upstream,
			transport.NewFan(
				transport.NewIO[any](nil, nil),
				transport.NewIO[any](nil, nil),
			),
		)

		outputs := make([]float64, 0, 2)

		for out := range pipeline.Next(sequence.NewValue(1.0)) {
			outputs = append(outputs, *(*float64)(out))
		}

		So(upstream.count, ShouldEqual, 1.0)
		So(outputs, ShouldResemble, []float64{1.0, 1.0})
	})
}
