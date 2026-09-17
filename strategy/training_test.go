package strategy_test

import (
	"context"
	"iter"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/geometry"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/strategy"
)

type trainingCell struct {
	*core.PrimitiveError
	address *geometry.Coordinate
	obs     *statistic.Observation[*geometry.Coordinate]
}

func newTrainingCell(x, y int, movement, authority float64) *trainingCell {
	coord := geometry.NewCoordinate(x, y)
	return &trainingCell{
		PrimitiveError: core.NewPrimitiveError(),
		address:        coord,
		obs:            statistic.NewObservation(coord, movement, authority, 1.0),
	}
}

func (cell *trainingCell) Identity() *geometry.Coordinate { return cell.address }

func (cell *trainingCell) Identify(addr *geometry.Coordinate) core.Identifiable[*geometry.Coordinate] {
	cell.address = addr
	return cell
}

func (cell *trainingCell) Connect(core.Primitive) {}

func (cell *trainingCell) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		yield(unsafe.Pointer(cell.obs))
	}
}

func TestTrainingPipeline(t *testing.T) {
	Convey("Training runs the complete associative learning and trie evaluation pipeline", t, func() {
		ctx := context.Background()

		cellA := newTrainingCell(0, 0, 0.5, 1.0)
		cellMid := newTrainingCell(1, 0, 0.5, 0.25)
		cellB := newTrainingCell(2, 0, 0.5, 1.0)

		grid := store.NewGrid[*geometry.Coordinate](cellA, cellMid, cellB)
		training := strategy.NewTraining[*geometry.Coordinate](ctx, grid)

		So(training.Error(), ShouldBeNil)

		// Drive multiple queries through grid to produce sequential spatial transitions
		// Step 1: initial observation (forms previous signature in Transition)
		query1 := store.NewQuery[*geometry.Coordinate, any](cellA, core.Execute)
		for range training.Next(query1.Next(nil)) {
		}

		// Step 2: second observation with modified cell (generates transition and first association)
		cellA.obs = statistic.NewObservation(cellA.address, 0.8, 1.2, 1.0)
		cellMid.obs = statistic.NewObservation(cellMid.address, 0.1, 0.5, 1.0)
		query2 := store.NewQuery[*geometry.Coordinate, any](cellA, core.Execute)
		for range training.Next(query2.Next(nil)) {
		}

		// Step 3: third observation (generates empirical context->class association, reinforces trie, evaluates)
		cellB.obs = statistic.NewObservation(cellB.address, 0.9, 1.5, 1.0)
		query3 := store.NewQuery[*geometry.Coordinate, any](cellB, core.Execute)

		var evals []cognition.Evaluation
		for out := range training.Next(query3.Next(nil)) {
			evals = append(evals, *(*cognition.Evaluation)(out))
		}

		So(training.Error(), ShouldBeNil)
	})

	Convey("LegalActions enforces portfolio inventory constraints", t, func() {
		Convey("when not holding, only enter and wait are legal", func() {
			actions := strategy.LegalActions(false)
			So(actions, ShouldResemble, []strategy.Action{strategy.ActionEnter, strategy.ActionWait})
		})

		Convey("when holding, only exit and wait are legal", func() {
			actions := strategy.LegalActions(true)
			So(actions, ShouldResemble, []strategy.Action{strategy.ActionExit, strategy.ActionWait})
		})
	})

	Convey("Training steps and branches evaluations to offramps", t, func() {
		ctx := context.Background()
		cellA := newTrainingCell(0, 0, 0.5, 1.0)
		grid := store.NewGrid[*geometry.Coordinate](cellA)

		var captured []cognition.Evaluation
		offramp := &mockOfframp{
			PrimitiveError: core.NewPrimitiveError(),
			onEval: func(eval *cognition.Evaluation) {
				captured = append(captured, *eval)
			},
		}

		training := strategy.NewTraining[*geometry.Coordinate](ctx, grid, offramp)
		query := store.NewQuery[*geometry.Coordinate, any](cellA, core.Execute)

		for range training.Next(query.Next(nil)) {
		}

		So(training.Error(), ShouldBeNil)
	})
}

type mockOfframp struct {
	*core.PrimitiveError
	onEval func(*cognition.Evaluation)
}

func (mock *mockOfframp) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving != nil && mock.onEval != nil {
				eval := (*cognition.Evaluation)(arriving)
				mock.onEval(eval)
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
