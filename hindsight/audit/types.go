package audit

/*
ContractBreach records one metric's violation of its mathematical contract.
*/
type ContractBreach struct {
	Metric         string  `json:"metric"`
	DeclaredUnit   string  `json:"declared_unit"`
	DeclaredDomain string  `json:"declared_domain"`
	ViolationType  string  `json:"violation_type"`
	BreachCount    int     `json:"breach_count"`
	TotalSamples   int     `json:"total_samples"`
	BreachFraction float64 `json:"breach_fraction"`
	MinVal         float64 `json:"min_val"`
	MaxVal         float64 `json:"max_val"`
	MeanVal        float64 `json:"mean_val"`
}

/*
MetricNormAudit audits normalization and standardization invariants across all metrics.
*/
type MetricNormAudit struct {
	TotalMetricsAudited     int     `json:"total_metrics_audited"`
	NormalizationBreaches   int     `json:"normalization_breaches"`
	StandardizationBreaches int     `json:"standardization_breaches"`
	MeanZScore              float64 `json:"mean_z_score"`
	VarianceZScore          float64 `json:"variance_z_score"`
	MaxAbsoluteZ            float64 `json:"max_absolute_z"`
	SaturatedNormFraction   float64 `json:"saturated_norm_fraction"`
	SummaryText             string  `json:"summary_text"`
	Passed                  bool    `json:"passed"`
}

/*
MeasurementStateAudit audits Coherence, Maturity, Confidence, and WORM invariants across all measurements.
*/
type MeasurementStateAudit struct {
	TotalMeasurementsAudited int     `json:"total_measurements_audited"`
	UnlockedBreaches         int     `json:"unlocked_breaches"`
	CoherenceBreaches        int     `json:"coherence_breaches"`
	MaturityBreaches         int     `json:"maturity_breaches"`
	ConfidenceBreaches       int     `json:"confidence_breaches"`
	IdentityBreaches         int     `json:"identity_breaches"`
	MeanCoherence            float64 `json:"mean_coherence"`
	MedianCoherence          float64 `json:"median_coherence"`
	P95Coherence             float64 `json:"p95_coherence"`
	MeanMaturity             float64 `json:"mean_maturity"`
	MedianMaturity           float64 `json:"median_maturity"`
	P95Maturity              float64 `json:"p95_maturity"`
	MeanConfidence           float64 `json:"mean_confidence"`
	MedianConfidence         float64 `json:"median_confidence"`
	P95Confidence            float64 `json:"p95_confidence"`
	ColdStartFraction        float64 `json:"cold_start_fraction"`
	SettledFraction          float64 `json:"settled_fraction"`
	SummaryText              string  `json:"summary_text"`
	Passed                   bool    `json:"passed"`
}

/*
Stage0Contract contains the mathematical contract audit of all ingested metrics.
*/
type Stage0Contract struct {
	TotalMetricsChecked   int                   `json:"total_metrics_checked"`
	BreachingMetricsCount int                   `json:"breaching_metrics_count"`
	TotalBreaches         int                   `json:"total_breaches"`
	Breaches              []ContractBreach      `json:"breaches"`
	MetricNorm            MetricNormAudit       `json:"metric_norm"`
	MeasurementState      MeasurementStateAudit `json:"measurement_state"`
	DiagnosisText         string                `json:"diagnosis_text"`
	SummaryText           string                `json:"summary_text"`
	Passed                bool                  `json:"passed"`
}

/*
MetricStat holds vitality metrics for a single observed metric or canonical grid cell.
*/
type MetricStat struct {
	Name         string  `json:"name"`
	Count        int     `json:"count"`
	TotalTicks   int     `json:"total_ticks"`
	Coverage     float64 `json:"coverage"`
	Mean         float64 `json:"mean"`
	Variance     float64 `json:"variance"`
	Min          float64 `json:"min"`
	Max          float64 `json:"max"`
	ZeroFraction float64 `json:"zero_fraction"`
	IsConstant   bool    `json:"is_constant"`
	Status       string  `json:"status"` // "HEALTHY", "SPORADIC", "DEAD", "ZERO"
}

