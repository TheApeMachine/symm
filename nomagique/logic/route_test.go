package logic

import (
	"errors"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestIfRouting(t *testing.T) {
	predicateState := types.MustIntern("test/logic/if/predicate_state")
	branchState := types.MustIntern("test/logic/if/branch_state")
	branchOutput := types.MustIntern("test/logic/if/branch_output")

	predicate := func(condition float64) types.Primitive {
		return func(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
			state.Put(predicateState, 1)
			output := input
			output.Put(SymbolCondition, condition)
			return state, output, nil
		}
	}

	Convey("If evaluates its predicate and exactly one selected branch", t, func() {
		trueCalls := 0
		falseCalls := 0
		whenTrue := func(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
			trueCalls++
			state.Put(branchState, 1)
			output := input
			output.Put(branchOutput, 10)
			return state, output, nil
		}
		whenFalse := func(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
			falseCalls++
			state.Put(branchState, -1)
			output := input
			output.Put(branchOutput, 20)
			return state, output, nil
		}
		next, output, err := If(predicate(1), whenTrue, whenFalse)(types.Frame{}, types.Frame{})
		So(err, ShouldBeNil)
		So(trueCalls, ShouldEqual, 1)
		So(falseCalls, ShouldEqual, 0)
		So(next.MustGet(predicateState), ShouldEqual, 1.0)
		So(next.MustGet(branchState), ShouldEqual, 1.0)
		So(output.MustGet(branchOutput), ShouldEqual, 10.0)
	})

	Convey("False is exactly zero; every finite non-zero value is true", t, func() {
		for _, condition := range []float64{-7, -0.1, 0.1, 7} {
			_, output, err := If(predicate(condition), types.Assign(branchOutput, 1), types.Assign(branchOutput, 0))(
				types.Frame{}, types.Frame{},
			)
			So(err, ShouldBeNil)
			So(output.MustGet(branchOutput), ShouldEqual, 1.0)
		}
		_, output, err := If(predicate(0), types.Assign(branchOutput, 1), types.Assign(branchOutput, 0))(
			types.Frame{}, types.Frame{},
		)
		So(err, ShouldBeNil)
		So(output.MustGet(branchOutput), ShouldEqual, 0.0)
	})

	Convey("Non-finite and missing predicate output is refused", t, func() {
		for _, bad := range []types.Primitive{
			predicate(math.NaN()),
			predicate(math.Inf(1)),
			types.Identity,
		} {
			_, _, err := If(bad, types.Identity, types.Identity)(types.Frame{}, types.Frame{})
			So(err, ShouldNotBeNil)
		}
	})

	Convey("A selected branch error rolls predicate and branch state back", t, func() {
		initial := types.Frame{}.Set(branchState, 4)
		badBranch := func(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
			state.Put(branchState, 999)
			return state, input, errors.New("branch rejected")
		}
		next, output, err := If(predicate(1), badBranch, nil)(initial, types.Frame{})
		So(err, ShouldNotBeNil)
		So(next.Equal(initial), ShouldBeTrue)
		So(output.Count(), ShouldEqual, 0)
	})
}

func TestCircuitRouting(t *testing.T) {
	firstState := types.MustIntern("test/logic/circuit/first_state")
	selected := types.MustIntern("test/logic/circuit/selected")
	predicate := func(condition float64, mutate types.Symbol) types.Primitive {
		return func(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
			if mutate != 0 {
				state.Put(mutate, 1)
			}
			output := input
			output.Put(SymbolCondition, condition)
			return state, output, nil
		}
	}

	Convey("Circuit executes only the first matching rule", t, func() {
		calls := 0
		branch := func(value float64) types.Primitive {
			return func(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
				calls++
				output := input
				output.Put(selected, value)
				return state, output, nil
			}
		}
		program := Circuit([]Rule{
			{When: predicate(0, firstState), Then: branch(1)},
			{When: predicate(1, 0), Then: branch(2)},
			{When: predicate(1, 0), Then: branch(3)},
		}, branch(4))
		next, output, err := program(types.Frame{}, types.Frame{})
		So(err, ShouldBeNil)
		So(calls, ShouldEqual, 1)
		So(next.MustGet(firstState), ShouldEqual, 1.0)
		So(output.MustGet(selected), ShouldEqual, 2.0)
	})

	Convey("Circuit runs fallback only when no rule matches", t, func() {
		program := Circuit([]Rule{{When: predicate(0, 0), Then: types.Assign(selected, 1)}}, types.Assign(selected, 9))
		_, output, err := program(types.Frame{}, types.Frame{})
		So(err, ShouldBeNil)
		So(output.MustGet(selected), ShouldEqual, 9.0)
	})

	Convey("A late predicate or fallback failure rolls all earlier candidates back", t, func() {
		initial := types.Frame{}.Set(firstState, 4)
		failure := func(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
			state.Put(firstState, 999)
			return state, input, errors.New("reject")
		}
		program := Circuit([]Rule{{When: predicate(0, firstState), Then: nil}}, failure)
		next, output, err := program(initial, types.Frame{})
		So(err, ShouldNotBeNil)
		So(next.Equal(initial), ShouldBeTrue)
		So(output.Count(), ShouldEqual, 0)
	})
}
