package strategy

import (
	"context"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
	"github.com/theapemachine/symm/nomagique/runtime"
)

type Action string

const (
	ActionEnter Action = "enter"
	ActionExit  Action = "exit"
	ActionWait  Action = "wait"
)

func LegalActions(holding bool) []Action {
	if !holding {
		return []Action{ActionEnter, ActionWait}
	}

	return []Action{ActionExit, ActionWait}
}

/*
Training owns the cognitive precursor learning model: one shared radix trie
that compiles completed tape excursions directly into associative basins and
runs offline REM consolidation and periodic decay pruning.
*/
type Training struct {
	*runtime.System
	fragments []iter.Seq[unsafe.Pointer]
	pipeline  *nomagique.Number
	trader    *Trader
}

func NewStrategy(ctx context.Context, api *websocket.API) *Training {
	training := &Training{
		pipeline: nomagique.NewNumber(
			grid.NewSpace(),
			cognition.NewEngine(cognition.Config{}),
		),
		trader: NewTrader(ctx, api),
	}

	training.System = runtime.NewSystem(ctx, "strategy", training)
	return training
}

/*
Step implements the runtime.Node interface, and this is the active trading path.
*/
func (training *Training) Step(measurement *data.Measurement[float64]) *data.Measurement[float64] {
	action := data.Read[Action](training.pipeline.Next(data.NewValue(measurement)))

	switch action {
	case ActionEnter, ActionExit:
		training.trader.OnAction(measurement.Label, action)
	case ActionWait:
		// TODO: Do nothing.
	}

	return measurement
}

func (training *Training) Register() *data.Measurement[float64] {
	return data.NewMeasurement("training", map[string]data.Metric[float64]{})
}

/*
Learn takes measurements and trains the engine on the measurements.
*/
func (training *Training) Learn(fragments iter.Seq[iter.Seq[unsafe.Pointer]]) {
	go func() {
		for fragment := range fragments {
			training.fragments = append(training.fragments, fragment)
			training.pipeline.Next(fragment)
		}

		for {
			for _, fragment := range training.fragments {
				action := data.Read[Action](training.pipeline.Next(fragment))

				switch action {
				case ActionEnter:
					// TODO: Check if this is a friction clearing upwards movement.
					// If so, strengthen this path in the model.
					// If not, weaken it.
				case ActionExit:
					// TODO: Check if this is close to the perfect exit moment.
					// If so, strengthen this path in the model.
					// If not, weaken it.
				case ActionWait:
					// TODO: Check if this is the correct thing to do.
					// If so, strengthen this path in the model.
					// If not, weaken it.
				}
			}
		}
	}()
}
