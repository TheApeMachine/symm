package market

import (
	"math"
	"time"

	"github.com/theapemachine/symm/nomagique/data"
)

/*
ExcursionProfile defines the geometric and economic structure of a synthetic market tape.
It allows creating deterministic, realistic market sequences including baseline precursor
consolidation, directional ignition, noisy runs with flash wicks (stop-loss hunts),
confirmed structural reversals, and post-excursion tail margins.
*/
type ExcursionProfile struct {
	Symbol          string
	BasePrice       float64
	Spread          float64
	PrecursorTicks  int
	RunTicks        int
	PeakReturn      float64
	Wicks           map[int]float64 // map of relative run tick index -> price multiplier (e.g. 0.96 for a 4% wick down)
	ReversalTicks   int
	ReversalReturn  float64
	TailTicks       int
	MicroNoiseRatio float64 // Bid-ask bouncing magnitude relative to spread
}

/*
GenerateTape constructs a multi-leg sequence of measurements reflecting the specified
excursion dynamics with realistic timestamps and Level 3 metrics.
*/
func (profile ExcursionProfile) GenerateTape(startSeq int64) []*data.Measurement[float64] {
	totalTicks := profile.PrecursorTicks + profile.RunTicks + profile.ReversalTicks + profile.TailTicks
	frames := make([]*data.Measurement[float64], 0, totalTicks)

	currentSeq := startSeq
	eventTime := time.Unix(1700000000, 0)

	emit := func(p float64, source string) {
		m := data.NewMeasurement[float64](source, nil)
		m.SeqIdx = currentSeq
		m.Label = profile.Symbol
		m.At = eventTime
		halfSpread := profile.Spread / 2
		bid := p - halfSpread
		ask := p + halfSpread
		m.WriteStandardized("price", p)
		m.WriteStandardized("spread", profile.Spread)
		m.WriteStandardized("bid", bid)
		m.WriteStandardized("ask", ask)
		frames = append(frames, m)
		currentSeq++
		eventTime = eventTime.Add(100 * time.Millisecond)
	}

	// 1. Precursor consolidation (Point A)
	for i := 0; i < profile.PrecursorTicks; i++ {
		noise := (float64((i%3)-1) * profile.Spread * profile.MicroNoiseRatio)
		emit(profile.BasePrice+noise, "precursor")
	}

	// 2. Active Run (Point B Ignition -> Peak)
	stepReturn := profile.PeakReturn / float64(max(1, profile.RunTicks))
	peakPrice := profile.BasePrice

	for i := 1; i <= profile.RunTicks; i++ {
		idealPrice := profile.BasePrice * (1.0 + stepReturn*float64(i))
		price := idealPrice

		// Apply adversarial wick if declared at this step
		if mult, isWick := profile.Wicks[i]; isWick && mult > 0 {
			price = idealPrice * mult
		} else {
			noise := (float64((i%2)*2-1) * profile.Spread * 0.2)
			price += noise
			if price > peakPrice {
				peakPrice = price
			}
		}

		emit(price, "run")
	}

	// 3. Exhaustion & Structural Reversal (Point C)
	revStep := profile.ReversalReturn / float64(max(1, profile.ReversalTicks))
	reversalBase := peakPrice

	for i := 1; i <= profile.ReversalTicks; i++ {
		price := reversalBase * (1.0 - revStep*float64(i))
		emit(price, "reversal")
	}

	// 4. Tail margin (Point D)
	finalPrice := reversalBase * (1.0 - profile.ReversalReturn)
	for i := 0; i < profile.TailTicks; i++ {
		noise := (float64((i%3)-1) * profile.Spread * 0.2)
		emit(finalPrice+noise, "tail")
	}

	return frames
}

/*
NewProfitableUpperTape generates an upward breakout that comfortably clears round-trip friction
for a $40 position (+3.5% peak return, well above 0.52% fee + spread), with a single-tick stop-loss
hunter sweep (-3% wick) that immediately rebounds to new highs.
*/
func NewProfitableUpperTape(symbol string, basePrice, spread float64) []*data.Measurement[float64] {
	return ExcursionProfile{
		Symbol:         symbol,
		BasePrice:      basePrice,
		Spread:         spread,
		PrecursorTicks: 4,
		RunTicks:       8,
		PeakReturn:     0.035, // +3.5%
		Wicks: map[int]float64{
			4: 0.97, // 3% stop-loss sweep wick at step 4
		},
		ReversalTicks:   4,
		ReversalReturn:  0.02, // Retraces 2% from peak
		TailTicks:       4,
		MicroNoiseRatio: 0.3,
	}.GenerateTape(1)
}