/*
RedundantPair holds two metrics that exhibit near-identical correlation.
*/
type RedundantPair struct {
	MetricA     string  `json:"metric_a"`
	MetricB     string  `json:"metric_b"`
	Correlation float64 `json:"correlation"`
}

/*
Stage1Vitality contains the results of the metric vitality and redundancy analysis,
split cleanly between raw producer output health and canonical grid input health.
*/
type Stage1Vitality struct {
	// Raw producer outputs (e.g. 7,134 peer-qualified named series)
	RawProducerMetrics int          `json:"raw_producer_metrics"`
	RawHealthyMetrics  int          `json:"raw_healthy_metrics"`
	RawDeadMetrics     int          `json:"raw_dead_metrics"`
	RawSporadicMetrics int          `json:"raw_sporadic_metrics"`
	RawMetrics         []MetricStat `json:"raw_metrics"`

	// Canonical grid cells (e.g. 377 canonical dimensions after peer aggregation)
	CanonicalGridCells     int             `json:"canonical_grid_cells"`
	CanonicalHealthyCells  int             `json:"canonical_healthy_cells"`
	CanonicalDeadCells     int             `json:"canonical_dead_cells"`
	CanonicalSporadicCells int             `json:"canonical_sporadic_cells"`
	CanonicalCells         []MetricStat    `json:"canonical_cells"`
	RedundantPairs         []RedundantPair `json:"redundant_pairs"`
	SummaryText            string          `json:"summary_text"`
	Status                 string          `json:"status"` // "MEASURED", "INSUFFICIENT_DATA"
	Passed                 bool            `json:"passed"` // compatibility: true when experiment executed with sufficient data
}

/*
SympathyNullDistribution holds the empirical null distribution generated by permutation testing.
*/
type SympathyNullDistribution struct {
	MeanConcordance float64   `json:"mean_concordance"`
	StdConcordance  float64   `json:"std_concordance"`
	Percentile95    float64   `json:"percentile_95"`
	Percentile99    float64   `json:"percentile_99"`
	HistogramBins   []float64 `json:"histogram_bins"`
	HistogramCounts []int     `json:"histogram_counts"`
}

/*
Stage2Sympathy contains pairwise relationship statistics compared against the shuffled null.
*/
type Stage2Sympathy struct {
	TotalPairs       int                      `json:"total_pairs"`
	PositivePairs    int                      `json:"positive_pairs"`
	InversePairs     int                      `json:"inverse_pairs"`
	RealMean         float64                  `json:"real_mean"`
	RealStd          float64                  `json:"real_std"`
	RealBins         []float64                `json:"real_bins"`
	RealCounts       []int                    `json:"real_counts"`
	NullDistribution SympathyNullDistribution `json:"null_distribution"`
	SeparationRatio  float64                  `json:"separation_ratio"` // Fraction of pairs exceeding 95th percentile null
	KSStatistic      float64                  `json:"ks_statistic"`     // Kolmogorov-Smirnov distance vs null
	SummaryText      string                   `json:"summary_text"`
	Status           string                   `json:"status"` // "MEASURED", "INSUFFICIENT_DATA"
	Passed           bool                     `json:"passed"` // compatibility: true when experiment executed with sufficient data
}

/*
GridPartitionStat holds information about one developed grid's partition.
*/
type GridPartitionStat struct {
	PeriodName     string         `json:"period_name"`
	CellCount      int            `json:"cell_count"`
	RegionCount    int            `json:"region_count"`
	RegionSizes    map[string]int `json:"region_sizes"`
	MaxRegionShare float64        `json:"max_region_share"`
	IsDegenerate   bool           `json:"is_degenerate"`
}

