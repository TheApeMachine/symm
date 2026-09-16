package strategy

import (
	"context"
	"github.com/theapemachine/errnie"

	"unsafe"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/strategy/impulse"
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
Training composes the live owner-addressed impulse map with precursor encoding
and one shared radix trie. Complete historical tapes train the trie through
an independent map; the live path only evaluates current regions.
Training does not submit orders: quoted outcome evidence is not execution proof.
*/
type Training struct {
	*runtime.System
	measurement *data.Measurement[float64]
	precursor   *Precursor
	pipeline    *nomagique.Number
	engine      *cognition.Engine
	Rehearsal   *Rehearsal
	space       *impulse.Map
	sequence    int64
}

func NewTraining(ctx context.Context, epoch int64, price *broker.Price) *Training {
	training := &Training{
		precursor: NewPrecursor(), space: impulse.NewMap(),
		engine: cognition.NewEngine(cognition.Config{MemoryScale: 1}),
	}
	training.Rehearsal = NewRehearsal(ctx, epoch, price, training.engine)
	training.pipeline = nomagique.NewNumber(training.space, training.precursor, training.engine)
	training.Register()
	training.System = runtime.NewSystem(ctx, "training")
	return training
}

/*
Register initializes and returns the canonical training telemetry measurement
populated with all metrics required by the frontend dashboard.
*/
func (training *Training) Register() *data.Measurement[float64] {
	if training.measurement != nil {
		return training.measurement
	}

	training.measurement = data.NewMeasurement("training", map[string]data.Metric[float64]{
		"evaluated":       {Label: "evaluated"},
		"unsupported":     {Label: "unsupported"},
		"previous_input":  {Label: "previous_input"},
		"invalid_inputs":  {Label: "invalid_inputs"},
		"impulse_version": {Label: "impulse_version", Raw: grid.FormatVersion},
		"input_count":     data.NewMetric[float64]("input_count", data.UnitCount, data.TimescaleInstantaneous, 0, 0),
		"steps":           data.NewMetric[float64]("steps", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"decisions":       data.NewMetric[float64]("decisions", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"resolved":        data.NewMetric[float64]("resolved", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"confidence":      data.NewMetric[float64]("confidence", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"contrast":        data.NewMetric[float64]("contrast", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"surprisal":       data.NewMetric[float64]("surprisal", data.UnitNat, data.TimescaleInstantaneous, 0, 1),
		"ambiguity":       data.NewMetric[float64]("ambiguity", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"action":          data.NewMetric[float64]("action", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"win_rate":        data.NewMetric[float64]("win_rate", data.UnitPercent, data.TimescaleInstantaneous, 0, 1),
		"edge":            data.NewMetric[float64]("edge", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"progress":        data.NewMetric[float64]("progress", data.UnitPercent, data.TimescaleInstantaneous, 0, 1),
		"accuracy":        data.NewMetric[float64]("accuracy", data.UnitPercent, data.TimescaleInstantaneous, 0, 1),
		"support":         data.NewMetric[float64]("support", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"quality":         data.NewMetric[float64]("quality", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
	})

	training.measurement.Label = "learner"
	training.measurement.Metadata["peer-interest"] = "*"

	return training.measurement
}

/* Step evaluates the current owner-held metric publications in sequence order. */
func (training *Training) Step(measurement *data.Measurement[float64]) *data.Measurement[float64] {
	if training.Status() != runtime.READY {
		errnie.Warn(training.Name() + ": Step called before READY; dropping event")
		return measurement
	}

	if measurement == nil {
		return nil
	}

	current := measurement

	if measurement.Source != "training" {
		current = training.measurement
	}

	if reading := training.Rehearsal.published.Load(); reading != nil {
		current.Metrics["decisions"] = current.Metrics["decisions"].Write(float64(reading.Learned))
		current.Metrics["resolved"] = current.Metrics["resolved"].Write(float64(reading.Resolved))
		current.Metrics["unsupported"] = current.Metrics["unsupported"].Write(float64(reading.Unsupported))
		current.Metrics["evaluated"] = current.Metrics["evaluated"].Write(float64(reading.Predicted))
		if reading.Entered > 0 {
			current.Metrics["edge"] = current.Metrics["edge"].Write(reading.Return / float64(reading.Entered))
			current.Metrics["win_rate"] = current.Metrics["win_rate"].Write(float64(reading.Profitable) / float64(reading.Entered))
		}
		if reading.Predicted > 0 {
			current.Metrics["accuracy"] = current.Metrics["accuracy"].Write(float64(reading.Correct) / float64(reading.Predicted))
		}
	}

	current.Metrics["previous_input"] = current.Metrics["previous_input"].Write(float64(training.sequence))
	current.Metrics["input_count"] = current.Metrics["input_count"].Write(float64(len(measurement.Peers)))
	input := func(yield func(unsafe.Pointer) bool) { yield(unsafe.Pointer(measurement)) }

	for output := range training.pipeline.Next(input) {
		evaluation := *(*cognition.Evaluation)(output)
		current.Label = training.space.Current.Symbol
		current.Result = training.space.Current
		current.SeqIdx = measurement.SeqIdx
		current.At = training.space.Current.At
		current.Metrics["steps"] = current.Metrics["steps"].Write(current.Metrics["steps"].Raw + 1)
		current.Metrics["support"] = current.Metrics["support"].Write(float64(evaluation.Support))
		current.Metrics["confidence"] = current.Metrics["confidence"].Write(evaluation.Confidence)
		current.Metrics["contrast"] = current.Metrics["contrast"].Write(evaluation.Contrast)
		current.Metrics["surprisal"] = current.Metrics["surprisal"].Write(evaluation.Surprisal)
		current.Metrics["ambiguity"] = current.Metrics["ambiguity"].Write(evaluation.Ambiguity)
		action := Action(evaluation.WinnerClass)
		actionValue := 0.0

		if action == ActionEnter {
			actionValue = 1
		}

		if action == ActionExit {
			actionValue = 2
		}

		current.Metrics["action"] = current.Metrics["action"].Write(actionValue)

	}

	if err := training.pipeline.Error(); err != nil {
		current.Err = err
		training.Error(err)
		training.Transition(runtime.ERROR)
	}

	current.Metrics["invalid_inputs"] = current.Metrics["invalid_inputs"].Write(float64(training.space.Invalid))
	training.sequence = measurement.SeqIdx
	training.measurement = current
	return current
}
