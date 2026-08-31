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
		output := initial
		Step(nil, &output)
		So(output.Err, ShouldNotBeNil)
		So(output.Equal(initial), ShouldBeTrue)

		bad := func(input *Frame) {
			input.Put(primitiveShared, 99)
			input.Err = errors.New("reject")
		}
		output = initial
		Step(bad, &output)
		So(output.Err, ShouldNotBeNil)
		So(output.MustGet(primitiveShared), ShouldEqual, 99.0)
	})

	Convey("Pipe is ordered and stops at the first failure", t, func() {
		first := func(input *Frame) {
			input.Put(primitiveFirstState, 1)
			input.Put(primitiveFirstOutput, input.MustGet(primitiveInput)+1)
		}
		second := func(input *Frame) {
			So(input.MustGet(primitiveFirstState), ShouldEqual, 1.0)
			So(input.MustGet(primitiveFirstOutput), ShouldEqual, 3.0)
			input.Put(primitiveSecondState, 1)
		}
		output := Frame{}.Set(primitiveInput, 2)
		Pipe(first, second)(&output)
		So(output.Err, ShouldBeNil)
		So(output.MustGet(primitiveSecondState), ShouldEqual, 1.0)
		So(output.MustGet(primitiveFirstOutput), ShouldEqual, 3.0)

		failed := Frame{}.Set(primitiveInput, 2)
		Pipe(first, func(input *Frame) {
			input.Err = errors.New("forced")
		})(&failed)
		So(failed.Err, ShouldNotBeNil)
	})

	Convey("An empty Pipe is the identity relation", t, func() {
		frame := Frame{}.Set(primitiveShared, 1).Set(primitiveInput, 2)
		output := frame
		Pipe()(&output)
		So(output.Err, ShouldBeNil)
		So(output.Equal(frame), ShouldBeTrue)
	})
}

func TestForkContracts(t *testing.T) {
	Convey("Fork is permissive fan-out: branches see the same input", t, func() {
		first := func(input *Frame) {
			input.Put(primitiveFirstState, 1)
			input.Put(primitiveFirstOutput, 10)
		}
		second := func(input *Frame) {
			if input.Has(primitiveFirstState) {
				input.Err = errors.New("second branch observed first branch state")

				return
			}

			if input.Has(primitiveFirstOutput) {
				input.Err = errors.New("second branch observed first branch output")

				return
			}

			input.Put(primitiveSecondState, 1)
			input.Put(primitiveSecondOut, 20)
		}
		output := Frame{}.Set(primitiveInput, 1)
		Fork(first, second)(&output)
		So(output.Err, ShouldBeNil)
		So(output.MustGet(primitiveFirstState), ShouldEqual, 1.0)
		So(output.MustGet(primitiveSecondState), ShouldEqual, 1.0)
		So(output.MustGet(primitiveFirstOutput), ShouldEqual, 10.0)
		So(output.MustGet(primitiveSecondOut), ShouldEqual, 20.0)
	})

	Convey("Given forked branches that each accumulate their own state", t, func() {
		// Every branch starts from the same pre-fork snapshot, so each carries
		// that snapshot's whole populated mask. Merging a branch wholesale
		// therefore wrote the stale snapshot value back over the branches
		// before it, and only the last branch's state ever advanced.
		count := func(slot Symbol) Primitive {
			return func(input *Frame) {
				previous, _ := input.Get(slot)
				input.Put(slot, previous+1)
			}
		}

		machine := Fork(count(primitiveFirstState), count(primitiveSecondState))
		output := Frame{}

		for range 4 {
			machine(&output)
		}

		Convey("every branch's state advances, not only the last one's", func() {
			So(output.Err, ShouldBeNil)
			So(output.MustGet(primitiveFirstState), ShouldEqual, 4.0)
			So(output.MustGet(primitiveSecondState), ShouldEqual, 4.0)
		})
	})

	Convey("Given a forked branch that writes nothing at all", t, func() {
		machine := Fork(
			func(input *Frame) {
				previous, _ := input.Get(primitiveFirstState)
				input.Put(primitiveFirstState, previous+1)
			},
			func(input *Frame) {},
		)
		output := Frame{}

		for range 4 {
			machine(&output)
		}

		Convey("it leaves the other branch's state alone", func() {
			So(output.MustGet(primitiveFirstState), ShouldEqual, 4.0)
		})
	})

	Convey("ForkStrict rejects conflicting writes transactionally", t, func() {
		write := func(value float64) Primitive {
			return func(input *Frame) {
				input.Put(primitiveShared, value)
			}
		}
		output := Frame{}.Set(primitiveShared, 0)
		ForkStrict(write(1), write(2))(&output)
		So(output.Err, ShouldNotBeNil)
		So(output.Err.Error(), ShouldContainSubstring, "collision")
	})

	Convey("ForkStrict accepts identical writes", t, func() {
		write := func(input *Frame) {
			input.Put(primitiveShared, 7)
		}
		output := Frame{}
		ForkStrict(write, write)(&output)
		So(output.Err, ShouldBeNil)
		So(output.MustGet(primitiveShared), ShouldEqual, 7.0)
	})

	Convey("ForkStrict rejects collisions that permissive Fork overlays", t, func() {
		write := func(value float64) Primitive {
			return func(input *Frame) {
				input.Put(primitiveShared, value)
			}
		}
		permissive := Frame{}
		Fork(write(1), write(2))(&permissive)
		So(permissive.Err, ShouldBeNil)
		So(permissive.MustGet(primitiveShared), ShouldEqual, 2.0)
		strict := Frame{}
		ForkStrict(write(1), write(2))(&strict)
		So(strict.Err, ShouldNotBeNil)
	})
}