/*
GridStabilityObservation is one independent comparison of two disjoint
chronological grid-development windows.
*/
type GridStabilityObservation struct {
	WindowTicks  int     `json:"window_ticks"`
	PairIndex    int     `json:"pair_index"`
	SharedCells  int     `json:"shared_cells"`
	Overlap      float64 `json:"overlap"`
	AdjustedRand float64 `json:"adjusted_rand"`
	NullMeanARI  float64 `json:"null_mean_ari"`
}

/*
Stage3GridStability compares grids developed from disjoint chronological market segments.
The primary fields retain the largest half-vs-half comparison; StabilityCurve
contains repeated disjoint comparisons at multiple evidence sizes.
*/
type Stage3GridStability struct {
	GridA                GridPartitionStat          `json:"grid_a"`
	GridB                GridPartitionStat          `json:"grid_b"`
	SharedUniverse       int                        `json:"shared_universe"`
	OverlapFraction      float64                    `json:"overlap_fraction"`
	RandIndex            float64                    `json:"rand_index"`
	AdjustedRandIdx      float64                    `json:"adjusted_rand_idx"`
	NullAdjustedRandMean float64                    `json:"null_adjusted_rand_mean"`
	DistributionJSD      float64                    `json:"distribution_jsd"`
	DistributionTVD      float64                    `json:"distribution_tvd"`
	IsStationary         bool                       `json:"is_stationary"`
	StabilityCurve       []GridStabilityObservation `json:"stability_curve"`
	SummaryText          string                     `json:"summary_text"`
	Status               string                     `json:"status"` // "MEASURED", "INSUFFICIENT_DATA"
	Passed               bool                       `json:"passed"` // compatibility: true when experiment executed with sufficient data
}

/*
RegionStrengthStat records excitation magnitude, active cell support, and margin
over runner-up regions when a specific region lights up.
*/
type RegionStrengthStat struct {
	Region       string  `json:"region"`
	Emissions    int     `json:"emissions"`
	MeanScore    float64 `json:"mean_score"`
	MinScore     float64 `json:"min_score"`
	MaxScore     float64 `json:"max_score"`
	MeanActive   float64 `json:"mean_active"`
	MeanMembers  float64 `json:"mean_members"`
	MeanCoverage float64 `json:"mean_coverage"`
	MeanMargin   float64 `json:"mean_margin"` // separation margin over runner-up region
}

/*
GridDampeningAudit audits attenuation across regions caused by Coherence, Maturity, and Confidence.
*/
type GridDampeningAudit struct {
	AuditedPasses         int     `json:"audited_passes"`
	MeanDampeningRatio    float64 `json:"mean_dampening_ratio"`
	MedianDampeningRatio  float64 `json:"median_dampening_ratio"`
	MinDampeningRatio     float64 `json:"min_dampening_ratio"`
	MaxDampeningRatio     float64 `json:"max_dampening_ratio"`
	TokenDisplacements    int     `json:"token_displacements"`
	DisplacementRate      float64 `json:"displacement_rate"`
	SuppressedActivations int     `json:"suppressed_activations"`
	MeanWinnerConfidence  float64 `json:"mean_winner_confidence"`
	SummaryText           string  `json:"summary_text"`
	Status                string  `json:"status"` // "MEASURED", "INSUFFICIENT_DATA"
	Passed                bool    `json:"passed"`
}

