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
	UnitPriceImpact              Unit = "price_impact"
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
	"ingress": {
		"ask":         {Unit: UnitPrice, Timescale: TimescaleInstantaneous},
		"ask_qty":     {Unit: UnitQuantity, Timescale: TimescaleInstantaneous},
		"bid":         {Unit: UnitPrice, Timescale: TimescaleInstantaneous},
		"bid_qty":     {Unit: UnitQuantity, Timescale: TimescaleInstantaneous},
		"limit_price": {Unit: UnitPrice, Timescale: TimescaleInstantaneous},
		"order_qty":   {Unit: UnitQuantity, Timescale: TimescaleInstantaneous},
		"price":       {Unit: UnitPrice, Timescale: TimescaleInstantaneous},
		"qty":         {Unit: UnitQuantity, Timescale: TimescaleInstantaneous},
		"volume":      {Unit: UnitVolume, Timescale: TimescaleInstantaneous},
	},
	"correlation": {
		"absolute_correlation":              {Unit: UnitCorrelation, Timescale: TimescaleRollingWindow},
		"cohort_absolute_correlation":       {Unit: UnitCorrelation, Timescale: TimescaleRollingWindow},
		"cohort_correlation_dispersion":     {Unit: UnitStandardDeviation, Timescale: TimescaleRollingWindow},
		"cohort_effective_peer_count":       {Unit: UnitCount, Timescale: TimescaleRollingWindow},
		"cohort_peer_count":                 {Unit: UnitCount, Timescale: TimescaleRollingWindow},
		"cohort_signed_correlation":         {Unit: UnitCorrelation, Timescale: TimescaleRollingWindow},
		"correlation_baseline":              {Unit: UnitCorrelation, Timescale: TimescaleRollingWindow},
		"correlation_divergence":            {Unit: UnitCorrelation, Timescale: TimescaleRollingWindow},
		"correlation_p_value":               {Unit: UnitProbability, Timescale: TimescaleRollingWindow},
		"correlation_standard_error_fisher": {Unit: UnitStandardDeviation, Timescale: TimescaleRollingWindow},
		"correlation_velocity":              {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
		"correlation_zscore":                {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"covariance":                        {Unit: UnitCovariance, Timescale: TimescaleRollingWindow},
		"effective_sample_count":            {Unit: UnitCount, Timescale: TimescaleRollingWindow},
		"focal_return_energy_rate":          {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"historical_path_distance":          {Unit: UnitDistance, Timescale: TimescaleRollingWindow},
		"historical_path_percentile":        {Unit: UnitProbability, Timescale: TimescaleRollingWindow},
		"last_price":                        {Unit: UnitPrice, Timescale: TimescaleInstantaneous},
		"observation_count":                 {Unit: UnitCount, Timescale: TimescaleRollingWindow},
		"overlap_density":                   {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"overlap_pair_count":                {Unit: UnitCount, Timescale: TimescaleRollingWindow},
		"peer_return_energy_rate":           {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"relative_cohort_return_energy":     {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"relative_return_energy":            {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"relative_return_energy_baseline":   {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"relative_return_energy_divergence": {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"relative_return_energy_velocity":   {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
		"relative_return_energy_zscore":     {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"return_energy:measured":            {Unit: UnitVariance, Timescale: TimescaleRollingWindow},
		"return_energy:reference":           {Unit: UnitVariance, Timescale: TimescaleRollingWindow},
		"return_energy_rate:measured":       {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"return_energy_rate:reference":      {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"shared_time":                       {Unit: UnitSecond, Timescale: TimescaleRollingWindow},
		"signed_correlation":                {Unit: UnitCorrelation, Timescale: TimescaleRollingWindow},
		"supported_return_count:measured":   {Unit: UnitCount, Timescale: TimescaleRollingWindow},
		"supported_return_count:reference":  {Unit: UnitCount, Timescale: TimescaleRollingWindow},
	},
	"cvd": {
		"aggressive_notional:buy":            {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"aggressive_notional:sell":           {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"buy_notional_rate":                  {Unit: UnitNotionalRate, Timescale: TimescaleRollingWindow},
		"cumulative_notional_delta":          {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"cumulative_volume_delta":            {Unit: UnitQuantity, Timescale: TimescaleInstantaneous},
		"cvd_epoch_from":                     {Unit: UnitNanosecond, Timescale: TimescaleInstantaneous},
		"executed_quantity:buy":              {Unit: UnitQuantity, Timescale: TimescaleInstantaneous},
		"executed_quantity:sell":             {Unit: UnitQuantity, Timescale: TimescaleInstantaneous},
		"flow_aligned_midpoint_return":       {Unit: UnitLogReturn, Timescale: TimescaleRollingWindow},
		"gross_executed_quantity":            {Unit: UnitQuantity, Timescale: TimescaleInstantaneous},
		"gross_notional":                     {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"gross_notional_rate":                {Unit: UnitNotionalRate, Timescale: TimescaleRollingWindow},
		"gross_notional_rate_baseline":       {Unit: UnitNotionalRate, Timescale: TimescaleRollingWindow},
		"gross_notional_rate_divergence":     {Unit: UnitNotionalRate, Timescale: TimescaleRollingWindow},
		"gross_notional_rate_ratio":          {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"gross_notional_rate_velocity":       {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
		"gross_notional_rate_zscore":         {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"historical_path_distance":          {Unit: UnitDistance, Timescale: TimescaleRollingWindow},
		"historical_path_percentile":        {Unit: UnitProbability, Timescale: TimescaleRollingWindow},
		"mean_trade_notional":                {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"midpoint_log_return":                {Unit: UnitLogReturn, Timescale: TimescaleRollingWindow},
		"midpoint_response_per_net_notional": {Unit: UnitPriceImpact, Timescale: TimescaleRollingWindow},
		"midpoint_return_rate":               {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"midpoint_return_rate_baseline":      {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"midpoint_return_rate_divergence":    {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"midpoint_return_rate_zscore":        {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"net_executed_quantity":              {Unit: UnitQuantity, Timescale: TimescaleInstantaneous},
		"net_notional":                       {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"net_notional_rate":                  {Unit: UnitNotionalRate, Timescale: TimescaleRollingWindow},
		"net_notional_rate_velocity":         {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
		"response_midpoint:at":               {Unit: UnitPrice, Timescale: TimescaleInstantaneous},
		"response_midpoint:from":             {Unit: UnitPrice, Timescale: TimescaleInstantaneous},
		"sell_notional_rate":                 {Unit: UnitNotionalRate, Timescale: TimescaleRollingWindow},
		"signed_count_fraction":              {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"signed_net_fraction":                {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"signed_net_fraction:buy":            {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"signed_net_fraction_baseline":       {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"signed_net_fraction_divergence":     {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"signed_net_fraction_mean":           {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"signed_net_fraction_zscore":         {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"trade_count":                        {Unit: UnitCount, Timescale: TimescaleInstantaneous},
		"trade_count:buy":                    {Unit: UnitCount, Timescale: TimescaleInstantaneous},
		"trade_count:sell":                   {Unit: UnitCount, Timescale: TimescaleInstantaneous},
		"trade_rate":                         {Unit: UnitTradeRate, Timescale: TimescaleRollingWindow},
	},
	"depthflow": {
		"add_notional:ask":                          {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"add_notional:bid":                          {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"added_notional:ask":                        {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"added_notional:bid":                        {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"added_notional_rate:ask":                   {Unit: UnitNotionalRate, Timescale: TimescaleRollingWindow},
		"added_notional_rate:bid":                   {Unit: UnitNotionalRate, Timescale: TimescaleRollingWindow},
		"book_imbalance":                            {Unit: UnitRatio, Timescale: TimescaleInstantaneous},
		"book_imbalance_baseline":                   {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"book_imbalance_divergence":                 {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"book_imbalance_velocity":                   {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
		"book_imbalance_zscore":                     {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"book_notional":                             {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"book_notional:ask":                         {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"book_notional:bid":                         {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"book_turnover_rate":                        {Unit: UnitNotionalRate, Timescale: TimescaleRollingWindow},
		"delete_count:ask":                          {Unit: UnitCount, Timescale: TimescaleInstantaneous},
		"delete_count:bid":                          {Unit: UnitCount, Timescale: TimescaleInstantaneous},
		"flow_activity_imbalance":                   {Unit: UnitRatio, Timescale: TimescaleInstantaneous},
		"historical_path_distance":                  {Unit: UnitDistance, Timescale: TimescaleRollingWindow},
		"historical_path_percentile":                {Unit: UnitProbability, Timescale: TimescaleRollingWindow},
		"imbalance_resolution_distance":             {Unit: UnitDistance, Timescale: TimescaleInstantaneous},
		"imbalance_resolution_gap":                  {Unit: UnitRatio, Timescale: TimescaleInstantaneous},
		"modify_remaining_notional:ask":             {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"modify_remaining_notional:bid":             {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"mutation_activity_imbalance":               {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"mutation_count":                            {Unit: UnitCount, Timescale: TimescaleInstantaneous},
		"mutation_count:ask":                        {Unit: UnitCount, Timescale: TimescaleInstantaneous},
		"mutation_count:bid":                        {Unit: UnitCount, Timescale: TimescaleInstantaneous},
		"mutation_count_diff":                       {Unit: UnitCount, Timescale: TimescaleInstantaneous},
		"net_book_change_rate":                      {Unit: UnitNotionalRate, Timescale: TimescaleRollingWindow},
		"net_book_change_rate_baseline":             {Unit: UnitNotionalRate, Timescale: TimescaleRollingWindow},
		"net_book_change_rate_divergence":           {Unit: UnitNotionalRate, Timescale: TimescaleRollingWindow},
		"net_book_change_rate_zscore":               {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"net_displayed_flow:ask":                    {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"net_displayed_flow:bid":                    {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"net_displayed_flow_rate:ask":               {Unit: UnitNotionalRate, Timescale: TimescaleRollingWindow},
		"net_displayed_flow_rate:bid":               {Unit: UnitNotionalRate, Timescale: TimescaleRollingWindow},
		"observed_notional":                         {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"observed_notional:ask":                     {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"observed_notional:bid":                     {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"observed_notional_diff":                    {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"observed_notional_imbalance":               {Unit: UnitRatio, Timescale: TimescaleInstantaneous},
		"observed_notional_imbalance_baseline":      {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"observed_notional_imbalance_divergence":    {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"observed_notional_imbalance_mean":          {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"observed_notional_imbalance_zscore":        {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"observed_notional_rate":                    {Unit: UnitNotionalRate, Timescale: TimescaleRollingWindow},
		"observed_notional_rate_baseline":           {Unit: UnitNotionalRate, Timescale: TimescaleRollingWindow},
		"observed_notional_rate_divergence":         {Unit: UnitNotionalRate, Timescale: TimescaleRollingWindow},
		"observed_notional_rate_mean":               {Unit: UnitNotionalRate, Timescale: TimescaleRollingWindow},
		"observed_notional_rate_zscore":             {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"removed_notional:ask":                      {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"removed_notional:bid":                      {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"removed_notional_rate:ask":                 {Unit: UnitNotionalRate, Timescale: TimescaleRollingWindow},
		"removed_notional_rate:bid":                 {Unit: UnitNotionalRate, Timescale: TimescaleRollingWindow},
		"resolution_gap_baseline":                   {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"resolution_gap_divergence":                 {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"resolution_gap_velocity":                   {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
		"resolution_gap_zscore":                     {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"signed_net_displayed_flow_rate":            {Unit: UnitNotionalRate, Timescale: TimescaleRollingWindow},
		"signed_net_displayed_flow_rate_baseline":   {Unit: UnitNotionalRate, Timescale: TimescaleRollingWindow},
		"signed_net_displayed_flow_rate_divergence": {Unit: UnitNotionalRate, Timescale: TimescaleRollingWindow},
		"signed_net_displayed_flow_rate_zscore":     {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"touch_imbalance":                           {Unit: UnitRatio, Timescale: TimescaleInstantaneous},
		"turnover_baseline":                         {Unit: UnitNotionalRate, Timescale: TimescaleRollingWindow},
		"turnover_divergence":                       {Unit: UnitNotionalRate, Timescale: TimescaleRollingWindow},
		"turnover_ratio":                            {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"turnover_zscore":                           {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
	},
	"derivatives": {
		"basis":                           {Unit: UnitRelativeSpread, Timescale: TimescaleRollingWindow},
		"basis_baseline":                  {Unit: UnitRelativeSpread, Timescale: TimescaleRollingWindow},
		"basis_change":                    {Unit: UnitRelativeSpread, Timescale: TimescaleInstantaneous},
		"basis_closure_error":             {Unit: UnitRelativeSpread, Timescale: TimescaleInstantaneous},
		"basis_rate":                      {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"basis_velocity":                  {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
		"basis_zscore":                    {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"derivative_index_log_basis":      {Unit: UnitLogReturn, Timescale: TimescaleRollingWindow},
		"derivative_log_return":           {Unit: UnitLogReturn, Timescale: TimescaleRollingWindow},
		"derivative_price":                {Unit: UnitPrice, Timescale: TimescaleInstantaneous},
		"derivative_spot_log_basis":       {Unit: UnitLogReturn, Timescale: TimescaleRollingWindow},
		"gross_derivative_trade_notional": {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"gross_liquidation_notional":      {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"index_spot_log_basis":            {Unit: UnitLogReturn, Timescale: TimescaleRollingWindow},
		"liquidation_notional:buy":        {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"liquidation_notional:sell":       {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"liquidation_notional_rate":       {Unit: UnitNotionalRate, Timescale: TimescaleRollingWindow},
		"liquidation_share":               {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"liquidation_share_velocity":      {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
		"liquidation_signed_fraction":     {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"log_basis":                       {Unit: UnitLogReturn, Timescale: TimescaleRollingWindow},
		"net_liquidation_notional":        {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"open_interest":                   {Unit: UnitQuantity, Timescale: TimescaleInstantaneous},
		"open_interest_change":            {Unit: UnitQuantity, Timescale: TimescaleInstantaneous},
		"open_interest_growth_baseline":   {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"open_interest_growth_rate":       {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"open_interest_growth_velocity":   {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
		"open_interest_growth_zscore":     {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"open_interest_log_change":        {Unit: UnitLogReturn, Timescale: TimescaleInstantaneous},
		"reference_log_return":            {Unit: UnitLogReturn, Timescale: TimescaleRollingWindow},
		"reference_price":                 {Unit: UnitPrice, Timescale: TimescaleInstantaneous},
		"return_gap":                      {Unit: UnitLogReturn, Timescale: TimescaleRollingWindow},
		"return_gap_velocity":             {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
		"return_gap_zscore":               {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"spot_price":                      {Unit: UnitPrice, Timescale: TimescaleInstantaneous},
	},
	"exhaustion": {
		"book_imbalance":                {Unit: UnitRatio, Timescale: TimescaleInstantaneous},
		"book_imbalance_baseline":       {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"book_imbalance_change":         {Unit: UnitRatio, Timescale: TimescaleInstantaneous},
		"book_imbalance_velocity":       {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
		"book_imbalance_zscore":         {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"depth_baseline:ask":            {Unit: UnitNotional, Timescale: TimescaleRollingWindow},
		"depth_baseline:bid":            {Unit: UnitNotional, Timescale: TimescaleRollingWindow},
		"depth_divergence:ask":          {Unit: UnitNotional, Timescale: TimescaleRollingWindow},
		"depth_divergence:bid":          {Unit: UnitNotional, Timescale: TimescaleRollingWindow},
		"depth_divergence_velocity:ask": {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
		"depth_divergence_velocity:bid": {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
		"depth_ratio:ask":               {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"depth_ratio:bid":               {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"depth_zscore:ask":              {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"depth_zscore:bid":              {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"displayed_depth_notional":      {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"displayed_depth_notional:ask":  {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"displayed_depth_notional:bid":  {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"midpoint_log_return":           {Unit: UnitLogReturn, Timescale: TimescaleRollingWindow},
		"previous_book_imbalance":       {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"relative_spread":               {Unit: UnitRelativeSpread, Timescale: TimescaleInstantaneous},
		"relative_spread_baseline":      {Unit: UnitRelativeSpread, Timescale: TimescaleRollingWindow},
		"spread":                        {Unit: UnitSpread, Timescale: TimescaleInstantaneous},
		"spread_divergence":             {Unit: UnitSpread, Timescale: TimescaleRollingWindow},
		"spread_divergence_velocity":    {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
		"spread_ratio":                  {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"spread_zscore":                 {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"total_depth_baseline":          {Unit: UnitNotional, Timescale: TimescaleRollingWindow},
		"total_depth_ratio":             {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"total_depth_zscore":            {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
	},
	"hawkes": {
		"arrival_rate":                               {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"arrival_rate:buy":                           {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"arrival_rate:sell":                          {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"background_rate":                            {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"background_rate:buy":                        {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"background_rate:sell":                       {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"branching_spectral_radius":                  {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"compensator:buy":                            {Unit: UnitCount, Timescale: TimescaleInstantaneous},
		"compensator:sell":                           {Unit: UnitCount, Timescale: TimescaleInstantaneous},
		"conditional_intensity":                      {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"conditional_intensity:buy":                  {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"conditional_intensity:sell":                 {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"conditional_intensity_velocity":             {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
		"count_innovation:buy":                       {Unit: UnitCount, Timescale: TimescaleInstantaneous},
		"count_innovation:sell":                      {Unit: UnitCount, Timescale: TimescaleInstantaneous},
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
		"excitation_decay:buy_from_buy":              {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"excitation_decay:buy_from_sell":             {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"excitation_decay:sell_from_buy":             {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"excitation_decay:sell_from_sell":            {Unit: UnitRate, Timescale: TimescaleRollingWindow},
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
		"excitation_timescale:buy_from_buy":          {Unit: UnitSecond, Timescale: TimescaleRollingWindow},
		"excitation_timescale:buy_from_sell":         {Unit: UnitSecond, Timescale: TimescaleRollingWindow},
		"excitation_timescale:sell_from_buy":         {Unit: UnitSecond, Timescale: TimescaleRollingWindow},
		"excitation_timescale:sell_from_sell":        {Unit: UnitSecond, Timescale: TimescaleRollingWindow},
		"expected_descendants_from_buy":              {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"expected_descendants_from_sell":             {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"historical_path_distance":                  {Unit: UnitDistance, Timescale: TimescaleRollingWindow},
		"historical_path_percentile":                {Unit: UnitProbability, Timescale: TimescaleRollingWindow},
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
		"snr":                                        {Unit: UnitSNR, Timescale: TimescaleRollingWindow},
		"spectral_radius_velocity":                   {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
		"standardized_innovation:buy":                {Unit: UnitZScore, Timescale: TimescaleInstantaneous},
		"standardized_innovation:sell":               {Unit: UnitZScore, Timescale: TimescaleInstantaneous},
	},
	"leadlag": {
		"absolute_correlation_gain":     {Unit: UnitCorrelation, Timescale: TimescaleRollingWindow},
		"best_lag_correlation":          {Unit: UnitCorrelation, Timescale: TimescaleRollingWindow},
		"best_lag_correlation_baseline": {Unit: UnitCorrelation, Timescale: TimescaleRollingWindow},
		"best_lag_correlation_zscore":   {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"best_lag_index":                {Unit: UnitDimensionless, Timescale: TimescaleRollingWindow},
		"best_lag_seconds":              {Unit: UnitSecond, Timescale: TimescaleRollingWindow},
		"contemporaneous_correlation":   {Unit: UnitCorrelation, Timescale: TimescaleRollingWindow},
		"correlation_gain_baseline":     {Unit: UnitCorrelation, Timescale: TimescaleRollingWindow},
		"correlation_gain_velocity":     {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
		"correlation_gain_zscore":       {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"correlation_p_value":           {Unit: UnitProbability, Timescale: TimescaleRollingWindow},
		"effective_sample_count":        {Unit: UnitCount, Timescale: TimescaleRollingWindow},
		"historical_path_distance":      {Unit: UnitDistance, Timescale: TimescaleRollingWindow},
		"historical_path_percentile":    {Unit: UnitProbability, Timescale: TimescaleRollingWindow},
		"lag_baseline_seconds":          {Unit: UnitSecond, Timescale: TimescaleRollingWindow},
		"lag_divergence_seconds":        {Unit: UnitSecond, Timescale: TimescaleRollingWindow},
		"lag_fraction":                  {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"lag_noise_scale_seconds":       {Unit: UnitSecond, Timescale: TimescaleRollingWindow},
		"lag_peak_curvature":            {Unit: UnitDimensionless, Timescale: TimescaleRollingWindow},
		"lag_peak_prominence":           {Unit: UnitCorrelation, Timescale: TimescaleRollingWindow},
		"lag_search_resolution_seconds": {Unit: UnitSecond, Timescale: TimescaleRollingWindow},
		"lag_search_span":               {Unit: UnitDimensionless, Timescale: TimescaleRollingWindow},
		"lag_velocity":                  {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
		"lag_zscore":                    {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"last_price":                    {Unit: UnitPrice, Timescale: TimescaleInstantaneous},
		"measured_return_count":         {Unit: UnitCount, Timescale: TimescaleRollingWindow},
		"observation_count":             {Unit: UnitCount, Timescale: TimescaleRollingWindow},
		"overlap_pair_count":            {Unit: UnitCount, Timescale: TimescaleRollingWindow},
		"reference_return_count":        {Unit: UnitCount, Timescale: TimescaleRollingWindow},
		"reference_symbol":              {Unit: UnitDimensionless, Timescale: TimescaleRollingWindow},
		"search_adjusted_p_value":       {Unit: UnitProbability, Timescale: TimescaleRollingWindow},
		"search_count":                  {Unit: UnitCount, Timescale: TimescaleRollingWindow},
	},
	"liquidity": {
		"best_ask_price":                 {Unit: UnitPrice, Timescale: TimescaleInstantaneous},
		"best_bid_price":                 {Unit: UnitPrice, Timescale: TimescaleInstantaneous},
		"depth_divergence:ask":           {Unit: UnitNotional, Timescale: TimescaleRollingWindow},
		"depth_divergence:bid":           {Unit: UnitNotional, Timescale: TimescaleRollingWindow},
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
		"spread_divergence":              {Unit: UnitSpread, Timescale: TimescaleRollingWindow},
		"spread_divergence_velocity":     {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
		"spread_divergence_velocity_snr": {Unit: UnitSNR, Timescale: TimescaleRollingWindow},
		"spread_noise_scale":             {Unit: UnitSpread, Timescale: TimescaleRollingWindow},
		"spread_ratio":                   {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"spread_zscore":                  {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"touch_notional:ask":             {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"touch_notional:bid":             {Unit: UnitNotional, Timescale: TimescaleInstantaneous},
		"touch_notional_baseline:ask":    {Unit: UnitNotional, Timescale: TimescaleRollingWindow},
		"touch_notional_baseline:bid":    {Unit: UnitNotional, Timescale: TimescaleRollingWindow},
		"touch_notional_imbalance":       {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
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
		"midpoint_return_baseline":        {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"midpoint_return_divergence":      {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"midpoint_return_rate":            {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"midpoint_return_rate_divergence": {Unit: UnitRate, Timescale: TimescaleRollingWindow},
		"midpoint_return_rate_zscore":     {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"midpoint_return_velocity":        {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
		"midpoint_return_zscore":          {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"negative_midpoint_return":        {Unit: UnitLogReturn, Timescale: TimescaleRollingWindow},
		"notional_rate":                   {Unit: UnitNotionalRate, Timescale: TimescaleRollingWindow},
		"notional_rate_baseline":          {Unit: UnitNotionalRate, Timescale: TimescaleRollingWindow},
		"notional_rate_divergence":        {Unit: UnitNotionalRate, Timescale: TimescaleRollingWindow},
		"notional_rate_ratio":             {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"notional_rate_velocity":          {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
		"notional_rate_zscore":            {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"positive_midpoint_return":        {Unit: UnitLogReturn, Timescale: TimescaleRollingWindow},
		"relative_spread":                 {Unit: UnitRelativeSpread, Timescale: TimescaleInstantaneous},
		"relative_spread_baseline":        {Unit: UnitRelativeSpread, Timescale: TimescaleRollingWindow},
		"spread":                          {Unit: UnitSpread, Timescale: TimescaleInstantaneous},
		"spread_divergence":               {Unit: UnitSpread, Timescale: TimescaleRollingWindow},
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
		"volume_bar_target_quantity":      {Unit: UnitQuantity, Timescale: TimescaleVolumeBar},
		"volume_bar_trade_count":          {Unit: UnitCount, Timescale: TimescaleVolumeBar},
		"volume_rate":                     {Unit: UnitVolumeRate, Timescale: TimescaleVolumeBar},
	},
	"sentiment": {
		"absolute_return":                  {Unit: UnitLogReturn, Timescale: TimescaleRollingWindow},
		"advance_count":                    {Unit: UnitCount, Timescale: TimescaleRollingWindow},
		"advance_fraction":                 {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"asof_age_seconds":                 {Unit: UnitSecond, Timescale: TimescaleInstantaneous},
		"breadth":                          {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"breadth_baseline":                 {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"breadth_divergence":               {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"breadth_velocity":                 {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
		"breadth_zscore":                   {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"cohort_horizon_seconds":           {Unit: UnitSecond, Timescale: TimescaleRollingWindow},
		"cohort_member_count":              {Unit: UnitCount, Timescale: TimescaleRollingWindow},
		"decline_count":                    {Unit: UnitCount, Timescale: TimescaleRollingWindow},
		"decline_fraction":                 {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"directional_agreement":            {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"directional_consensus":            {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
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
		"median_return_divergence":         {Unit: UnitLogReturn, Timescale: TimescaleRollingWindow},
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
		"fill_fraction_divergence:ask":        {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"fill_fraction_divergence:bid":        {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"fill_fraction_mean":                  {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
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
		"retreat_fraction_baseline":           {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"retreat_fraction_baseline:ask":       {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"retreat_fraction_baseline:bid":       {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"retreat_fraction_divergence":         {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
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
		"withdrawal_fraction_baseline":        {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"withdrawal_fraction_baseline:ask":    {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"withdrawal_fraction_baseline:bid":    {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"withdrawal_fraction_divergence:ask":  {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"withdrawal_fraction_divergence:bid":  {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"withdrawal_fraction_velocity:ask":    {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
		"withdrawal_fraction_velocity:bid":    {Unit: UnitVelocity, Timescale: TimescaleRollingWindow},
		"withdrawal_fraction_zscore:ask":      {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
		"withdrawal_fraction_zscore:bid":      {Unit: UnitZScore, Timescale: TimescaleRollingWindow},
	},
	"strategy": {
		"accuracy":          {Unit: UnitProbability, Timescale: TimescaleRollingWindow},
		"action":            {Unit: UnitRatio, Timescale: TimescaleTick},
		"agent_entry":       {Unit: UnitCount, Timescale: TimescaleInstantaneous},
		"agent_exit":        {Unit: UnitCount, Timescale: TimescaleInstantaneous},
		"b_idx":             {Unit: UnitCount, Timescale: TimescaleTick},
		"b_price":           {Unit: UnitPrice, Timescale: TimescaleInstantaneous},
		"b_tick":            {Unit: UnitCount, Timescale: TimescaleTick},
		"c_idx":             {Unit: UnitCount, Timescale: TimescaleTick},
		"c_price":           {Unit: UnitPrice, Timescale: TimescaleInstantaneous},
		"c_tick":            {Unit: UnitCount, Timescale: TimescaleTick},
		"confidence":        {Unit: UnitConfidence, Timescale: TimescaleRollingWindow},
		"contrast":          {Unit: UnitRatio, Timescale: TimescaleRollingWindow},
		"cvd_value":         {Unit: UnitQuantity, Timescale: TimescaleTick},
		"decisions":         {Unit: UnitCount, Timescale: TimescaleSession},
		"delayed_target":    {Unit: UnitRatio, Timescale: TimescaleTick},
		"depth":             {Unit: UnitCount, Timescale: TimescaleTick},
		"edge":              {Unit: UnitPercent, Timescale: TimescaleRollingWindow},
		"edge_sample_count": {Unit: UnitCount, Timescale: TimescaleSession},
		"end_tick":          {Unit: UnitCount, Timescale: TimescaleTick},
		"excursion_mag":     {Unit: UnitSpread, Timescale: TimescaleEvent},
		"excursion_type":    {Unit: UnitCount, Timescale: TimescaleEvent},
		"frozen_prediction": {Unit: UnitRatio, Timescale: TimescaleTick},
		"grid_cells":        {Unit: UnitCount, Timescale: TimescaleSession},
		"grid_regions":      {Unit: UnitCount, Timescale: TimescaleSession},
		"impulse_version":   {Unit: UnitDimensionless, Timescale: TimescaleInstantaneous},
		"input_count":       {Unit: UnitCount, Timescale: TimescaleInstantaneous},
		"mark_a":            {Unit: UnitCount, Timescale: TimescaleEvent},
		"mark_b":            {Unit: UnitCount, Timescale: TimescaleEvent},
		"mark_c":            {Unit: UnitCount, Timescale: TimescaleEvent},
		"precursor_length":  {Unit: UnitCount, Timescale: TimescaleEvent},
		"predicted_entry":   {Unit: UnitCount, Timescale: TimescaleInstantaneous},
		"predicted_exit":    {Unit: UnitCount, Timescale: TimescaleInstantaneous},
		"previous_input":    {Unit: UnitCount, Timescale: TimescaleInstantaneous},
		"resolved":          {Unit: UnitCount, Timescale: TimescaleSession},
		"stage_code":        {Unit: UnitCount, Timescale: TimescaleSession},
		"start_idx":         {Unit: UnitCount, Timescale: TimescaleTick},
		"start_tick":        {Unit: UnitCount, Timescale: TimescaleTick},
		"steps":             {Unit: UnitCount, Timescale: TimescaleSession},
		"trading":           {Unit: UnitProbability, Timescale: TimescaleInstantaneous},
		"value":             {Unit: UnitCount, Timescale: TimescaleTick},
		"win_rate":          {Unit: UnitProbability, Timescale: TimescaleRollingWindow},
	},
	"manifold": {
		"coherence_mag2":     {Unit: UnitDimensionless, Timescale: TimescaleInstantaneous},
		"divergence":         {Unit: UnitRate, Timescale: TimescaleInstantaneous},
		"energy":             {Unit: UnitDimensionless, Timescale: TimescaleInstantaneous},
		"gas_internal":       {Unit: UnitDimensionless, Timescale: TimescaleInstantaneous},
		"gas_kinetic":        {Unit: UnitDimensionless, Timescale: TimescaleInstantaneous},
		"guidance_speed":     {Unit: UnitVelocity, Timescale: TimescaleInstantaneous},
		"kuramoto_psi":       {Unit: UnitDimensionless, Timescale: TimescaleInstantaneous},
		"kuramoto_r":         {Unit: UnitDimensionless, Timescale: TimescaleInstantaneous},
		"max_mach":           {Unit: UnitDimensionless, Timescale: TimescaleInstantaneous},
		"particle_count":     {Unit: UnitCount, Timescale: TimescaleInstantaneous},
		"particle_kinetic":   {Unit: UnitDimensionless, Timescale: TimescaleInstantaneous},
		"particle_thermal":   {Unit: UnitDimensionless, Timescale: TimescaleInstantaneous},
		"pressure_grad_norm": {Unit: UnitAcceleration, Timescale: TimescaleInstantaneous},
		"strain_rms":         {Unit: UnitRate, Timescale: TimescaleInstantaneous},
		"surprise":           {Unit: UnitNat, Timescale: TimescaleInstantaneous},
		"viscosity_proxy":    {Unit: UnitDimensionless, Timescale: TimescaleInstantaneous},
		"vorticity_rms":      {Unit: UnitRate, Timescale: TimescaleInstantaneous},
		"wave_norm":          {Unit: UnitDimensionless, Timescale: TimescaleInstantaneous},
	},
}

var metricDimensions = make(map[string]Dimension, 600)

func init() {
	for producer, metrics := range SignalMetrics {
		for name, dimension := range metrics {
			if existing, exists := metricDimensions[name]; exists && existing != dimension {
				panic("conflicting dimension definition for metric: " + name + " in producer: " + producer)
			}

			metricDimensions[name] = dimension
		}
	}
}

func stripScope(label string) string {
	base := label
	if atIndex := strings.IndexByte(base, '@'); atIndex != -1 {
		base = base[:atIndex]
	}

	if colonIndex := strings.IndexByte(base, ':'); colonIndex != -1 {
		base = base[:colonIndex]
	}

	return base
}

func patternDimension(base string) (Dimension, bool) {
	if strings.HasSuffix(base, "_zscore") {
		return Dimension{Unit: UnitZScore, Timescale: TimescaleRollingWindow}, true
	}

	if strings.HasSuffix(base, "_velocity") {
		return Dimension{Unit: UnitVelocity, Timescale: TimescaleRollingWindow}, true
	}

	if strings.HasSuffix(base, "_correlation") {
		return Dimension{Unit: UnitCorrelation, Timescale: TimescaleRollingWindow}, true
	}

	if strings.HasSuffix(base, "_count") || strings.HasPrefix(base, "count_") {
		return Dimension{Unit: UnitCount, Timescale: TimescaleInstantaneous}, true
	}

	if strings.HasSuffix(base, "_rate") {
		return Dimension{Unit: UnitRate, Timescale: TimescaleRollingWindow}, true
	}

	if strings.HasSuffix(base, "_seconds") || strings.HasSuffix(base, "_duration") {
		return Dimension{Unit: UnitSecond, Timescale: TimescaleInstantaneous}, true
	}

	if strings.HasSuffix(base, "_fraction") || strings.HasSuffix(base, "_ratio") || strings.HasSuffix(base, "_imbalance") {
		return Dimension{Unit: UnitRatio, Timescale: TimescaleRollingWindow}, true
	}

	if strings.HasSuffix(base, "_price") {
		return Dimension{Unit: UnitPrice, Timescale: TimescaleInstantaneous}, true
	}

	if strings.HasSuffix(base, "_quantity") || strings.HasSuffix(base, "_qty") {
		return Dimension{Unit: UnitQuantity, Timescale: TimescaleInstantaneous}, true
	}

	if strings.HasSuffix(base, "_notional") {
		return Dimension{Unit: UnitNotional, Timescale: TimescaleInstantaneous}, true
	}

	if strings.HasSuffix(base, "_p_value") || strings.HasSuffix(base, "_percentile") {
		return Dimension{Unit: UnitProbability, Timescale: TimescaleRollingWindow}, true
	}

	if strings.HasSuffix(base, "_distance") {
		return Dimension{Unit: UnitDistance, Timescale: TimescaleRollingWindow}, true
	}

	return Dimension{}, false
}


/*
CanonicalDimensions maps a metric label and its candidate unit and timescale
to its honest canonical physical unit and operational timescale.
*/
func CanonicalDimensions(label string, unit Unit, timescale Timescale) (Unit, Timescale) {
	base := label
	if atIndex := strings.IndexByte(base, '@'); atIndex != -1 {
		base = base[:atIndex]
	}

	if dimension, found := metricDimensions[base]; found {
		resolvedUnit := dimension.Unit
		if resolvedUnit == "" {
			resolvedUnit = unit
		}
		if resolvedUnit == "" {
			resolvedUnit = UnitDimensionless
		}

		resolvedTimescale := dimension.Timescale
		if resolvedTimescale == "" {
			resolvedTimescale = timescale
		}
		if resolvedTimescale == "" {
			resolvedTimescale = TimescaleInstantaneous
		}

		return resolvedUnit, resolvedTimescale
	}

	if patternDim, found := patternDimension(stripScope(base)); found {
		resolvedUnit := unit
		if resolvedUnit == "" || resolvedUnit == UnitDimensionless {
			resolvedUnit = patternDim.Unit
		}

		resolvedTimescale := timescale
		if resolvedTimescale == "" || resolvedTimescale == TimescaleInstantaneous {
			resolvedTimescale = patternDim.Timescale
		}

		return resolvedUnit, resolvedTimescale
	}

	resolvedUnit := unit
	if resolvedUnit == "" {
		resolvedUnit = UnitDimensionless
	}

	resolvedTimescale := timescale
	if resolvedTimescale == "" {
		resolvedTimescale = TimescaleInstantaneous
	}

	return resolvedUnit, resolvedTimescale
}

/*
CanonicalUnit maps a metric label and its candidate unit to the canonical
physical unit.
*/
func CanonicalUnit(label string, unit Unit) Unit {
	canonicalUnit, _ := CanonicalDimensions(label, unit, "")
	return canonicalUnit
}

/*
CanonicalTimescale maps a metric label and its candidate timescale to the
canonical operational timescale.
*/
func CanonicalTimescale(label string, timescale Timescale) Timescale {
	_, canonicalTimescale := CanonicalDimensions(label, "", timescale)
	return canonicalTimescale
}
