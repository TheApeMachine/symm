package strategy

import (
	"context"
	"strings"
	"sync"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmstore "github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/strategy/tokenizer"
	"golang.org/x/sync/errgroup"
)

/*
Stage is the grammar fork: grid observation and sequencer tokens for each
symbol on the tick. It appends only under stage/<symbol>/… on the shared
workspace value; it never touches the desk.
*/
type Stage struct {
	*runtime.System
	grid       *nmstore.Grid
	desk       *broker.Desk
	client     *store.Client
	sequencers *sync.Map
}

func NewStage(ctx context.Context, desk *broker.Desk) *Stage {
	return &Stage{
		System:     runtime.NewSystem(ctx, "stage"),
		grid:       nmstore.NewGrid(ctx),
		desk:       desk,
		sequencers: &sync.Map{},
	}
}

func (stage *Stage) AttachStore(client *store.Client) {
	if stage == nil {
		return
	}
	stage.client = client
}

func (stage *Stage) RestoreSequencers(ctx context.Context) {
	if stage == nil || stage.client == nil {
		return
	}

	tokenizer.RestoreCatalog(ctx, stage.client, stage.RestoreSequencer)
}

func (stage *Stage) Sequencer(symbol string) *tokenizer.Sequencer {
	if stage == nil || symbol == "" {
		return tokenizer.NewSequencer()
	}

	loaded, _ := stage.sequencers.LoadOrStore(symbol, tokenizer.NewSequencer())
	return loaded.(*tokenizer.Sequencer)
}

func (stage *Stage) RestoreSequencer(symbol string, state tokenizer.SymbolState) {
	if stage == nil || symbol == "" {
		return
	}

	sequencer := stage.Sequencer(symbol)
	sequencer.Restore(state)
	stage.sequencers.Store(symbol, sequencer)
}

func (stage *Stage) PersistSequencer(ctx context.Context, symbol string) {
	if stage == nil || stage.client == nil || symbol == "" {
		return
	}
	stage.Sequencer(symbol).Persist(ctx, stage.client, symbol)
}

func (stage *Stage) ResetSequencer(symbol string) {
	if stage == nil || symbol == "" {
		return
	}

	loaded, ok := stage.sequencers.Load(symbol)
	if !ok {
		return
	}

	loaded.(*tokenizer.Sequencer).Reset()
}

func (stage *Stage) Step(tick *data.Value) *data.Value {
	if stage == nil || tick == nil {
		return tick
	}

	bySymbol := SignalSourcesBySymbol(tick)

	if len(bySymbol) == 0 {
		return tick
	}

	group, ctx := errgroup.WithContext(stage.Context())

	for symbol, sources := range bySymbol {
		group.Go(func() error {
			observed, err := stage.grid.Observe(
				symbol, sources, stage.desk.Has(symbol, true),
			)

			if err != nil {
				return stage.Error(err)
			}

			tick.Append(observed, "stage", symbol, "regions")

			if parts := strings.Split(symbol, "/"); len(parts) == 2 {
				tick.Append(observed, append([]string{"train", "regions"}, parts...)...)
			}

			holdingPaper := stage.desk.Has(symbol, false)
			holdingVirtual := stage.desk.Has(symbol, true)
			holding := holdingPaper || holdingVirtual

			sequencer := stage.Sequencer(symbol)
			step := sequencer.Step(observed, holding)

			tick.Append(step, "stage", symbol, "step")
			tick.Append(step.Token, "stage", symbol, "cut")

			if len(step.Spell) > 0 && stage.client != nil {
				sequencer.Persist(ctx, stage.client, symbol)
			}

			return nil
		})
	}

	if err := group.Wait(); err != nil {
		stage.Error(err)
	}

	return tick
}
