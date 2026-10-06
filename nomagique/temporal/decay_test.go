package temporal_test

import (
	"iter"
	"math"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/temporal"
)

type fixedElapsed struct {
	*core.PrimitiveError
	elapsed float64
	output  data.Map[float64]
}

func newFixedElapsed(elapsed float64) *fixedElapsed {
	output := data.NewOutputMap()
	output.Values["elapsed"] = elapsed

	return &fixedElapsed{
		PrimitiveError: core.NewPrimitiveError(),
		elapsed:        elapsed,
		output:         output,
	}
}

func (op *fixedElapsed) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			adapter := *(**data.Adapter)(arriving)

			if adapter == nil {
				op.Error(core.ErrShape)
				return
			}

			op.output.Values["elapsed"] = op.elapsed

			for range adapter.Next(data.NewValue(op.output)) {
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			if !yield(arriving) {
				return
			}
		}
	}
}

type expNegShape struct {
	*core.PrimitiveError
	input  data.Map[string]
	output data.Map[float64]
}

func newExpNegShape() *expNegShape {
	output := data.NewOutputMap()
	output.Values["factor"] = 0

	return &expNegShape{
		PrimitiveError: core.NewPrimitiveError(),
		input:          data.NewMap("elapsed", "elapsed"),
		output:         output,
	}
}

func (op *expNegShape) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			adapter := *(**data.Adapter)(arriving)

			if adapter == nil {
				op.Error(core.ErrShape)
				return
			}

			var values data.Map[float64]

			for pointer := range adapter.Next(data.NewValue(op.input)) {
				values = *(*data.Map[float64])(pointer)
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			elapsed, ok := values.Values["elapsed"]

			if !ok {
				op.Error(core.ErrNotHeld)
				return
			}

			op.output.Values["factor"] = math.Exp(-elapsed)

			for range adapter.Next(data.NewValue(op.output)) {
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			if !yield(arriving) {
				return
			}
		}
	}
}

func decayValue(clock, shape core.Primitive, value float64) (float64, error) {
	adapter := data.NewAdapter(nil, data.NewState(data.NewMap()))
	issued := data.NewOutputMap()
	issued.Values["value"] = value

	for range adapter.Next(data.NewValue(issued)) {
	}

	if err := adapter.Error(); err != nil {
		return 0, err
	}

	node := temporal.NewDecay(clock, shape)

	for range node.Next(data.NewValue(adapter)) {
	}

	if err := node.Error(); err != nil {
		return 0, err
	}

	var values data.Map[float64]

	for pointer := range adapter.Next(data.NewValue(data.NewMap("value", "value"))) {
		values = *(*data.Map[float64])(pointer)
	}

	if err := adapter.Error(); err != nil {
		return 0, err
	}

	return values.Values["value"], nil
}

func TestDecayNext(t *testing.T) {
	Convey("A missing clock extinguishes a finite input", t, func() {
		out, err := decayValue(nil, nil, 10)
		So(err, ShouldBeNil)
		So(out, ShouldEqual, 0)
	})

	Convey("Linear retention uses one minus elapsed, floored at zero", t, func() {
		out, err := decayValue(newFixedElapsed(0.25), nil, 10)
		So(err, ShouldBeNil)
		So(out, ShouldEqual, 7.5)
	})

	Convey("A configured shape sees elapsed time, not a second protocol", t, func() {
		out, err := decayValue(newFixedElapsed(math.Ln2), newExpNegShape(), 10)
		So(err, ShouldBeNil)
		So(out, ShouldEqual, 5)
	})
}
