package strategy

import (
	"context"
	"strings"
	"sync"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmstore "github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/strategy/tokenizer"
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

func (stage *Stage) Sequencer(symbol string) *tokenizer.Sequencer {
	if stage == nil || symbol == "" {
		return tokenizer.NewSequencer()
	}

	loaded, _ := stage.sequencers.LoadOrStore(symbol, tokenizer.NewSequencer())
	return loaded.(*tokenizer.Sequencer)
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

	for symbol, sources := range bySymbol {
		observed, err := stage.grid.Observe(
			symbol, sources, stage.desk.Has(symbol, true),
		)

		if err != nil {
			stage.Error(err)
			return tick
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
	}

	return tick
}
