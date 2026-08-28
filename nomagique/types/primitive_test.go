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
	Convey("Step rejects nil primitives and propagates primitive errors", t, func() {
		initial := Frame{}.Set(primitiveShared, 4)
		output := Step(nil, initial)
		So(output.Err, ShouldNotBeNil)
		So(output.Equal(initial), ShouldBeTrue)

		bad := func(input Frame) Frame {
			input.Put(primitiveShared, 99)
			input.Err = errors.New("reject")

			return input
		}
		output = Step(bad, initial)
		So(output.Err, ShouldNotBeNil)
		So(output.MustGet(primitiveShared), ShouldEqual, 99.0)
	})

	Convey("Pipe is ordered and stops at the first failure", t, func() {
		first := func(input Frame) Frame {
			input.Put(primitiveFirstState, 1)
			input.Put(primitiveFirstOutput, input.MustGet(primitiveInput)+1)

			return input
		}
		second := func(input Frame) Frame {
			So(input.MustGet(primitiveFirstState), ShouldEqual, 1.0)
			So(input.MustGet(primitiveFirstOutput), ShouldEqual, 3.0)
			input.Put(primitiveSecondState, 1)

			return input
		}
		input := Frame{}.Set(primitiveInput, 2)
		output := Pipe(first, second)(input)
		So(output.Err, ShouldBeNil)
		So(output.MustGet(primitiveSecondState), ShouldEqual, 1.0)
		So(output.MustGet(primitiveFirstOutput), ShouldEqual, 3.0)

		failed := Pipe(first, func(input Frame) Frame {
			input.Err = errors.New("forced")

			return input
		})(Frame{}.Set(primitiveInput, 2))
		So(failed.Err, ShouldNotBeNil)
	})

	Convey("An empty Pipe is the identity relation", t, func() {
		frame := Frame{}.Set(primitiveShared, 1).Set(primitiveInput, 2)
		output := Pipe()(frame)
		So(output.Err, ShouldBeNil)
		So(output.Equal(frame), ShouldBeTrue)
	})
}

func TestForkContracts(t *testing.T) {
	Convey("Fork is permissive fan-out: branches see the same input", t, func() {
		first := func(input Frame) Frame {
			input.Put(primitiveFirstState, 1)
			input.Put(primitiveFirstOutput, 10)

			return input
		}
		second := func(input Frame) Frame {
			if input.Has(primitiveFirstState) {
				input.Err = errors.New("second branch observed first branch state")

				return input
			}

			if input.Has(primitiveFirstOutput) {
				input.Err = errors.New("second branch observed first branch output")

				return input
			}

			input.Put(primitiveSecondState, 1)
			input.Put(primitiveSecondOut, 20)

			return input
		}
		output := Fork(first, second)(Frame{}.Set(primitiveInput, 1))
		So(output.Err, ShouldBeNil)
		So(output.MustGet(primitiveFirstState), ShouldEqual, 1.0)
		So(output.MustGet(primitiveSecondState), ShouldEqual, 1.0)
		So(output.MustGet(primitiveFirstOutput), ShouldEqual, 10.0)
		So(output.MustGet(primitiveSecondOut), ShouldEqual, 20.0)
	})

	Convey("ForkStrict rejects conflicting writes transactionally", t, func() {
		write := func(value float64) Primitive {
			return func(input Frame) Frame {
				input.Put(primitiveShared, value)

				return input
			}
		}
		initial := Frame{}.Set(primitiveShared, 0)
		output := ForkStrict(write(1), write(2))(initial)
		So(output.Err, ShouldNotBeNil)
		So(output.Err.Error(), ShouldContainSubstring, "collision")
	})

	Convey("ForkStrict accepts identical writes", t, func() {
		write := func(input Frame) Frame {
			input.Put(primitiveShared, 7)

			return input
		}
		output := ForkStrict(write, write)(Frame{})
		So(output.Err, ShouldBeNil)
		So(output.MustGet(primitiveShared), ShouldEqual, 7.0)
	})

	Convey("ForkStrict rejects collisions that permissive Fork overlays", t, func() {
		write := func(value float64) Primitive {
			return func(input Frame) Frame {
				input.Put(primitiveShared, value)

				return input
			}
		}
		permissive := Fork(write(1), write(2))(Frame{})
		So(permissive.Err, ShouldBeNil)
		So(permissive.MustGet(primitiveShared), ShouldEqual, 2.0)
		strict := ForkStrict(write(1), write(2))(Frame{})
		So(strict.Err, ShouldNotBeNil)
	})
}

