package signal

import (
	"encoding/json"
	"iter"
	"math"
	"math/rand"
	"strings"
	"sync"
	"time"

	testtypes "github.com/theapemachine/symm/tests/types"
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
	currentState testtypes.MarketState
	targetState  testtypes.MarketState
	momentum     float64

	/*
		sourceProfile is the regime the market is transitioning away from and
		progress runs from 0 to 1 as it settles into the target regime.
	*/
	sourceProfile testtypes.RegimeProfile
	progress      float64

	/*
		burst is the unspent fraction of the ignition impulse that opens the
		current regime. It remains fully armed while the blended precursor is
		sampled, then decays only after the first ignition sample.
	*/
	burst float64

	profiles  map[testtypes.MarketState]testtypes.RegimeProfile
	midPrice  float64
	openPrice float64
	cumVolume float64
	cumValue  float64
	highPrice float64
	lowPrice  float64
	currTime  time.Time
	sequence  int64
	l3Bid     float64
	l3Ask     float64
	l3BidQty  float64
	l3AskQty  float64
}

func NewGenerator(symbol string, startPrice float64, seed int64) *Generator {
	return &Generator{
		rng:          rand.New(rand.NewSource(seed)),
		symbol:       symbol,
		currentState: testtypes.Baseline,
		targetState:  testtypes.Baseline,
		profiles:     testtypes.DefaultProfiles,

		/*
			A new generator is already settled in the baseline regime, so the
			transition starts complete.
		*/
		sourceProfile: testtypes.DefaultProfiles[testtypes.Baseline],
		progress:      1.0,
		momentum:      1.0,

		midPrice:  startPrice,
		openPrice: startPrice,
		highPrice: startPrice,
		lowPrice:  startPrice,
		currTime:  time.Now().UTC(),
	}
}

/*
baselineTransitionTicks is how many ticks a regime shift takes at unit
momentum. Stronger moves complete in proportionally fewer ticks.
*/
const baselineTransitionTicks = 20.0

/*
SetState begins a gradual transition from the current regime to the given
one. Rather than snapping, the generator blends the two regime profiles over
subsequent Steps, so the market shifts the way a real one does.

Momentum sets how forcefully the market enters the new state, and its
magnitude determines the speed: a flash crash (-1.2) completes in a handful
of ticks, a slow bleed (-0.3) takes far longer. The sign carries direction
and is already reflected in the target profile's drift, so only the
magnitude affects pacing. Momentum of zero settles at the baseline pace.

A transition that is still in flight is re-based from the profile currently
in effect, so redirecting mid-move continues from where the market actually
is instead of snapping back to the previous regime.
*/
func (generator *Generator) SetState(state testtypes.MarketState, momentum ...float64) {
	generator.mu.Lock()
	defer generator.mu.Unlock()

	speed := 0.0

	if len(momentum) > 0 {
		speed = math.Abs(momentum[0])
	}

	if speed == 0 {
		speed = 1.0
	}

	generator.sourceProfile = generator.activeProfile()
	generator.currentState = state
	generator.targetState = state
	generator.momentum = speed
	generator.progress = 0

	// The transition samples its blended precursor before this impulse fires.
	generator.burst = 1.0
}

/*
PrecursorPending reports whether Step will still sample the blended approach
to the target regime. Once it becomes false, any configured ignition remains
fully armed and unsampled for the following Step.
*/
func (generator *Generator) PrecursorPending() bool {
	generator.mu.RLock()
	defer generator.mu.RUnlock()

	return generator.progress < 1.0
}

/*
IgnitionArmed reports whether the target regime's discontinuous price and
volume event has not yet been sampled.
*/
func (generator *Generator) IgnitionArmed() bool {
	generator.mu.RLock()
	defer generator.mu.RUnlock()

	profile := generator.profiles[generator.targetState]

	return generator.progress >= 1.0 && generator.burst == 1.0 &&
		profile.IgnitionMove != 0
}

/*
activeProfile returns the profile currently in effect, which mid-transition
is the blend between the source and target regimes.
*/
func (generator *Generator) activeProfile() testtypes.RegimeProfile {
	target, ok := generator.profiles[generator.targetState]

	if !ok {
		target = generator.profiles[testtypes.Baseline]
	}

	return testtypes.Blend(generator.sourceProfile, target, generator.progress)
}

