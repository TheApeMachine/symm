package types

import (
	"errors"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

var (
	primitiveInput       = MustIntern("test/primitive/input")
	primitiveControl     = MustIntern("test/primitive/control")
	primitiveMetric      = MustIntern("test/primitive/metric")
	primitiveFirstState  = MustIntern("test/primitive/state/first")
	primitiveSecondState = MustIntern("test/primitive/state/second")
	primitiveShared      = MustIntern("test/primitive/shared")
	primitiveFirstOutput = MustIntern("test/primitive/output/first")
	primitiveSecondOut   = MustIntern("test/primitive/output/second")
)

func TestPrimitiveTransactions(t *testing.T) {
	Convey("Step rejects nil primitives and candidate state returned with an error", t, func() {
		initial := Frame{}.Set(primitiveShared, 4)
		next, output, err := Step(nil, initial, Frame{})
		So(err, ShouldNotBeNil)
		So(next.Equal(initial), ShouldBeTrue)
		So(output.Count(), ShouldEqual, 0)

		bad := func(state Frame, input Frame) (Frame, Frame, error) {
			state.Put(primitiveShared, 99)
			return state, input, errors.New("reject")
		}
		next, output, err = Step(bad, initial, Frame{})
		So(err, ShouldNotBeNil)
		So(next.Equal(initial), ShouldBeTrue)
		So(output.Count(), ShouldEqual, 0)
	})

	Convey("Pipe is ordered and rolls every prior stage back on failure", t, func() {
		first := func(state Frame, input Frame) (Frame, Frame, error) {
			state.Put(primitiveFirstState, 1)
			output := input
			output.Put(primitiveFirstOutput, input.MustGet(primitiveInput)+1)
			return state, output, nil
		}
		second := func(state Frame, input Frame) (Frame, Frame, error) {
			So(state.MustGet(primitiveFirstState), ShouldEqual, 1.0)
			So(input.MustGet(primitiveFirstOutput), ShouldEqual, 3.0)
			state.Put(primitiveSecondState, 1)
			return state, input, nil
		}
		input := Frame{}.Set(primitiveInput, 2)
		next, output, err := Pipe(first, second)(Frame{}, input)
		So(err, ShouldBeNil)
		So(next.MustGet(primitiveSecondState), ShouldEqual, 1.0)
		So(output.MustGet(primitiveFirstOutput), ShouldEqual, 3.0)

		failedState, failedOutput, failedErr := Pipe(first, func(state Frame, input Frame) (Frame, Frame, error) {
			return state, input, errors.New("forced")
		})(Frame{}, input)
		So(failedErr, ShouldNotBeNil)
		So(failedState.Count(), ShouldEqual, 0)
		So(failedOutput.Count(), ShouldEqual, 0)
	})

	Convey("An empty Pipe is the identity relation", t, func() {
		state := Frame{}.Set(primitiveShared, 1)
		input := Frame{}.Set(primitiveInput, 2)
		next, output, err := Pipe()(state, input)
		So(err, ShouldBeNil)
		So(next.Equal(state), ShouldBeTrue)
		So(output.Equal(input), ShouldBeTrue)
	})
}

func TestForkContracts(t *testing.T) {
	Convey("Fork is genuine fan-out: branches see the same original state and input", t, func() {
		first := func(state Frame, input Frame) (Frame, Frame, error) {
			state.Put(primitiveFirstState, 1)
			output := input
			output.Put(primitiveFirstOutput, 10)
			return state, output, nil
		}
		second := func(state Frame, input Frame) (Frame, Frame, error) {
			if state.Has(primitiveFirstState) {
				return state, Frame{}, errors.New("second branch observed first branch state")
			}
			if input.Has(primitiveFirstOutput) {
				return state, Frame{}, errors.New("second branch observed first branch output")
			}
			state.Put(primitiveSecondState, 1)
			output := input
			output.Put(primitiveSecondOut, 20)
			return state, output, nil
		}
		next, output, err := Fork(first, second)(Frame{}, Frame{}.Set(primitiveInput, 1))
		So(err, ShouldBeNil)
		So(next.MustGet(primitiveFirstState), ShouldEqual, 1.0)
		So(next.MustGet(primitiveSecondState), ShouldEqual, 1.0)
		So(output.MustGet(primitiveFirstOutput), ShouldEqual, 10.0)
		So(output.MustGet(primitiveSecondOut), ShouldEqual, 20.0)
	})

	Convey("Conflicting state writes are rejected transactionally", t, func() {
		write := func(value float64) Primitive {
			return func(state Frame, input Frame) (Frame, Frame, error) {
				state.Put(primitiveShared, value)
				return state, input, nil
			}
		}
		initial := Frame{}.Set(primitiveShared, 0)
		next, output, err := Fork(write(1), write(2))(initial, Frame{})
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "collision")
		So(next.Equal(initial), ShouldBeTrue)
		So(output.Count(), ShouldEqual, 0)
	})

	Convey("Identical state writes are compatible", t, func() {
		write := func(state Frame, input Frame) (Frame, Frame, error) {
			state.Put(primitiveShared, 7)
			return state, input, nil
		}
		next, _, err := Fork(write, write)(Frame{}, Frame{})
		So(err, ShouldBeNil)
		So(next.MustGet(primitiveShared), ShouldEqual, 7.0)
	})

	Convey("ForkStrict rejects output collisions that permissive Fork overlays", t, func() {
		write := func(value float64) Primitive {
			return func(state Frame, input Frame) (Frame, Frame, error) {
				output := input
				output.Put(primitiveShared, value)
				return state, output, nil
			}
		}
		_, permissive, permissiveErr := Fork(write(1), write(2))(Frame{}, Frame{})
		So(permissiveErr, ShouldBeNil)
		So(permissive.MustGet(primitiveShared), ShouldEqual, 2.0)
		_, strict, strictErr := ForkStrict(write(1), write(2))(Frame{}, Frame{})
		So(strictErr, ShouldNotBeNil)
		So(strict.Count(), ShouldEqual, 0)
	})
}