/*
Stage4TokenDynamics records region token emissions, excitation strengths, and state transition structure on unseen data.
*/
type Stage4TokenDynamics struct {
	TotalEmissions                 int                           `json:"total_emissions"`
	UniqueTokens                   int                           `json:"unique_tokens"`
	TokenFrequencies               map[string]int                `json:"token_frequencies"`
	MaxTokenDominance              float64                       `json:"max_token_dominance"`
	MeanExcitationStrength         float64                       `json:"mean_excitation_strength"`
	PeakExcitationStrength         float64                       `json:"peak_excitation_strength"`
	MeanActiveCoverage             float64                       `json:"mean_active_coverage"`
	MeanRunnerUpMargin             float64                       `json:"mean_runner_up_margin"`
	RegionStrengths                map[string]RegionStrengthStat `json:"region_strengths,omitempty"`
	Dampening                      GridDampeningAudit            `json:"dampening"`
	TransitionEntropy              float64                       `json:"transition_entropy"`
	NullTransitionEntropy          float64                       `json:"null_transition_entropy"`
	EntropyReductionBits           float64                       `json:"entropy_reduction_bits"`
	Transitions                    map[string]map[string]int     `json:"transitions"`
	CompressedEmissions            int                           `json:"compressed_emissions"`
	CompressedUniqueTokens         int                           `json:"compressed_unique_tokens"`
	CompressedTransitions          map[string]map[string]int     `json:"compressed_transitions,omitempty"`
	CompressedTransitionEntropy    float64                       `json:"compressed_transition_entropy"`
	CompressedNullEntropy          float64                       `json:"compressed_null_entropy"`
	CompressedEntropyReductionBits float64                       `json:"compressed_entropy_reduction_bits"`
	AlwaysStayAccuracy             float64                       `json:"always_stay_accuracy"`
	MarginalAccuracy               float64                       `json:"marginal_accuracy"`
	SummaryText                    string                        `json:"summary_text"`
	Status                         string                        `json:"status"` // "MEASURED", "INSUFFICIENT_DATA"
	Passed                         bool                          `json:"passed"` // compatibility: true when experiment executed with sufficient data
}

/*
PrecursorHypothesis evaluates one specific precursor hypothesis against its control.
*/
type PrecursorHypothesis struct {
	Name              string         `json:"name"`
	Description       string         `json:"description"`
	EventTokens       map[string]int `json:"event_tokens"`
	ControlTokens     map[string]int `json:"control_tokens"`
	EventTokenCount   int            `json:"event_token_count"`
	ControlTokenCount int            `json:"control_token_count"`
	DivergenceBits    float64        `json:"divergence_bits"`
	NullDivergence95  float64        `json:"null_divergence_95"`
	SeparationRatio   float64        `json:"separation_ratio"`
	Status            string         `json:"status"` // "PASS", "FAIL", "INSUFFICIENT_DATA"
	Passed            bool           `json:"passed"`
}

/*
PrecursorPredictiveSkill records held-out classification capability of precursor tokens anticipating excursions.
*/
type PrecursorPredictiveSkill struct {
	EvaluatedSamples   int      `json:"evaluated_samples"`
	TopPrecursorTokens []string `json:"top_precursor_tokens,omitempty"`
	Precision          float64  `json:"precision"`
	Recall             float64  `json:"recall"`
	BalancedAccuracy   float64  `json:"balanced_accuracy"`
	MCC                float64  `json:"mcc"`
	PriorBaseRate      float64  `json:"prior_base_rate"`
	PredictiveGainBits float64  `json:"predictive_gain_bits"` // Mutual information I(Token; Excursion)
	Status             string   `json:"status"`               // "MEASURED", "INSUFFICIENT_DATA"
	Passed             bool     `json:"passed"`
}

/*
PrecursorEconomicRelevance measures whether detected excursions yield returns clearing explicit taker frictions.
*/
type PrecursorEconomicRelevance struct {
	TakerFeeRate           float64 `json:"taker_fee_rate"`
	RoundTripFeeRate       float64 `json:"round_trip_fee_rate"`
	EvaluatedExcursions    int     `json:"evaluated_excursions"`
	GrossMeanReturn        float64 `json:"gross_mean_return"`
	NetMeanReturn          float64 `json:"net_mean_return"`
	FrictionClearanceRate  float64 `json:"friction_clearance_rate"` // fraction of excursions with NetReturn > 0
	ProfitableExcursions   int     `json:"profitable_excursions"`
	UnprofitableExcursions int     `json:"unprofitable_excursions"`
	Status                 string  `json:"status"` // "MEASURED", "INSUFFICIENT_DATA"
	Passed                 bool    `json:"passed"`
}

