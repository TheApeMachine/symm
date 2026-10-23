package strategy

import (
	"context"
	"io"
	"slices"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmstore "github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/strategy/tokenizer"
)

type Training struct {
	*runtime.System
	cfg     *Config
	model   *Model
	price   *broker.Price
	desk    Desk
	client  *store.Client
	grid    *nmstore.Grid
	hub     io.Writer
	metrics *metricsAccumulator
}

func NewTraining(
	ctx context.Context,
	price *broker.Price,
	desk *broker.Desk,
	client *store.Client,
	hub io.Writer,
) *Training {
	cfg := LoadConfig()
	metrics := newMetricsAccumulator()

	training := &Training{
		System:  runtime.NewSystem(ctx, "training", price),
		cfg:     cfg,
		model:   NewModel(ctx, desk, client, hub, cfg),
		price:   price,
		desk:    desk,
		client:  client,
		grid:    nmstore.NewGrid(ctx),
		hub:     hub,
		metrics: metrics,
	}

	training.model.BindMetrics(metrics)

	return training
}

func (training *Training) PublishPolicy() {
	if training == nil || training.model == nil {
		return
	}

	training.model.PublishPolicy()
}

func (training *Training) Step(value *data.Value) *data.Value {
	if value == nil {
		return nil
	}

	observed, err := training.grid.Observe(value)
	if err != nil {
		training.Error(err)
		return nil
	}

	if len(observed) == 0 {
		return value
	}

	bySymbol := make(map[string][]tokenizer.Token)
	regionsBySymbol := make(map[string][]*nmstore.Region)

	for _, region := range observed {
		token := tokenizer.NewTokenFromRegion(region.Key, region.Brightness)
		bySymbol[region.Symbol] = append(bySymbol[region.Symbol], token)
		regionsBySymbol[region.Symbol] = append(regionsBySymbol[region.Symbol], region)
	}

	symbols := make([]string, 0, len(bySymbol))
	for symbol := range bySymbol {
		symbols = append(symbols, symbol)
	}

	slices.Sort(symbols)

	training.metrics.reset()

	for _, symbol := range symbols {
		training.model.NoteTick(symbol, regionsBySymbol[symbol], training.grid)

		regime := training.model.Ingest(symbol, bySymbol[symbol])
		training.model.TrackSequitur(symbol, regime)

		chord := tokenizer.ChordToken(bySymbol[symbol])
		if len(chord.Symbols) == 0 {
			training.model.Touch(symbol)
			continue
		}

		training.model.Evaluate(symbol, chord)
	}

	value.Append(observed, "train", "regions")

	for _, cv := range training.desk.ClosedVirtuals() {
		training.model.Settle(cv)
	}

	training.metrics.publish(training.hub)

	return value
}
