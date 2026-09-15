package strategy

import (
	"context"
	"github.com/theapemachine/errnie"
	"iter"
	"sync"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/hindsight/tables"
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
	wg          sync.WaitGroup
	mu          sync.RWMutex
	measurement *data.Measurement[float64]
	precursor   *Precursor
	excursions  []tables.ExcursionRecord
	pipeline    *nomagique.Number
	engine      *cognition.Engine
	trader      *Trader
	space       *grid.Space
}

func NewTraining(ctx context.Context, api *websocket.API, space ...*grid.Space) *Training {
	engine := cognition.NewEngine(cognition.Config{})
	precursor := NewPrecursor()

	gridSpace := grid.NewSpace()
	if len(space) > 0 && space[0] != nil {
		gridSpace = space[0]
	}

	training := &Training{
		precursor: precursor,
		space:     gridSpace,
		pipeline: nomagique.NewNumber(
			gridSpace,
			precursor,
			engine,
		),
		engine: engine,
		trader: NewTrader(ctx, api),
	}

	training.Register()
	training.System = runtime.NewSystem(ctx, "strategy", training)
	return training
}

func (training *Training) Reset() {
	training.mu.Lock()
	defer training.mu.Unlock()
	training.precursor.Reset()
	if training.space != nil {
		training.space.Reset()
	}
}

