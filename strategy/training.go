package strategy

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/nomagique/store"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/pipeline"
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

			gridStarted := time.Now()
			regions, err := training.grid.Observe(
				symbol,
				sources,
				training.desk.Has(symbol),
				training.price.MoverVolumeOK(symbol),
				training.price.SessionMoveOK(symbol),
				spreadOK,
				training.price.BookIntact(symbol),
				training.price.EntryPeakOK(symbol, hot),
			)
			pipeline.Time("grid", time.Since(gridStarted))

			if err != nil {
				training.Error(err)
				return
			}

			cutStarted := time.Now()
			cut := training.stepCut(symbol, regions)
			pipeline.Time("sequencer", time.Since(cutStarted))

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
stepCut feeds Sequencer.Step from the phase net (detector + support − opposition).
Opposition keys R02/R05/R08 are not path tokens; when they dominate, the cut is empty.
*/
func (training *Training) stepCut(symbol string, regions []*runtime.Region) *runtime.Tokens {
	var detector, support, opposition *runtime.Region

	for _, region := range regions {
		if region == nil || region.Key > 8 {
			continue
		}

		switch store.Catalog.ByKey(region.Key).Side {
		case "detector":
			detector = region
		case "support":
			support = region
		case "opposition":
			opposition = region
		}
	}

	bright := func(region *runtime.Region) float64 {
		if region == nil {
			return 0
		}

		return region.Brightness
	}

	if bright(detector)+bright(support)-bright(opposition) <= 0 {
		cut := runtime.Tokens{}
		return &cut
	}

	action := detector

	if bright(support) > bright(action) {
		action = support
	}

	if action == nil || action.Brightness <= 0 {
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
