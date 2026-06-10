package broker

import (
	"sync/atomic"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/market"
)

/*
RiskContext carries trading limits read once per order so hot paths avoid viper
or mutex access during sizing and pre-trade checks.
*/
type RiskContext struct {
	PositionFraction       float64
	MaxSlippageBps         float64
	MaxSpreadBps           float64
	MaxQuoteAge            time.Duration
	MaxConcurrentPositions int
	NoneSizeScale          float64
	DeadSizeScale          float64
	TrendingSizeScale      float64
	BullishSizeScale       float64
	ChoppySizeScale        float64
	BearishSizeScale       float64
}

var currentRiskContext atomic.Pointer[RiskContext]

/*
LoadRiskContext builds a snapshot from the active viper configuration.
*/
func LoadRiskContext() RiskContext {
	return RiskContext{
		PositionFraction:       viper.GetFloat64("trading.position_fraction"),
		MaxSlippageBps:         viper.GetFloat64("trading.max_slippage_bps"),
		MaxSpreadBps:           viper.GetFloat64("trading.max_spread_bps"),
		MaxQuoteAge:            viper.GetDuration("trading.max_quote_age"),
		MaxConcurrentPositions: viper.GetInt("trading.max_concurrent_positions"),
		NoneSizeScale:          viper.GetFloat64("trading.replay.none_size_scale"),
		DeadSizeScale:          viper.GetFloat64("trading.replay.dead_size_scale"),
		TrendingSizeScale:      viper.GetFloat64("trading.replay.trending_size_scale"),
		BullishSizeScale:       viper.GetFloat64("trading.replay.bullish_size_scale"),
		ChoppySizeScale:        viper.GetFloat64("trading.replay.choppy_size_scale"),
		BearishSizeScale:       viper.GetFloat64("trading.replay.bearish_size_scale"),
	}
}

/*
RefreshRiskContext stores the latest configuration snapshot for readers.
*/
func RefreshRiskContext() {
	loaded := LoadRiskContext()
	currentRiskContext.Store(&loaded)
}

/*
CurrentRiskContext returns the latest risk snapshot.
*/
func CurrentRiskContext() RiskContext {
	pointer := currentRiskContext.Load()

	if pointer == nil {
		loaded := LoadRiskContext()
		currentRiskContext.Store(&loaded)

		return loaded
	}

	return *pointer
}

/*
EntryScaleForRegime selects the configured size scale for the dominant regime axis.
*/
func (risk RiskContext) EntryScaleForRegime(mean market.RegimeStrengths) float64 {
	if mean.Choppiness >= mean.Trend &&
		mean.Choppiness >= mean.Bullish &&
		mean.Choppiness >= mean.Bearish {
		return risk.ChoppySizeScale
	}

	if mean.Bearish >= mean.Bullish && mean.Bearish >= mean.Trend {
		return risk.BearishSizeScale
	}

	if mean.Bullish >= mean.Trend {
		return risk.BullishSizeScale
	}

	if mean.Trend > 0 {
		return risk.TrendingSizeScale
	}

	if mean.Volatility <= 0 {
		return risk.NoneSizeScale
	}

	return risk.DeadSizeScale
}
