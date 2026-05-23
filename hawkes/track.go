package hawkes

import (
	"sync"
	"time"
)

const confidenceHistoryCap = 64

/*
TrackStore holds per-symbol Hawkes confidence history and liquidity state.
*/
type TrackStore struct {
	mu       sync.Mutex
	bySymbol map[string]*SymbolTrack
}

/*
SymbolTrack stores rolling confidence samples, daily quote volume, and warm-start fits.
*/
type SymbolTrack struct {
	confidenceHistory []float64
	dailyQuoteVol     float64
	buyFit            SideFit
	sellFit           SideFit
	hasBuyFit         bool
	hasSellFit        bool
}

/*
NewTrackStore creates an empty Hawkes track store.
*/
func NewTrackStore() *TrackStore {
	return &TrackStore{
		bySymbol: make(map[string]*SymbolTrack),
	}
}

/*
ApplyTicker ingests 24h quote volume for liquidity filtering.
*/
func (trackStore *TrackStore) ApplyTicker(symbol string, last, volumeBase float64) {
	if symbol == "" || last <= 0 {
		return
	}

	trackStore.mu.Lock()
	defer trackStore.mu.Unlock()

	track := trackStore.ensure(symbol)
	track.dailyQuoteVol = volumeBase * last
}

/*
PassesLiquidity keeps symbols below the live cross-section median daily quote volume.
*/
func (trackStore *TrackStore) PassesLiquidity(symbol string) bool {
	trackStore.mu.Lock()
	defer trackStore.mu.Unlock()

	track, ok := trackStore.bySymbol[symbol]

	if !ok || track.dailyQuoteVol <= 0 {
		return false
	}

	quoteVolumes := make([]float64, 0, len(trackStore.bySymbol))

	for _, candidate := range trackStore.bySymbol {
		if candidate.dailyQuoteVol <= 0 {
			continue
		}

		quoteVolumes = append(quoteVolumes, candidate.dailyQuoteVol)
	}

	if len(quoteVolumes) < 2 {
		return false
	}

	return track.dailyQuoteVol < crossSectionMedian(quoteVolumes)
}

/*
FitSide warm-starts Hawkes MLE from the symbol's prior side fit.
*/
func (trackStore *TrackStore) FitSide(
	symbol string,
	side string,
	events []time.Time,
	horizon time.Time,
) SideFit {
	trackStore.mu.Lock()
	defer trackStore.mu.Unlock()

	track := trackStore.ensure(symbol)
	prior := SideFit{}

	switch side {
	case "buy":
		if track.hasBuyFit {
			prior = track.buyFit
		}
	case "sell":
		if track.hasSellFit {
			prior = track.sellFit
		}
	}

	fit := fitSideWithPrior(events, horizon, prior)

	switch side {
	case "buy":
		if fit.mu > 0 {
			track.buyFit = fit
			track.hasBuyFit = true
		}
	case "sell":
		if fit.mu > 0 {
			track.sellFit = fit
			track.hasSellFit = true
		}
	}

	return fit
}

/*
RecordScore stores one Hawkes confidence sample and returns it for the trader batch.
*/
func (trackStore *TrackStore) RecordScore(symbol string, confidence float64) float64 {
	if confidence <= 0 {
		return 0
	}

	trackStore.mu.Lock()
	defer trackStore.mu.Unlock()

	track := trackStore.ensure(symbol)
	track.recordConfidence(confidence)

	return confidence
}

func (track *SymbolTrack) recordConfidence(confidence float64) {
	if confidence <= 0 {
		return
	}

	track.confidenceHistory = append(track.confidenceHistory, confidence)

	if len(track.confidenceHistory) > confidenceHistoryCap {
		track.confidenceHistory = track.confidenceHistory[len(track.confidenceHistory)-confidenceHistoryCap:]
	}
}

func (trackStore *TrackStore) ensure(symbol string) *SymbolTrack {
	track, ok := trackStore.bySymbol[symbol]

	if ok {
		return track
	}

	track = &SymbolTrack{
		confidenceHistory: make([]float64, 0, confidenceHistoryCap),
	}
	trackStore.bySymbol[symbol] = track

	return track
}
