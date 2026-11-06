package strategy

import (
	"context"
	"errors"
	"sync"

	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/nomagique/store"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/strategy/tokenizer"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/ui"
)

/*
Training handles the grid observations, and sequence state management.
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
NewTraining initializes a new training engine.
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
Step is the main entry point for the training engine. It takes an envelope and
returns a new envelope. This makes it compatible with the LMAX Disruptor pipeline.
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
		wg.Add(1)

		system.Q.Submit(func() {
			defer wg.Done()

			regions, err := training.grid.Observe(symbol, sources)

			if err != nil {
				training.Error(err)
				return
			}

			training.lightBroker(symbol, regions)

			open := false

			for range training.desk.Positions(broker.TierTypePaper, symbol) {
				open = true
				break
			}

			training.grid.Emit(regions, open)

			cut := training.stepCut(symbol, regions)

			mu.Lock()
			envelope.Regions[symbol] = regions
			envelope.Paths[symbol] = cut
			mu.Unlock()
		})
	}

	wg.Wait()

	if payload := envelope.Publish("regions"); len(payload) > 0 {
		ui.UI.Write(payload)
	}

	if payload := envelope.Publish("paths"); len(payload) > 0 {
		ui.UI.Write(payload)
	}

	return envelope
}

/*
Sequencer returns the sequencer for the given symbol.
*/
func (training *Training) Sequencer(symbol string) *tokenizer.Sequencer {
	if symbol == "" {
		return tokenizer.NewSequencer()
	}

	loaded, _ := training.sequencers.LoadOrStore(symbol, tokenizer.NewSequencer())
	return loaded.(*tokenizer.Sequencer)
}

/*
restoreSequencer restores the sequencer for the given symbol and state.
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
ResetSequencer resets the sequencer for the given symbol.
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
lightBroker paints volume / day / spread / book / peak oracle sides before Emit.
*/
func (training *Training) lightBroker(symbol string, regions []*runtime.Region) {
	if len(regions) < store.RegionCount {
		return
	}

	price := training.price
	risk := system.Cfg.Risk

	training.shine(regions, 9, 10, price.VolumeUSD(symbol) >= risk.MinMoverVolumeUSD)
	training.shine(regions, 11, 12, price.ChangePct(symbol) >= risk.MinMoverChangePct)

	// MaxEntrySpreadPct <= 0 disables the spread oracle (lab zero = no limit).
	spreadOK := risk.MaxEntrySpreadPct <= 0
	if !spreadOK {
		if spread, ok := price.SpreadPct(symbol); ok && spread != nil {
			spreadOK = spread.InexactFloat64() <= risk.MaxEntrySpreadPct
		}
	}

	training.shine(regions, 13, 14, spreadOK)
	training.shine(regions, 15, 16, price.BookIntact(symbol))
	training.shine(regions, 23, 24, price.BelowPeak(symbol))
}

/*
shine paints the given region with the given brightness.
*/
func (training *Training) shine(regions []*runtime.Region, veto, approve uint8, ok bool) {
	if ok {
		store.Shine(regions[approve])
		zeroRegion(regions[veto])
		return
	}

	store.Shine(regions[veto])
	zeroRegion(regions[approve])
}

/*
zeroRegion zeros the given region.
*/
func zeroRegion(region *runtime.Region) {
	if region == nil {
		return
	}

	region.Brightness = 0
	region.Deformation = 0
	region.Counts = 0
}

/*
stepCut steps the sequencer for the given symbol and regions.
*/
func (training *Training) stepCut(symbol string, regions []*runtime.Region) *runtime.Tokens {
	token, brightness := store.PathToken(regions)

	if token == "" {
		cut := runtime.Tokens{}
		return &cut
	}

	seq := training.Sequencer(symbol)
	cut := seq.Step(token, brightness)

	if err := training.catalog.Persist(
		training.Context(), training.client, symbol, seq.State(),
	); err != nil {
		training.Error(err)
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

/*
Board is the Emit region set. Tradable means approve winners only — no veto.
*/
type Board []*runtime.Region

/*
Tradable returns true if the board is tradable.
*/
func (board Board) Tradable() bool {
	approved := false

	for _, region := range board {
		if region == nil || region.Brightness <= 0 {
			continue
		}

		key := int(region.Key)

		if key < 9 || key >= store.RegionCount {
			continue
		}

		if key&1 == 1 {
			return false
		}

		approved = true
	}

	return approved
}
