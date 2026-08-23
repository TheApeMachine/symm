package types

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/types"
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
	add := func(state Frame, input Frame) (Frame, Frame, error) {
		if input.Count() != 2 || input.Has(wireUnrelated) {
			return state, types.Frame{}, errors.New("primitive received an unbound fact")
		}
		output := types.Frame{}
		output.Put(wirePortResult, input.MustGet(wirePortA)+input.MustGet(wirePortB))
		return state, output, nil
	}
	wired := Wire(
		add,
		In(wireFactA, wirePortA),
		In(wireFactB, wirePortB),
		Out(wirePortResult, wireFactResult),
	)

	Convey("Wire gives a primitive only deliberately bound local ports", t, func() {
		input := types.Frame{}.
			Set(wireFactA, 3).
			Set(wireFactB, 4).
			Set(wireUnrelated, 99)
		_, output, err := wired(Frame{}, input)
		So(err, ShouldBeNil)
		So(output.MustGet(wireFactResult), ShouldEqual, 7.0)
		So(output.MustGet(wireUnrelated), ShouldEqual, 99.0)
		So(output.Has(wirePortResult), ShouldBeFalse)
	})

	Convey("Wire never guesses a substitute for a missing fact", t, func() {
		input := types.Frame{}.
			Set(wireFactA, 3).
			Set(wirePortB, 4).
			Set(wireUnrelated, 5)
		_, output, err := wired(Frame{}, input)
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "missing")
		So(output.Count(), ShouldEqual, 0)
	})

	Convey("A missing declared output is an incompatibility, not a zero", t, func() {
		bad := Wire(
			func(state Frame, input Frame) (Frame, Frame, error) { return state, types.Frame{}, nil },
			In(wireFactA, wirePortA),
			Out(wirePortResult, wireFactResult),
		)
		_, output, err := bad(Frame{}, types.Frame{}.Set(wireFactA, 1))
		So(err, ShouldNotBeNil)
		So(output.Count(), ShouldEqual, 0)
	})

	Convey("Ambiguous bindings are rejected before the primitive can run", t, func() {
		calls := 0
		primitive := func(state Frame, input Frame) (Frame, Frame, error) {
			calls++
			return state, input, nil
		}
		ambiguousInput := Wire(
			primitive,
			In(wireFactA, wirePortA),
			In(wireFactB, wirePortA),
		)
		_, _, err := ambiguousInput(Frame{}, types.Frame{}.Set(wireFactA, 1).Set(wireFactB, 2))
		So(err, ShouldNotBeNil)
		So(calls, ShouldEqual, 0)

		ambiguousOutput := Wire(
			primitive,
			Out(wirePortA, wireFactResult),
			Out(wirePortB, wireFactResult),
		)
		_, _, err = ambiguousOutput(Frame{}, types.Frame{})
		So(err, ShouldNotBeNil)
		So(calls, ShouldEqual, 0)
	})

	Convey("Established wiring performs no heap allocation", t, func() {
		input := types.Frame{}.Set(wireFactA, 3).Set(wireFactB, 4)
		allocations := testing.AllocsPerRun(1000, func() {
			_, _, err := wired(Frame{}, input)
			if err != nil {
				panic(err)
			}
		})
		So(allocations, ShouldEqual, 0.0)
	})
}

func TestWireStateBoundary(t *testing.T) {
	accumulate := func(state Frame, input Frame) (Frame, Frame, error) {
		total, _ := state.Get(wirePortState)
		total += input.MustGet(wirePortA)
		state.Put(wirePortState, total)
		output := types.Frame{}.Set(wirePortResult, total)
		return state, output, nil
	}

	Convey("Mapped local state commits back to its named outer fact", t, func() {
		wired := Wire(
			accumulate,
			In(wireFactA, wirePortA),
			State(wireOuterStateA, wirePortState),
			Out(wirePortResult, wireFactResult),
		)
		state := types.Frame{}.Set(wireOuterStateA, 5).Set(wireUnrelated, 9)
		next, output, err := wired(state, types.Frame{}.Set(wireFactA, 2))
		So(err, ShouldBeNil)
		So(next.MustGet(wireOuterStateA), ShouldEqual, 7.0)
		So(next.MustGet(wireUnrelated), ShouldEqual, 9.0)
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
		state := types.Frame{}.Set(wireOuterStateA, 10).Set(wireOuterStateB, 100)
		input := types.Frame{}.Set(wireFactA, 1).Set(wireFactB, 2)
		next, output, err := ForkStrict(first, second)(state, input)
		So(err, ShouldBeNil)
		So(next.MustGet(wireOuterStateA), ShouldEqual, 11.0)
		So(next.MustGet(wireOuterStateB), ShouldEqual, 102.0)
		So(output.MustGet(wireFactA), ShouldEqual, 11.0)
		So(output.MustGet(wireFactB), ShouldEqual, 102.0)
	})

	Convey("Unbound local state writes are rejected transactionally", t, func() {
		bad := Wire(func(state Frame, input Frame) (Frame, Frame, error) {
			state.Put(wirePortState, 1)
			return state, input, nil
		})
		initial := types.Frame{}.Set(wireOuterStateA, 7)
		next, output, err := bad(initial, types.Frame{})
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "unbound state")
		So(next.Equal(initial), ShouldBeTrue)
		So(output.Count(), ShouldEqual, 0)
	})

	Convey("Deleting a mapped local state port deletes its outer fact", t, func() {
		clear := Wire(
			func(state Frame, input Frame) (Frame, Frame, error) {
				state.Delete(wirePortState)
				return state, input, nil
			},
			State(wireOuterStateA, wirePortState),
		)
		next, _, err := clear(Frame{}.Set(wireOuterStateA, 1), types.Frame{})
		So(err, ShouldBeNil)
		So(next.Has(wireOuterStateA), ShouldBeFalse)
	})
}
