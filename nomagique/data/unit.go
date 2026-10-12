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
		"cohort_effective_peer_count":        {Unit: UnitCount, Timescale: TimescaleRollingWindow},
		"cohort_peer_count":                  {Unit: UnitCount, Timescale: TimescaleRollingWindow},
		"covariance":                         {Unit: UnitCovariance, Timescale: TimescaleRollingWindow},
		"historical_path_distance":           {Unit: UnitDistance, Timescale: TimescaleRollingWindow},
		"historical_path_percentile":         {Unit: UnitProbability, Timescale: TimescaleRollingWindow},
		"last_price":                         {Unit: UnitPrice, Timescale: TimescaleInstantaneous},
		"observation_count":                  {Unit: UnitCount, Timescale: TimescaleRollingWindow},
		"overlap_density":                    {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"overlap_pair_count":                 {Unit: UnitCount, Timescale: TimescaleRollingWindow},
		"peer_return_energy_rate":            {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"relative_return_energy":             {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"relative_return_energy_baseline":    {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"relative_return_energy_divergence":  {Unit: UnitDivergence, Timescale: TimescaleRollingWindow},
		"relative_return_energy_velocity":    {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
		"relative_return_energy_zscore":      {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"return_energy:measured":             {Unit: UnitVariance, Timescale: TimescaleRollingWindow},
		"return_energy:reference":            {Unit: UnitVariance, Timescale: TimescaleRollingWindow},
		"return_energy_rate:measured":        {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"return_energy_rate:reference":       {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"shared_time":                        {Unit: UnitSecond, Timescale: TimescaleRollingWindow},
		"supported_return_count:measured":    {Unit: UnitCount, Timescale: TimescaleRollingWindow},
		"supported_return_count:reference":   {Unit: UnitCount, Timescale: TimescaleRollingWindow},
		"absolute_covariance_score":          {Unit: UnitCovarianceScore, Timescale: TimescaleRollingWindow},
		"cohort_absolute_covariance_score":   {Unit: UnitCovarianceScore, Timescale: TimescaleRollingWindow},
		"cohort_covariance_score":            {Unit: UnitCovarianceScore, Timescale: TimescaleRollingWindow},
		"cohort_covariance_score_dispersion": {Unit: UnitStandardDeviation, Timescale: TimescaleRollingWindow},
		"covariance_p_value":                 {Unit: UnitProbability, Timescale: TimescaleRollingWindow},
		"covariance_score":                   {Unit: UnitCovarianceScore, Timescale: TimescaleRollingWindow},
		"covariance_score_baseline":          {Unit: UnitCovarianceScore, Timescale: TimescaleRollingWindow},
		"covariance_score_divergence":        {Unit: UnitDivergence, Timescale: TimescaleRollingWindow},
		"covariance_score_velocity":          {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
		"covariance_score_zscore":            {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"covariance_standard_error":          {Unit: UnitCovariance, Timescale: TimescaleRollingWindow},
	},
	"cvd": {
		"aggressive_notional:buy":            {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"aggressive_notional:sell":           {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"buy_notional_rate":                  {Unit: UnitNotionalRate, Timescale: TimescaleRollingWindow},
		"cumulative_notional_delta":          {Unit: UnitSignedNotional, Timescale: TimescaleInstantaneous},
		"cumulative_volume_delta":            {Unit: UnitSignedQuantity, Timescale: TimescaleInstantaneous},
		"executed_quantity:buy":              {Unit: UnitQuantity, Timescale: TimescaleInstantaneous},
		"executed_quantity:sell":             {Unit: UnitQuantity, Timescale: TimescaleInstantaneous},
		"flow_aligned_midpoint_return":       {Unit: UnitLogReturn, Timescale: TimescaleRollingWindow},
		"gross_executed_quantity":            {Unit: UnitQuantity, Timescale: TimescaleInstantaneous},
		"gross_notional":                     {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"gross_notional_rate":                {Unit: UnitNotionalRate, Timescale: TimescaleRollingWindow},
		"gross_notional_rate_baseline":       {Unit: UnitNotionalRate, Timescale: TimescaleRollingWindow},
		"gross_notional_rate_divergence":     {Unit: UnitDivergence, Timescale: TimescaleRollingWindow},
		"gross_notional_rate_ratio":          {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"gross_notional_rate_velocity":       {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
		"gross_notional_rate_zscore":         {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"historical_path_distance":           {Unit: UnitDistance, Timescale: TimescaleRollingWindow},
		"historical_path_percentile":         {Unit: UnitProbability, Timescale: TimescaleRollingWindow},
		"mean_trade_notional":                {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"midpoint_log_return":                {Unit: UnitLogReturn, Timescale: TimescaleRollingWindow},
		"midpoint_response_per_net_notional": {Unit: UnitPriceImpact, Timescale: TimescaleRollingWindow},
		"midpoint_return_rate":               {Unit: UnitSignedRate, Timescale: TimescaleRollingWindow},
		"midpoint_return_rate_baseline":      {Unit: UnitSignedRate, Timescale: TimescaleRollingWindow},
		"midpoint_return_rate_divergence":    {Unit: UnitDivergence, Timescale: TimescaleRollingWindow},
		"midpoint_return_rate_zscore":        {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"net_executed_quantity":              {Unit: UnitSignedQuantity, Timescale: TimescaleInstantaneous},
		"net_notional":                       {Unit: UnitSignedNotional, Timescale: TimescaleInstantaneous},
		"net_notional_rate":                  {Unit: UnitSignedNotionalRate, Timescale: TimescaleRollingWindow},
		"net_notional_rate_velocity":         {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
		"response_midpoint:at":               {Unit: UnitPrice, Timescale: TimescaleInstantaneous},
		"response_midpoint:from":             {Unit: UnitPrice, Timescale: TimescaleInstantaneous},
		"sell_notional_rate":                 {Unit: UnitNotionalRate, Timescale: TimescaleRollingWindow},
		"signed_count_fraction":              {Unit: UnitImbalance, Timescale: TimescaleRollingWindow},
		"signed_net_fraction":                {Unit: UnitImbalance, Timescale: TimescaleRollingWindow},
		"signed_net_fraction:buy":            {Unit: UnitImbalance, Timescale: TimescaleRollingWindow},
		"signed_net_fraction_baseline":       {Unit: UnitImbalance, Timescale: TimescaleRollingWindow},
		"signed_net_fraction_divergence":     {Unit: UnitDivergence, Timescale: TimescaleRollingWindow},
		"signed_net_fraction_mean":           {Unit: UnitImbalance, Timescale: TimescaleRollingWindow},
		"signed_net_fraction_zscore":         {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"trade_count":                        {Unit: UnitCount, Timescale: TimescaleInstantaneous},
		"trade_count:buy":                    {Unit: UnitCount, Timescale: TimescaleInstantaneous},
		"trade_count:sell":                   {Unit: UnitCount, Timescale: TimescaleInstantaneous},
		"trade_rate":                         {Unit: UnitTradeRate, Timescale: TimescaleRollingWindow},
		"quantity:buy":                       {Unit: UnitQuantity, Timescale: TimescaleInstantaneous},
		"quantity:sell":                      {Unit: UnitQuantity, Timescale: TimescaleInstantaneous},
		"churn":                              {Unit: UnitQuantity, Timescale: TimescaleInstantaneous},
	},
	"depthflow": {
		"add_notional:ask":                          {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"add_notional:bid":                          {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"added_notional:ask":                        {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"added_notional:bid":                        {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"added_notional_rate:ask":                   {Unit: UnitNotionalRate, Timescale: TimescaleRollingWindow},
		"added_notional_rate:bid":                   {Unit: UnitNotionalRate, Timescale: TimescaleRollingWindow},
		"book_imbalance":                            {Unit: UnitImbalance, Timescale: TimescaleInstantaneous},
		"book_imbalance_baseline":                   {Unit: UnitImbalance, Timescale: TimescaleRollingWindow},
		"book_imbalance_divergence":                 {Unit: UnitDivergence, Timescale: TimescaleRollingWindow},
		"book_imbalance_velocity":                   {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
		"book_imbalance_zscore":                     {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"book_notional":                             {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"book_notional:ask":                         {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"book_notional:bid":                         {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"book_turnover_rate":                        {Unit: UnitNotionalRate, Timescale: TimescaleRollingWindow},
		"delete_count:ask":                          {Unit: UnitCount, Timescale: TimescaleInstantaneous},
		"delete_count:bid":                          {Unit: UnitCount, Timescale: TimescaleInstantaneous},
		"flow_activity_imbalance":                   {Unit: UnitImbalance, Timescale: TimescaleInstantaneous},
		"historical_path_distance":                  {Unit: UnitDistance, Timescale: TimescaleRollingWindow},
		"historical_path_percentile":                {Unit: UnitProbability, Timescale: TimescaleRollingWindow},
		"imbalance_resolution_distance":             {Unit: UnitDistance, Timescale: TimescaleInstantaneous},
		"imbalance_resolution_gap":                  {Unit: UnitDivergence, Timescale: TimescaleInstantaneous},
		"modify_remaining_notional:ask":             {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"modify_remaining_notional:bid":             {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"mutation_activity_imbalance":               {Unit: UnitImbalance, Timescale: TimescaleRollingWindow},
		"mutation_count":                            {Unit: UnitCount, Timescale: TimescaleInstantaneous},
		"mutation_count:ask":                        {Unit: UnitCount, Timescale: TimescaleInstantaneous},
		"mutation_count:bid":                        {Unit: UnitCount, Timescale: TimescaleInstantaneous},
		"mutation_count_diff":                       {Unit: UnitCountResidual, Timescale: TimescaleInstantaneous},
		"net_book_change_rate":                      {Unit: UnitSignedNotionalRate, Timescale: TimescaleRollingWindow},
		"net_book_change_rate_baseline":             {Unit: UnitSignedNotionalRate, Timescale: TimescaleRollingWindow},
		"net_book_change_rate_divergence":           {Unit: UnitDivergence, Timescale: TimescaleRollingWindow},
		"net_book_change_rate_zscore":               {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"net_displayed_flow:ask":                    {Unit: UnitSignedNotional, Timescale: TimescaleInstantaneous},
		"net_displayed_flow:bid":                    {Unit: UnitSignedNotional, Timescale: TimescaleInstantaneous},
		"net_displayed_flow_rate:ask":               {Unit: UnitSignedNotionalRate, Timescale: TimescaleRollingWindow},
		"net_displayed_flow_rate:bid":               {Unit: UnitSignedNotionalRate, Timescale: TimescaleRollingWindow},
		"observed_notional_diff":                    {Unit: UnitSignedNotional, Timescale: TimescaleInstantaneous},
		"observed_notional_imbalance_baseline":      {Unit: UnitImbalance, Timescale: TimescaleRollingWindow},
		"observed_notional_imbalance_divergence":    {Unit: UnitDivergence, Timescale: TimescaleRollingWindow},
		"observed_notional_imbalance_mean":          {Unit: UnitImbalance, Timescale: TimescaleRollingWindow},
		"observed_notional_imbalance_zscore":        {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"observed_notional_rate":                    {Unit: UnitNotionalRate, Timescale: TimescaleRollingWindow},
		"observed_notional_rate_baseline":           {Unit: UnitNotionalRate, Timescale: TimescaleRollingWindow},
		"observed_notional_rate_divergence":         {Unit: UnitDivergence, Timescale: TimescaleRollingWindow},
		"observed_notional_rate_mean":               {Unit: UnitNotionalRate, Timescale: TimescaleRollingWindow},
		"observed_notional_rate_zscore":             {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"removed_notional:ask":                      {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"removed_notional:bid":                      {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"removed_notional_rate:ask":                 {Unit: UnitNotionalRate, Timescale: TimescaleRollingWindow},
		"removed_notional_rate:bid":                 {Unit: UnitNotionalRate, Timescale: TimescaleRollingWindow},
		"resolution_gap_baseline":                   {Unit: UnitDivergence, Timescale: TimescaleRollingWindow},
		"resolution_gap_divergence":                 {Unit: UnitDivergence, Timescale: TimescaleRollingWindow},
		"resolution_gap_velocity":                   {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
		"resolution_gap_zscore":                     {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"signed_net_displayed_flow_rate":            {Unit: UnitSignedNotionalRate, Timescale: TimescaleRollingWindow},
		"signed_net_displayed_flow_rate_baseline":   {Unit: UnitSignedNotionalRate, Timescale: TimescaleRollingWindow},
		"signed_net_displayed_flow_rate_divergence": {Unit: UnitDivergence, Timescale: TimescaleRollingWindow},
		"signed_net_displayed_flow_rate_zscore":     {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"touch_imbalance":                           {Unit: UnitImbalance, Timescale: TimescaleInstantaneous},
		"turnover_baseline":                         {Unit: UnitNotionalRate, Timescale: TimescaleRollingWindow},
		"turnover_divergence":                       {Unit: UnitDivergence, Timescale: TimescaleRollingWindow},
		"turnover_ratio":                            {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"turnover_zscore":                           {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"removed:ask":                               {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"removed:bid":                               {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"spread":                                    {Unit: UnitSpread, Timescale: TimescaleInstantaneous},
	},
	"hawkes": {
		"arrival_rate":                               {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"arrival_rate:buy":                           {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"arrival_rate:sell":                          {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"background_rate":                            {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"background_rate:buy":                        {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"background_rate:sell":                       {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"branching_spectral_radius":                  {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"buy_from_buy":                               {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"compensator:buy":                            {Unit: UnitCount, Timescale: TimescaleInstantaneous},
		"compensator:sell":                           {Unit: UnitCount, Timescale: TimescaleInstantaneous},
		"conditional_intensity":                      {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"conditional_intensity:buy":                  {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"conditional_intensity:sell":                 {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"conditional_intensity_velocity":             {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
		"count_innovation:buy":                       {Unit: UnitCountResidual, Timescale: TimescaleInstantaneous},
		"count_innovation:sell":                      {Unit: UnitCountResidual, Timescale: TimescaleInstantaneous},
		"event_count":                                {Unit: UnitCount, Timescale: TimescaleInstantaneous},
		"event_count:buy":                            {Unit: UnitCount, Timescale: TimescaleInstantaneous},
		"event_count:sell":                           {Unit: UnitCount, Timescale: TimescaleInstantaneous},
		"event_fraction:buy":                         {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"event_fraction:sell":                        {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"excitation_amplitude:buy_from_buy":          {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"excitation_amplitude:buy_from_sell":         {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"excitation_amplitude:sell_from_buy":         {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"excitation_amplitude:sell_from_sell":        {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"excitation_decay":                           {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"excitation_fraction:buy":                    {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"excitation_fraction:sell":                   {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"excitation_intensity:buy":                   {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"excitation_intensity:sell":                  {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"excitation_mass:buy":                        {Unit: UnitCount, Timescale: TimescaleRollingWindow},
		"excitation_mass:sell":                       {Unit: UnitCount, Timescale: TimescaleRollingWindow},
		"excitation_share":                           {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"excitation_share:buy":                       {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"excitation_share:sell":                      {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"excitation_timescale":                       {Unit: UnitSecond, Timescale: TimescaleRollingWindow},
		"expected_descendants_from_buy":              {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"expected_descendants_from_sell":             {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"log_likelihood:hawkes":                      {Unit: UnitNat, Timescale: TimescaleRollingWindow},
		"log_likelihood:poisson":                     {Unit: UnitNat, Timescale: TimescaleRollingWindow},
		"log_likelihood:self_only":                   {Unit: UnitNat, Timescale: TimescaleRollingWindow},
		"log_likelihood_gain_per_event_vs_poisson":   {Unit: UnitNat, Timescale: TimescaleRollingWindow},
		"log_likelihood_gain_per_event_vs_self_only": {Unit: UnitNat, Timescale: TimescaleRollingWindow},
		"log_likelihood_gain_vs_poisson":             {Unit: UnitNat, Timescale: TimescaleRollingWindow},
		"log_likelihood_gain_vs_self_only":           {Unit: UnitNat, Timescale: TimescaleRollingWindow},
		"log_likelihood_per_event:hawkes":            {Unit: UnitNat, Timescale: TimescaleRollingWindow},
		"offspring:buy_from_buy":                     {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"offspring:buy_from_sell":                    {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"offspring:sell_from_buy":                    {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"offspring:sell_from_sell":                   {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"spectral_radius_velocity":                   {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
		"standardized_innovation:buy":                {Unit: UnitZScore, Timescale: TimescaleInstantaneous},
		"standardized_innovation:sell":               {Unit: UnitZScore, Timescale: TimescaleInstantaneous},
		"historical_path_distance":                   {Unit: UnitDistance, Timescale: TimescaleRollingWindow},
		"historical_path_percentile":                 {Unit: UnitProbability, Timescale: TimescaleRollingWindow},
		"intensity:buy":                              {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"intensity:sell":                             {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"branching_ratio":                            {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"sell_from_sell":                             {Unit: UnitRate, Timescale: TimescaleRollingWindow},
	},
	"leadlag": {
		"best_lag_correlation_baseline": {Unit: UnitCorrelation, Timescale: TimescaleRollingWindow},
		"best_lag_correlation_zscore":   {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"best_lag_seconds_mad":          {Unit: UnitDuration, Timescale: TimescaleRollingWindow},
		"best_lag_seconds_median":       {Unit: UnitTimeOffset, Timescale: TimescaleRollingWindow},
		"correlation_gain_baseline":     {Unit: UnitCorrelation, Timescale: TimescaleRollingWindow},
		"covariance_score_gain_mean":    {Unit: UnitCovarianceScore, Timescale: TimescaleRollingWindow},
		"covariance_score_gain_median":  {Unit: UnitCovarianceScore, Timescale: TimescaleRollingWindow},
		"correlation_gain_velocity":     {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
		"correlation_gain_zscore":       {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"correlation_p_value":           {Unit: UnitProbability, Timescale: TimescaleRollingWindow},
		"defined_peer_count":            {Unit: UnitCount, Timescale: TimescaleRollingWindow},
		"effective_sample_count":        {Unit: UnitCount, Timescale: TimescaleRollingWindow},
		"historical_path_distance":      {Unit: UnitDistance, Timescale: TimescaleRollingWindow},
		"historical_path_percentile":    {Unit: UnitProbability, Timescale: TimescaleRollingWindow},
		"lag_baseline_seconds":          {Unit: UnitTimeOffset, Timescale: TimescaleRollingWindow},
		"lag_divergence_seconds":        {Unit: UnitDivergence, Timescale: TimescaleRollingWindow},
		"lag_noise_scale_seconds":       {Unit: UnitSecond, Timescale: TimescaleRollingWindow},
		"lag_velocity":                  {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
		"lag_zscore":                    {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"last":                          {Unit: UnitPrice, Timescale: TimescaleInstantaneous},
		"last_price":                    {Unit: UnitPrice, Timescale: TimescaleInstantaneous},
		"led_peer_share":                {Unit: UnitProbability, Timescale: TimescaleRollingWindow},
		"measured_return_count":         {Unit: UnitCount, Timescale: TimescaleRollingWindow},
		"observation_count":             {Unit: UnitCount, Timescale: TimescaleRollingWindow},
		"reference_return_count":        {Unit: UnitCount, Timescale: TimescaleRollingWindow},
		"search_adjusted_p_value":       {Unit: UnitProbability, Timescale: TimescaleRollingWindow},
	},
	"liquidity": {
		"best_ask_price":                 {Unit: UnitPrice, Timescale: TimescaleInstantaneous},
		"best_bid_price":                 {Unit: UnitPrice, Timescale: TimescaleInstantaneous},
		"depth_divergence:ask":           {Unit: UnitDivergence, Timescale: TimescaleRollingWindow},
		"depth_divergence:bid":           {Unit: UnitDivergence, Timescale: TimescaleRollingWindow},
		"depth_noise_scale:ask":          {Unit: UnitNotional, Timescale: TimescaleRollingWindow},
		"depth_noise_scale:bid":          {Unit: UnitNotional, Timescale: TimescaleRollingWindow},
		"depth_ratio:ask":                {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"depth_ratio:bid":                {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"depth_zscore:ask":               {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"depth_zscore:bid":               {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"divergence_velocity:ask":        {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
		"divergence_velocity:bid":        {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
		"divergence_velocity_snr:ask":    {Unit: UnitSNR, Timescale: TimescaleRollingWindow},
		"divergence_velocity_snr:bid":    {Unit: UnitSNR, Timescale: TimescaleRollingWindow},
		"historical_path_distance":       {Unit: UnitDistance, Timescale: TimescaleRollingWindow},
		"historical_path_percentile":     {Unit: UnitProbability, Timescale: TimescaleRollingWindow},
		"midpoint":                       {Unit: UnitPrice, Timescale: TimescaleInstantaneous},
		"relative_spread":                {Unit: UnitRelativeSpread, Timescale: TimescaleInstantaneous},
		"relative_spread_baseline":       {Unit: UnitRelativeSpread, Timescale: TimescaleRollingWindow},
		"spread":                         {Unit: UnitSpread, Timescale: TimescaleInstantaneous},
		"spread_baseline":                {Unit: UnitSpread, Timescale: TimescaleRollingWindow},
		"spread_divergence":              {Unit: UnitDivergence, Timescale: TimescaleRollingWindow},
		"spread_divergence_velocity":     {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
		"spread_divergence_velocity_snr": {Unit: UnitSNR, Timescale: TimescaleRollingWindow},
		"spread_noise_scale":             {Unit: UnitSpread, Timescale: TimescaleRollingWindow},
		"spread_ratio":                   {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"spread_zscore":                  {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"touch_notional:ask":             {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"touch_notional:bid":             {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"touch_notional_baseline:ask":    {Unit: UnitNotional, Timescale: TimescaleRollingWindow},
		"touch_notional_baseline:bid":    {Unit: UnitNotional, Timescale: TimescaleRollingWindow},
		"touch_notional_imbalance":       {Unit: UnitImbalance, Timescale: TimescaleRollingWindow},
		"touch_quantity:ask":             {Unit: UnitQuantity, Timescale: TimescaleInstantaneous},
		"touch_quantity:bid":             {Unit: UnitQuantity, Timescale: TimescaleInstantaneous},
		"two_sided_touch_notional":       {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
	},
	"morphology": {
		"book_shape_distance":        {Unit: UnitDistance, Timescale: TimescaleInstantaneous},
		"book_shape_ks":              {Unit: UnitRatio, Timescale: TimescaleInstantaneous},
		"concentration:ask":          {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"concentration:bid":          {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"entropy:ask":                {Unit: UnitEntropy, Timescale: TimescaleInstantaneous},
		"entropy:bid":                {Unit: UnitEntropy, Timescale: TimescaleInstantaneous},
		"morphology_change":          {Unit: UnitRatio, Timescale: TimescaleInstantaneous},
		"morphology_change_baseline": {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"morphology_change_zscore":   {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"historical_path_distance":   {Unit: UnitDistance, Timescale: TimescaleRollingWindow},
		"historical_path_percentile": {Unit: UnitProbability, Timescale: TimescaleRollingWindow},
		"wasserstein":                {Unit: UnitDistance, Timescale: TimescaleInstantaneous},
	},
	"pumpdump": {
		"best_ask":                        {Unit: UnitPrice, Timescale: TimescaleInstantaneous},
		"best_bid":                        {Unit: UnitPrice, Timescale: TimescaleInstantaneous},
		"completed_bars":                  {Unit: UnitCount, Timescale: TimescaleVolumeBar},
		"completed_volume_bar_ordinal":    {Unit: UnitCount, Timescale: TimescaleVolumeBar},
		"midpoint":                        {Unit: UnitPrice, Timescale: TimescaleInstantaneous},
		"midpoint:at":                     {Unit: UnitPrice, Timescale: TimescaleInstantaneous},
		"midpoint:from":                   {Unit: UnitPrice, Timescale: TimescaleInstantaneous},
		"midpoint_log_return":             {Unit: UnitLogReturn, Timescale: TimescaleRollingWindow},
		"midpoint_return_baseline":        {Unit: UnitSignedRate, Timescale: TimescaleRollingWindow},
		"midpoint_return_divergence":      {Unit: UnitDivergence, Timescale: TimescaleRollingWindow},
		"midpoint_return_rate":            {Unit: UnitSignedRate, Timescale: TimescaleRollingWindow},
		"midpoint_return_rate_divergence": {Unit: UnitDivergence, Timescale: TimescaleRollingWindow},
		"midpoint_return_rate_zscore":     {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"midpoint_return_velocity":        {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
		"midpoint_return_zscore":          {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"negative_midpoint_return":        {Unit: UnitLogReturn, Timescale: TimescaleRollingWindow},
		"notional_rate":                   {Unit: UnitNotionalRate, Timescale: TimescaleRollingWindow},
		"notional_rate_baseline":          {Unit: UnitNotionalRate, Timescale: TimescaleRollingWindow},
		"notional_rate_divergence":        {Unit: UnitDivergence, Timescale: TimescaleRollingWindow},
		"notional_rate_ratio":             {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"notional_rate_velocity":          {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
		"notional_rate_zscore":            {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"positive_midpoint_return":        {Unit: UnitLogReturn, Timescale: TimescaleRollingWindow},
		"relative_spread":                 {Unit: UnitRelativeSpread, Timescale: TimescaleInstantaneous},
		"relative_spread_baseline":        {Unit: UnitRelativeSpread, Timescale: TimescaleRollingWindow},
		"spread":                          {Unit: UnitSpread, Timescale: TimescaleInstantaneous},
		"spread_divergence":               {Unit: UnitDivergence, Timescale: TimescaleRollingWindow},
		"spread_divergence_velocity":      {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
		"spread_ratio":                    {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"spread_zscore":                   {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"trade_interval_seconds":          {Unit: UnitSecond, Timescale: TimescaleInstantaneous},
		"trade_notional":                  {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"trade_price":                     {Unit: UnitPrice, Timescale: TimescaleInstantaneous},
		"trade_quantity":                  {Unit: UnitQuantity, Timescale: TimescaleInstantaneous},
		"trade_rate":                      {Unit: UnitTradeRate, Timescale: TimescaleRollingWindow},
		"volume_bar_duration":             {Unit: UnitSecond, Timescale: TimescaleVolumeBar},
		"volume_bar_notional":             {Unit: UnitNotional, Timescale: TimescaleVolumeBar},
		"volume_bar_quantity":             {Unit: UnitQuantity, Timescale: TimescaleVolumeBar},
		"volume_bar_trade_count":          {Unit: UnitCount, Timescale: TimescaleVolumeBar},
		"volume_rate":                     {Unit: UnitVolumeRate, Timescale: TimescaleVolumeBar},
		"historical_path_distance":        {Unit: UnitDistance, Timescale: TimescaleRollingWindow},
		"historical_path_percentile":      {Unit: UnitProbability, Timescale: TimescaleRollingWindow},
	},
	"sentiment": {
		"absolute_return":                  {Unit: UnitLogReturn, Timescale: TimescaleRollingWindow},
		"advance_count":                    {Unit: UnitCount, Timescale: TimescaleRollingWindow},
		"advance_fraction":                 {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"breadth":                          {Unit: UnitImbalance, Timescale: TimescaleRollingWindow},
		"breadth_baseline":                 {Unit: UnitImbalance, Timescale: TimescaleRollingWindow},
		"breadth_divergence":               {Unit: UnitDivergence, Timescale: TimescaleRollingWindow},
		"breadth_velocity":                 {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
		"breadth_zscore":                   {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"cohort_horizon_seconds":           {Unit: UnitSecond, Timescale: TimescaleRollingWindow},
		"cohort_member_count":              {Unit: UnitCount, Timescale: TimescaleRollingWindow},
		"decline_count":                    {Unit: UnitCount, Timescale: TimescaleRollingWindow},
		"decline_fraction":                 {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"directional_agreement":            {Unit: UnitImbalance, Timescale: TimescaleRollingWindow},
		"directional_consensus":            {Unit: UnitImbalance, Timescale: TimescaleRollingWindow},
		"directional_participation":        {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"excluded_member_count":            {Unit: UnitCount, Timescale: TimescaleRollingWindow},
		"from_age_seconds":                 {Unit: UnitSecond, Timescale: TimescaleInstantaneous},
		"historical_path_distance":         {Unit: UnitDistance, Timescale: TimescaleRollingWindow},
		"historical_path_percentile":       {Unit: UnitProbability, Timescale: TimescaleRollingWindow},
		"largest_absolute_return":          {Unit: UnitLogReturn, Timescale: TimescaleRollingWindow},
		"largest_move_excess":              {Unit: UnitLogReturn, Timescale: TimescaleRollingWindow},
		"largest_move_mad_excess":          {Unit: UnitLogReturn, Timescale: TimescaleRollingWindow},
		"largest_move_ratio":               {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"largest_move_ratio_baseline":      {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"largest_move_ratio_zscore":        {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"largest_move_share":               {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"largest_move_share_baseline":      {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"largest_move_share_zscore":        {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"largest_move_tie_count":           {Unit: UnitCount, Timescale: TimescaleInstantaneous},
		"largest_signed_return":            {Unit: UnitLogReturn, Timescale: TimescaleRollingWindow},
		"magnitude_mad":                    {Unit: UnitLogReturn, Timescale: TimescaleRollingWindow},
		"max_asof_age_seconds":             {Unit: UnitSecond, Timescale: TimescaleInstantaneous},
		"mean_absolute_return":             {Unit: UnitLogReturn, Timescale: TimescaleRollingWindow},
		"median_absolute_return":           {Unit: UnitLogReturn, Timescale: TimescaleRollingWindow},
		"median_absolute_return_baseline":  {Unit: UnitLogReturn, Timescale: TimescaleRollingWindow},
		"median_absolute_return_ratio":     {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"median_absolute_return_velocity":  {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
		"median_absolute_return_zscore":    {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"median_asof_age_seconds":          {Unit: UnitSecond, Timescale: TimescaleInstantaneous},
		"median_from_age_seconds":          {Unit: UnitSecond, Timescale: TimescaleInstantaneous},
		"median_return":                    {Unit: UnitLogReturn, Timescale: TimescaleRollingWindow},
		"median_return_baseline":           {Unit: UnitLogReturn, Timescale: TimescaleRollingWindow},
		"median_return_divergence":         {Unit: UnitDivergence, Timescale: TimescaleRollingWindow},
		"median_return_velocity":           {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
		"median_return_zscore":             {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"opposite_direction_peer_count":    {Unit: UnitCount, Timescale: TimescaleRollingWindow},
		"opposite_direction_peer_fraction": {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"peer_magnitude_mad":               {Unit: UnitLogReturn, Timescale: TimescaleRollingWindow},
		"peer_median_absolute_return":      {Unit: UnitLogReturn, Timescale: TimescaleRollingWindow},
		"return":                           {Unit: UnitLogReturn, Timescale: TimescaleRollingWindow},
		"return_dispersion_baseline":       {Unit: UnitLogReturn, Timescale: TimescaleRollingWindow},
		"return_dispersion_ratio":          {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"return_dispersion_velocity":       {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
		"return_dispersion_zscore":         {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"return_interquartile_range":       {Unit: UnitLogReturn, Timescale: TimescaleRollingWindow},
		"return_mad":                       {Unit: UnitLogReturn, Timescale: TimescaleRollingWindow},
		"rms_return":                       {Unit: UnitLogReturn, Timescale: TimescaleRollingWindow},
		"same_direction_peer_count":        {Unit: UnitCount, Timescale: TimescaleRollingWindow},
		"same_direction_peer_fraction":     {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"unchanged_count":                  {Unit: UnitCount, Timescale: TimescaleRollingWindow},
		"unchanged_fraction":               {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"valid_member_count":               {Unit: UnitCount, Timescale: TimescaleRollingWindow},
		"zero_return_peer_count":           {Unit: UnitCount, Timescale: TimescaleRollingWindow},
		"zero_return_peer_fraction":        {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
	},
	"toxicity": {
		"best_price:ask":                      {Unit: UnitPrice, Timescale: TimescaleInstantaneous},
		"best_price:bid":                      {Unit: UnitPrice, Timescale: TimescaleInstantaneous},
		"bracket_trade_quantity":              {Unit: UnitQuantity, Timescale: TimescaleInstantaneous},
		"fill_fraction_baseline:ask":          {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"fill_fraction_baseline:bid":          {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"fill_fraction_divergence:ask":        {Unit: UnitDivergence, Timescale: TimescaleRollingWindow},
		"fill_fraction_divergence:bid":        {Unit: UnitDivergence, Timescale: TimescaleRollingWindow},
		"fill_fraction_velocity:ask":          {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
		"fill_fraction_velocity:bid":          {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
		"fill_fraction_zscore:ask":            {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"fill_fraction_zscore:bid":            {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"matched_touch_trade_quantity:ask":    {Unit: UnitQuantity, Timescale: TimescaleInstantaneous},
		"matched_touch_trade_quantity:bid":    {Unit: UnitQuantity, Timescale: TimescaleInstantaneous},
		"net_replenished_quantity:ask":        {Unit: UnitQuantity, Timescale: TimescaleInstantaneous},
		"net_replenished_quantity:bid":        {Unit: UnitQuantity, Timescale: TimescaleInstantaneous},
		"net_replenishment_fraction:ask":      {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"net_replenishment_fraction:bid":      {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"net_replenishment_rate:ask":          {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"net_replenishment_rate:bid":          {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"net_withdrawal_fraction:ask":         {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"net_withdrawal_fraction:bid":         {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"net_withdrawal_rate:ask":             {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"net_withdrawal_rate:bid":             {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"net_withdrawn_quantity:ask":          {Unit: UnitQuantity, Timescale: TimescaleInstantaneous},
		"net_withdrawn_quantity:bid":          {Unit: UnitQuantity, Timescale: TimescaleInstantaneous},
		"previous_best_price:ask":             {Unit: UnitPrice, Timescale: TimescaleInstantaneous},
		"previous_best_price:bid":             {Unit: UnitPrice, Timescale: TimescaleInstantaneous},
		"previous_touch_quantity:ask":         {Unit: UnitQuantity, Timescale: TimescaleInstantaneous},
		"previous_touch_quantity:bid":         {Unit: UnitQuantity, Timescale: TimescaleInstantaneous},
		"replenishment_fraction_baseline:ask": {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"replenishment_fraction_baseline:bid": {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"retreat_fraction:ask":                {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"retreat_fraction:bid":                {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"retreat_fraction_baseline:ask":       {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"retreat_fraction_baseline:bid":       {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"retreat_fraction_zscore:ask":         {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"retreat_fraction_zscore:bid":         {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"retreat_rate:ask":                    {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"retreat_rate:bid":                    {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"retreated_quantity:ask":              {Unit: UnitQuantity, Timescale: TimescaleInstantaneous},
		"retreated_quantity:bid":              {Unit: UnitQuantity, Timescale: TimescaleInstantaneous},
		"touch_fill_fraction:ask":             {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"touch_fill_fraction:bid":             {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"touch_fill_quantity:ask":             {Unit: UnitQuantity, Timescale: TimescaleInstantaneous},
		"touch_fill_quantity:bid":             {Unit: UnitQuantity, Timescale: TimescaleInstantaneous},
		"touch_fill_rate:ask":                 {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"touch_fill_rate:bid":                 {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"touch_price_log_change:ask":          {Unit: UnitLogReturn, Timescale: TimescaleInstantaneous},
		"touch_price_log_change:bid":          {Unit: UnitLogReturn, Timescale: TimescaleInstantaneous},
		"touch_quantity:ask":                  {Unit: UnitQuantity, Timescale: TimescaleInstantaneous},
		"touch_quantity:bid":                  {Unit: UnitQuantity, Timescale: TimescaleInstantaneous},
		"unfilled_residual_quantity:ask":      {Unit: UnitQuantity, Timescale: TimescaleInstantaneous},
		"unfilled_residual_quantity:bid":      {Unit: UnitQuantity, Timescale: TimescaleInstantaneous},
		"withdrawal_fraction_baseline:ask":    {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"withdrawal_fraction_baseline:bid":    {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"withdrawal_fraction_divergence:ask":  {Unit: UnitDivergence, Timescale: TimescaleRollingWindow},
		"withdrawal_fraction_divergence:bid":  {Unit: UnitDivergence, Timescale: TimescaleRollingWindow},
		"withdrawal_fraction_velocity:ask":    {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
		"withdrawal_fraction_velocity:bid":    {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
		"withdrawal_fraction_zscore:ask":      {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"withdrawal_fraction_zscore:bid":      {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"historical_path_distance":            {Unit: UnitDistance, Timescale: TimescaleRollingWindow},
		"historical_path_percentile":          {Unit: UnitProbability, Timescale: TimescaleRollingWindow},
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

	for slot := 0; slot < MatchKeySlots; slot++ {
		training[fmt.Sprintf("match_key:%03d", slot)] = Dimension{Unit: UnitDimensionless, Timescale: TimescaleInstantaneous}
	}

	for idx := 0; idx < TrainingPathSlots; idx++ {
		for part := 0; part < RegionSlots; part++ {
			training[fmt.Sprintf("path_token:%02d:%d", idx, part)] = Dimension{Unit: UnitDimensionless, Timescale: TimescaleInstantaneous}
		}

		for _, prefix := range []string{"match_candidates", "match_enter", "match_exit"} {
			training[fmt.Sprintf("%s:%02d", prefix, idx+1)] = Dimension{Unit: UnitCount, Timescale: TimescaleInstantaneous}
		}
	}
}