// Step generates the next relative sample frame based on current state.
func (generator *Generator) Step() testtypes.Sample {
	generator.mu.Lock()
	defer generator.mu.Unlock()

	/*
		Advance the regime transition before sampling, so each step reflects
		a market that is part way between the two regimes rather than
		snapping from one set of parameters to the other.
	*/
	precursor := generator.progress < 1.0

	if precursor {
		generator.progress = math.Min(1.0, generator.progress+
			generator.momentum/baselineTransitionTicks,
		)
	}

	profile := generator.activeProfile()

	// 1. Advance Time Cadence
	generator.currTime = generator.currTime.Add(profile.Cadence)

	// 2. Calculate Price Motion (Geometric Random Walk + Drift)
	dt := profile.Cadence.Seconds()
	noise := generator.rng.NormFloat64()
	deltaPct := (profile.Drift * dt * 0.01) + (profile.Volatility * math.Sqrt(dt) * noise * 0.01)

	/*
		The blended approach is the observable precursor: spread, flow, and
		volume migrate into their target regime without leaking the future gap.
		The first post-transition sample receives the full configured ignition;
		only its continuation tail decays.
	*/
	impulse := 0.0

	if !precursor && generator.burst > 0 {
		impulse = profile.IgnitionMove * generator.burst
		generator.burst *= profile.IgnitionDecay
	}

	generator.midPrice = math.Max(0.01, generator.midPrice*(1.0+deltaPct+impulse))

	// 3. Dynamic Spread & Bid/Ask Construction
	baseSpread := generator.midPrice * 0.0005
	actualSpread := math.Max(0.01, baseSpread*profile.SpreadScale)

	halfSpread := actualSpread / 2.0
	bid := math.Trunc((generator.midPrice-halfSpread)*100) / 100
	ask := math.Trunc((generator.midPrice+halfSpread)*100) / 100
	aggressorSide := profile.AggressorSide

	if aggressorSide == "" {
		aggressorSide = "buy"

		if deltaPct+impulse < 0 {
			aggressorSide = "sell"
		}
	}

	last := ask

	if aggressorSide == "sell" {
		last = bid
	}

	// 4. Calculate Asymmetric Quantities
	bidJitter := testtypes.QuantityJitterMinimum +
		(testtypes.QuantityJitterMaximum-testtypes.QuantityJitterMinimum)*
			generator.rng.Float64()
	askJitter := testtypes.QuantityJitterMinimum +
		(testtypes.QuantityJitterMaximum-testtypes.QuantityJitterMinimum)*
			generator.rng.Float64()
	bidQty := profile.BaseQty * profile.BidAskAsymmetry * bidJitter
	askQty := profile.BaseQty * (1.0 / profile.BidAskAsymmetry) * askJitter

	// 5. Volume & Cumulative VWAP
	volumeJitter := testtypes.VolumeJitterMinimum +
		(testtypes.VolumeJitterMaximum-testtypes.VolumeJitterMinimum)*
			generator.rng.Float64()
	stepVolume := profile.BaseQty * profile.VolumeScale * volumeJitter

	/*
		Volume leads price into an ignition, so the burst multiplies executed
		quantity on the same step the gap prints, then decays with it.
	*/
	if impulse != 0 && profile.IgnitionVolume > 1 {
		impulseFraction := math.Abs(impulse / profile.IgnitionMove)
		stepVolume *= 1.0 + (profile.IgnitionVolume-1.0)*impulseFraction
	}

	if generator.burst < 0.001 {
		generator.burst = 0
	}
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

	return testtypes.Sample{
		Symbol:        generator.symbol,
		AggressorSide: aggressorSide,
		Bid:           bid,
		BidQty:        math.Round(bidQty*100) / 100,
		Ask:           ask,
		AskQty:        math.Round(askQty*100) / 100,
		Last:          math.Round(last*100) / 100,
		Volume:        math.Round(generator.cumVolume*100) / 100,
		StepVolume:    math.Round(stepVolume*100) / 100,
		VWAP:          math.Round(vwap*100) / 100,
		Low:           math.Round(generator.lowPrice*100) / 100,
		High:          math.Round(generator.highPrice*100) / 100,
		Change:        math.Round(change*100) / 100,
		ChangePct:     math.Round(changePct*100) / 100,
		Timestamp:     generator.currTime,
	}
}

