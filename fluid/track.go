package fluid

import (
	"sync"
	"time"
)

const fieldHistoryCap = 64

/*
TrackStore holds per-symbol fluid field histories.
*/
type TrackStore struct {
	mu       sync.Mutex
	bySymbol map[string]*SymbolField
}

/*
SymbolField tracks density, velocity, spread, and confidence samples.
*/
type SymbolField struct {
	samples           []fieldSample
	velocities        []float64
	sourceHistory     []float64
	shockHistory      []float64
	confidenceHistory []float64
	dailyQuoteVol     float64
	lastPrice         float64
	lastSample        fieldSample
	lastAt            time.Time
	hasPrior          bool
}

/*
NewTrackStore creates an empty fluid track store.
*/
func NewTrackStore() *TrackStore {
	return &TrackStore{
		bySymbol: make(map[string]*SymbolField),
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
Sample ingests one fluid field observation for a symbol.
*/
func (trackStore *TrackStore) Sample(
	symbol string,
	density, price, spreadBPS, flow, buyPressure float64,
	now time.Time,
) (float64, string) {
	trackStore.mu.Lock()
	defer trackStore.mu.Unlock()

	track := trackStore.ensure(symbol)

	current := fieldSample{
		density:   density,
		viscosity: spreadBPS,
		flow:      flow,
	}

	if track.hasPrior && !track.lastAt.IsZero() {
		elapsed := now.Sub(track.lastAt).Seconds()

		if elapsed > 0 && track.lastPrice > 0 && price > 0 {
			current.velocity = (price - track.lastPrice) / track.lastPrice / elapsed
		}
	}

	if !track.hasPrior {
		track.lastPrice = price
		track.lastSample = current
		track.lastAt = now
		track.hasPrior = true
		return 0, ""
	}

	source := continuitySource(current, track.lastSample)
	shock := burgersShock(current, track.lastSample)

	sourceFence := ratioFence(track.sourceHistory)
	shockFence := ratioFence(track.shockHistory)

	quiet := quietVelocity(track.velocities, current.velocity)
	accumulating := quiet && sourceFence > 0 && source > sourceFence
	shocking := shockFence > 0 && shock > shockFence
	confidence := fieldConfidence(source, shock, buyPressure, quiet)
	reason := ""

	if accumulating {
		reason = "accumulation"
	}

	if shocking {
		reason = "shock"
	}

	if source > 0 {
		track.sourceHistory = append(track.sourceHistory, source)
	}

	if shock > 0 {
		track.shockHistory = append(track.shockHistory, shock)
	}

	track.trimHistories()
	track.velocities = append(track.velocities, current.velocity)
	track.samples = append(track.samples, current)

	if len(track.velocities) > fieldHistoryCap {
		track.velocities = track.velocities[len(track.velocities)-fieldHistoryCap:]
	}

	if len(track.samples) > fieldHistoryCap {
		track.samples = track.samples[len(track.samples)-fieldHistoryCap:]
	}

	track.lastPrice = price
	track.lastSample = current
	track.lastAt = now
	track.recordConfidence(confidence)

	return confidence, reason
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
SampledCount returns symbols with at least one stored sample.
*/
func (trackStore *TrackStore) SampledCount() int {
	trackStore.mu.Lock()
	defer trackStore.mu.Unlock()

	count := 0

	for _, track := range trackStore.bySymbol {
		if len(track.samples) > 0 {
			count++
		}
	}

	return count
}

/*
WarmingCount returns symbols with ticker volume but no samples yet.
*/
func (trackStore *TrackStore) WarmingCount() int {
	trackStore.mu.Lock()
	defer trackStore.mu.Unlock()

	count := 0

	for _, track := range trackStore.bySymbol {
		if track.dailyQuoteVol > 0 && len(track.samples) == 0 {
			count++
		}
	}

	return count
}

func (track *SymbolField) trimHistories() {
	if len(track.sourceHistory) > fieldHistoryCap {
		track.sourceHistory = track.sourceHistory[len(track.sourceHistory)-fieldHistoryCap:]
	}

	if len(track.shockHistory) > fieldHistoryCap {
		track.shockHistory = track.shockHistory[len(track.shockHistory)-fieldHistoryCap:]
	}
}

func (track *SymbolField) recordConfidence(confidence float64) {
	if confidence <= 0 {
		return
	}

	track.confidenceHistory = append(track.confidenceHistory, confidence)

	if len(track.confidenceHistory) > fieldHistoryCap {
		track.confidenceHistory = track.confidenceHistory[len(track.confidenceHistory)-fieldHistoryCap:]
	}
}

func (trackStore *TrackStore) ensure(symbol string) *SymbolField {
	track, ok := trackStore.bySymbol[symbol]

	if ok {
		return track
	}

	track = &SymbolField{
		samples:           make([]fieldSample, 0, fieldHistoryCap),
		velocities:        make([]float64, 0, fieldHistoryCap),
		sourceHistory:     make([]float64, 0, fieldHistoryCap),
		shockHistory:      make([]float64, 0, fieldHistoryCap),
		confidenceHistory: make([]float64, 0, fieldHistoryCap),
	}
	trackStore.bySymbol[symbol] = track

	return track
}