func TestTryForkContracts(t *testing.T) {
	Convey("TryFork drops a branch that fails without writing any slot", t, func() {
		ready := func(input *Frame) {
			input.Put(primitiveFirstOutput, 10)
		}
		notYetObserved := func(input *Frame) {
			input.Err = errors.New("required fact absent")
		}
		output := Frame{}.Set(primitiveInput, 1)
		TryFork(ready, notYetObserved)(&output)
		So(output.Err, ShouldBeNil)
		So(output.MustGet(primitiveFirstOutput), ShouldEqual, 10.0)
		So(output.Has(primitiveSecondOut), ShouldBeFalse)
	})

	Convey("TryFork still composes every branch once each has arrived", t, func() {
		first := func(input *Frame) {
			input.Put(primitiveFirstOutput, 10)
		}
		second := func(input *Frame) {
			input.Put(primitiveSecondOut, 20)
		}
		output := Frame{}.Set(primitiveInput, 1)
		TryFork(first, second)(&output)
		So(output.Err, ShouldBeNil)
		So(output.MustGet(primitiveFirstOutput), ShouldEqual, 10.0)
		So(output.MustGet(primitiveSecondOut), ShouldEqual, 20.0)
	})

	Convey("TryFork propagates a branch failure that already wrote a slot", t, func() {
		wroteThenFailed := func(input *Frame) {
			input.Put(primitiveFirstState, 1)
			input.Err = errors.New("genuine defect")
		}
		output := Frame{}.Set(primitiveInput, 1)
		TryFork(wroteThenFailed)(&output)
		So(output.Err, ShouldNotBeNil)
	})

	Convey("TryFork propagates a branch failure that mutated an already-populated slot in place", t, func() {
		// The mask is unchanged (primitiveShared was already populated before
		// this branch ran), but the underlying data was overwritten. A
		// mask-only "did this branch write anything" check would wrongly
		// forgive this as an untouched, not-yet-observed branch.
		mutatedThenFailed := func(input *Frame) {
			input.Put(primitiveShared, 999)
			input.Err = errors.New("genuine defect after in-place mutation")
		}
		output := Frame{}.Set(primitiveShared, 1).Set(primitiveInput, 1)
		TryFork(mutatedThenFailed)(&output)
		So(output.Err, ShouldNotBeNil)
	})
}

func TestConfigureContracts(t *testing.T) {
	producerState := MustIntern("test/configure/producer_state")
	consumerState := MustIntern("test/configure/consumer_state")
	consumerFact := MustIntern("test/configure/consumer_fact")

	producer := func(input *Frame) {
		input.Put(producerState, 1)
		input.Put(primitiveControl, 3)
		input.Put(primitiveMetric, 8)
		input.Put(primitiveShared, 1)
	}
	consumer := func(input *Frame) {
		if input.MustGet(primitiveInput) != 2 || input.MustGet(primitiveControl) != 3 {
			input.Err = errors.New("consumer did not receive original input plus control")

			return
		}

		if input.MustGet(producerState) != 1 {
			input.Err = errors.New("consumer did not observe producer state")

			return
		}

		input.Put(consumerState, 1)
		input.Put(consumerFact, 9)
		input.Put(primitiveShared, 2)
	}

	Convey("Configure preserves producer metrics and lets consumer output win", t, func() {
		output := Frame{}.Set(primitiveInput, 2)
		Configure(producer, primitiveControl, consumer)(&output)
		So(output.Err, ShouldBeNil)
		So(output.MustGet(producerState), ShouldEqual, 1.0)
		So(output.MustGet(consumerState), ShouldEqual, 1.0)
		So(output.MustGet(primitiveMetric), ShouldEqual, 8.0)
		So(output.MustGet(consumerFact), ShouldEqual, 9.0)
		So(output.MustGet(primitiveShared), ShouldEqual, 2.0)
	})

	Convey("Missing and non-finite control values are rejected", t, func() {
		missing := func(input *Frame) {}
		output := Frame{}
		Configure(missing, primitiveControl, Identity)(&output)
		So(output.Err, ShouldNotBeNil)

		nonfinite := Assign(primitiveControl, math.NaN())
		output = Frame{}
		Configure(nonfinite, primitiveControl, Identity)(&output)
		So(output.Err, ShouldNotBeNil)
	})

	Convey("A consumer failure propagates its error", t, func() {
		initial := Frame{}.Set(primitiveShared, 5).Merged(Frame{}.Set(primitiveInput, 2))
		output := initial
		Configure(producer, primitiveControl, func(input *Frame) {
			input.Err = errors.New("consumer failure")
		})(&output)
		So(output.Err, ShouldNotBeNil)
	})
}
