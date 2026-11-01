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

		training.lightBroker(symbol, regions)
		training.grid.Emit(regions, training.open(symbol))
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
	if symbol == "" {
		return tokenizer.NewSequencer()
	}

	loaded, _ := training.sequencers.LoadOrStore(symbol, tokenizer.NewSequencer())
	return loaded.(*tokenizer.Sequencer)
}

func (training *Training) ResetSequencer(symbol string) {
	if symbol == "" {
		return
	}

	loaded, ok := training.sequencers.Load(symbol)
	if !ok {
		return
	}

	loaded.(*tokenizer.Sequencer).Reset()
}

func (training *Training) open(symbol string) bool {
	if symbol == "" {
		return false
	}

	position := training.desk.Position(symbol)
	return position != nil && !position.IsClosed()
}

// lightBroker paints mover / book oracle sides before Emit collapses each pair.
func (training *Training) lightBroker(symbol string, regions []*runtime.Region) {
	if symbol == "" {
		return
	}

	if len(regions) < store.RegionCount {
		return
	}

	price := training.price
	risk := system.Cfg.Risk

	training.shine(regions, 9, 10, price.VolumeUSD(symbol) >= risk.MinMoverVolumeUSD)
	training.shine(regions, 11, 12, price.ChangePct(symbol) >= risk.MinMoverChangePct)

	spreadOK := false

	if spread, ok := price.SpreadPct(symbol); ok && spread != nil {
		spreadOK = spread.MustFloat64() <= risk.MaxEntrySpreadPct
	}

	training.shine(regions, 13, 14, spreadOK)
	training.shine(regions, 15, 16, price.BookIntact(symbol))
}

func (training *Training) shine(regions []*runtime.Region, veto, approve uint8, ok bool) {
	if ok {
		store.Shine(regions[approve])
		return
	}

	store.Shine(regions[veto])
}

// stepCut feeds one regime letter per tick into the time-series sequencer.
func (training *Training) stepCut(symbol string, regions []*runtime.Region) tokenizer.Tokens {
	token, brightness := store.PathToken(regions)

	if token == "" {
		return tokenizer.Tokens{}
	}

	return training.Sequencer(symbol).Step(token, brightness)
}

/*
Board is the Emit region set. Tradable means approve winners only — no veto.
*/
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
