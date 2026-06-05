package perspectives

import (
	"math"
	"sync"

	"github.com/spf13/viper"
)

/*
RegimeFeatures is the price-action character of one symbol over a recent window,
derived only from the prices carried on past measurements (Measurement.Last).

It carries no lookahead: every value is a function of measurements at or before
the decision tick. Because the live story and the optimizer replay both feed
ClassifyRegime the same market.RingSnapshot output, the classification is
identical on both paths by construction — a regime edge the optimizer discovers
in replay reproduces exactly when traded live.

The continuous fields double as the dashboard's regime radar axes.
*/
type RegimeFeatures struct {
	Regime        Regime  // the discrete state branches gate on
	Volatility    float64 // realized vol: stdev of the distinct-price log returns
	Drift         float64 // net log return over the window (signed: >0 up, <0 down)
	TrendStrength float64 // |t-stat| of the mean step return — drift signal vs. noise
	Choppiness    float64 // 0..1 movement-without-direction score (0 when dead)
	Samples       int     // price-bearing measurements observed in the window
}

/*
regimeConfig holds the classifier thresholds. They are read from viper once and
then treated as immutable for the process lifetime; live boot loads config before
systems start, and replay/tune should not pay Viper lookup cost on every tick.

The thresholds are cadence-relative: realized volatility and the trend t-stat
are measured per price change, not per unit time. Live and replay share the same
measurement cadence (both window the same RingSnapshot), so the thresholds mean
the same thing on both paths. trend < strongTrend gives the conviction ladder
Choppy -> Trending -> Bullish/Bearish.
*/
type regimeConfig struct {
	window      int     // most-recent measurements to classify over
	minSamples  int     // price-bearing measurements required before we claim a regime
	deadVol     float64 // realized-vol floor below which a non-trending market is Dead
	trend       float64 // trend t-stat for RegimeTrending
	strongTrend float64 // trend t-stat for signed RegimeBullish/RegimeBearish
}

var (
	cachedRegimeConfig regimeConfig
	regimeConfigOnce   sync.Once
)

func loadRegimeConfig() regimeConfig {
	regimeConfigOnce.Do(func() {
		cachedRegimeConfig = readRegimeConfig()
	})

	return cachedRegimeConfig
}

func readRegimeConfig() regimeConfig {
	return regimeConfig{
		window:      viperIntDefault("regime.window", 256),
		minSamples:  viperIntDefault("regime.min_samples", 16),
		deadVol:     viperFloatDefault("regime.dead_vol", 0.0005),
		trend:       viperFloatDefault("regime.trend_threshold", 1.5),
		strongTrend: viperFloatDefault("regime.strong_trend", 3.0),
	}
}

/*
RegimeWindow returns the configured price-action window used by live and replay
components that need the same recent distinct-price horizon as ClassifyRegime.
*/
func RegimeWindow() int {
	return loadRegimeConfig().window
}

func viperFloatDefault(key string, fallback float64) float64 {
	if value := viper.GetFloat64(key); value > 0 {
		return value
	}

	return fallback
}

func viperIntDefault(key string, fallback int) int {
	if value := viper.GetInt(key); value > 0 {
		return value
	}

	return fallback
}

