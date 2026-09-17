package ui

import (
	"iter"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/types"
)

/*
TrainingPublisher is a non-blocking primitive off-ramp that maps cognition.Evaluation
records into telemetry data.Measurement[float64] instances and pushes them to UITee and StoreTee.
*/
type TrainingPublisher struct {
	*core.PrimitiveError
	uiTee    *UITee
	storeTee *hindsight.StoreTee
	symbol   string
}

func NewTrainingPublisher(
	uiTee *UITee,
	storeTee *hindsight.StoreTee,
	symbol string,
) *TrainingPublisher {
	defaultSymbol := symbol

	if defaultSymbol == "" {
		defaultSymbol = "BTC/USD"
	}

	return &TrainingPublisher{
		PrimitiveError: core.NewPrimitiveError(),
		uiTee:          uiTee,
		storeTee:       storeTee,
		symbol:         defaultSymbol,
	}
}

func (publisher *TrainingPublisher) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				continue
			}

			evaluation := (*cognition.Evaluation)(arriving)

			if evaluation == nil {
				continue
			}

			actionValue := 0.0

			switch evaluation.WinnerClass {
			case "enter":
				actionValue = 1.0
			case "exit":
				actionValue = 2.0
			case "retreat":
				actionValue = 3.0
			default:
				actionValue = 0.0
			}

			symbol := types.Focus()

			if symbol == "" {
				symbol = publisher.symbol
			}

			measurement := data.NewMeasurement[float64]("training", map[string]data.Metric[float64]{
				"confidence": {Label: "confidence", Raw: evaluation.Confidence},
				"contrast":   {Label: "contrast", Raw: evaluation.Contrast},
				"surprisal":  {Label: "surprisal", Raw: evaluation.Surprisal},
				"ambiguity":  {Label: "ambiguity", Raw: evaluation.Ambiguity},
				"support":    {Label: "support", Raw: float64(evaluation.Support)},
				"steps":      {Label: "steps", Raw: float64(evaluation.Step)},
				"action":     {Label: "action", Raw: actionValue},
				"edge":       {Label: "edge", Raw: evaluation.Contrast},
			})

			now := time.Now()
			measurement.Label = symbol
			measurement.At = now
			measurement.Timestamp = now.UnixNano()
			measurement.SeqIdx = int64(evaluation.Step)
			measurement.Provenance["owner"] = "training"

			if publisher.uiTee != nil {
				publisher.uiTee.Push(measurement)
			}

			if publisher.storeTee != nil {
				publisher.storeTee.Push(measurement)
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
