package data

import "strings"

/*
Unit describes the physical dimension of a measured value. It grounds numbers
in their honest physical reality: currency, price, size, time, rate, or information.
*/
type Unit string

const (
	// Dimensionless & Relative
	UnitDimensionless     Unit = "dimensionless"
	UnitRatio             Unit = "ratio"
	UnitPercent           Unit = "percent"
	UnitBasisPoints       Unit = "bps"
	UnitLogReturn         Unit = "log_return"
	UnitStandardDeviation Unit = "sigma"
	UnitSNR               Unit = "snr"
	UnitZScore            Unit = "zscore"
	UnitCorrelation       Unit = "correlation"
	UnitProbability       Unit = "probability"
	UnitConfidence        Unit = "confidence"
	UnitEntropy           Unit = "entropy"
	UnitNat               Unit = "nat"

	// Market Prices & Cash Flows
	UnitPrice                    Unit = "price"
	UnitCurrency                 Unit = "currency"
	UnitSpread                   Unit = "spread"
	UnitRelativeSpread           Unit = "relative_spread"
	UnitQuoteCurrencyPerBaseUnit Unit = "quote_per_base"
	UnitBaseCurrency             Unit = "base_currency"
	UnitQuoteCurrency            Unit = "quote_currency"
	UnitQuantity                 Unit = "quantity"
	UnitVolume                   Unit = "volume"
	UnitNotional                 Unit = "notional"
	UnitDistance                 Unit = "distance"

	// Discrete Counts
	UnitCount Unit = "count"

	// Rates & Dynamics
	UnitRate         Unit = "rate"
	UnitPerSecond    Unit = "per_second"
	UnitTradeRate    Unit = "trades_per_second"
	UnitVolumeRate   Unit = "volume_per_second"
	UnitNotionalRate Unit = "notional_per_second"
	UnitVelocity     Unit = "velocity"
	UnitAcceleration Unit = "acceleration"
	UnitVariance     Unit = "variance"
	UnitCovariance   Unit = "covariance"

	// Time & Duration
	UnitDuration    Unit = "duration"
	UnitNanosecond  Unit = "nanosecond"
	UnitMicrosecond Unit = "microsecond"
	UnitMillisecond Unit = "millisecond"
	UnitSecond      Unit = "second"
	UnitMinute      Unit = "minute"
	UnitHour        Unit = "hour"
)

/*
Timescale describes the horizon or domain over which a measured value accrues
or is statistically aggregated.
*/
type Timescale string

const (
	// Point-in-time / discrete events
	TimescaleInstantaneous Timescale = "instantaneous"
	TimescaleTick          Timescale = "tick"
	TimescaleEvent         Timescale = "event"

	// Market clock domains
	TimescaleVolumeBar     Timescale = "volume_bar"
	TimescaleTickWindow    Timescale = "tick_window"
	TimescaleRollingWindow Timescale = "rolling_window"
	TimescaleSession       Timescale = "session"
	TimescaleEpoch         Timescale = "epoch"

	// Chronological time domains
	TimescaleMicrosecond Timescale = "microsecond"
	TimescaleMillisecond Timescale = "millisecond"
	TimescaleSecond      Timescale = "second"
	TimescalePerSecond   Timescale = "per_second"
	TimescaleMinute      Timescale = "minute"
	TimescalePerMinute   Timescale = "per_minute"
	TimescaleHour        Timescale = "hour"
	TimescalePerHour     Timescale = "per_hour"
	TimescaleDay         Timescale = "day"
	TimescalePerDay      Timescale = "per_day"
)

/*
CanonicalUnit maps a metric label and its recorded unit to the canonical
physical unit. This resolves historical Parquet encodings and ensures that
asynchronous covariation ratios (Hayashi-Yoshida), signed lags, signed
covariances, and multiple-testing scale factors carry their honest physical
dimensions.
*/
func CanonicalUnit(label string, unit Unit) Unit {
	base := label
	if atIdx := strings.IndexByte(base, '@'); atIdx != -1 {
		base = base[:atIdx]
	}

	switch base {
	case "covariance", "best_lag_covariance":
		return UnitCovariance
	case "best_lag_index", "lag_search_scale", "absolute_correlation_gain", "lag_peak_prominence":
		return UnitDimensionless
	case "signed_correlation",
		"absolute_correlation",
		"best_lag_correlation",
		"contemporaneous_correlation",
		"cohort_signed_correlation",
		"cohort_absolute_correlation",
		"correlation_baseline",
		"best_lag_correlation_baseline",
		"correlation_gain_baseline":
		if unit == UnitCorrelation || unit == "" {
			return UnitDimensionless
		}
	}

	return unit
}