/*
ClassifyRegime derives the current price-action regime for a symbol from the
ordered measurement snapshot (oldest to newest) returned by market.RingSnapshot.

Every signal source stamps the latest traded price onto its Measurement.Last, so
the snapshot repeats each price many times between trades. We collapse it to the
distinct price path before taking any return, so a market that is quoting but not
trading reads as Dead rather than as low-but-nonzero noise.
*/
func ClassifyRegime(snapshots []Measurement) RegimeFeatures {
	cfg := loadRegimeConfig()

	if len(snapshots) > cfg.window {
		snapshots = snapshots[len(snapshots)-cfg.window:]
	}

	samples := 0
	stats := distinctReturnStats{}

	for _, measurement := range snapshots {
		if measurement.Last <= 0 {
			continue
		}

		samples++
		stats.observe(measurement.Last)
	}

	features := RegimeFeatures{Samples: samples}

	// Not enough observed price data to claim a regime honestly.
	if samples < cfg.minSamples {
		features.Regime = RegimeNone

		return features
	}

	// Enough activity observed but the price never moved: a dead, flat market.
	if stats.returns == 0 {
		features.Regime = RegimeDead

		return features
	}

	vol := stats.volatility()
	drift := stats.drift()

	features.Volatility = vol
	features.Drift = drift

	switch {
	case vol > 0:
		// How many standard errors the mean step return sits from zero.
		features.TrendStrength = math.Abs(stats.mean) * math.Sqrt(float64(stats.returns)) / vol
	case drift != 0:
		// Perfectly steady geometric move: zero return variance, maximal trend.
		features.TrendStrength = cfg.strongTrend
	}

	switch {
	case features.TrendStrength >= cfg.strongTrend:
		if drift >= 0 {
			features.Regime = RegimeBullish
		} else {
			features.Regime = RegimeBearish
		}
	case features.TrendStrength >= cfg.trend:
		features.Regime = RegimeTrending
	case vol < cfg.deadVol:
		features.Regime = RegimeDead
	default:
		features.Regime = RegimeChoppy
	}

	if features.Regime != RegimeDead {
		features.Choppiness = 1 - math.Min(1, features.TrendStrength/cfg.strongTrend)
	}

	return features
}

/*
Radar normalizes the regime features to 0..1 axes for the dashboard's regime
radar: Volatility, Trend, Bullish, Bearish, Choppiness. Bullish/Bearish are the
signed split of Trend, so the shape reads at a glance — a trending-up market
spikes Trend + Bullish, a range spikes Volatility + Choppiness, a dead market
sits near the origin.
*/
func (features RegimeFeatures) Radar() map[string]float64 {
	cfg := loadRegimeConfig()
	volScale := viperFloatDefault("regime.vol_scale", 0.01)

	trend := math.Min(1, features.TrendStrength/cfg.strongTrend)
	bullish := 0.0
	bearish := 0.0

	switch {
	case features.Drift > 0:
		bullish = trend
	case features.Drift < 0:
		bearish = trend
	}

	return map[string]float64{
		"volatility": math.Min(1, features.Volatility/volScale),
		"trend":      trend,
		"bullish":    bullish,
		"bearish":    bearish,
		"choppiness": features.Choppiness,
	}
}

/*
DistinctPriceVolatility reports the realized volatility of the distinct price path.
Repeated prices are collapsed before log returns are measured, matching the regime
classifier so quote-cache, live desk, and replay trigger math share one definition.
*/
func DistinctPriceVolatility(prices []float64) float64 {
	stats := distinctReturnStats{}

	for _, price := range prices {
		stats.observe(price)
	}

	return stats.volatility()
}

type distinctReturnStats struct {
	firstPrice    float64
	lastPrice     float64
	previousPrice float64
	mean          float64
	sumSquares    float64
	returns       int
}

func (stats *distinctReturnStats) observe(price float64) {
	if price <= 0 {
		return
	}

	if stats.previousPrice == 0 {
		stats.firstPrice = price
		stats.lastPrice = price
		stats.previousPrice = price

		return
	}

	if price == stats.previousPrice {
		return
	}

	logReturn := math.Log(price / stats.previousPrice)
	stats.returns++

	delta := logReturn - stats.mean
	stats.mean += delta / float64(stats.returns)
	stats.sumSquares += delta * (logReturn - stats.mean)
	stats.lastPrice = price
	stats.previousPrice = price
}

func (stats distinctReturnStats) volatility() float64 {
	if stats.returns < 2 {
		return 0
	}

	return math.Sqrt(stats.sumSquares / float64(stats.returns-1))
}

func (stats distinctReturnStats) drift() float64 {
	if stats.firstPrice <= 0 || stats.lastPrice <= 0 {
		return 0
	}

	return math.Log(stats.lastPrice / stats.firstPrice)
}
