package strategy

import (
	"context"
	"strings"
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/nomagique/runtime"
	nmstore "github.com/theapemachine/nomagique/store"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
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
	grammar    tokenizer.GrammarCatalog
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

func (stage *Stage) PersistSequencer(ctx context.Context, symbol string) {
	if stage == nil || stage.client == nil || symbol == "" {
		return
	}

	seq := stage.Sequencer(symbol)
	value := data.NewValue("model", "grammar", symbol)
	value.Append(seq.State(), "state")

	if _, err := stage.client.WriteAtPrefix(stage.grammar.ObjectKey(symbol), value.Bytes()); err != nil {
		errnie.Error(errnie.Err(errnie.IO, "[stage] persist grammar "+symbol, err))
	}
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

func (stage *Stage) isBookStale(symbol string) bool {
	if stage == nil || stage.desk == nil {
		return false
	}
	return stage.desk.BookStale(symbol)
}

func (stage *Stage) Step(tick *data.Value) *data.Value {
	if stage == nil || tick == nil {
		return tick
	}

	bySymbol := NewSymbols(tick).GroupBySymbol(stage.isBookStale)

	if len(bySymbol) == 0 {
		return tick
	}

	group, _ := errgroup.WithContext(stage.Context())

	for symbol, sources := range bySymbol {
		group.Go(func() error {
			holding := stage.desk.Has(symbol)

			observed, err := stage.grid.Observe(symbol, sources)

			if err != nil {
				errnie.Warn("[stage.Step] grid.Observe failed", "symbol", symbol, "err", err)
				return stage.Error(err)
			}

			tick.Append(observed, "stage", symbol, "regions")

			if parts := strings.Split(symbol, "/"); len(parts) == 2 {
				tick.Append(observed, append([]string{"train", "regions"}, parts...)...)
			}

			seq := stage.Sequencer(symbol)
			cut := stage.stepCut(seq, observed)

			phase := "enter"
			if holding {
				phase = "hold"
			}

			audit.Record.Write(map[string]any{
				"stage":   "sequencer",
				"symbol":  symbol,
				"phase":   phase,
				"cut":     cut.String(),
				"cutLen":  cut.Len(),
				"regions": litKeys(observed),
			})

			tick.Append(cut, "stage", symbol, "cut")
			tick.Append(phase, "stage", symbol, "phase")

			if cut.Len() > 0 && stage.client != nil {
				client := stage.client
				saved := seq.State()

				go func() {
					value := data.NewValue("model", "grammar", symbol)
					value.Append(saved, "state")
					if _, err := client.WriteAtPrefix(stage.grammar.ObjectKey(symbol), value.Bytes()); err != nil {
						errnie.Error(errnie.Err(errnie.IO, "[sequencer] persist "+symbol, err))
					}
				}()
			}

			return nil
		})
	}

	if err := group.Wait(); err != nil {
		stage.Error(err)
	}

	return tick
}

func (stage *Stage) stepCut(seq *tokenizer.Sequencer, lit []*nmstore.Region) tokenizer.Tokens {
	var best *nmstore.Region

	for _, region := range lit {
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

	return seq.Step(tokenizer.RegionLabel(uint8(best.Key)), best.Brightness)
}

func litKeys(regions []*nmstore.Region) []int {
	keys := make([]int, 0, len(regions))
	for _, region := range regions {
		if region == nil || region.Brightness <= 0 {
			continue
		}
		keys = append(keys, int(region.Key))
	}
	return keys
}
