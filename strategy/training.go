package strategy

import (
	"context"
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
	catalog    *tokenizer.GrammarCatalog
	sequencers *sync.Map
}

func NewTraining(
	ctx context.Context,
	price *broker.Price,
	desk *broker.Desk,
) *Training {
	training := &Training{
		System:     runtime.NewSystem(ctx, "training", price),
		price:      price,
		desk:       desk,
		grid:       store.NewGrid(ctx),
		client:     store.NewClient(ctx, system.Cfg.Storage.S3.Bucket),
		catalog:    &tokenizer.GrammarCatalog{},
		sequencers: &sync.Map{},
	}

	training.catalog.RestoreCatalog(ctx, training.client, training.restoreSequencer)

	return training
}

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

		task := func() {
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
		}

		if system.Q.Submit(task) != nil {
			task()
		}
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

func (training *Training) Sequencer(symbol string) *tokenizer.Sequencer {
	if symbol == "" {
		return tokenizer.NewSequencer()
	}

	loaded, _ := training.sequencers.LoadOrStore(symbol, tokenizer.NewSequencer())
	return loaded.(*tokenizer.Sequencer)
}

func (training *Training) restoreSequencer(symbol string, state tokenizer.SequencerState) {
	if symbol == "" {
		return
	}

	seq := tokenizer.NewSequencer()
	seq.Restore(state)
	training.sequencers.Store(symbol, seq)
}

func (training *Training) ResetSequencer(symbol string) {
	loaded, ok := training.sequencers.Load(symbol)

	if !ok {
		return
	}

	seq := loaded.(*tokenizer.Sequencer)
	seq.Reset()
	training.catalog.Persist(training.Context(), training.client, symbol, seq.State())
}

// lightBroker paints volume / day / spread / book oracle sides before Emit.
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
}

func (training *Training) shine(regions []*runtime.Region, veto, approve uint8, ok bool) {
	if ok {
		store.Shine(regions[approve])
		zeroRegion(regions[veto])
		return
	}

	store.Shine(regions[veto])
	zeroRegion(regions[approve])
}

func zeroRegion(region *runtime.Region) {
	if region == nil {
		return
	}

	region.Brightness = 0
	region.Deformation = 0
	region.Counts = 0
}

func (training *Training) stepCut(symbol string, regions []*runtime.Region) *runtime.Tokens {
	token, brightness := store.PathToken(regions)

	if token == "" {
		cut := runtime.Tokens{}
		return &cut
	}

	seq := training.Sequencer(symbol)
	cut := seq.Step(token, brightness)
	training.catalog.Persist(training.Context(), training.client, symbol, seq.State())

	return &cut
}

// Board is the Emit region set. Tradable means approve winners only — no veto.
type Board []*runtime.Region

func (board Board) Tradable() bool {
	approved := false

	for _, region := range board {
		if region == nil || region.Brightness <= 0 {
			continue
		}

		key := int(region.Key)
		if key < 9 || key > 22 {
			continue
		}

		if key&1 == 1 {
			return false
		}

		approved = true
	}

	return approved
}
