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
		return func(input *types.Frame) {
			input.Put(predicateState, 1)
			input.Put(SymbolCondition, condition)
		}
	}

	Convey("If evaluates its predicate and exactly one selected branch", t, func() {
		trueCalls := 0
		falseCalls := 0
		whenTrue := func(input *types.Frame) {
			trueCalls++
			input.Put(branchState, 1)
			input.Put(branchOutput, 10)
		}
		whenFalse := func(input *types.Frame) {
			falseCalls++
			input.Put(branchState, -1)
			input.Put(branchOutput, 20)
		}
		output := types.Frame{}
		If(predicate(1), whenTrue, whenFalse)(&output)
		So(output.Err, ShouldBeNil)
		So(trueCalls, ShouldEqual, 1)
		So(falseCalls, ShouldEqual, 0)
		So(output.MustGet(predicateState), ShouldEqual, 1.0)
		So(output.MustGet(branchState), ShouldEqual, 1.0)
		So(output.MustGet(branchOutput), ShouldEqual, 10.0)
	})

	Convey("False is exactly zero; every finite non-zero value is true", t, func() {
		for _, condition := range []float64{-7, -0.1, 0.1, 7} {
			output := types.Frame{}
			If(predicate(condition), types.Assign(branchOutput, 1), types.Assign(branchOutput, 0))(&output)
			So(output.Err, ShouldBeNil)
			So(output.MustGet(branchOutput), ShouldEqual, 1.0)
		}
		output := types.Frame{}
		If(predicate(0), types.Assign(branchOutput, 1), types.Assign(branchOutput, 0))(&output)
		So(output.Err, ShouldBeNil)
		So(output.MustGet(branchOutput), ShouldEqual, 0.0)
	})

	Convey("Non-finite and missing predicate output is refused", t, func() {
		for _, bad := range []types.Primitive{
			predicate(math.NaN()),
			predicate(math.Inf(1)),
			types.Identity,
		} {
			output := types.Frame{}
			If(bad, types.Identity, types.Identity)(&output)
			So(output.Err, ShouldNotBeNil)
		}
	})

	Convey("A selected branch error rolls the committed state back", t, func() {
		initial := types.Frame{}.Set(branchState, 4)
		badBranch := func(input *types.Frame) {
			input.Put(branchState, 999)
			input.Err = errors.New("branch rejected")
		}
		stream := types.NewStream(If(predicate(1), badBranch, nil), initial)
		output := stream.Step(types.Frame{})
		So(output.Err, ShouldNotBeNil)
		So(stream.Project().Equal(initial), ShouldBeTrue)
	})
}

func TestCircuitRouting(t *testing.T) {
	firstState := types.MustIntern("test/logic/circuit/first_state")
	selected := types.MustIntern("test/logic/circuit/selected")
	predicate := func(condition float64, mutate types.Symbol) types.Primitive {
		return func(input *types.Frame) {
			if mutate != 0 {
				input.Put(mutate, 1)
			}
			input.Put(SymbolCondition, condition)
		}
	}

	Convey("Circuit executes only the first matching rule", t, func() {
		calls := 0
		branch := func(value float64) types.Primitive {
			return func(input *types.Frame) {
				calls++
				input.Put(selected, value)
			}
		}
		program := Circuit([]Rule{
			{When: predicate(0, firstState), Then: branch(1)},
			{When: predicate(1, 0), Then: branch(2)},
			{When: predicate(1, 0), Then: branch(3)},
		}, branch(4))
		output := types.Frame{}
		program(&output)
		So(output.Err, ShouldBeNil)
		So(calls, ShouldEqual, 1)
		So(output.MustGet(firstState), ShouldEqual, 1.0)
		So(output.MustGet(selected), ShouldEqual, 2.0)
	})

	Convey("Circuit runs fallback only when no rule matches", t, func() {
		program := Circuit([]Rule{{When: predicate(0, 0), Then: types.Assign(selected, 1)}}, types.Assign(selected, 9))
		output := types.Frame{}
		program(&output)
		So(output.Err, ShouldBeNil)
		So(output.MustGet(selected), ShouldEqual, 9.0)
	})

	Convey("A late predicate or fallback failure rolls the committed state back", t, func() {
		initial := types.Frame{}.Set(firstState, 4)
		failure := func(input *types.Frame) {
			input.Put(firstState, 999)
			input.Err = errors.New("reject")
		}
		program := Circuit([]Rule{{When: predicate(0, firstState), Then: nil}}, failure)
		stream := types.NewStream(program, initial)
		output := stream.Step(types.Frame{})
		So(output.Err, ShouldNotBeNil)
		So(stream.Project().Equal(initial), ShouldBeTrue)
	})
}
