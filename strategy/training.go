package strategy

import (
	"context"
	"slices"
	"strings"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmstore "github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/system"
	"golang.org/x/sync/errgroup"
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

func (training *Training) Store() *store.Client {
	if training == nil {
		return nil
	}
	return training.client
}

func NewTraining(
	ctx context.Context,
	price *broker.Price,
	desk *broker.Desk,
	stage *Stage,
) *Training {
	client := store.NewClient(ctx, system.Cfg.Storage.S3.Bucket)
	if stage != nil {
		stage.AttachStore(client)
		stage.RestoreSequencers(ctx)
	}

	return &Training{
		System:    runtime.NewSystem(ctx, "training", price),
		model:     NewModel(ctx, desk, stage),
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

	for _, symbol := range training.desk.Pending() {
		if !slices.Contains(symbols, symbol) {
			symbols = append(symbols, symbol)
		}
	}

	slices.Sort(symbols)

	if len(symbols) == 0 {
		training.telemetry.EndTick(training.model, training.desk)
		return value
	}

	group, _ := errgroup.WithContext(training.Context())

	for _, symbol := range symbols {
		group.Go(func() error {
			out := data.NewValue(
				"training", "step", symbol,
			)
			
			out.Epoch, out.Tick, out.Timestamp = value.Epoch, value.Tick, value.Timestamp

			var observed []*nmstore.Region
			value.Lookup(&observed, strings.Join([]string{"stage", symbol, "regions"}, "/"))

			training.model.Evaluate(out, observed, value)

			training.telemetry.CompleteSymbolStep(
				out,
				training.stage.Sequencer(symbol),
				training.model,
			)

			training.telemetry.NoteWalk(symbol, out)

			return nil
		})
	}

	if err := group.Wait(); err != nil {
		training.Error(err)
	}

	training.telemetry.PublishWalks()
	training.telemetry.EndTick(training.model, training.desk)

	return value
}
