package strategy

import (
	"context"
	"strings"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmstore "github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

type Training struct {
	*runtime.System
	model     *Model
	price     *broker.Price
	desk      *broker.Desk
	client    *store.Client
	grid      *nmstore.Grid
	telemetry *Telemetry
}

func NewTraining(
	ctx context.Context,
	price *broker.Price,
	desk *broker.Desk,
) *Training {
	client := store.NewClient(ctx, system.Cfg.Storage.S3.Bucket)

	return &Training{
		System:    runtime.NewSystem(ctx, "training", price),
		model:     NewModel(ctx, desk, client),
		price:     price,
		desk:      desk,
		client:    client,
		grid:      nmstore.NewGrid(ctx),
		telemetry: NewTelemetry(ctx),
	}
}

func appendGridRegions(out *data.Value, lit []*nmstore.Region) {
	if out == nil || len(lit) == 0 {
		return
	}

	out.Append(lit, "grid", "regions")
	out.Append(lit, "train", "regions")
}

func (training *Training) Step(value *data.Value) *data.Value {
	if value == nil {
		return nil
	}

	bySymbol := training.bySymbol(value)

	if len(bySymbol) == 0 {
		training.telemetry.EndTick(training.model, training.desk)
		return value
	}

	// Grid priors and model path/desk are per-tick shared state: one symbol
	// at a time (strategy ring handler goroutine, same as Path).
	for symbol, sources := range bySymbol {
		out := data.NewValue(
			"training", "step", symbol,
		)

		observed, err := training.grid.Observe(
			symbol, sources, training.desk.Has(symbol, true),
		)

		if err != nil {
			training.Error(err)
			continue
		}

		training.model.Evaluate(out, observed)
		training.telemetry.NoteStep(out)
		training.telemetry.NoteWalk(symbol, out)
	}

	training.telemetry.PublishWalks()
	training.telemetry.EndTick(training.model, training.desk)
	return value
}

func (training *Training) bySymbol(value *data.Value) map[string]map[string]map[string]float64 {
	if value == nil {
		return nil
	}

	out := make(map[string]map[string]map[string]float64)

	for _, source := range types.SignalSourceStrings {
		for _, layer := range value.LookupPrefix(source) {
			parts := strings.Split(layer.Key, "/")

			if len(parts) < 4 {
				continue
			}

			symbol := parts[1] + "/" + parts[2]

			var values []float64

			if err := layer.Decode(&values); err != nil {
				continue
			}

			bySource := out[symbol]

			if bySource == nil {
				bySource = make(map[string]map[string]float64)
				out[symbol] = bySource
			}

			labels := bySource[source]

			if labels == nil {
				labels = make(map[string]float64)
				bySource[source] = labels
			}

			for i, label := range parts[3:] {
				if i >= len(values) {
					break
				}

				labels[label] = values[i]
			}
		}
	}

	return out
}