/*
Stage5PrecursorSeparation tests whether token sequences preceding B (ignition) and C (exhaustion)
are distinct from negative controls and pure background tape, and evaluates predictive skill and economic relevance.
*/
type Stage5PrecursorSeparation struct {
	DetectionsFound      int                        `json:"detections_found"`
	ExcursionsFound      []string                   `json:"excursions_found,omitempty"`
	IgnitionHypothesis   PrecursorHypothesis        `json:"ignition_hypothesis"`   // A -> B
	ExhaustionHypothesis PrecursorHypothesis        `json:"exhaustion_hypothesis"` // B -> C
	PredictiveSkill      PrecursorPredictiveSkill   `json:"predictive_skill"`      // Anticipation & Classification
	EconomicRelevance    PrecursorEconomicRelevance `json:"economic_relevance"`    // Friction Clearance
	BackgroundTokens     map[string]int             `json:"background_tokens,omitempty"`
	SummaryText          string                     `json:"summary_text"`
	Passed               bool                       `json:"passed"`
}

/*
TrieNodeMetrics details the structural graph metrics of the association trie.
*/
type TrieNodeMetrics struct {
	TotalNodes      int     `json:"total_nodes"`
	MaxDepth        int     `json:"max_depth"`
	MeanDepth       float64 `json:"mean_depth"`
	BranchingFactor float64 `json:"branching_factor"`
}

/*
TrieSkillMetrics records prequential recall predictive performance versus baselines and nulls.
*/
type TrieSkillMetrics struct {
	TotalCalls               int     `json:"total_calls"`
	Hits                     int     `json:"hits"`
	HitRate                  float64 `json:"hit_rate"`
	BalancedAccuracy         float64 `json:"balanced_accuracy"`
	MCC                      float64 `json:"mcc"`
	EnterPrecision           float64 `json:"enter_precision"`
	EnterRecall              float64 `json:"enter_recall"`
	BaselineHits             int     `json:"baseline_hits"`
	BaselineHitRate          float64 `json:"baseline_hit_rate"`
	BestBaselinePolicy       string  `json:"best_baseline_policy"`
	BaselineBalancedAccuracy float64 `json:"baseline_balanced_accuracy"`
	NullMeanHits             float64 `json:"null_mean_hits"`
	NullStdHits              float64 `json:"null_std_hits"`
	Null95thPercentileHits   float64 `json:"null_95th_percentile_hits"`
	NullMeanBalancedAccuracy float64 `json:"null_mean_balanced_accuracy"`
	Null95thBalancedAccuracy float64 `json:"null_95th_balanced_accuracy"`
	SeparatesFromNull        bool    `json:"separates_from_null"`
	EmpiricalPValue          float64 `json:"empirical_p_value"`
}

/*
Stage0Timing contains the microstructure timing and clock synchronization audit.
Verifies exchange-to-local clock drift, latency jitter, and ingress monotonicity.
*/
type Stage0Timing struct {
	TotalChecked       int     `json:"total_checked"`
	MeanDriftMs        float64 `json:"mean_drift_ms"`
	MaxDriftMs         float64 `json:"max_drift_ms"`
	P95DriftMs         float64 `json:"p95_drift_ms"`
	LatencySpikes      int     `json:"latency_spikes"`
	SequenceInversions int     `json:"sequence_inversions"`
	SummaryText        string  `json:"summary_text"`
	Status             string  `json:"status"` // "MEASURED", "INSUFFICIENT_DATA"
	Passed             bool    `json:"passed"`
}

/*
TrieRetentionMetrics measures catastrophic interference / memory preservation after sequential updates.
*/
type TrieRetentionMetrics struct {
	TotalTaught   int     `json:"total_taught"`
	RetainedCount int     `json:"retained_count"`
	RetentionRate float64 `json:"retention_rate"`
}

