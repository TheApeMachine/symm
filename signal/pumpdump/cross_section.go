package pumpdump

import (
	"math"
	"sync"
	"time"

	nomadaptive "github.com/theapemachine/nomagique/adaptive"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
)

/*
CrossSection accumulates per-symbol volume and spread baselines for verticality scoring.
*/
type CrossSection struct {
	halflife time.Duration
	epsilon  float64
	universe sync.Map
}

type featureState struct {
	rvolTracker     *nomadaptive.TimeElasticMemory
	compTracker     *nomadaptive.TimeElasticMemory
	lastRvol        float64
	lastCompression float64
}

/*
NewCrossSection returns a cross-section store with time-elastic volume and spread trackers.
*/
func NewCrossSection(halflife time.Duration, epsilon float64) *CrossSection {
	if halflife <= 0 {
		halflife = time.Minute
	}

	if epsilon <= 0 {
		epsilon = 1e-6
	}

	return &CrossSection{
		halflife: halflife,
		epsilon:  epsilon,
	}
}

func (crossSection *CrossSection) ensure(symbol string) *featureState {
	raw, _ := crossSection.universe.LoadOrStore(symbol, &featureState{
		rvolTracker: nomadaptive.NewTimeElasticMemory(crossSection.halflife, crossSection.epsilon),
		compTracker: nomadaptive.NewTimeElasticMemory(crossSection.halflife, crossSection.epsilon),
	})

	state, ok := raw.(*featureState)

	if !ok {
		return nil
	}

	return state
}

func (crossSection *CrossSection) observeTrade(trade *krakenmarket.TradeUpdate) error {
	if trade == nil || trade.Symbol == "" || trade.Price <= 0 || trade.Qty <= 0 {
		return nil
	}

	state := crossSection.ensure(trade.Symbol)

	if state == nil {
		return nil
	}

	relative, err := state.rvolTracker.Update(trade.Timestamp, trade.Qty)

	if err != nil {
		return err
	}

	state.lastRvol = relative
	state.lastCompression = 0

	return nil
}

func (crossSection *CrossSection) observeBook(book *krakenmarket.BookUpdate) error {
	if book == nil || book.Symbol == "" || len(book.Bids) == 0 || len(book.Asks) == 0 {
		return nil
	}

	spread := book.Asks[0].Price - book.Bids[0].Price

	if spread <= 0 {
		return nil
	}

	state := crossSection.ensure(book.Symbol)

	if state == nil {
		return nil
	}

	volume := book.Bids[0].Qty + book.Asks[0].Qty

	relative, err := state.rvolTracker.Update(book.Timestamp, volume)

	if err != nil {
		return err
	}

	state.lastRvol = relative

	spreadRatio, err := state.compTracker.Update(book.Timestamp, spread)

	if err != nil {
		return err
	}

	if spreadRatio > 0 {
		state.lastCompression = 1.0 / spreadRatio
	}

	return nil
}

/*
Ready reports whether the scoped symbol has warmed its volume baseline.
*/
func (crossSection *CrossSection) Ready(symbol string) bool {
	raw, ok := crossSection.universe.Load(symbol)

	if !ok {
		return false
	}

	state, ok := raw.(*featureState)

	if !ok {
		return false
	}

	return state.rvolTracker.Initialized()
}

/*
LastRvol returns the latest relative volume score for the symbol.
*/
func (crossSection *CrossSection) LastRvol(symbol string) float64 {
	raw, ok := crossSection.universe.Load(symbol)

	if !ok {
		return 0
	}

	state, ok := raw.(*featureState)

	if !ok {
		return 0
	}

	return state.lastRvol
}

/*
verticalityPayload returns rvol, precursor, compression, and move for the verticality stage.
*/
func (crossSection *CrossSection) verticalityPayload(
	symbol string,
	move, precursor float64,
) ([]float64, bool) {
	raw, ok := crossSection.universe.Load(symbol)

	if !ok {
		return nil, false
	}

	state, ok := raw.(*featureState)

	if !ok || !state.rvolTracker.Initialized() {
		return nil, false
	}

	precursorScale := math.Max(math.Abs(move), math.SmallestNonzeroFloat64)
	precursorNorm := math.Abs(precursor) / precursorScale

	return []float64{
		state.lastRvol,
		precursorNorm,
		state.lastCompression,
		move,
	}, true
}