/*
NewUnprofitableUpperTape generates a weak upward ignition that exhausts after gaining only +0.15%,
failing to cover round-trip taker fees (0.52%), followed by structural reversal.
*/
func NewUnprofitableUpperTape(symbol string, basePrice, spread float64) []*data.Measurement[float64] {
	return ExcursionProfile{
		Symbol:          symbol,
		BasePrice:       basePrice,
		Spread:          spread,
		PrecursorTicks:  4,
		RunTicks:        4,
		PeakReturn:      0.0015, // +0.15% (unprofitable on $40 after 52 bps fees)
		ReversalTicks:   4,
		ReversalReturn:  0.0025,
		TailTicks:       4,
		MicroNoiseRatio: 0.1,
	}.GenerateTape(1)
}

/*
NewDownwardBreakdownTape generates a downward departure breaking below lower hurdles.
*/
func NewDownwardBreakdownTape(symbol string, basePrice, spread float64) []*data.Measurement[float64] {
	return ExcursionProfile{
		Symbol:         symbol,
		BasePrice:      basePrice,
		Spread:         spread,
		PrecursorTicks: 4,
		RunTicks:       6,
		PeakReturn:     -0.03, // -3.0% drop
		Wicks: map[int]float64{
			3: 1.02, // 2% short-squeeze wick upward
		},
		ReversalTicks:   4,
		ReversalReturn:  -0.015,
		TailTicks:       4,
		MicroNoiseRatio: 0.2,
	}.GenerateTape(1)
}

/*
NewChopWhipsawTape generates high-frequency oscillating noise that constantly flips direction
within the friction deadband without ever sustaining a directional breakout.
*/
func NewChopWhipsawTape(symbol string, basePrice, spread float64, ticks int) []*data.Measurement[float64] {
	frames := make([]*data.Measurement[float64], ticks)
	eventTime := time.Unix(1700000000, 0)

	for i := range ticks {
		// Oscillate between +0.8*spread and -0.8*spread
		oscillation := math.Sin(float64(i)*1.5) * spread * 0.8
		p := basePrice + oscillation

		m := data.NewMeasurement[float64]("chop", nil)
		m.SeqIdx = int64(i + 1)
		m.Label = symbol
		m.At = eventTime
		m.WriteStandardized("price", p)
		m.WriteStandardized("spread", spread)
		frames[i] = m
		eventTime = eventTime.Add(50 * time.Millisecond)
	}

	return frames
}

/*
NewFlatQuiescentTape generates a flatline tape where price remains virtually motionless.
*/
func NewFlatQuiescentTape(symbol string, basePrice, spread float64, ticks int) []*data.Measurement[float64] {
	frames := make([]*data.Measurement[float64], ticks)
	eventTime := time.Unix(1700000000, 0)

	for i := range ticks {
		m := data.NewMeasurement[float64]("flat", nil)
		m.SeqIdx = int64(i + 1)
		m.Label = symbol
		m.At = eventTime
		m.WriteStandardized("price", basePrice)
		m.WriteStandardized("spread", spread)
		frames[i] = m
		eventTime = eventTime.Add(100 * time.Millisecond)
	}

	return frames
}

/*
NewAdversarialMultiWickTape generates a strong upward trending excursion that encounters
MULTIPLE successive adversarial stop-loss hunter wicks (e.g. at step 3, step 6, step 9),
each punching down sharply before the tape immediately recovers to new higher highs.
*/
func NewAdversarialMultiWickTape(symbol string, basePrice, spread float64) []*data.Measurement[float64] {
	return ExcursionProfile{
		Symbol:         symbol,
		BasePrice:      basePrice,
		Spread:         spread,
		PrecursorTicks: 4,
		RunTicks:       12,
		PeakReturn:     0.05, // +5.0%
		Wicks: map[int]float64{
			3: 0.98, // First sweep: -2%
			7: 0.97, // Second sweep: -3%
			9: 0.97, // Third sweep: -3%
		},
		ReversalTicks:   5,
		ReversalReturn:  0.03, // Sustained reversal of 3%
		TailTicks:       5,
		MicroNoiseRatio: 0.2,
	}.GenerateTape(1)
}

