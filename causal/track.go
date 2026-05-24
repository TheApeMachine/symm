package causal

import (
	"math"
	"time"

	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/stats"
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
	engine.GaugeScan
	shard    engine.ShardedStore
	bySymbol map[string]*SymbolTrack
}

/*
SymbolTrack stores rolling causal samples and effect histories.
*/
type SymbolTrack struct {
	engine.SymbolLock
	samples           []causalSample
	interventionHist  []float64
	confidenceHistory []float64
	lastPrice         float64
	lastAt            time.Time
	lastElapsed       time.Duration
	hasPrior          bool
	dailyQuoteVol     float64
	calibrator        engine.PredictionCalibrator
	liveScore         float64
}

/*
BeginScan clears per-tick live gauge scores before the next scan set runs.
*/
func (trackStore *TrackStore) BeginScan() {
	trackStore.ResetGaugeScan()

	trackStore.shard.LockMap()
	defer trackStore.shard.UnlockMap()

	for _, track := range trackStore.bySymbol {
		track.liveScore = 0
	}
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

	trackStore.shard.LockMap()
	defer trackStore.shard.UnlockMap()

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
	trackStore.shard.LockMap()
	defer trackStore.shard.UnlockMap()

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
BuildSample constructs one causal observation without appending it to history.
*/
func (trackStore *TrackStore) BuildSample(
	symbol string,
	macroMomentum, liquidity, localFlow, price float64,
	now time.Time,
) (causalSample, bool) {
	trackStore.shard.LockMap()
	defer trackStore.shard.UnlockMap()

	track := trackStore.ensure(symbol)
	elapsed := time.Duration(0)

	if track.hasPrior && !track.lastAt.IsZero() {
		elapsed = now.Sub(track.lastAt)
	}

	velocity := 0.0

	if track.hasPrior && !track.lastAt.IsZero() && track.lastPrice > 0 && price > 0 {
		elapsedSec := elapsed.Seconds()

		if elapsedSec > 0 {
			velocity = (price - track.lastPrice) / track.lastPrice / elapsedSec
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

	return sample, len(track.samples) >= minCausalHistory
}

/*
CommitSample appends one evaluated sample to symbol history.
*/
func (trackStore *TrackStore) CommitSample(
	symbol string,
	sample causalSample,
	price float64,
	now time.Time,
) {
	trackStore.shard.LockMap()
	defer trackStore.shard.UnlockMap()

	track := trackStore.ensure(symbol)
	elapsed := time.Duration(0)

	if !track.lastAt.IsZero() {
		elapsed = now.Sub(track.lastAt)
	}

	track.samples = append(track.samples, sample)

	if len(track.samples) > causalHistoryCap {
		track.samples = track.samples[len(track.samples)-causalHistoryCap:]
	}

	track.lastPrice = price
	track.lastAt = now
	track.lastElapsed = elapsed
}

/*
Record ingests one causal observation for a symbol.
*/
func (trackStore *TrackStore) Record(
	symbol string,
	macroMomentum, liquidity, localFlow, price float64,
	now time.Time,
) (causalSample, bool) {
	sample, ready := trackStore.BuildSample(
		symbol, macroMomentum, liquidity, localFlow, price, now,
	)

	if !ready {
		return sample, false
	}

	trackStore.CommitSample(symbol, sample, price, now)

	return sample, true
}

/*
ApplyPredictionFeedback updates intervention calibration from one settled forecast.
*/
func (trackStore *TrackStore) ApplyPredictionFeedback(feedback engine.PredictionFeedback) {
	if feedback.Symbol == "" || feedback.PredictedReturn <= 0 {
		return
	}

	trackStore.shard.LockMap()
	defer trackStore.shard.UnlockMap()

	track := trackStore.ensure(feedback.Symbol)
	track.calibrator.Apply(feedback)
}

/*
Evaluate scores rung-2 intervention and rung-3 counterfactual uplift for one symbol.
*/
func (trackStore *TrackStore) Evaluate(
	symbol string,
	current causalSample,
) (float64, float64, time.Duration, string) {
	trackStore.shard.LockMap()
	defer trackStore.shard.UnlockMap()

	track, ok := trackStore.bySymbol[symbol]

	if !ok || len(track.samples) < minCausalHistory {
		return 0, 0, 0, ""
	}

	rawConfidence, reason := track.evaluateLocked(current)

	if rawConfidence <= 0 {
		return 0, 0, 0, ""
	}

	normalized := engine.NormalizeConfidence(rawConfidence, track.confidenceHistory)
	track.liveScore = normalized

	runway := opportunityRunway(track.samples, track.lastElapsed)

	if runway <= 0 {
		return 0, 0, 0, ""
	}

	track.recordConfidence(rawConfidence)
	expectedReturn := track.forecastReturn(current, reason, runway)

	return normalized, expectedReturn, runway, reason
}

/*
SymbolLiveScore returns the latest normalized gauge reading for one symbol.
*/
func (trackStore *TrackStore) SymbolLiveScore(symbol string) float64 {
	trackStore.shard.LockMap()
	defer trackStore.shard.UnlockMap()

	track, ok := trackStore.bySymbol[symbol]

	if !ok {
		return 0
	}

	return track.liveScore
}

/*
PeakLiveConfidence returns the highest unit-scale score across all symbols.
*/
func (trackStore *TrackStore) PeakLiveConfidence() float64 {
	trackStore.shard.LockMap()
	defer trackStore.shard.UnlockMap()

	peak := 0.0

	for _, track := range trackStore.bySymbol {
		if track.liveScore > peak {
			peak = track.liveScore
		}
	}

	return peak
}

/*
PeakSymbolScore returns the symbol with the highest live score.
*/
func (trackStore *TrackStore) PeakSymbolScore() (string, float64) {
	trackStore.shard.LockMap()
	defer trackStore.shard.UnlockMap()

	bestSymbol := ""
	bestScore := 0.0

	for symbol, track := range trackStore.bySymbol {
		if track.liveScore <= bestScore {
			continue
		}

		bestScore = track.liveScore
		bestSymbol = symbol
	}

	return bestSymbol, bestScore
}

func (track *SymbolTrack) forecastReturn(
	current causalSample, reason string, runway time.Duration,
) float64 {
	if runway <= 0 {
		return 0
	}

	runwaySeconds := runway.Seconds()

	if reason == "counterfactual" {
		model, fitOK := fitNonLinearStructural(track.samples)

		if fitOK {
			uplift := nonLinearCounterfactualUplift(
				current, model, flowInterventionLevel(track.samples),
			)

			if uplift > 0 {
				return uplift * runwaySeconds
			}
		}
	}

	return current.priceVelocity * runwaySeconds
}

func (track *SymbolTrack) evaluateLocked(current causalSample) (float64, string) {
	samples := track.samples
	association := associationEffect(samples)
	intervention := kernelBackdoorFlowEffect(samples) * track.calibrator.Scale()

	if intervention <= 0 {
		track.recordIntervention(intervention)
		return 0, ""
	}

	track.recordIntervention(intervention)

	model, fitOK := fitNonLinearStructural(samples)

	if !fitOK {
		return intervention, "intervention"
	}

	interventionFlow := flowInterventionLevel(samples)
	uplift := nonLinearCounterfactualUplift(current, model, interventionFlow)

	if uplift <= 0 {
		return intervention, "intervention"
	}

	confounded := math.Abs(intervention-association) > math.Abs(association)*0.25
	reason := "intervention"

	if confounded && uplift > intervention*0.5 {
		reason = "counterfactual"
	}

	confidence := intervention * uplift

	if current.localFlow <= 0 || current.liquidity <= 0 {
		return intervention, reason
	}

	if confidence <= 0 {
		return intervention, reason
	}

	return confidence, reason
}

func (track *SymbolTrack) recordConfidence(confidence float64) {
	if confidence <= 0 {
		return
	}

	track.confidenceHistory = append(track.confidenceHistory, confidence)

	if len(track.confidenceHistory) > causalHistoryCap {
		track.confidenceHistory = track.confidenceHistory[len(track.confidenceHistory)-causalHistoryCap:]
	}
}

/*
PassesLiquidity keeps symbols below the live cross-section median daily quote volume.
*/
func (trackStore *TrackStore) PassesLiquidity(symbol string) bool {
	trackStore.shard.LockMap()
	defer trackStore.shard.UnlockMap()

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
	return trackStore.ensureLocked(symbol)
}

func (trackStore *TrackStore) track(symbol string) *SymbolTrack {
	trackStore.shard.LockMap()
	defer trackStore.shard.UnlockMap()

	return trackStore.ensureLocked(symbol)
}

func (trackStore *TrackStore) ensureLocked(symbol string) *SymbolTrack {
	track, ok := trackStore.bySymbol[symbol]

	if ok {
		return track
	}

	track = &SymbolTrack{
		samples:           make([]causalSample, 0, causalHistoryCap),
		interventionHist:  make([]float64, 0, causalHistoryCap),
		confidenceHistory: make([]float64, 0, causalHistoryCap),
		calibrator:        engine.NewPredictionCalibrator(),
	}
	trackStore.bySymbol[symbol] = track

	return track
}

func percentileSorted(sorted []float64, quantile float64) float64 {
	return stats.PercentileSorted(sorted, quantile)
}

func copySorted(values []float64) []float64 {
	return stats.CopySorted(values)
}

func maxFloat(values []float64) float64 {
	return stats.Max(values)
}
