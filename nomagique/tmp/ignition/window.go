package equation

import (
	"math"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
ignitionWindow owns the bounded volume-clock history and held scores for one
symbol. Its target quantities, normalizers, and counter-evidence scales all
come from retained observations from that same symbol.
*/
type ignitionWindow struct {
	capacity int

	initialized bool
	classified  bool
	bars        int
	lastTime    time.Time
	haveTime    bool
	barVolume   float64
	barOpenTime time.Time
	prevClose   float64
	lastRVOL    float64

	deltas     []float64
	rates      []float64
	returns    []float64
	precursors []float64
	spreads    []float64
	cached     types.Map[string, types.Value[float64]]
}

/*
observe validates and advances one symbol without mixing its state into the
public multi-symbol coordinator.
*/
func (window *ignitionWindow) observe(
	input types.Map[string, types.Value[float64]],
) (types.Map[string, types.Value[float64]], bool, float64, error) {
	spread := input.Ask - input.Bid

	if !window.initialized {
		window.initialized = true
		window.prevClose = input["last"].Read()
		window.lastTime = input["at"].Read()
		window.barOpenTime = input["at"].Read()
		window.barVolume = input["volume"].Read()
		window.deltas = window.appendPositive(window.deltas, input["volume"].Read())
		window.haveTime = !input["at"].IsZero()
		window.spreads = window.appendPositive(window.spreads, spread)

		return window.compose(spread), window.ready(), window.maturity(), nil
	}

	if window.haveTime && !input["at"].IsZero() && input["at"].Before(window.lastTime) {
		return types.Map[string, types.Value[float64]]{}, false, 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"ignition: observation time cannot move backwards",
			nil,
		))
	}

	candidate := window.clone()

	if err := candidate.advance(input, spread); err != nil {
		return types.Map[string, types.Value[float64]]{}, false, 0, err
	}

	candidate.spreads = candidate.appendPositive(candidate.spreads, spread)
	*window = candidate

	return window.compose(spread), window.ready(), window.maturity(), nil
}

func (window *ignitionWindow) clone() ignitionWindow {
	candidate := *window
	candidate.deltas = append([]float64(nil), window.deltas...)
	candidate.rates = append([]float64(nil), window.rates...)
	candidate.returns = append([]float64(nil), window.returns...)
	candidate.precursors = append([]float64(nil), window.precursors...)
	candidate.spreads = append([]float64(nil), window.spreads...)

	return candidate
}

/*
advance accumulates executed quantity, advances event time, and closes at most
one indivisible observation into the empirical volume clock.
*/
func (window *ignitionWindow) advance(input types.Map[string, types.Value[float64]], spread float64) error {
	if !input["at"].IsZero() {
		window.lastTime = input["at"].Read()

		if !window.haveTime {
			window.haveTime = true
			window.barOpenTime = input["at"].Read()
		}
	}

	barTarget, targetReady := statistic.MedianOf(window.deltas)
	window.barVolume += input["volume"].Read()

	if !targetReady || barTarget <= 0 || window.barVolume < barTarget ||
		!window.haveTime || !input["at"].After(window.barOpenTime) {
		window.deltas = window.appendPositive(window.deltas, input["volume"].Read())

		return nil
	}

	if err := window.closeBar(
		input["last"].Read(),
		input["at"].Read(),
		window.barVolume,
		spread,
	); err != nil {
		return err
	}

	window.deltas = window.appendPositive(window.deltas, input["volume"].Read())
	window.barVolume = 0

	return nil
}

/*
closeBar scores an empirical volume bar, then retains it for later baselines.
*/
func (window *ignitionWindow) closeBar(
	closePrice float64,
	at time.Time,
	barVolume float64,
	spread float64,
) error {
	priceMove := math.Log(closePrice / window.prevClose)
	duration := at.Sub(window.barOpenTime).Seconds()

	if duration <= 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"ignition: volume bar requires positive elapsed event time",
			nil,
		))
	}

	barRate := barVolume / duration

	if err := window.score(barRate, priceMove, spread); err != nil {
		return err
	}

	window.rates = window.appendPositive(window.rates, barRate)
	window.returns = window.appendNonNegative(window.returns, math.Abs(priceMove))
	window.precursors = window.appendPositive(window.precursors, math.Abs(priceMove))
	window.prevClose = closePrice
	window.barOpenTime = at
	window.bars++

	return nil
}

