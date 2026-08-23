package signal

import (
	"math"
	"math/rand"
	"sync"
	"time"

	tes "github.com/theapemachine/symm/tests/types"
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
	currentState tes.MarketState
	targetState  tes.MarketState
	momentum     float64

	/*
		sourceProfile is the regime the market is transitioning away from and
		progress runs from 0 to 1 as it settles into the target regime.
	*/
	sourceProfile tes.RegimeProfile
	progress      float64

	/*
		burst is the unspent fraction of the ignition impulse that opens the
		current regime. It remains fully armed while the blended precursor is
		sampled, then decays only after the first ignition sample.
	*/
	burst float64

	profiles       map[tes.MarketState]tes.RegimeProfile
	midPrice       float64
	trendPrice     float64
	openPrice      float64
	priceIncrement float64
	pricePrecision int
	quantityScale  float64
	spreadFraction float64
	factorLoading  float64
	depthLevels    int
	depthScale     float64
	cumVolume      float64
	cumValue       float64
	highPrice      float64
	lowPrice       float64
	currTime       time.Time
	steps          int64
	sequence       int64
	l3Bids         []tes.DepthLevel
	l3Asks         []tes.DepthLevel
}

func NewGenerator(
	symbol string,
	startPrice float64,
	priceIncrement float64,
	pricePrecision int,
	seed int64,
) *Generator {
	profiles := tes.CloneProfiles(tes.DefaultProfiles)
	return &Generator{
		rng:          rand.New(rand.NewSource(seed)),
		symbol:       symbol,
		currentState: tes.Baseline,
		targetState:  tes.Baseline,
		profiles:     profiles,

		/*
			A new generator is already settled in the baseline regime, so the
			transition starts complete.
		*/
		sourceProfile: profiles[tes.Baseline],
		progress:      1.0,
		momentum:      1.0,

		midPrice:       startPrice,
		trendPrice:     startPrice,
		openPrice:      startPrice,
		priceIncrement: priceIncrement,
		pricePrecision: pricePrecision,
		quantityScale:  math.Pow10(tes.DefaultQuantityPrecision),
		spreadFraction: tes.DefaultBaseSpreadFraction,
		depthLevels:    tes.DefaultBookDepthLevels,
		depthScale:     tes.DefaultBookDepthQuantityScale,
		highPrice:      startPrice,
		lowPrice:       startPrice,
		currTime:       time.Now().UTC(),
	}
}

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
func (generator *Generator) SetState(state tes.MarketState, momentum ...float64) {
	generator.mu.Lock()
	defer generator.mu.Unlock()
	speed := generator.transitionMomentum(state, momentum)

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
IgnitionSpent reports whether the target regime's discontinuous event has been
sampled and its continuation has fully decayed.

PrecursorPending marks the beginning of the move a regime describes and this
marks its end, so a test that has to observe what the whole move produced can
run to it rather than guessing at a number of ticks. A regime that configures no
ignition has nothing to spend and is complete as soon as it has settled.
*/
func (generator *Generator) IgnitionSpent() bool {
	generator.mu.RLock()
	defer generator.mu.RUnlock()

	profile := generator.profiles[generator.targetState]

	return generator.progress >= 1.0 &&
		(profile.IgnitionMove == 0 || generator.burst == 0)
}

/*
activeProfile returns the profile currently in effect, which mid-transition
is the blend between the source and target regimes.
*/
func (generator *Generator) activeProfile() tes.RegimeProfile {
	target := generator.profiles[generator.targetState]

	return tes.Blend(generator.sourceProfile, target, generator.progress)
}

/*
Step generates the next relative sample frame based on current state. An
optional shared standard-normal shock creates configured positive, negative,
or independent cross-symbol returns without exposing a regime label.
*/
func (generator *Generator) Step(sharedShock ...float64) tes.Sample {
	generator.mu.Lock()
	defer generator.mu.Unlock()

	/*
		Advance the regime transition before sampling, so each step reflects
		a market that is part way between the two regimes rather than
		snapping from one set of parameters to the other.
	*/
	precursor := generator.progress < 1.0

	if precursor {
		transitionObservations := generator.profiles[tes.Baseline].
			Precursor.MinimumObservations
		targetObservations := generator.profiles[generator.targetState].
			Precursor.MinimumObservations

		if targetObservations > 0 {
			transitionObservations = targetObservations
		}

		generator.progress = math.Min(1.0, generator.progress+
			generator.momentum/float64(transitionObservations),
		)
	}

	profile := generator.activeProfile()
	generator.steps++

	// 1. Advance Time Cadence
	generator.currTime = generator.currTime.Add(profile.Cadence)

	// 2. Calculate a latent trend with stationary observation noise.
	dt := profile.Cadence.Seconds()
	marketShock := 0.0

	if len(sharedShock) > 0 {
		marketShock = sharedShock[0]
	}

	idiosyncraticShock := generator.rng.NormFloat64()
	idiosyncraticWeight := math.Sqrt(
		math.Max(0, 1-math.Pow(generator.factorLoading, 2)),
	)
	noise := generator.factorLoading*marketShock +
		idiosyncraticWeight*idiosyncraticShock
	driftPct := profile.Drift * dt * 0.01
	diffusionPct := profile.Diffusion * math.Sqrt(dt) * noise * 0.01
	noisePct := profile.Volatility * math.Sqrt(dt) * noise * 0.01

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

	meanReversionPct := profile.MeanReversion * dt *
		(generator.openPrice/generator.trendPrice - 1)
	oscillation := profile.OscillationMove

	if generator.steps%2 == 0 {
		oscillation = -oscillation
	}

	generator.trendPrice = math.Max(
		generator.priceIncrement,
		generator.trendPrice*(1.0+driftPct+diffusionPct+
			meanReversionPct+oscillation+impulse),
	)
	generator.midPrice = math.Max(
		generator.priceIncrement,
		generator.trendPrice*(1.0+noisePct),
	)

	// 3. Dynamic Spread & Bid/Ask Construction
	baseSpread := generator.midPrice * generator.spreadFraction
	spreadNoise := math.Exp(
		profile.SpreadJitter * math.Sqrt(dt) * generator.rng.NormFloat64(),
	)
	actualSpread := math.Max(
		generator.priceIncrement,
		baseSpread*profile.SpreadScale*spreadNoise,
	)

	spreadTicks := math.Max(
		1, math.Round(actualSpread/generator.priceIncrement),
	)
	quotedSpread := spreadTicks * generator.priceIncrement
	halfSpread := quotedSpread / 2.0
	bid := generator.floorPrice(generator.midPrice - halfSpread)
	ask := generator.roundPrice(bid + quotedSpread)

	if bid < generator.priceIncrement {
		bid = generator.priceIncrement
	}

	if ask <= bid {
		ask = generator.roundPrice(bid + generator.priceIncrement)
	}

	aggressorSide := profile.AggressorSide

	if aggressorSide == "" {
		aggressorSide = "buy"

		movement := driftPct + diffusionPct + meanReversionPct +
			oscillation + noisePct + impulse

		if movement < 0 {
			aggressorSide = "sell"
		}

		if movement == 0 && generator.rng.Intn(2) == 0 {
			aggressorSide = "sell"
		}
	}

	last := ask

	if aggressorSide == "sell" {
		last = bid
	}

	// 4. Calculate Asymmetric Quantities
	bidJitter := tes.QuantityJitterMinimum +
		(tes.QuantityJitterMaximum-tes.QuantityJitterMinimum)*
			generator.rng.Float64()
	askJitter := tes.QuantityJitterMinimum +
		(tes.QuantityJitterMaximum-tes.QuantityJitterMinimum)*
			generator.rng.Float64()
	bidQty := profile.BaseQty * profile.BidAskAsymmetry * bidJitter
	askQty := profile.BaseQty * (1.0 / profile.BidAskAsymmetry) * askJitter

	// 5. Volume & Cumulative VWAP
	volumeJitter := tes.VolumeJitterMinimum +
		(tes.VolumeJitterMaximum-tes.VolumeJitterMinimum)*
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
	roundedBidQuantity := generator.roundQuantity(bidQty)
	roundedAskQuantity := generator.roundQuantity(askQty)
	bids, asks := generator.depth(bid, bidQty, ask, askQty)

	return tes.Sample{
		Symbol:        generator.symbol,
		AggressorSide: aggressorSide,
		Bid:           bid,
		BidQty:        roundedBidQuantity,
		Ask:           ask,
		AskQty:        roundedAskQuantity,
		Last:          generator.roundPrice(last),
		Volume:        generator.roundQuantity(generator.cumVolume),
		StepVolume:    generator.roundQuantity(stepVolume),
		VWAP:          generator.roundPrice(vwap),
		Low:           generator.roundPrice(generator.lowPrice),
		High:          generator.roundPrice(generator.highPrice),
		Change:        generator.roundPrice(change),
		ChangePct:     math.Round(changePct*100) / 100,
		Timestamp:     generator.currTime,
		Bids:          bids,
		Asks:          asks,
	}
}
