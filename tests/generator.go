package tests

import (
	"math"
	"math/rand"
	"sync"
	"time"
)

type Generator struct {
	mu           sync.RWMutex
	rng          *rand.Rand
	symbol       string
	currentState MarketState
	profiles     map[MarketState]RegimeProfile
	midPrice     float64
	openPrice    float64
	cumVolume    float64
	cumValue     float64
	highPrice    float64
	lowPrice     float64
	currTime     time.Time
}

func NewGenerator(symbol string, startPrice float64, seed int64) *Generator {
	return &Generator{
		rng:          rand.New(rand.NewSource(seed)),
		symbol:       symbol,
		currentState: Baseline,
		profiles:     DefaultProfiles,
		midPrice:     startPrice,
		openPrice:    startPrice,
		highPrice:    startPrice,
		lowPrice:     startPrice,
		currTime:     time.Now().UTC(),
	}
}

// SetState dynamically switches the market condition.
func (generator *Generator) SetState(state MarketState) {
	generator.mu.Lock()
	defer generator.mu.Unlock()
	generator.currentState = state
}

// Step generates the next relative sample frame based on current state.
func (generator *Generator) Step() Sample {
	generator.mu.Lock()
	defer generator.mu.Unlock()

	profile, ok := generator.profiles[generator.currentState]

	if !ok {
		profile = generator.profiles[Baseline]
	}

	// 1. Advance Time Cadence
	generator.currTime = generator.currTime.Add(profile.Cadence)

	// 2. Calculate Price Motion (Geometric Random Walk + Drift)
	// Price Step = Mid * (1 + Drift*dt + Volatility * GaussianNoise)
	dt := profile.Cadence.Seconds()
	noise := generator.rng.NormFloat64()
	deltaPct := (profile.Drift * dt * 0.01) + (profile.Volatility * math.Sqrt(dt) * noise * 0.01)

	generator.midPrice = math.Max(0.01, generator.midPrice*(1.0+deltaPct))

	// 3. Dynamic Spread & Bid/Ask Construction
	baseSpread := generator.midPrice * 0.0005 // 5 bps base spread
	actualSpread := math.Max(0.01, baseSpread*profile.SpreadScale)

	halfSpread := actualSpread / 2.0
	bid := math.Trunc((generator.midPrice-halfSpread)*100) / 100
	ask := math.Trunc((generator.midPrice+halfSpread)*100) / 100
	last := bid + (generator.rng.Float64() * (ask - bid)) // Last trade executes inside spread

	// 4. Calculate Asymmetric Quantities
	bidQty := profile.BaseQty * profile.BidAskAsymmetry * (0.8 + 0.4*generator.rng.Float64())
	askQty := profile.BaseQty * (1.0 / profile.BidAskAsymmetry) * (0.8 + 0.4*generator.rng.Float64())

	// 5. Volume & Cumulative VWAP
	stepVolume := profile.BaseQty * profile.VolumeScale * (0.5 + generator.rng.Float64())
	generator.cumVolume += stepVolume
	generator.cumValue += (last * stepVolume)
	vwap := generator.cumValue / generator.cumVolume

	// 6. Track High / Low / Changes
	if last > generator.highPrice {
		generator.highPrice = last
	}

	if last < generator.lowPrice {
		generator.lowPrice = last
	}

	change := last - generator.openPrice
	changePct := (change / generator.openPrice) * 100.0

	return Sample{
		Symbol:    generator.symbol,
		Bid:       bid,
		BidQty:    math.Round(bidQty*100) / 100,
		Ask:       ask,
		AskQty:    math.Round(askQty*100) / 100,
		Last:      math.Round(last*100) / 100,
		Volume:    math.Round(generator.cumVolume*100) / 100,
		VWAP:      math.Round(vwap*100) / 100,
		Low:       math.Round(generator.lowPrice*100) / 100,
		High:      math.Round(generator.highPrice*100) / 100,
		Change:    math.Round(change*100) / 100,
		ChangePct: math.Round(changePct*100) / 100,
		Timestamp: generator.currTime,
	}
}