/*
score derives one held event classification from prior empirical baselines. A
dependent score remains zero when any scale it needs has not yet been observed;
calculation failures are returned instead of being converted into evidence.
*/
func (window *ignitionWindow) score(
	barRate float64,
	priceMove float64,
	spread float64,
) error {
	rateBaseline, rateReady := statistic.MedianOf(window.rates)
	spreadBaseline, spreadReady := statistic.MedianOf(window.spreads)
	precursorBaseline, precursorReady := statistic.MedianOf(window.precursors)
	moveBaseline, moveReady := statistic.MedianOf(window.returns)

	rvol := ignitionRatio(barRate, rateBaseline, rateReady)
	buyPrecursor := ignitionRatio(
		math.Max(0, priceMove),
		precursorBaseline,
		precursorReady,
	)
	sellPrecursor := ignitionRatio(
		math.Max(0, -priceMove),
		precursorBaseline,
		precursorReady,
	)
	compression := 0.0

	if spreadReady && spreadBaseline > 0 {
		compression = math.Max(0, 1-spread/spreadBaseline)
	}

	buyRejection := ignitionRatio(math.Max(0, -priceMove), moveBaseline, moveReady)
	sellRejection := ignitionRatio(math.Max(0, priceMove), moveBaseline, moveReady)
	rvolScale := ignitionRatioScale(window.rates, rateBaseline)
	precursorScale := ignitionRatioScale(window.precursors, precursorBaseline)
	compressionScale := ignitionCompressionScale(window.spreads, spreadBaseline)
	buy, err := ignitionFamilies(
		rvol,
		buyPrecursor,
		compression,
		rvolScale,
		precursorScale,
		compressionScale,
	)

	if err != nil {
		return err
	}

	sell, err := ignitionFamilies(
		rvol,
		sellPrecursor,
		compression,
		rvolScale,
		precursorScale,
		compressionScale,
	)

	if err != nil {
		return err
	}

	buy.Exhaustion = ignitionExhaustion(
		window.lastRVOL,
		rvol,
		buyRejection,
		ignitionRatioScale(window.returns, moveBaseline),
	)

	buy.Strength = max(
		buy.Ignition,
		buy.Compression,
		buy.Trend,
		buy.Exhaustion,
	)

	buy.Value = buy.Strength
	
	sell.Exhaustion = ignitionExhaustion(
		window.lastRVOL,
		rvol,
		sellRejection,
		ignitionRatioScale(window.returns, moveBaseline),
	)
	
	sell.Strength = max(
		sell.Ignition,
		sell.Compression,
		sell.Trend,
		sell.Exhaustion,
	)
	
	sell.Value = sell.Strength
	legacy := buy

	if sell.Strength > buy.Strength {
		legacy = sell
	}

	output := types.Map[string, types.Value[float64]]{
		"value":       types.NewValue(legacy.Value),
		"rvol":        types.NewValue(rvol),
		"precursor":   legacy.Precursor,
		"compression": legacy.Compression,
		"ignition":    legacy.Ignition,
		"trend":       legacy.Trend,
		"exhaustion":  legacy.Exhaustion,
		"strength":    legacy.Strength,
		"category":    legacy.Category,
		"buy":         types.NewValue(buy),
		"sell":        types.NewValue(sell),
	}

	if math.IsNaN(output["strength"].Read().(float64)) || math.IsInf(output["strength"].Read().(float64), 0) {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"ignition: calculated strength must be finite",
			nil,
		))
	}

	window.lastRVOL = rvol
	window.cached = output
	window.classified = rateReady && spreadReady

	return nil
}

/*
compose overlays the current executable spread on the last closed-bar scores.
*/
func (window *ignitionWindow) compose(spread float64) types.Map[string, types.Value[float64]] {
	output := window.cached
	output.Spread = spread

	return output
}

/*
ready reports that a causal volume bar and live spread history exist.
*/
func (window *ignitionWindow) ready() bool {
	return window.classified
}

/*
maturity reports a bounded closed-bar ratio without a configured horizon.
*/
func (window *ignitionWindow) maturity() float64 {
	return float64(window.bars) / float64(window.bars+1)
}

/*
appendPositive retains a finite positive baseline sample.
*/
func (window *ignitionWindow) appendPositive(
	values []float64,
	value float64,
) []float64 {
	if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return values
	}

	return window.append(values, value)
}

/*
appendNonNegative retains a finite baseline sample where zero is meaningful.
*/
func (window *ignitionWindow) appendNonNegative(
	values []float64,
	value float64,
) []float64 {
	if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return values
	}

	return window.append(values, value)
}

/*
append bounds retained history to the market feed's explicit capacity.
*/
func (window *ignitionWindow) append(values []float64, value float64) []float64 {
	if len(values) < window.capacity {
		return append(values, value)
	}

	copy(values, values[1:])
	values[len(values)-1] = value

	return values
}