/*
TrieTopologyMetrics contains census counts and node depth geometry from the trie export.
*/
type TrieTopologyMetrics struct {
	RecordsCount float64         `json:"records_count"`
	SpanCount    float64         `json:"span_count"`
	EnterBasins  float64         `json:"enter_basins"`
	ExitBasins   float64         `json:"exit_basins"`
	TotalBasins  int             `json:"total_basins"`
	NodeStats    TrieNodeMetrics `json:"node_stats"`
}

/*
Stage6CognitiveTrie audits the associative memory and Radix Trie learning dynamics:
prequential predictive skill vs constant policy and shuffled null, memory retention,
basin structure, and background false-alarm rates.
*/
type Stage6CognitiveTrie struct {
	DetectionsEvaluated int                  `json:"detections_evaluated"`
	PhasesFormed        int                  `json:"phases_formed"`
	ActionCounts        map[string]int       `json:"action_counts"`
	Skill               TrieSkillMetrics     `json:"skill"`
	Retention           TrieRetentionMetrics `json:"retention"`
	Topology            TrieTopologyMetrics  `json:"topology"`
	AbstentionRate      float64              `json:"abstention_rate"`
	MeanConfidence      float64              `json:"mean_confidence"`
	MeanContrast        float64              `json:"mean_contrast"`
	SpuriousTriggerRate float64              `json:"spurious_trigger_rate"`
	S3Memory            S3MemoryAudit        `json:"s3_memory"`
	SummaryText         string               `json:"summary_text"`
	Status              string               `json:"status"` // "MEASURED", "INSUFFICIENT_DATA", "CONTRACT_BREACH"
	Passed              bool                 `json:"passed"`
}

/*
S3MemoryAudit audits the S3-compatible prefix-memory key tree:
prefix collisions, enter vs exit continuation conflicts, support counts,
time-to-disambiguation, prequential retrieval accuracy, and key growth.
*/
type S3MemoryAudit struct {
	TotalPrefixKeys          int     `json:"total_prefix_keys"`
	PrefixCollisions         int     `json:"prefix_collisions"`
	ConflictingContinuations int     `json:"conflicting_continuations"`
	MeanPrefixDepth          float64 `json:"mean_prefix_depth"`
	MaxPrefixDepth           int     `json:"max_prefix_depth"`
	TimeToDisambiguation     int     `json:"time_to_disambiguation"`
	PrequentialRetrievalAcc  float64 `json:"prequential_retrieval_acc"`
	StorageBytes             int64   `json:"storage_bytes"`
	SummaryText              string  `json:"summary_text"`
	Passed                   bool    `json:"passed"`
}

/*
EquivalenceDiscrepancy records a single difference between production and audit pipelines.
*/
type EquivalenceDiscrepancy struct {
	Tick       int64   `json:"tick"`
	Symbol     string  `json:"symbol"`
	Source     string  `json:"source"`
	Metric     string  `json:"metric"`
	AuditVal   float64 `json:"audit_val"`
	ProdVal    float64 `json:"prod_val"`
	Difference float64 `json:"difference"`
}

/*
EquivalenceAudit verifies that audit execution produces identical metrics,
tokens, and excursion boundaries when compared to production paths.
*/
type EquivalenceAudit struct {
	CommitHash         string                   `json:"commit_hash"`
	ExecutablePath     string                   `json:"executable_path"`
	TotalTicksReplayed int                      `json:"total_ticks_replayed"`
	TotalTokensChecked int                      `json:"total_tokens_checked"`
	TokenMismatches    int                      `json:"token_mismatches"`
	MetricMismatches   int                      `json:"metric_mismatches"`
	Discrepancies      []EquivalenceDiscrepancy `json:"discrepancies,omitempty"`
	SummaryText        string                   `json:"summary_text"`
	Passed             bool                     `json:"passed"`
}

