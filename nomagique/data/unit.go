package data

import (
	"fmt"
	"strings"

	"github.com/theapemachine/errnie"
)

/*
Unit describes the physical dimension of a measured value. It grounds numbers
in their honest physical reality: currency, price, size, time, rate, or information.
*/
type Unit string

const (
	// Dimensionless & Relative
	UnitNone              Unit = "none"
	UnitDimensionless     Unit = "dimensionless"
	UnitRatio             Unit = "ratio"
	UnitPercent           Unit = "percent"
	UnitBasisPoints       Unit = "bps"
	UnitLogReturn         Unit = "log_return"
	UnitStandardDeviation Unit = "sigma"
	UnitSNR               Unit = "snr"
	UnitZScore            Unit = "zscore"
	UnitCorrelation       Unit = "correlation"
	// UnitCovarianceScore is a covariance divided by its null standard error:
	// signed, unbounded, standard normal under no co-movement.
	UnitCovarianceScore Unit = "covariance_score"
	UnitProbability     Unit = "probability"
	UnitConfidence      Unit = "confidence"
	UnitEntropy         Unit = "entropy"
	UnitNat             Unit = "nat" // signed: log-likelihoods; non-negative information is UnitEntropy
	UnitImbalance       Unit = "imbalance"
	UnitDivergence      Unit = "divergence"

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
	UnitPriceImpact              Unit = "price_impact"
	UnitDistance                 Unit = "distance"
	UnitSignedQuantity           Unit = "signed_quantity"
	UnitSignedNotional           Unit = "signed_notional"

	// Discrete Counts
	UnitCount Unit = "count"
	// UnitCountResidual is an observed-minus-expected event count: signed.
	UnitCountResidual Unit = "count_residual"

	// Rates & Dynamics
	UnitRate               Unit = "rate"
	UnitPerSecond          Unit = "per_second"
	UnitTradeRate          Unit = "trades_per_second"
	UnitVolumeRate         Unit = "volume_per_second"
	UnitNotionalRate       Unit = "notional_per_second"
	UnitSignedRate         Unit = "signed_rate"
	UnitSignedNotionalRate Unit = "signed_notional_per_second"
	UnitVelocity           Unit = "velocity"
	UnitAcceleration       Unit = "acceleration"
	UnitVariance           Unit = "variance"
	UnitCovariance         Unit = "covariance"

	// Time & Duration
	UnitDuration    Unit = "duration"
	UnitTimeOffset  Unit = "time_offset"
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
	TimescaleNone Timescale = "none"
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
Dimension pairs a physical unit with its statistical or operational timescale.
*/
type Dimension struct {
	Unit      Unit
	Timescale Timescale
}

/*
SignalMetrics maps each signal producer to its owned metrics and declared dimensions.
*/
var SignalMetrics = map[string]map[string]Dimension{
	"correlation": {
		"cohort_covariance_score":   {Unit: UnitCovarianceScore, Timescale: TimescaleRollingWindow},
		"historical_path_distance":   {Unit: UnitDistance, Timescale: TimescaleRollingWindow},
		"historical_path_percentile": {Unit: UnitProbability, Timescale: TimescaleRollingWindow},
	},
	"cvd": {
		"midpoint_log_return":          {Unit: UnitLogReturn, Timescale: TimescaleRollingWindow},
		"flow_aligned_midpoint_return": {Unit: UnitLogReturn, Timescale: TimescaleRollingWindow},
		"gross_notional":               {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"signed_net_fraction":          {Unit: UnitImbalance, Timescale: TimescaleRollingWindow},
	},
	"depthflow": {
		"book_imbalance":              {Unit: UnitImbalance, Timescale: TimescaleInstantaneous},
		"touch_notional:bid":          {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"net_displayed_flow:bid":      {Unit: UnitSignedNotional, Timescale: TimescaleInstantaneous},
		"mutation_activity_imbalance": {Unit: UnitImbalance, Timescale: TimescaleRollingWindow},
	},
	"hawkes": {
		"arrival_rate:buy":               {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"arrival_rate:sell":              {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"branching_spectral_radius":      {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"conditional_intensity_velocity": {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
	},
	"leadlag": {
		"defined_peer_count":           {Unit: UnitCount, Timescale: TimescaleRollingWindow},
		"best_lag_seconds_median":      {Unit: UnitTimeOffset, Timescale: TimescaleRollingWindow},
		"best_lag_seconds_mad":         {Unit: UnitDuration, Timescale: TimescaleRollingWindow},
		"led_peer_share":               {Unit: UnitProbability, Timescale: TimescaleRollingWindow},
		"covariance_score_gain_median": {Unit: UnitCovarianceScore, Timescale: TimescaleRollingWindow},
	},
	"liquidity": {
		"best_ask_price":  {Unit: UnitPrice, Timescale: TimescaleInstantaneous},
		"best_bid_price":  {Unit: UnitPrice, Timescale: TimescaleInstantaneous},
		"relative_spread": {Unit: UnitRelativeSpread, Timescale: TimescaleInstantaneous},
	},
	"morphology": {
		"book_shape_distance": {Unit: UnitDistance, Timescale: TimescaleInstantaneous},
		"book_shape_ks":       {Unit: UnitRatio, Timescale: TimescaleInstantaneous},
		"concentration:bid":   {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"concentration:ask":   {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"entropy:bid":         {Unit: UnitEntropy, Timescale: TimescaleInstantaneous},
		"entropy:ask":         {Unit: UnitEntropy, Timescale: TimescaleInstantaneous},
		"morphology_change":   {Unit: UnitRatio, Timescale: TimescaleInstantaneous},
	},
	"pumpdump": {
		"volume_rate":              {Unit: UnitVolumeRate, Timescale: TimescaleVolumeBar},
		"trade_rate":               {Unit: UnitTradeRate, Timescale: TimescaleRollingWindow},
		"positive_midpoint_return": {Unit: UnitLogReturn, Timescale: TimescaleRollingWindow},
		"negative_midpoint_return": {Unit: UnitLogReturn, Timescale: TimescaleRollingWindow},
		"spread_ratio":             {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
	},
	"sentiment": {
		"breadth":                       {Unit: UnitImbalance, Timescale: TimescaleRollingWindow},
		"same_direction_peer_count":     {Unit: UnitCount, Timescale: TimescaleRollingWindow},
		"opposite_direction_peer_count": {Unit: UnitCount, Timescale: TimescaleRollingWindow},
		"breadth_velocity":              {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
		"historical_path_distance":      {Unit: UnitDistance, Timescale: TimescaleRollingWindow},
		"historical_path_percentile":    {Unit: UnitProbability, Timescale: TimescaleRollingWindow},
	},
	"toxicity": {
		"retreat_fraction:bid":   {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"net_withdrawal_rate:bid": {Unit: UnitRate, Timescale: TimescaleRollingWindow},
	},
	"spot:trade": {
		"price":      {Unit: UnitPrice, Timescale: TimescaleInstantaneous},
		"qty":        {Unit: UnitQuantity, Timescale: TimescaleInstantaneous},
		"book_depth": {Unit: UnitCount, Timescale: TimescaleInstantaneous},
		"bid_price":  {Unit: UnitPrice, Timescale: TimescaleInstantaneous},
		"bid_qty":    {Unit: UnitQuantity, Timescale: TimescaleInstantaneous},
		"ask_price":  {Unit: UnitPrice, Timescale: TimescaleInstantaneous},
		"ask_qty":    {Unit: UnitQuantity, Timescale: TimescaleInstantaneous},
	},
	"spot:level3": {
		"limit_price": {Unit: UnitPrice, Timescale: TimescaleInstantaneous},
		"order_qty":   {Unit: UnitQuantity, Timescale: TimescaleInstantaneous},
	},
	"detector": {
		"start_idx":  {Unit: UnitCount, Timescale: TimescaleTick},
		"start_tick": {Unit: UnitCount, Timescale: TimescaleTick},
		"b_idx":      {Unit: UnitCount, Timescale: TimescaleTick},
		"b_tick":     {Unit: UnitCount, Timescale: TimescaleTick},
		"c_idx":      {Unit: UnitCount, Timescale: TimescaleTick},
		"c_tick":     {Unit: UnitCount, Timescale: TimescaleTick},
		"end_tick":   {Unit: UnitCount, Timescale: TimescaleTick},
		"b_price":    {Unit: UnitPrice, Timescale: TimescaleTick},
		"c_price":    {Unit: UnitPrice, Timescale: TimescaleTick},
	},
	"training": {
		"region_brightness:R00":  {Unit: UnitZScore, Timescale: TimescaleInstantaneous},
		"region_brightness:R01":  {Unit: UnitZScore, Timescale: TimescaleInstantaneous},
		"region_brightness:R02":  {Unit: UnitZScore, Timescale: TimescaleInstantaneous},
		"region_brightness:R03":  {Unit: UnitZScore, Timescale: TimescaleInstantaneous},
		"region_brightness:R04":  {Unit: UnitZScore, Timescale: TimescaleInstantaneous},
		"region_brightness:R05":  {Unit: UnitZScore, Timescale: TimescaleInstantaneous},
		"region_brightness:R06":  {Unit: UnitZScore, Timescale: TimescaleInstantaneous},
		"region_brightness:R07":  {Unit: UnitZScore, Timescale: TimescaleInstantaneous},
		"region_brightness:R08":  {Unit: UnitZScore, Timescale: TimescaleInstantaneous},
		"region_brightness:R09":  {Unit: UnitZScore, Timescale: TimescaleInstantaneous},
		"region_brightness:R10":  {Unit: UnitZScore, Timescale: TimescaleInstantaneous},
		"region_brightness:R11":  {Unit: UnitZScore, Timescale: TimescaleInstantaneous},
		"region_members:R00":     {Unit: UnitCount, Timescale: TimescaleInstantaneous},
		"region_members:R01":     {Unit: UnitCount, Timescale: TimescaleInstantaneous},
		"region_members:R02":     {Unit: UnitCount, Timescale: TimescaleInstantaneous},
		"region_members:R03":     {Unit: UnitCount, Timescale: TimescaleInstantaneous},
		"region_members:R04":     {Unit: UnitCount, Timescale: TimescaleInstantaneous},
		"region_members:R05":     {Unit: UnitCount, Timescale: TimescaleInstantaneous},
		"region_members:R06":     {Unit: UnitCount, Timescale: TimescaleInstantaneous},
		"region_members:R07":     {Unit: UnitCount, Timescale: TimescaleInstantaneous},
		"region_members:R08":     {Unit: UnitCount, Timescale: TimescaleInstantaneous},
		"region_members:R09":     {Unit: UnitCount, Timescale: TimescaleInstantaneous},
		"region_members:R10":     {Unit: UnitCount, Timescale: TimescaleInstantaneous},
		"region_members:R11":     {Unit: UnitCount, Timescale: TimescaleInstantaneous},
		"position_open":          {Unit: UnitDimensionless, Timescale: TimescaleInstantaneous},
		"position_exiting":       {Unit: UnitDimensionless, Timescale: TimescaleInstantaneous},
		"position_qty":           {Unit: UnitBaseCurrency, Timescale: TimescaleInstantaneous},
		"position_bid_depth_qty": {Unit: UnitBaseCurrency, Timescale: TimescaleInstantaneous},
		"position_held_s":        {Unit: UnitSecond, Timescale: TimescaleInstantaneous},
		"position_entry":         {Unit: UnitPrice, Timescale: TimescaleInstantaneous},
		"position_bid":           {Unit: UnitPrice, Timescale: TimescaleInstantaneous},
		"position_ask":           {Unit: UnitPrice, Timescale: TimescaleInstantaneous},
		"position_exit_vwap":     {Unit: UnitPrice, Timescale: TimescaleInstantaneous},
		"position_cost":          {Unit: UnitQuoteCurrency, Timescale: TimescaleInstantaneous},
		"position_entry_fee":     {Unit: UnitQuoteCurrency, Timescale: TimescaleInstantaneous},
		"position_value":         {Unit: UnitQuoteCurrency, Timescale: TimescaleInstantaneous},
		"position_pnl":           {Unit: UnitQuoteCurrency, Timescale: TimescaleInstantaneous},
		"position_pnl_pct":       {Unit: UnitRatio, Timescale: TimescaleInstantaneous},
		"position_exit_room":     {Unit: UnitRatio, Timescale: TimescaleInstantaneous},
		"token_depth":            {Unit: UnitCount, Timescale: TimescaleInstantaneous},
		"token_capacity":         {Unit: UnitCount, Timescale: TimescaleInstantaneous},
		"token_strength":         {Unit: UnitZScore, Timescale: TimescaleInstantaneous},
		"token_threshold":        {Unit: UnitZScore, Timescale: TimescaleInstantaneous},
		"stored_paths":           {Unit: UnitCount, Timescale: TimescaleInstantaneous},
	},
}

func stripScope(label string) string {
	base := label

	if atIndex := strings.IndexByte(base, '@'); atIndex != -1 {
		base = base[:atIndex]
	}

	if lastUnder := strings.LastIndexByte(base, '_'); lastUnder != -1 && lastUnder < len(base)-1 {
		allDigits := true
		for i := lastUnder + 1; i < len(base); i++ {
			if base[i] < '0' || base[i] > '9' {
				allDigits = false
				break
			}
		}
		if allDigits {
			base = base[:lastUnder]
		}
	}

	return base
}

/*
Dimension returns the canonical dimensions of a metric.
*/
func GetDimension(source string, label string) (unit Unit, timescale Timescale) {
	producer, ok := SignalMetrics[source]

	if !ok {
		errnie.Error(errnie.Err(
			errnie.NotFound,
			"[data.unit] producer not found",
			nil,
		), "source", source, "label", label)

		return UnitNone, TimescaleNone
	}

	dimension, ok := producer[label]

	if !ok {
		dimension, ok = producer[stripScope(label)]
	}

	if !ok {
		errnie.Error(errnie.Err(
			errnie.NotFound,
			"[data.unit] dimension not found",
			nil,
		), "source", source, "label", label)

		return UnitNone, TimescaleNone
	}

	return dimension.Unit, dimension.Timescale
}

/*
TrainingPathSlots bounds the indexed training labels registered below: token
positions (path_token:IDX:PART, a region key) and prefix depths (match_*:k, a
count of stored paths) up to this many tokens, with up to RegionSlots parts.
*/
const (
	TrainingPathSlots = 64
	MatchKeySlots     = 512
	RegionSlots       = 12
)

func init() {
	training := SignalMetrics["training"]
	training["stored_paths_hash"] = Dimension{Unit: UnitDimensionless, Timescale: TimescaleInstantaneous}

	for slot := range MatchKeySlots {
		training[fmt.Sprintf("match_key:%03d", slot)] = Dimension{Unit: UnitDimensionless, Timescale: TimescaleInstantaneous}
	}

	for idx := range TrainingPathSlots {
		for part := range RegionSlots {
			training[fmt.Sprintf("path_token:%02d:%d", idx, part)] = Dimension{Unit: UnitDimensionless, Timescale: TimescaleInstantaneous}
		}

		for _, prefix := range []string{"match_candidates", "match_enter", "match_exit", "match_noop"} {
			training[fmt.Sprintf("%s:%02d", prefix, idx+1)] = Dimension{Unit: UnitCount, Timescale: TimescaleInstantaneous}
		}
	}
}