/*
Register initializes and returns the canonical training telemetry measurement
populated with all metrics required by the frontend dashboard.
*/
func (training *Training) Register() *data.Measurement[float64] {
	training.mu.Lock()
	defer training.mu.Unlock()

	if training.measurement != nil {
		return training.measurement
	}

	training.measurement = data.NewMeasurement("training", map[string]data.Metric[float64]{
		"steps":      data.NewMetric[float64]("steps", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"decisions":  data.NewMetric[float64]("decisions", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"resolved":   data.NewMetric[float64]("resolved", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"confidence": data.NewMetric[float64]("confidence", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"contrast":   data.NewMetric[float64]("contrast", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"surprisal":  data.NewMetric[float64]("surprisal", data.UnitNat, data.TimescaleInstantaneous, 0, 1),
		"ambiguity":  data.NewMetric[float64]("ambiguity", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"action":     data.NewMetric[float64]("action", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"win_rate":   data.NewMetric[float64]("win_rate", data.UnitPercent, data.TimescaleInstantaneous, 0, 1),
		"edge":       data.NewMetric[float64]("edge", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"progress":   data.NewMetric[float64]("progress", data.UnitPercent, data.TimescaleInstantaneous, 0, 1),
		"accuracy":   data.NewMetric[float64]("accuracy", data.UnitPercent, data.TimescaleInstantaneous, 0, 1),
		"support":    data.NewMetric[float64]("support", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"quality":    data.NewMetric[float64]("quality", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
	})

	training.measurement.Label = "learner"
	training.measurement.Metadata["peer-interest"] = "*"

	return training.measurement
}

/*
Step implements the runtime.Node interface. It remains inert on market entry
until the model is confident enough, while updating and returning the held
telemetry measurement so telemetryTee streams it to the frontend.
*/
func (training *Training) Step(measurement *data.Measurement[float64]) *data.Measurement[float64] {
	if training.Status() != runtime.READY {
		errnie.Warn(training.Name() + ": Step called before READY; dropping event")
		return measurement
	}

	training.mu.Lock()
	current := training.measurement

	if current == nil {
		training.mu.Unlock()
		current = training.Register()
		training.mu.Lock()
	}

	if measurement != nil {
		if measurement.Source == "training" {
			training.measurement = measurement
			current = measurement
		}

		if measurement.Source != "training" {
			current.Label = measurement.Label
			current.At = measurement.At
			current.SeqIdx = measurement.SeqIdx
		}

		for _, peer := range measurement.Peers {
			if peer == nil || peer.Label == "" || peer.Label == "learner" {
				continue
			}

			current.Label = peer.Label
			current.At = peer.At
			current.SeqIdx = peer.SeqIdx
			break
		}
	}

	if current.Label == "" {
		current.Label = "learner"
	}
	training.mu.Unlock()

	if measurement == nil {
		return current
	}

	wireIn := func(yield func(unsafe.Pointer) bool) {
		yield(unsafe.Pointer(measurement))
	}
	eval := data.Read[cognition.Evaluation](training.pipeline.Next(wireIn))
	action := Action(eval.WinnerClass)

	// Step remains inert on market action until model has support, positive contrast, and low ambiguity.
	confident := eval.Support > 0 && !eval.IsBreak && eval.Contrast > 0 && eval.Ambiguity < 1.0

	if confident && (action == ActionEnter || action == ActionExit) {
		training.trader.OnAction(current.Label, action)
	}

	training.mu.Lock()
	stepsMetric := current.Metrics["steps"]
	stepsMetric = stepsMetric.Write(stepsMetric.Raw + 1)
	current.Metrics["steps"] = stepsMetric
	current.Metrics["support"] = current.Metrics["support"].Write(float64(eval.Support))
	current.Metrics["decisions"] = current.Metrics["decisions"].Write(stepsMetric.Raw)

	current.Metrics["confidence"] = current.Metrics["confidence"].Write(eval.Confidence)
	current.Metrics["contrast"] = current.Metrics["contrast"].Write(eval.Contrast)
	current.Metrics["surprisal"] = current.Metrics["surprisal"].Write(eval.Surprisal)
	current.Metrics["ambiguity"] = current.Metrics["ambiguity"].Write(eval.Ambiguity)

	actionVal := 0.0
	if action == ActionEnter {
		actionVal = 1.0
	}
	if action == ActionExit {
		actionVal = 2.0
	}
	current.Metrics["action"] = current.Metrics["action"].Write(actionVal)

	training.mu.Unlock()

	return current
}

/*
Learn takes excursions and measurement fragments to train the engine,
updating the held telemetry measurement throughout the learning process.
*/
func (training *Training) Learn(
	excursions []tables.ExcursionRecord,
	fragments iter.Seq[iter.Seq[unsafe.Pointer]],
) {
	training.excursions = excursions

	training.wg.Go(func() {
		index := 0
		totalSteps := 0
		totalWins := 0

		for fragment := range fragments {
			var excursion *tables.ExcursionRecord
			if index < len(training.excursions) {
				excursion = &training.excursions[index]
				training.precursor.SetExcursion(excursion)
				if training.space != nil {
					training.space.Reset()
				}
			}

			fragmentSteps := 0
			for range training.pipeline.Next(fragment) {
				totalSteps++
				fragmentSteps++
			}

			if fragmentSteps > 0 && excursion != nil && excursion.Direction == "upward" && excursion.ClearsFriction {
				totalWins++
			}

			training.mu.Lock()
			if training.measurement != nil {
				held := training.measurement
				held.At = time.Now()
				if excursion != nil {
					held.Label = excursion.Symbol
				}
				held.Metrics["steps"] = held.Metrics["steps"].Write(float64(totalSteps))
				held.Metrics["decisions"] = held.Metrics["decisions"].Write(float64(totalSteps))
				held.Metrics["resolved"] = held.Metrics["resolved"].Write(float64(index + 1))

				if len(training.excursions) > 0 {
					held.Metrics["progress"] = held.Metrics["progress"].Write(float64(index+1) / float64(len(training.excursions)))
				}

				if totalSteps > 0 && index+1 > 0 {
					winRate := float64(totalWins) / float64(index+1)
					held.Metrics["win_rate"] = held.Metrics["win_rate"].Write(winRate)
					held.Metrics["edge"] = held.Metrics["edge"].Write((winRate - 0.5) * 100)
					held.Metrics["accuracy"] = held.Metrics["accuracy"].Write(winRate * 100)
				}
			}
			training.mu.Unlock()

			index++
		}

		training.precursor.SetExcursion(nil)
		if training.space != nil {
			training.space.Reset()
		}
		training.excursions = nil
	})
}