func TestConfigureContracts(t *testing.T) {
	producerState := MustIntern("test/configure/producer_state")
	consumerState := MustIntern("test/configure/consumer_state")
	consumerFact := MustIntern("test/configure/consumer_fact")

	producer := func(state Frame, input Frame) (Frame, Frame, error) {
		state.Put(producerState, 1)
		output := input
		output.Put(primitiveControl, 3)
		output.Put(primitiveMetric, 8)
		output.Put(primitiveShared, 1)
		return state, output, nil
	}
	consumer := func(state Frame, input Frame) (Frame, Frame, error) {
		if input.MustGet(primitiveInput) != 2 || input.MustGet(primitiveControl) != 3 {
			return state, Frame{}, errors.New("consumer did not receive original input plus control")
		}
		if state.MustGet(producerState) != 1 {
			return state, Frame{}, errors.New("consumer did not observe producer state")
		}
		state.Put(consumerState, 1)
		output := input
		output.Put(consumerFact, 9)
		output.Put(primitiveShared, 2)
		return state, output, nil
	}

	Convey("Configure preserves producer metrics and lets consumer output win", t, func() {
		next, output, err := Configure(producer, primitiveControl, consumer)(
			Frame{}, Frame{}.Set(primitiveInput, 2),
		)
		So(err, ShouldBeNil)
		So(next.MustGet(producerState), ShouldEqual, 1.0)
		So(next.MustGet(consumerState), ShouldEqual, 1.0)
		So(output.MustGet(primitiveMetric), ShouldEqual, 8.0)
		So(output.MustGet(consumerFact), ShouldEqual, 9.0)
		So(output.MustGet(primitiveShared), ShouldEqual, 2.0)
	})

	Convey("Missing and non-finite control values are rejected", t, func() {
		missing := func(state Frame, input Frame) (Frame, Frame, error) { return state, input, nil }
		_, _, err := Configure(missing, primitiveControl, Identity)(Frame{}, Frame{})
		So(err, ShouldNotBeNil)
		nonfinite := Assign(primitiveControl, math.NaN())
		_, _, err = Configure(nonfinite, primitiveControl, Identity)(Frame{}, Frame{})
		So(err, ShouldNotBeNil)
	})

	Convey("A consumer failure rolls producer state back", t, func() {
		initial := Frame{}.Set(primitiveShared, 5)
		_, output, err := Configure(producer, primitiveControl, func(state Frame, input Frame) (Frame, Frame, error) {
			return state, input, errors.New("consumer failure")
		})(initial, Frame{}.Set(primitiveInput, 2))
		So(err, ShouldNotBeNil)
		So(output.Count(), ShouldEqual, 0)
	})
}
