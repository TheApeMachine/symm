package temporal_test

import (
	"errors"
	"iter"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/tests"
)

type collectionMean struct {
	err error
	out float64
}

func (op *collectionMean) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			values := *(*[]float64)(arriving)
			var sum float64

			for _, value := range values {
				sum += value
			}

			op.out = sum / float64(len(values))

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *collectionMean) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

func TestGovernorNext(t *testing.T) {
	Convey("Governor reduces a retained tail once two observations exist", t, func() {
		op := temporal.NewGovernor(2, &collectionMean{})
		vals := []float64{1.0, 3.0, 7.0, 9.0}
		in := func(yield func(unsafe.Pointer) bool) {
			for i := range vals {
				if !yield(unsafe.Pointer(&vals[i])) {
					return
				}
			}
		}
		out := tests.CollectSeq[float64](op.Next(in))
		So(out, ShouldResemble, []float64{0, 2, 5, 8})
		So(op.Error(), ShouldBeNil)
	})
}
