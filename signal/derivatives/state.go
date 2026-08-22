package derivatives

import (
	"sync"
	"time"
)

type priceSample struct {
	at   time.Time
	spot float64
	perp float64
}

/*
SymbolState maintains rolling dynamic state for spot and derivatives streams
for a single instrument.
*/
type SymbolState struct {
	mu sync.Mutex

	LastSpotPrice    float64
	LastSpotTime     time.Time
	LastPerpPrice    float64
	LastIndexPrice   float64
	LastFundingRate  float64
	LastOpenInterest float64
	PrevOpenInterest float64
	OIDelta          float64
	OIVelocity       float64
	OIAcceleration   float64
	Basis            float64
	BasisVelocity    float64
	PrevBasis        float64
	IndexBasis       float64
	TripartiteDiv    float64

	FuturesBuyVolume  float64
	FuturesSellVolume float64
	FuturesCVD        float64
	LiqBuyVolume      float64
	LiqSellVolume     float64

	PriceHistory []priceSample
	LeadLagTau   float64
	LeadLagCorr  float64

	LastUpdate time.Time
}

func NewSymbolState() *SymbolState {
	return &SymbolState{
		PriceHistory: make([]priceSample, 0, 128),
	}
}

func (state *SymbolState) RecordPriceSample(at time.Time, spot, perp float64) {
	if at.IsZero() || (spot == 0 && perp == 0) {
		return
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	if spot > 0 {
		state.LastSpotPrice = spot
		state.LastSpotTime = at
	}

	if perp > 0 {
		state.LastPerpPrice = perp
	}

	currentSpot := state.LastSpotPrice
	currentPerp := state.LastPerpPrice

	if currentSpot > 0 && currentPerp > 0 {
		state.PriceHistory = append(state.PriceHistory, priceSample{
			at:   at,
			spot: currentSpot,
			perp: currentPerp,
		})

		cutoff := at.Add(-10 * time.Second)
		startIndex := 0

		for startIndex < len(state.PriceHistory) && state.PriceHistory[startIndex].at.Before(cutoff) {
			startIndex++
		}

		if startIndex > 0 {
			state.PriceHistory = state.PriceHistory[startIndex:]
		}

		state.updateLeadLag()
	}
}

func (state *SymbolState) updateLeadLag() {
	count := len(state.PriceHistory)

	if count < 5 {
		return
	}

	bestLag := 0.0
	bestCorr := 0.0

	// Check lags from -3 to +3 steps
	for lag := -3; lag <= 3; lag++ {
		corr := calculateLagCorrelation(state.PriceHistory, lag)

		if corr > bestCorr {
			bestCorr = corr
			bestLag = float64(lag)
		}
	}

	state.LeadLagTau = bestLag
	state.LeadLagCorr = bestCorr
}

func calculateLagCorrelation(history []priceSample, lag int) float64 {
	count := len(history)
	start := 1

	if lag > 0 {
		start = 1 + lag
	}

	end := count

	if lag < 0 {
		end = count + lag
	}

	if start >= end || end-start < 3 {
		return 0
	}

	var sumSpotDiff, sumPerpDiff, sumCross, sumSpotSq, sumPerpSq float64
	n := float64(end - start)

	for i := start; i < end; i++ {
		spotDiff := history[i].spot - history[i-1].spot
		perpIdx := i - lag
		perpDiff := history[perpIdx].perp - history[perpIdx-1].perp

		sumSpotDiff += spotDiff
		sumPerpDiff += perpDiff
		sumCross += spotDiff * perpDiff
		sumSpotSq += spotDiff * spotDiff
		sumPerpSq += perpDiff * perpDiff
	}

	meanSpot := sumSpotDiff / n
	meanPerp := sumPerpDiff / n

	covariance := (sumCross / n) - (meanSpot * meanPerp)
	varSpot := (sumSpotSq / n) - (meanSpot * meanSpot)
	varPerp := (sumPerpSq / n) - (meanPerp * meanPerp)

	denom := varSpot * varPerp

	if denom <= 0 {
		return 0
	}

	return covariance / (denom)
}