/*
Generate yields raw JSON []byte payload frames derived by updating the given template with signal steps.
*/
func (generator *Generator) Generate(template []byte) iter.Seq[[]byte] {
	return func(yield func([]byte) bool) {
		sample := generator.Step()
		frame := generator.Render(template, sample)

		if len(frame) > 0 {
			yield(frame)
		}
	}
}

/*
Render writes one already-sampled market state into a channel template.

Keeping sampling separate from rendering lets one venue tick publish the same
price, quantity, and timestamp through ticker, book, trade, and level3 rather
than advancing the simulated market once per channel.
*/
func (generator *Generator) Render(template []byte, sample testtypes.Sample) []byte {
	if len(template) == 0 {
		payload, _ := json.Marshal(map[string]any{
			"channel": "ticker",
			"type":    "update",
			"data":    []testtypes.Sample{sample},
		})

		return payload
	}

	var wire map[string]any

	if err := json.Unmarshal(template, &wire); err != nil {
		return template
	}

	channel, _ := wire["channel"].(string)
	wireType, _ := wire["type"].(string)
	stamp := sample.Timestamp.Format(time.RFC3339Nano)
	generator.sequence++

	if data, ok := wire["data"].([]any); ok && len(data) > 0 {
		if row, ok := data[0].(map[string]any); ok {
			row["symbol"] = sample.Symbol
			row["timestamp"] = stamp

			switch channel {
			case "book":
				row["bids"] = []any{map[string]any{
					"price": sample.Bid, "qty": sample.BidQty,
				}}
				row["asks"] = []any{map[string]any{
					"price": sample.Ask, "qty": sample.AskQty,
				}}
			case "level3":
				if wireType == "snapshot" {
					generator.l3Bid = 0
					generator.l3Ask = 0
				}

				orderSymbol := strings.NewReplacer("/", "-", ".", "-").
					Replace(sample.Symbol)
				bidOrders := make([]any, 0, 2)
				askOrders := make([]any, 0, 2)
				bidEvent := "add"
				askEvent := "add"

				if generator.l3Bid > 0 && generator.l3Bid != sample.Bid {
					bidOrders = append(bidOrders, map[string]any{
						"event":       "delete",
						"order_id":    "OBID-" + orderSymbol,
						"limit_price": generator.l3Bid,
						"order_qty":   generator.l3BidQty,
						"timestamp":   stamp,
					})
				}

				if generator.l3Ask > 0 && generator.l3Ask != sample.Ask {
					askOrders = append(askOrders, map[string]any{
						"event":       "delete",
						"order_id":    "OASK-" + orderSymbol,
						"limit_price": generator.l3Ask,
						"order_qty":   generator.l3AskQty,
						"timestamp":   stamp,
					})
				}

				if generator.l3Bid == sample.Bid {
					bidEvent = "modify"
				}

				if generator.l3Ask == sample.Ask {
					askEvent = "modify"
				}

				bidOrders = append(bidOrders, map[string]any{
					"event":       bidEvent,
					"order_id":    "OBID-" + orderSymbol,
					"limit_price": sample.Bid,
					"order_qty":   sample.BidQty,
					"timestamp":   stamp,
				})
				askOrders = append(askOrders, map[string]any{
					"event":       askEvent,
					"order_id":    "OASK-" + orderSymbol,
					"limit_price": sample.Ask,
					"order_qty":   sample.AskQty,
					"timestamp":   stamp,
				})
				row["bids"] = bidOrders
				row["asks"] = askOrders
				generator.l3Bid = sample.Bid
				generator.l3Ask = sample.Ask
				generator.l3BidQty = sample.BidQty
				generator.l3AskQty = sample.AskQty
			case "trade":
				row["side"] = sample.AggressorSide
				row["price"] = sample.Last

				/*
					A trade reports the quantity that actually executed, not
					the size resting on the bid. Ignition scores volume rate
					against its own median, so the executed quantity is the
					only figure that carries the surge.
				*/
				row["qty"] = sample.StepVolume
				row["ord_type"] = "limit"
				row["trade_id"] = generator.sequence
			default:
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
			}

			wire["data"] = []any{row}
		}
	}

	if wireType != "snapshot" {
		wire["type"] = "update"
	}
	payload, err := json.Marshal(wire)

	if err != nil {
		return template
	}

	return payload
}