/*
MetricDiscrepancy records an inconsistency between raw market events and computed signal metrics.
*/
type MetricDiscrepancy struct {
	Signal      string  `json:"signal"`
	Metric      string  `json:"metric"`
	ObservedVal float64 `json:"observed_val"`
	ExpectedVal float64 `json:"expected_val"`
	Error       float64 `json:"error"`
}

/*
TruthfulnessAudit verifies that published metrics truthfully reflect raw market events
according to their mathematical specification.
*/
type TruthfulnessAudit struct {
	TotalChecked        int                 `json:"total_checked"`
	ViolationsCount     int                 `json:"violations_count"`
	ZeroFilledMidpoints int                 `json:"zero_filled_midpoints"`
	SyntheticTimeSteps  int                 `json:"synthetic_time_steps"`
	Discrepancies       []MetricDiscrepancy `json:"discrepancies,omitempty"`
	SummaryText         string              `json:"summary_text"`
	Passed              bool                `json:"passed"`
}

/*
CausalityAudit verifies temporal isolation (no future information leakage) and
cross-symbol state independence.
*/
type CausalityAudit struct {
	FuturePerturbationTicks int    `json:"future_perturbation_ticks"`
	LeakageDetected         bool   `json:"leakage_detected"`
	FirstDivergenceTick     int64  `json:"first_divergence_tick"`
	ContaminatedCount       int    `json:"contaminated_count"`
	CrossSymbolLeakage      bool   `json:"cross_symbol_leakage"`
	EpochIsolationPassed    bool   `json:"epoch_isolation_passed"`
	SummaryText             string `json:"summary_text"`
	Passed                  bool   `json:"passed"`
}

/*
FamilySensitivityStat records how removing a single signal family impacts the grid.
*/
type FamilySensitivityStat struct {
	Family            string  `json:"family"`
	RemovedJSD        float64 `json:"removed_jsd"`
	PrecursorSurvived bool    `json:"precursor_survived"`
	IsDominant        bool    `json:"is_dominant"`
}

/*
SensitivityAudit tests grid dependence on individual signal families,
resistance to family duplication, and resilience to temporal shuffling.
*/
type SensitivityAudit struct {
	FamiliesTested         []FamilySensitivityStat `json:"families_tested"`
	DuplicationResistant   bool                    `json:"duplication_resistant"`
	ShuffledNoiseResilient bool                    `json:"shuffled_noise_resilient"`
	SummaryText            string                  `json:"summary_text"`
	Passed                 bool                    `json:"passed"`
}

/*
AuditReport is the consolidated top-level payload written to audit_results.json.
*/
type AuditReport struct {
	Timestamp       string                    `json:"timestamp"`
	Epoch           int64                     `json:"epoch"`
	Symbol          string                    `json:"symbol"`
	TotalTicks      int                       `json:"total_ticks"`
	Contract        Stage0Contract            `json:"contract"`
	Timing          Stage0Timing              `json:"timing"`
	Vitality        Stage1Vitality            `json:"vitality"`
	Sympathy        Stage2Sympathy            `json:"sympathy"`
	GridStability   Stage3GridStability       `json:"grid_stability"`
	TokenDynamics   Stage4TokenDynamics       `json:"token_dynamics"`
	Precursor       Stage5PrecursorSeparation `json:"precursor"`
	CognitiveTrie   Stage6CognitiveTrie       `json:"cognitive_trie"`
	Equivalence     *EquivalenceAudit         `json:"equivalence,omitempty"`
	Truthfulness    *TruthfulnessAudit        `json:"truthfulness,omitempty"`
	Causality       *CausalityAudit           `json:"causality,omitempty"`
	Sensitivity     *SensitivityAudit         `json:"sensitivity,omitempty"`
	OverallHealthy  bool                      `json:"overall_healthy"`
	SummaryMarkdown string                    `json:"summary_markdown"`
}
