package causal

import (
	"math"
	"sync"
	"time"
)

const (
	causalHistoryCap  = 64
	minCausalHistory  = 12
	minLiquidityPairs = 2
)

/*
causalSample is one DAG observation:
MacroMomentum → PriceVelocity ← LocalFlow, with Liquidity as a backdoor node.
*/
type causalSample struct {
	macroMomentum float64
	liquidity     float64
	localFlow     float64
	priceVelocity float64
}

/*
TrackStore holds per-symbol causal histories and cross-section macro state.
*/
type TrackStore struct {
	mu       sync.Mutex
	bySymbol map[string]*SymbolTrack
}

/*
SymbolTrack stores rolling causal samples and effect histories.
*/
type SymbolTrack struct {
	samples           []causalSample
	interventionHist  []float64
	lastPrice         float64
	lastAt            time.Time
	hasPrior          bool
	dailyQuoteVol     float64
}

/*
NewTrackStore creates an empty causal track store.
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
MacroMomentum returns the cross-section median 24h change percent.
*/
func (trackStore *TrackStore) MacroMomentum(
	symbols []string,
	changePct func(symbol string) (float64, bool),
) float64 {
	trackStore.mu.Lock()
	defer trackStore.mu.Unlock()

	changes := make([]float64, 0, len(symbols))

	for _, symbol := range symbols {
		change, ok := changePct(symbol)

		if !ok {
			continue
		}

		changes = append(changes, change)
	}

	if len(changes) == 0 {
		return 0
	}

	return percentileSorted(copySorted(changes), 0.5)
}

/*
Record ingests one causal observation for a symbol.
*/
func (trackStore *TrackStore) Record(
	symbol string,
	macroMomentum, liquidity, localFlow, price float64,
	now time.Time,
) (causalSample, bool) {
	trackStore.mu.Lock()
	defer trackStore.mu.Unlock()

	track := trackStore.ensure(symbol)
	velocity := 0.0

	if track.hasPrior && !track.lastAt.IsZero() && track.lastPrice > 0 && price > 0 {
		elapsed := now.Sub(track.lastAt).Seconds()

		if elapsed > 0 {
			velocity = (price - track.lastPrice) / track.lastPrice / elapsed
		}
	}

	sample := causalSample{
		macroMomentum: macroMomentum,
		liquidity:     liquidity,
		localFlow:     localFlow,
		priceVelocity: velocity,
	}

	if !track.hasPrior {
		track.lastPrice = price
		track.lastAt = now
		track.hasPrior = true
		return sample, false
	}

	track.samples = append(track.samples, sample)

	if len(track.samples) > causalHistoryCap {
		track.samples = track.samples[len(track.samples)-causalHistoryCap:]
	}

	track.lastPrice = price
	track.lastAt = now

	return sample, len(track.samples) >= minCausalHistory
}

/*
Evaluate scores rung-2 intervention and rung-3 counterfactual uplift for one symbol.
*/
func (trackStore *TrackStore) Evaluate(
	symbol string,
	current causalSample,
) (confidence float64, reason string, fired bool) {
	trackStore.mu.Lock()
	defer trackStore.mu.Unlock()

	track, ok := trackStore.bySymbol[symbol]

	if !ok || len(track.samples) < minCausalHistory {
		return 0, "", false
	}

	samples := track.samples
	association := associationEffect(samples)
	intervention := backdoorFlowEffect(samples)
	fence := effectFence(track.interventionHist)

	if intervention <= 0 {
		track.recordIntervention(intervention)
		return 0, "", false
	}

	if len(track.interventionHist) >= minCausalHistory/2 && (fence <= 0 || intervention <= fence) {
		track.recordIntervention(intervention)
		return 0, "", false
	}

	coef, fitOK := fitStructural(samples)

	if !fitOK {
		track.recordIntervention(intervention)
		return 0, "", false
	}

	interventionFlow := flowInterventionLevel(samples)
	uplift := counterfactualUplift(current, coef, interventionFlow)

	if uplift <= 0 {
		track.recordIntervention(intervention)
		return 0, "", false
	}

	confounded := math.Abs(intervention-association) > math.Abs(association)*0.25
	reason = "intervention"

	if confounded && uplift > intervention*0.5 {
		reason = "counterfactual"
	}

	confidence = intervention * uplift

	if current.localFlow <= 0 || current.liquidity <= 0 {
		track.recordIntervention(intervention)
		return 0, "", false
	}

	track.recordIntervention(intervention)

	if confidence <= 0 {
		return 0, "", false
	}

	return trackStore.confidenceSpike(track, confidence), reason, true
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

	if len(quoteVolumes) < minLiquidityPairs {
		return false
	}

	return track.dailyQuoteVol < percentileSorted(copySorted(quoteVolumes), 0.5)
}

func (trackStore *TrackStore) confidenceSpike(track *SymbolTrack, score float64) float64 {
	if score <= 0 {
		return 0
	}

	history := make([]float64, 0, len(track.interventionHist))
	history = append(history, track.interventionHist...)

	if len(history) < minCausalHistory/2 {
		return score
	}

	fence := effectFence(history)

	if fence <= 0 || score <= fence {
		return 0
	}

	return score / fence
}

func (track *SymbolTrack) recordIntervention(effect float64) {
	if effect == 0 {
		return
	}

	track.interventionHist = append(track.interventionHist, effect)

	if len(track.interventionHist) > causalHistoryCap {
		track.interventionHist = track.interventionHist[len(track.interventionHist)-causalHistoryCap:]
	}
}

func (trackStore *TrackStore) ensure(symbol string) *SymbolTrack {
	track, ok := trackStore.bySymbol[symbol]

	if ok {
		return track
	}

	track = &SymbolTrack{
		samples:          make([]causalSample, 0, causalHistoryCap),
		interventionHist: make([]float64, 0, causalHistoryCap),
	}
	trackStore.bySymbol[symbol] = track

	return track
}

func effectFence(effects []float64) float64 {
	if len(effects) == 0 {
		return 0
	}

	lower, upper := quartiles(effects)
	spread := upper - lower

	if spread > 0 {
		return upper + spread + spread/2
	}

	return maxFloat(effects)
}

func quartiles(values []float64) (lower, upper float64) {
	if len(values) == 0 {
		return 0, 0
	}

	sorted := copySorted(values)

	return percentileSorted(sorted, 0.25), percentileSorted(sorted, 0.75)
}

func percentileSorted(sorted []float64, quantile float64) float64 {
	if len(sorted) == 0 {
		return 0
	}

	if quantile <= 0 {
		return sorted[0]
	}

	if quantile >= 1 {
		return sorted[len(sorted)-1]
	}

	position := quantile * float64(len(sorted)-1)
	lowerIndex := int(math.Floor(position))
	upperIndex := int(math.Ceil(position))
	weight := position - float64(lowerIndex)

	return sorted[lowerIndex]*(1-weight) + sorted[upperIndex]*weight
}

func copySorted(values []float64) []float64 {
	cp := append([]float64(nil), values...)
	sortFloats(cp)

	return cp
}

func maxFloat(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	peak := values[0]

	for _, value := range values[1:] {
		if value > peak {
			peak = value
		}
	}

	return peak
}

func sortFloats(values []float64) {
	for index := 1; index < len(values); index++ {
		for inner := index; inner > 0 && values[inner] < values[inner-1]; inner-- {
			values[inner], values[inner-1] = values[inner-1], values[inner]
		}
	}
}
