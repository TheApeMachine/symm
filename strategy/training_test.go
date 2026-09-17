package strategy_test

import (
	"iter"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/geometry"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/strategy"
)

type testCell struct {
	*core.PrimitiveError
	address *geometry.Coordinate
	obs     *statistic.Observation[*geometry.Coordinate]
}

func newTestCell(x, y int, movement, authority float64) *testCell {
	coord := geometry.NewCoordinate(x, y)
	return &testCell{
		PrimitiveError: core.NewPrimitiveError(),
		address:        coord,
		obs:            statistic.NewObservation(coord, movement, authority, 1.0),
	}
}

func (cell *testCell) Identity() *geometry.Coordinate { return cell.address }

func (cell *testCell) Identify(addr *geometry.Coordinate) core.Identifiable[*geometry.Coordinate] {
	cell.address = addr
	return cell
}

func (cell *testCell) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		yield(unsafe.Pointer(cell.obs))
	}
}

func TestTrainingPipeline(t *testing.T) {
	Convey("Training composes Grid, Region, Transition, and Cognition into a streaming pipeline", t, func() {
		cellA := newTestCell(0, 0, 0.5, 1.0)
		cellMid := newTestCell(1, 0, 0.5, 0.25)
		cellB := newTestCell(2, 0, 0.5, 1.0)

		training := strategy.NewTraining(
			t.Context(),
			nil,
			map[string]core.Identifiable[*geometry.Coordinate]{"BTC/USD": cellA},
			map[string]core.Identifiable[*geometry.Coordinate]{"BTC/USD": cellMid},
			map[string]core.Identifiable[*geometry.Coordinate]{"BTC/USD": cellB},
		)

		So(training, ShouldNotBeNil)

		// Verification 1: Same identity coordinates remain unchanged after relaxation
		origXA, origYA := cellA.address.X, cellA.address.Y
		origXB, origYB := cellB.address.X, cellB.address.Y

		expectedContext := "0,0;1,0->0,0;1,0"

		// Pre-train an association for the expected transition to take ActionEnter
		assoc := cognition.Association{
			Context:  []byte(expectedContext),
			Class:    []byte(strategy.ActionEnter),
			Feedback: 1.0,
			Graded:   true,
		}
		inAssoc := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&assoc))
		}
		for range training.Reinforce.Next(inAssoc) {
		}

		// Execute cell queries through the pipeline
		var evals []cognition.Evaluation
		for _, cell := range []*testCell{cellA, cellMid, cellB} {
			query := store.NewQuery[*geometry.Coordinate, any](nil, data.ActionExecute)
			query.Entity = "BTC/USD"
			query.Address = cell.Identity()

			for out := range training.Next(query.Next(nil)) {
				evals = append(evals, *(*cognition.Evaluation)(out))
			}
		}

		// Coordinates in the store remain unchanged
		So(cellA.address.X, ShouldEqual, origXA)
		So(cellA.address.Y, ShouldEqual, origYA)
		So(cellB.address.X, ShouldEqual, origXB)
		So(cellB.address.Y, ShouldEqual, origYB)

		// Query sequence: cellA primes sympathy (0 edges), cellMid emits first basin (primes transition),
		// cellB emits second basin which produces exactly 1 transition
		So(len(evals), ShouldEqual, 1)
		eval := evals[0]
		So(string(eval.Context), ShouldEqual, expectedContext)
		So(eval.WinnerClass, ShouldEqual, string(strategy.ActionEnter))
		So(eval.Confidence, ShouldBeGreaterThan, 0.5)

		// Downstream Decision integration
		holding := false
		decision := strategy.NewDecision(func() bool { return holding })

		inEval := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&eval))
		}

		var actions []strategy.Action
		for out := range decision.Next(inEval) {
			actions = append(actions, *(*strategy.Action)(out))
		}

		// When flat, ActionEnter is legal and emitted
		So(len(actions), ShouldEqual, 1)
		So(actions[0], ShouldEqual, strategy.ActionEnter)

		// When holding, ActionEnter is illegal and rejected downstream (abstains)
		holding = true
		actions = nil
		for out := range decision.Next(inEval) {
			actions = append(actions, *(*strategy.Action)(out))
		}

		So(len(actions), ShouldEqual, 0)
	})
}
