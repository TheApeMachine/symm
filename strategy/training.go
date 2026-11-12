package strategy

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/nomagique/store"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/strategy/tokenizer"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/ui"
)

/*
Training turns each tick's observe metrics into per-symbol region vectors and
path cuts on the envelope — Model consumes Paths/Regions but does not own grammar.
*/
type Training struct {
	*runtime.System
	price      *broker.Price
	desk       *broker.Desk
	grid       *store.Grid
	client     *store.Client
	catalog    *tokenizer.GrammarCatalog
	sequencers *sync.Map
}

/*
NewTraining restores per-symbol sequencers from object storage at boot so path
cuts survive restarts before the first grid tick mutates grammar.
*/
func NewTraining(
	ctx context.Context,
	price *broker.Price,
	desk *broker.Desk,
	client *store.Client,
) *Training {
	training := &Training{
		System:     runtime.NewSystem(ctx, "training", price),
		price:      price,
		desk:       desk,
		grid:       store.NewGrid(ctx),
		client:     client,
		catalog:    tokenizer.NewGrammarCatalog(),
		sequencers: &sync.Map{},
	}

	training.catalog.RestoreCatalog(
		ctx, training.client, training.restoreSequencer,
	)

	return training
}

/*
Step fans Observe per symbol with desk/book gates wired in, then writes Regions
and Paths — downstream Model never calls the grid directly on the hot path.
*/
func (training *Training) Step(envelope *runtime.Envelope) *runtime.Envelope {
	if envelope == nil {
		return nil
	}

	metrics := envelope.Metrics()
	var mu sync.Mutex
	var wg sync.WaitGroup

	for symbol, sources := range metrics {
		if symbol == "" || symbol == "aggregate" {
			continue
		}

		symbol, sources := symbol, sources
		wg.Go(func() {
			risk := system.Cfg.Risk
			spreadOK := risk.MaxEntrySpreadPct <= 0

			if !spreadOK {
				if spread, ok := training.price.SpreadPct(symbol); ok && spread != nil {
					spreadOK = spread.InexactFloat64() <= risk.MaxEntrySpreadPct
				}
			}

			hot := ignitionHot(envelope.Pumpdump[symbol])

			regions, err := training.grid.Observe(
				symbol,
				sources,
				training.desk.Has(symbol),
				training.price.MoverVolumeOK(symbol),
				training.price.ChangePct(symbol) >= risk.MinMoverChangePct,
				spreadOK,
				training.price.BookIntact(symbol),
				training.price.EntryPeakOK(symbol, hot),
			)

			if err != nil {
				training.Error(err)
				return
			}

			cut := training.stepCut(symbol, regions)

			mu.Lock()
			envelope.Regions[symbol] = regions
			envelope.Paths[symbol] = cut
			mu.Unlock()
		})
	}

	wg.Wait()

	if ui.UI != nil && ui.UI.Active() && types.WantsGrid() {
		if payload := envelope.Publish("regions"); len(payload) > 0 {
			ui.UI.Write(payload)
		}

		if payload := envelope.Publish("paths"); len(payload) > 0 {
			ui.UI.Write(payload)
		}
	}

	return envelope
}

/*
Sequencer keeps one grammar stream per symbol so BTC path learning does not
cross-contaminate ETH regime sequences on the same envelope tick.
*/
func (training *Training) Sequencer(symbol string) *tokenizer.Sequencer {
	if symbol == "" {
		return tokenizer.NewSequencer()
	}

	loaded, _ := training.sequencers.LoadOrStore(symbol, tokenizer.NewSequencer())
	return loaded.(*tokenizer.Sequencer)
}

/*
restoreSequencer is the GrammarCatalog callback — replays persisted root/rules/MDL
without running Sequitur discovery so boot matches last shutdown cut.
*/
func (training *Training) restoreSequencer(symbol string, state tokenizer.SequencerState) {
	if symbol == "" {
		return
	}

	seq := tokenizer.NewSequencer()
	seq.Restore(state)
	training.sequencers.Store(symbol, seq)
}

/*
ResetSequencer clears grammar after a model resetPath — persists immediately so
a crash after reset does not resurrect the old sentence on next boot.
*/
func (training *Training) ResetSequencer(symbol string) {
	loaded, ok := training.sequencers.Load(symbol)

	if !ok {
		return
	}

	seq := loaded.(*tokenizer.Sequencer)
	seq.Reset()

	if err := training.catalog.Persist(
		training.Context(), training.client, symbol, seq.State(),
	); err != nil {
		training.Error(err)
	}
}

/*
stepCut picks the winning action region (keys ≤8) and feeds its brightness into
Sequencer.Step — only action regions advance the path, not veto/metric tiles.
*/
func (training *Training) stepCut(symbol string, regions []*runtime.Region) *runtime.Tokens {
	var action *runtime.Region

	for _, region := range regions {
		if region == nil || region.Brightness <= 0 || region.Key > 8 {
			continue
		}

		if action == nil || region.Brightness > action.Brightness {
			action = region
		}
	}

	if action == nil {
		cut := runtime.Tokens{}
		return &cut
	}

	seq := training.Sequencer(symbol)
	cut, mutated := seq.Step(fmt.Sprintf("R%02d", action.Key), action.Brightness)

	if mutated {
		// Persist synchronously: WriteAtPrefix only enqueues, and a goroutine
		// per mutation could let an older snapshot land after a newer one or
		// after Flush.
		if err := training.catalog.Persist(
			training.Context(), training.client, symbol, seq.State(),
		); err != nil {
			training.Error(err)
		}
	}

	return &cut
}

/*
Flush persists every sequencer so a shutdown does not lose grammar state.
*/
func (training *Training) Flush() error {
	var errs error

	training.sequencers.Range(func(key, value any) bool {
		symbol, ok := key.(string)

		if !ok || symbol == "" {
			return true
		}

		seq, ok := value.(*tokenizer.Sequencer)

		if !ok || seq == nil {
			return true
		}

		if err := training.catalog.Persist(
			training.Context(), training.client, symbol, seq.State(),
		); err != nil {
			errs = errors.Join(errs, err)
		}

		return true
	})

	return errs
}