/*
NewInterleavedMultiAssetTape interleaves observations from multiple assets with contrasting
regimes into a single deterministic stream to stress test concurrent state isolation.
*/
func NewInterleavedMultiAssetTape() []*data.Measurement[float64] {
	const targetLen = 36
	btcBase := NewProfitableUpperTape("BTC/USD", 60000.0, 5.0)
	ethTape := NewFlatQuiescentTape("ETH/USD", 3000.0, 1.0, targetLen)
	solBase := NewDownwardBreakdownTape("SOL/USD", 150.0, 0.2)

	btcTape := make([]*data.Measurement[float64], targetLen)
	copy(btcTape, btcBase)
	lastBTC := btcBase[len(btcBase)-1]
	for i := len(btcBase); i < targetLen; i++ {
		m := CloneTestMeasurement(lastBTC)
		m.SeqIdx = int64(i + 1)
		btcTape[i] = m
	}

	solTape := make([]*data.Measurement[float64], targetLen)
	copy(solTape, solBase)
	lastSOL := solBase[len(solBase)-1]
	for i := len(solBase); i < targetLen; i++ {
		m := CloneTestMeasurement(lastSOL)
		m.SeqIdx = int64(i + 1)
		solTape[i] = m
	}

	interleaved := make([]*data.Measurement[float64], 0, targetLen*3)
	var globalSeq int64 = 1
	for i := range targetLen {
		fBTC := CloneTestMeasurement(btcTape[i])
		fBTC.SeqIdx = globalSeq
		globalSeq++

		fETH := CloneTestMeasurement(ethTape[i])
		fETH.SeqIdx = globalSeq
		globalSeq++

		fSOL := CloneTestMeasurement(solTape[i])
		fSOL.SeqIdx = globalSeq
		globalSeq++

		interleaved = append(interleaved, fBTC, fETH, fSOL)
	}

	return interleaved
}

func CloneTestMeasurement(src *data.Measurement[float64]) *data.Measurement[float64] {
	if src == nil {
		return nil
	}
	out := &data.Measurement[float64]{
		ID:         src.ID,
		Label:      src.Label,
		Source:     src.Source,
		SeqIdx:     src.SeqIdx,
		Timestamp:  src.Timestamp,
		At:         src.At,
		From:       src.From,
		Maturity:   src.Maturity,
		SNR:        src.SNR,
		SNRDefined: src.SNRDefined,
		Estimated:  src.Estimated,
		Err:        src.Err,
		Metrics:    make([]data.MetricEntry[float64], len(src.Metrics)),
		Metadata:   make([]data.StringEntry, len(src.Metadata)),
		Provenance: make([]data.StringEntry, len(src.Provenance)),
		Peers:      make([]*data.Measurement[float64], len(src.Peers)),
		Result:     src.Result,
	}
	copy(out.Metrics, src.Metrics)
	copy(out.Metadata, src.Metadata)
	copy(out.Provenance, src.Provenance)
	copy(out.Peers, src.Peers)
	return out
}

/*
NewFastPumpTape generates a sudden, violent upward breakout (+15% jump in 2 ticks)
following a 16-tick precursor consolidation, holding the higher level with a confirmed
reversal at the new plateau.
*/
func NewFastPumpTape(symbol string, basePrice, spread, pumpPct float64) []*data.Measurement[float64] {
	return ExcursionProfile{
		Symbol:          symbol,
		BasePrice:       basePrice,
		Spread:          spread,
		PrecursorTicks:  16,
		RunTicks:        2,
		PeakReturn:      pumpPct,
		ReversalTicks:   3,
		ReversalReturn:  pumpPct * 0.15,
		TailTicks:       4,
		MicroNoiseRatio: 0.1,
	}.GenerateTape(1)
}

/*
NewFlashSpikeDumpTape generates a 1-tick violent explosion (+20%) that immediately collapses
back to the baseline within 2 ticks (a pump-and-dump / flash spike / fat finger).
*/
func NewFlashSpikeDumpTape(symbol string, basePrice, spread, spikePct float64) []*data.Measurement[float64] {
	return ExcursionProfile{
		Symbol:          symbol,
		BasePrice:       basePrice,
		Spread:          spread,
		PrecursorTicks:  16,
		RunTicks:        1,
		PeakReturn:      spikePct,
		ReversalTicks:   1,
		ReversalReturn:  spikePct * 1.05,
		TailTicks:       4,
		MicroNoiseRatio: 0.1,
	}.GenerateTape(1)
}
