package types

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

var (
	wireFactA       = MustIntern("test/wire/fact/a")
	wireFactB       = MustIntern("test/wire/fact/b")
	wireFactResult  = MustIntern("test/wire/fact/result")
	wireUnrelated   = MustIntern("test/wire/unrelated")
	wirePortA       = MustIntern("test/wire/port/a")
	wirePortB       = MustIntern("test/wire/port/b")
	wirePortResult  = MustIntern("test/wire/port/result")
	wirePortState   = MustIntern("test/wire/port/state")
	wireOuterStateA = MustIntern("test/wire/state/a")
	wireOuterStateB = MustIntern("test/wire/state/b")
)

func TestWireDataBoundary(t *testing.T) {
	add := func(input Frame) Frame {
		if input.Count() != 2 || input.Has(wireUnrelated) {
			input.Err = errors.New("primitive received an unbound fact")

			return input
		}

		input.Put(wirePortResult, input.MustGet(wirePortA)+input.MustGet(wirePortB))

		return input
	}
	wired := Wire(
		add,
		In(wireFactA, wirePortA),
		In(wireFactB, wirePortB),
		Out(wirePortResult, wireFactResult),
	)

	Convey("Wire gives a primitive only deliberately bound local ports", t, func() {
		input := Frame{}.
			Set(wireFactA, 3).
			Set(wireFactB, 4).
			Set(wireUnrelated, 99)
		output := wired(input)
		So(output.Err, ShouldBeNil)
		So(output.MustGet(wireFactResult), ShouldEqual, 7.0)
		So(output.MustGet(wireUnrelated), ShouldEqual, 99.0)
		So(output.Has(wirePortResult), ShouldBeFalse)
	})

	Convey("Wire never guesses a substitute for a missing fact", t, func() {
		input := Frame{}.
			Set(wireFactA, 3).
			Set(wirePortB, 4).
			Set(wireUnrelated, 5)
		output := wired(input)
		So(output.Err, ShouldNotBeNil)
		So(output.Err.Error(), ShouldContainSubstring, "missing")
		So(output.Has(wireFactResult), ShouldBeFalse)
	})

	Convey("A missing declared output is an incompatibility, not a zero", t, func() {
		bad := Wire(
			func(input Frame) Frame { return Frame{} },
			In(wireFactA, wirePortA),
			Out(wirePortResult, wireFactResult),
		)
		output := bad(Frame{}.Set(wireFactA, 1))
		So(output.Err, ShouldNotBeNil)
		So(output.Has(wireFactResult), ShouldBeFalse)
	})

	Convey("Ambiguous bindings are rejected before the primitive can run", t, func() {
		calls := 0
		primitive := func(input Frame) Frame {
			calls++

			return input
		}
		ambiguousInput := Wire(
			primitive,
			In(wireFactA, wirePortA),
			In(wireFactB, wirePortA),
		)
		output := ambiguousInput(Frame{}.Set(wireFactA, 1).Set(wireFactB, 2))
		So(output.Err, ShouldNotBeNil)
		So(calls, ShouldEqual, 0)

		ambiguousOutput := Wire(
			primitive,
			Out(wirePortA, wireFactResult),
			Out(wirePortB, wireFactResult),
		)
		output = ambiguousOutput(Frame{})
		So(output.Err, ShouldNotBeNil)
		So(calls, ShouldEqual, 0)
	})

	Convey("Established wiring performs no heap allocation", t, func() {
		input := Frame{}.Set(wireFactA, 3).Set(wireFactB, 4)
		allocations := testing.AllocsPerRun(1000, func() {
			output := wired(input)

			if output.Err != nil {
				panic(output.Err)
			}
		})
		So(allocations, ShouldEqual, 0.0)
	})
}

func TestWireStateBoundary(t *testing.T) {
	accumulate := func(input Frame) Frame {
		total, _ := input.Get(wirePortState)
		total += input.MustGet(wirePortA)
		input.Put(wirePortState, total)
		input.Put(wirePortResult, total)

		return input
	}

	Convey("Mapped local state commits back to its named outer fact", t, func() {
		wired := Wire(
			accumulate,
			In(wireFactA, wirePortA),
			State(wireOuterStateA, wirePortState),
			Out(wirePortResult, wireFactResult),
		)
		input := Frame{}.
			Set(wireOuterStateA, 5).
			Set(wireUnrelated, 9).
			Set(wireFactA, 2)
		output := wired(input)
		So(output.Err, ShouldBeNil)
		So(output.MustGet(wireOuterStateA), ShouldEqual, 7.0)
		So(output.MustGet(wireUnrelated), ShouldEqual, 9.0)
		So(output.MustGet(wireFactResult), ShouldEqual, 7.0)
	})

	Convey("Two instances of one stateful atom can be scoped independently", t, func() {
		first := Wire(
			accumulate,
			In(wireFactA, wirePortA),
			State(wireOuterStateA, wirePortState),
			Out(wirePortResult, wireFactA),
		)
		second := Wire(
			accumulate,
			In(wireFactB, wirePortA),
			State(wireOuterStateB, wirePortState),
			Out(wirePortResult, wireFactB),
		)
		input := Frame{}.
			Set(wireOuterStateA, 10).
			Set(wireOuterStateB, 100).
			Set(wireFactA, 1).
			Set(wireFactB, 2)
		output := ForkStrict(first, second)(input)
		So(output.Err, ShouldBeNil)
		So(output.MustGet(wireOuterStateA), ShouldEqual, 11.0)
		So(output.MustGet(wireOuterStateB), ShouldEqual, 102.0)
		So(output.MustGet(wireFactA), ShouldEqual, 11.0)
		So(output.MustGet(wireFactB), ShouldEqual, 102.0)
	})

	Convey("Unbound local state writes are discarded", t, func() {
		bad := Wire(func(input Frame) Frame {
			input.Put(wirePortState, 1)

			return input
		})
		initial := Frame{}.Set(wireOuterStateA, 7)
		output := bad(initial)
		So(output.Err, ShouldBeNil)
		So(output.MustGet(wireOuterStateA), ShouldEqual, 7.0)
		So(output.Has(wirePortState), ShouldBeFalse)
	})

	Convey("Deleting a mapped local state port deletes its outer fact", t, func() {
		clear := Wire(
			func(input Frame) Frame {
				input.Delete(wirePortState)

				return input
			},
			State(wireOuterStateA, wirePortState),
		)
		output := clear(Frame{}.Set(wireOuterStateA, 1))
		So(output.Err, ShouldBeNil)
		So(output.Has(wireOuterStateA), ShouldBeFalse)
	})
}
