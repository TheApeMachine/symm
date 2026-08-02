package tests

import (
	"encoding/json"
	"iter"
	"math"
	"math/rand"
	"sync"
	"time"
)

/*
Generator is a signal template engine. It generates a function that produces
a signal, and uses a template where the signal is used to update the values
in the template. This allows us to generate many different kinds of scenarios,
purely by updating the function that generates the signal. We can make the
signal spike, dip, and anything in between.
*/
type Generator struct {
	mu           sync.RWMutex
	rng          *rand.Rand
	symbol       string
	currentState MarketState
	targetState  MarketState
	momentum     float64
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
		targetState:  Baseline,
		momentum:     1.0,
		profiles:     DefaultProfiles,
		midPrice:     startPrice,
		openPrice:    startPrice,
		highPrice:    startPrice,
		lowPrice:     startPrice,
		currTime:     time.Now().UTC(),
	}
}

/*
Set state should gradually transition from the current state to the new
state. It does this by iterating from the current state to the new state,
incrementing the state by one each time. This ensures that the market
gradually transitions from one state to another, rather than jumping
directly into the new state. Use momentum to determine how fast the
transition happens. A higher momentum value will cause the transition
to happen faster.
*/
func (generator *Generator) SetState(state MarketState, momentum ...float64) {
	generator.mu.Lock()
	defer generator.mu.Unlock()

	speed := 1.0

	if len(momentum) > 0 && momentum[0] > 0 {
		speed = momentum[0]
	}

	generator.targetState = state
	generator.momentum = speed
}

// Step generates the next relative sample frame based on current state.
func (generator *Generator) Step() Sample {
	generator.mu.Lock()
	defer generator.mu.Unlock()

	if generator.currentState != generator.targetState {
		if generator.currentState < generator.targetState {
			generator.currentState++
		}

		if generator.currentState > generator.targetState {
			generator.currentState--
		}
	}

	profile, ok := generator.profiles[generator.currentState]

	if !ok {
		profile = generator.profiles[Baseline]
	}

	// 1. Advance Time Cadence
	generator.currTime = generator.currTime.Add(profile.Cadence)

	// 2. Calculate Price Motion (Geometric Random Walk + Drift)
	dt := profile.Cadence.Seconds()
	noise := generator.rng.NormFloat64()
	deltaPct := (profile.Drift * dt * 0.01) + (profile.Volatility * math.Sqrt(dt) * noise * 0.01)

	generator.midPrice = math.Max(0.01, generator.midPrice*(1.0+deltaPct))

	// 3. Dynamic Spread & Bid/Ask Construction
	baseSpread := generator.midPrice * 0.0005
	actualSpread := math.Max(0.01, baseSpread*profile.SpreadScale)

	halfSpread := actualSpread / 2.0
	bid := math.Trunc((generator.midPrice-halfSpread)*100) / 100
	ask := math.Trunc((generator.midPrice+halfSpread)*100) / 100
	last := bid + (generator.rng.Float64() * (ask - bid))

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

/*
Generate yields raw JSON []byte payload frames derived by updating the given template with signal steps.
*/
func (generator *Generator) Generate(template []byte) iter.Seq[[]byte] {
	return func(yield func([]byte) bool) {
		sample := generator.Step()
		frame := generator.render(template, sample)

		if len(frame) > 0 {
			yield(frame)
		}
	}
}

func (generator *Generator) render(template []byte, sample Sample) []byte {
	if len(template) == 0 {
		payload, _ := json.Marshal(map[string]any{
			"channel": "ticker",
			"type":    "update",
			"data":    []Sample{sample},
		})

		return payload
	}

	var wire map[string]any

	if err := json.Unmarshal(template, &wire); err != nil {
		return template
	}

	if data, ok := wire["data"].([]any); ok && len(data) > 0 {
		if row, ok := data[0].(map[string]any); ok {
			row["symbol"] = sample.Symbol
			row["bid"] = sample.Bid
			row["bid_qty"] = sample.BidQty
			row["ask"] = sample.Ask
			row["ask_qty"] = sample.AskQty
			row["last"] = sample.Last
			row["volume"] = sample.Volume
			row["vwap"] = sample.VWAP
			row["low"] = sample.Low
			row["high"] = sample.High
			row["change"] = sample.Change
			row["change_pct"] = sample.ChangePct
			row["timestamp"] = sample.Timestamp.Format(time.RFC3339Nano)
		}
	}

	wire["type"] = "update"
	payload, err := json.Marshal(wire)

	if err != nil {
		return template
	}

	return payload
}
