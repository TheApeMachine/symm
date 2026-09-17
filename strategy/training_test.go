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
	Convey("Training composes Grid, Region, and Cognition into a streaming prediction pipeline", t, func() {
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

		// Train an association for region "r0" to take ActionEnter
		assoc := cognition.Association{
			Context:  []byte("r0"),
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

		// When the dominant region token matches the trained context, evaluate produces the winner
		if len(evals) > 0 {
			eval := evals[len(evals)-1]
			if string(eval.Context) == "r0" {
				So(eval.WinnerClass, ShouldEqual, string(strategy.ActionEnter))
				So(eval.Confidence, ShouldBeGreaterThan, 0.5)
			}
		}
	})
}
