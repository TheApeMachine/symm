package strategy

import (
	"context"
	"fmt"
	"sync"

	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/nomagique/store"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/strategy/tokenizer"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/ui"
)

type Training struct {
	*runtime.System
	price      *broker.Price
	desk       *broker.Desk
	grid       *store.Grid
	client     *store.Client
	sequencers *sync.Map
}

func NewTraining(
	ctx context.Context,
	price *broker.Price,
	desk *broker.Desk,
) *Training {
	return &Training{
		System:     runtime.NewSystem(ctx, "training", price),
		price:      price,
		desk:       desk,
		grid:       store.NewGrid(ctx),
		client:     store.NewClient(ctx, system.Cfg.Storage.S3.Bucket),
		sequencers: &sync.Map{},
	}
}

func (training *Training) Step(envelope *runtime.Envelope) *runtime.Envelope {
	if envelope == nil {
		return nil
	}

	for symbol, sources := range envelope.Metrics() {
		if symbol == "" || symbol == "aggregate" {
			continue
		}

		regions, err := training.grid.Observe(symbol, sources)

		if err != nil {
			training.Error(err)
			continue
		}

		envelope.Regions[symbol] = regions

		cut := training.stepCut(symbol, regions)
		envelope.Paths[symbol] = &cut
	}

	if frame := envelope.Publish("regions"); len(frame) > 0 {
		ui.UI.Write(frame)
	}

	if frame := envelope.Publish("paths"); len(frame) > 0 {
		ui.UI.Write(frame)
	}

	return envelope
}

func (training *Training) Sequencer(symbol string) *tokenizer.Sequencer {
	if training == nil || symbol == "" {
		return tokenizer.NewSequencer()
	}

	loaded, _ := training.sequencers.LoadOrStore(symbol, tokenizer.NewSequencer())
	return loaded.(*tokenizer.Sequencer)
}

func (training *Training) ResetSequencer(symbol string) {
	if training == nil || symbol == "" {
		return
	}

	loaded, ok := training.sequencers.Load(symbol)
	if !ok {
		return
	}

	loaded.(*tokenizer.Sequencer).Reset()
}

/*
stepCut feeds the brightest lit region into the symbol's sequencer and
returns the live path cut.
*/
func (training *Training) stepCut(symbol string, regions []*runtime.Region) tokenizer.Tokens {
	var best *runtime.Region

	for _, region := range regions {
		if region == nil || region.Brightness <= 0 {
			continue
		}

		if best == nil || region.Brightness > best.Brightness {
			best = region
		}
	}

	if best == nil {
		return tokenizer.Tokens{}
	}

	return training.Sequencer(symbol).Step(
		fmt.Sprintf("R%02d", best.Key),
		best.Brightness,
	)
}
