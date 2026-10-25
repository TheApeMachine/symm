package strategy

import (
	"context"
	"io"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/ui"
)

type Training struct {
	*runtime.System
	model     *Model
	price     *broker.Price
	desk      *broker.Desk
	client    *store.Client
	stage     *Stage
	telemetry *Telemetry
}

func (training *Training) Model() *Model {
	if training == nil {
		return nil
	}
	return training.model
}

func NewTraining(
	ctx context.Context,
	price *broker.Price,
	desk *broker.Desk,
	stage *Stage,
) *Training {
	client := store.NewClient(ctx, system.Cfg.Storage.S3.Bucket)

	return &Training{
		System:    runtime.NewSystem(ctx, "training", price),
		model:     NewModel(ctx, desk, client, stage),
		price:     price,
		desk:      desk,
		client:    client,
		stage:     stage,
		telemetry: NewTelemetry(ctx),
	}
}

func (training *Training) Step(value *data.Value) *data.Value {
	if value == nil {
		return nil
	}

	symbols := SymbolsOnTick(value)

	if len(symbols) == 0 {
		training.telemetry.EndTick(training.model, training.desk)
		return value
	}

	for _, symbol := range symbols {
		out := data.NewValue(
			"training", "step", symbol,
		)
		out.Epoch, out.Tick, out.Timestamp = value.Epoch, value.Tick, value.Timestamp

		training.model.Decide(out, value)
		training.telemetry.CompleteSymbolStep(
			out,
			training.stage.Sequencer(symbol),
			training.model,
		)
		training.telemetry.NoteWalk(symbol, out)
	}

	training.telemetry.PublishWalks()
	training.telemetry.EndTick(training.model, training.desk)

	if ui.UI != nil && ui.UI.Active() {
		io.Copy(ui.UI, value)
	}

	return value
}
