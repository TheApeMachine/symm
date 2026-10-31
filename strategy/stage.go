package strategy

import (
	"context"
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/nomagique/runtime"
	nmstore "github.com/theapemachine/nomagique/store"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/signal"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/strategy/tokenizer"
	"golang.org/x/sync/errgroup"
)

// Stage runs grid observe and the per-symbol sequencer on each tick.
type Stage struct {
	*runtime.System
	grid       *nmstore.Grid
	desk       *broker.Desk
	client     *store.Client
	sequencers *sync.Map
	grammar    tokenizer.GrammarCatalog
	emit       *signal.Emit
	regions    *Regions
}

func NewStage(ctx context.Context, desk *broker.Desk) *Stage {
	return &Stage{
		System:     runtime.NewSystem(ctx, "stage"),
		grid:       nmstore.NewGrid(ctx),
		desk:       desk,
		sequencers: &sync.Map{},
		emit:       signal.NewEmit(),
		regions:    NewRegions(),
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

	stage.grammar.RestoreCatalog(ctx, stage.client, stage.RestoreSequencer)
}

func (stage *Stage) Sequencer(symbol string) *tokenizer.Sequencer {
	if stage == nil || symbol == "" {
		return tokenizer.NewSequencer()
	}

	loaded, _ := stage.sequencers.LoadOrStore(symbol, tokenizer.NewSequencer())
	return loaded.(*tokenizer.Sequencer)
}

func (stage *Stage) RestoreSequencer(symbol string, state tokenizer.SequencerState) {
	if stage == nil || symbol == "" {
		return
	}

	stage.Sequencer(symbol).Restore(state)
	stage.sequencers.Store(symbol, stage.Sequencer(symbol))
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

func (stage *Stage) bookStale(symbol string) bool {
	if stage == nil || stage.desk == nil {
		return false
	}

	return stage.desk.BookStale(symbol)
}

func (stage *Stage) Step(tick *data.Value) *data.Value {
	if stage == nil || tick == nil {
		return tick
	}

	bySymbol := NewSymbols(tick).WithPrice(stage.desk.Price()).GroupBySymbol(stage.bookStale)

	if len(bySymbol) == 0 {
		return tick
	}

	group, _ := errgroup.WithContext(stage.Context())

	for symbol, sources := range bySymbol {
		group.Go(func() error {
			symbol, sources := symbol, sources

			reading, err := stage.grid.Observe(symbol, sources)

			if err != nil {
				errnie.Warn("[stage.Step] grid.Observe failed", "symbol", symbol, "err", err)
				return stage.Error(err)
			}

			lit := reading.Lit()

			stage.regions.Publish(tick, symbol, lit)

			seq := stage.Sequencer(symbol)
			var cut tokenizer.Tokens

			if reading.HasToken() {
				cut = seq.Step(reading.TokenSymbol(), reading.TokenBrightness())
			}

			audit.Record.Write(map[string]any{
				"node":    "sequencer",
				"symbol":  symbol,
				"cut":     cut.String(),
				"cutLen":  cut.Len(),
				"regions": stage.litKeys(lit),
			})

			stage.emit.Metric(tick, cut, "sequencer", symbol, "cut")

			if cut.Len() > 0 && stage.client != nil {
				client := stage.client
				saved := seq.State()

				value := data.NewValue("model", "grammar", symbol)
				value.Append(saved, "state")

				if _, err := client.WriteAtPrefix(
					stage.grammar.ObjectKey(symbol), value.Bytes(),
				); err != nil {
					errnie.Error(errnie.Err(errnie.IO, "[sequencer] persist "+symbol, err))
				}
			}

			return nil
		})
	}

	if err := group.Wait(); err != nil {
		stage.Error(err)
	}

	return tick
}

func (stage *Stage) litKeys(regions []*nmstore.Region) []int {
	keys := make([]int, 0, len(regions))

	for _, region := range regions {
		if region == nil || region.Brightness <= 0 {
			continue
		}

		keys = append(keys, int(region.Key))
	}

	return keys
}