func TestTryForkContracts(t *testing.T) {
	Convey("TryFork drops a branch that fails without writing any slot", t, func() {
		ready := func(input Frame) Frame {
			input.Put(primitiveFirstOutput, 10)

			return input
		}
		notYetObserved := func(input Frame) Frame {
			input.Err = errors.New("required fact absent")

			return input
		}
		output := TryFork(ready, notYetObserved)(Frame{}.Set(primitiveInput, 1))
		So(output.Err, ShouldBeNil)
		So(output.MustGet(primitiveFirstOutput), ShouldEqual, 10.0)
		So(output.Has(primitiveSecondOut), ShouldBeFalse)
	})

	Convey("TryFork still composes every branch once each has arrived", t, func() {
		first := func(input Frame) Frame {
			input.Put(primitiveFirstOutput, 10)

			return input
		}
		second := func(input Frame) Frame {
			input.Put(primitiveSecondOut, 20)

			return input
		}
		output := TryFork(first, second)(Frame{}.Set(primitiveInput, 1))
		So(output.Err, ShouldBeNil)
		So(output.MustGet(primitiveFirstOutput), ShouldEqual, 10.0)
		So(output.MustGet(primitiveSecondOut), ShouldEqual, 20.0)
	})

	Convey("TryFork propagates a branch failure that already wrote a slot", t, func() {
		wroteThenFailed := func(input Frame) Frame {
			input.Put(primitiveFirstState, 1)
			input.Err = errors.New("genuine defect")

			return input
		}
		output := TryFork(wroteThenFailed)(Frame{}.Set(primitiveInput, 1))
		So(output.Err, ShouldNotBeNil)
	})

	Convey("TryFork propagates a branch failure that mutated an already-populated slot in place", t, func() {
		// The mask is unchanged (primitiveShared was already populated before
		// this branch ran), but the underlying data was overwritten. A
		// mask-only "did this branch write anything" check would wrongly
		// forgive this as an untouched, not-yet-observed branch.
		mutatedThenFailed := func(input Frame) Frame {
			input.Put(primitiveShared, 999)
			input.Err = errors.New("genuine defect after in-place mutation")

			return input
		}
		output := TryFork(mutatedThenFailed)(Frame{}.Set(primitiveShared, 1).Set(primitiveInput, 1))
		So(output.Err, ShouldNotBeNil)
	})
}

func TestConfigureContracts(t *testing.T) {
	producerState := MustIntern("test/configure/producer_state")
	consumerState := MustIntern("test/configure/consumer_state")
	consumerFact := MustIntern("test/configure/consumer_fact")

	producer := func(input Frame) Frame {
		input.Put(producerState, 1)
		input.Put(primitiveControl, 3)
		input.Put(primitiveMetric, 8)
		input.Put(primitiveShared, 1)

		return input
	}
	consumer := func(input Frame) Frame {
		if input.MustGet(primitiveInput) != 2 || input.MustGet(primitiveControl) != 3 {
			input.Err = errors.New("consumer did not receive original input plus control")

			return input
		}

		if input.MustGet(producerState) != 1 {
			input.Err = errors.New("consumer did not observe producer state")

			return input
		}

		input.Put(consumerState, 1)
		input.Put(consumerFact, 9)
		input.Put(primitiveShared, 2)

		return input
	}

	Convey("Configure preserves producer metrics and lets consumer output win", t, func() {
		output := Configure(producer, primitiveControl, consumer)(
			Frame{}.Set(primitiveInput, 2),
		)
		So(output.Err, ShouldBeNil)
		So(output.MustGet(producerState), ShouldEqual, 1.0)
		So(output.MustGet(consumerState), ShouldEqual, 1.0)
		So(output.MustGet(primitiveMetric), ShouldEqual, 8.0)
		So(output.MustGet(consumerFact), ShouldEqual, 9.0)
		So(output.MustGet(primitiveShared), ShouldEqual, 2.0)
	})

	Convey("Missing and non-finite control values are rejected", t, func() {
		missing := func(input Frame) Frame { return input }
		output := Configure(missing, primitiveControl, Identity)(Frame{})
		So(output.Err, ShouldNotBeNil)
		nonfinite := Assign(primitiveControl, math.NaN())
		output = Configure(nonfinite, primitiveControl, Identity)(Frame{})
		So(output.Err, ShouldNotBeNil)
	})

	Convey("A consumer failure propagates its error", t, func() {
		initial := Frame{}.Set(primitiveShared, 5).Merged(Frame{}.Set(primitiveInput, 2))
		output := Configure(producer, primitiveControl, func(input Frame) Frame {
			input.Err = errors.New("consumer failure")

			return input
		})(initial)
		So(output.Err, ShouldNotBeNil)
	})
}
