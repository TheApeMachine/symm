package strategy

import (
	"context"

	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/data"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
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
Training reads the impulse map and addresses one shared radix trie. The inference
pipeline compares the two observed action counts. Ties select wait;
unseen contexts produce no action. The cold learner owns trie writes.
Training does not submit orders: quoted outcome evidence is not execution proof.
*/
type Training struct {
	*runtime.System
	measurement *data.Measurement[float64]
	precursor   *Precursor
	pipeline    *nomagique.Number
	model       *store.Radix[float64]
	keys        [2][]byte
	Rehearsal   *Rehearsal
	Grid        *impulse.Map
	sequence    int64
}

func NewTraining(ctx context.Context, epoch int64, price *broker.Price) *Training {
	training := &Training{
		precursor: NewPrecursor(), Grid: impulse.NewMap(),
		model: store.NewRadix[float64](),
	}

	training.Rehearsal = NewRehearsal(ctx, epoch, price, training.model)

	training.pipeline = nomagique.NewNumber(
		store.NewKeyQuery[float64](
			&training.keys[1],
			data.ActionRead,
		),
		training.model, sequence.
			NewZip2[float64](nomagique.NewNumber(
			store.NewKeyQuery[float64](
				&training.keys[0],
				data.ActionRead,
			),
			training.model,
		).Next(nil)), logic.NewGate(
			nomagique.NewNumber(
				arithmetic.NewAdd(), sequence.
					NewZip2[float64](sequence.
					NewValues(0.0).Next(nil),
				), logic.NewGreater(),
			),
			logic.NewGreater(),
			transport.NewDiscard(),
		),
	)

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
			current.Metrics["edge"] = data.Metric[float64]{Label: "edge"}.Write(reading.Return / float64(reading.Entered))
			current.Metrics["win_rate"] = data.Metric[float64]{Label: "win_rate"}.Write(float64(reading.Profitable) / float64(reading.Entered))
		}
		if reading.Predicted > 0 {
			current.Metrics["accuracy"] = data.Metric[float64]{Label: "accuracy"}.Write(float64(reading.Correct) / float64(reading.Predicted))
		}
	}

	current.Metrics["previous_input"] = current.Metrics["previous_input"].Write(float64(training.sequence))
	current.Metrics["input_count"] = current.Metrics["input_count"].Write(float64(len(measurement.Peers)))
	if err := training.Grid.Step(measurement); err != nil {
		current.Err = err
		training.Error(err)
		training.Transition(runtime.ERROR)
		return current
	}

	delete(current.Metrics, "action")

	for _, market := range training.Grid.Active {
		if market.Sequence != measurement.SeqIdx {
			continue
		}
		current.Label = market.Symbol
		current.SeqIdx, current.At = measurement.SeqIdx, market.At
		current.Metrics["steps"] = current.Metrics["steps"].Write(current.Metrics["steps"].Raw + 1)
		delete(current.Metrics, "action")
		key := training.precursor.Encode(&market.Impulse)
		if len(key) == 0 {
			continue
		}
		training.keys[0] = append(training.keys[0][:0], key...)
		training.keys[0] = append(training.keys[0], 0)
		training.keys[1] = append(training.keys[1][:0], key...)
		training.keys[1] = append(training.keys[1], 1)
		for output := range training.pipeline.Next(nil) {
			action := 0.0
			if *(*bool)(output) {
				action = 1
			}
			current.Metrics["action"] = data.Metric[float64]{Label: "action", Raw: action}
		}
	}

	if err := training.pipeline.Error(); err != nil {
		current.Err = err
		training.Error(err)
		training.Transition(runtime.ERROR)
	}

	current.Metrics["invalid_inputs"] = current.Metrics["invalid_inputs"].Write(float64(training.Grid.Invalid))
	training.sequence = measurement.SeqIdx
	training.measurement = current
	return current
}
