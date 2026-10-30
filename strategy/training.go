package strategy

import (
	"context"
	"slices"
	"strings"

	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/nomagique/runtime"
	nmstore "github.com/theapemachine/nomagique/store"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/strategy/tokenizer"
	"github.com/theapemachine/symm/system"
	"golang.org/x/sync/errgroup"
)

type Training struct {
	*runtime.System
	model     *Model
	price     *broker.Price
	desk      *broker.Desk
	client    *store.Client
	telemetry *Telemetry
}

func NewTraining(
	ctx context.Context,
	price *broker.Price,
	desk *broker.Desk,
) *Training {
	return &Training{
		System:    runtime.NewSystem(ctx, "training", price),
		model:     NewModel(ctx, desk, price),
		price:     price,
		desk:      desk,
		client:    store.NewClient(ctx, system.Cfg.Storage.S3.Bucket),
		telemetry: NewTelemetry(ctx),
	}
}

func (training *Training) Step(value *data.Value) *data.Value {
	if value == nil {
		return nil
	}

	symbols := NewSymbols(value).Names()

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

			var phase string
			value.Lookup(&phase, strings.Join([]string{"stage", symbol, "phase"}, "/"))
			out.Append(phase, "phase")

			cut := data.Get[tokenizer.Tokens](
				value, strings.Join([]string{"stage", symbol, "cut"}, "/"),
			)

			if cut.Len() > 0 {
				out.Append(cut.String(), "livePath")
			}

			training.telemetry.CompleteSymbolStep(out, training.model)

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
