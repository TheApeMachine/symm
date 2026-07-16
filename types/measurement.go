package types

import (
	"errors"
	"time"
)

/*
MetricType identifies the numerical quantity a signal measured. It keeps
measurement identity independent from the signal implementation that produced
the value, which lets logic compare compatible evidence without interpreting a
source name as a market state.
*/
type MetricType string

const (
	MetricEventCount           MetricType = "event_count"
	MetricArrivalRate          MetricType = "arrival_rate"
	MetricConditionalIntensity MetricType = "conditional_intensity"
	MetricBaselineIntensity    MetricType = "baseline_intensity"
	MetricExcitationAmplitude  MetricType = "excitation_amplitude"
	MetricDecayRate            MetricType = "decay_rate"
	MetricKernelMemory         MetricType = "kernel_memory"
	MetricSpectralRadius       MetricType = "spectral_radius"
	MetricHawkesPoissonDelta   MetricType = "hawkes_poisson_likelihood_delta"
	MetricCrossSelfDelta       MetricType = "cross_self_likelihood_delta"
	MetricImmediateOffspring   MetricType = "immediate_expected_offspring"
	MetricTotalDescendants     MetricType = "expected_total_descendants"
	MetricRVOL                 MetricType = "rvol"
	MetricPrecursor            MetricType = "precursor"
	MetricSpread               MetricType = "spread"
	MetricCompression          MetricType = "compression"
	MetricIgnition             MetricType = "ignition"
	MetricTrend                MetricType = "trend"
	MetricExhaustion           MetricType = "exhaustion"
	MetricStrength             MetricType = "strength"
	MetricValue                MetricType = "value"
	MetricCategory             MetricType = "category"
	MetricResonanceEnergy      MetricType = "resonance_energy"
	MetricResonanceSurprise    MetricType = "resonance_surprise"

	// exhaust (microstructure decay)
	MetricMechanical MetricType = "mechanical"
	MetricThermal    MetricType = "thermal"
	MetricFragile    MetricType = "fragile"
	MetricReversal   MetricType = "reversal"
	MetricUrgency    MetricType = "urgency"

	// depthflow (touch-level book imbalance)
	MetricLoadedScore  MetricType = "loaded_score"
	MetricSpoofScore   MetricType = "spoof_score"
	MetricThinScore    MetricType = "thin_score"
	MetricNeutralScore MetricType = "neutral_score"

	// cvd (signed aggressor flow)
	MetricAbsorption  MetricType = "absorption"
	MetricDrive       MetricType = "drive"
	MetricBalance     MetricType = "balance"
	MetricStarvation  MetricType = "starvation"
	MetricNet         MetricType = "net"
	MetricNetFraction MetricType = "net_fraction"

	// correlation and leadlag (cohort relation)
	MetricCorrelation              MetricType = "correlation"
	MetricSigned                   MetricType = "signed"
	MetricRelativeEnergy           MetricType = "relative_energy"
	MetricHerdScore                MetricType = "herd_score"
	MetricAlphaScore               MetricType = "alpha_score"
	MetricNoiseScore               MetricType = "noise_score"
	MetricStressScore              MetricType = "stress_score"
	MetricPeakScore                MetricType = "peak_score"
	MetricSignedCorrelation        MetricType = "signed_correlation"
	MetricSignedContempCorrelation MetricType = "signed_contemp_correlation"
	MetricSignedLagCorrelation     MetricType = "signed_lag_correlation"
	MetricLagFraction              MetricType = "lag_fraction"
	MetricSampleSupport            MetricType = "sample_support"
	MetricInefficient              MetricType = "inefficient"
	MetricSync                     MetricType = "sync"
	MetricDecoupled                MetricType = "decoupled"
	MetricStall                    MetricType = "stall"

	// sentiment (breadth and leadership)
	MetricChange         MetricType = "change"
	MetricBreadth        MetricType = "breadth"
	MetricLeaderStrength MetricType = "leader_strength"
	MetricLeaderEvidence MetricType = "leader_evidence"
	MetricRelativeLead   MetricType = "relative_lead"
	MetricSurgeScore     MetricType = "surge_score"
	MetricDivergentScore MetricType = "divergent_score"
	MetricSlumpScore     MetricType = "slump_score"

	// liquidity (peer notional and depth scarcity)
	MetricScarcityScore         MetricType = "scarcity_score"
	MetricPeerBalanceScore      MetricType = "peer_balance_score"
	MetricDepthScore            MetricType = "depth_score"
	MetricQuoteNotional         MetricType = "quote_notional"
	MetricQuoteNotionalMedian   MetricType = "quote_notional_median"
	MetricExecutableDepth       MetricType = "executable_depth"
	MetricExecutableDepthMedian MetricType = "executable_depth_median"
	MetricRelativeLiquidity     MetricType = "relative_liquidity"

	// fluid (mechanical order-book dynamics)
	MetricLaminarScore        MetricType = "laminar_score"
	MetricTurbulentScore      MetricType = "turbulent_score"
	MetricInertialScore       MetricType = "inertial_score"
	MetricViscousScore        MetricType = "viscous_score"
	MetricViscosity           MetricType = "viscosity"
	MetricReynolds            MetricType = "reynolds"
	MetricDivergenceV2        MetricType = "divergence_v2"
	MetricVelocityCurvatureV2 MetricType = "velocity_curvature_v2"
	MetricTurbulence          MetricType = "turbulence"
	MetricSourceBalance       MetricType = "source_balance"
	MetricMemory              MetricType = "memory"
	MetricMidAddRate          MetricType = "mid_add_rate"
	MetricMidExecuteRate      MetricType = "mid_execute_rate"

	// toxicity (level3 touch liquidity honesty)
	MetricTouchQuantity      MetricType = "touch_quantity"
	MetricBestPrice          MetricType = "best_price"
	MetricTradeVolume        MetricType = "trade_volume"
	MetricFillVolume         MetricType = "fill_volume"
	MetricRetreatingQuantity MetricType = "retreating_quantity"
	MetricCancelledQuantity  MetricType = "cancelled_quantity"
)

/*
SubjectType identifies the market object or model component described by a
metric. Metrics with the same unit are not necessarily comparable when their
subjects differ.
*/
type SubjectType string

const (
	SubjectTradeArrivals   SubjectType = "trade_arrivals"
	SubjectHawkesProcess   SubjectType = "hawkes_process"
	SubjectHawkesKernel    SubjectType = "hawkes_kernel"
	SubjectHawkesFit       SubjectType = "hawkes_fit"
	SubjectManifoldState   SubjectType = "manifold_state"
	SubjectPumpVolumeLift  SubjectType = "pump_volume_lift"
	SubjectPumpPriceLift   SubjectType = "pump_price_lift"
	SubjectPumpSpread      SubjectType = "pump_spread"
	SubjectPumpCompression SubjectType = "pump_compression"
	SubjectPumpIgnition    SubjectType = "pump_ignition"
	SubjectPumpTrend       SubjectType = "pump_trend"
	SubjectPumpExhaustion  SubjectType = "pump_exhaustion"
	SubjectPumpComposite   SubjectType = "pump_composite"
	SubjectBookImbalance   SubjectType = "book_imbalance"
	SubjectAggressorFlow   SubjectType = "aggressor_flow"
	SubjectPeerLiquidity   SubjectType = "peer_liquidity"
	SubjectLevel3Touch     SubjectType = "level3_touch"
	SubjectLevel3Tape      SubjectType = "level3_tape"
)

/*
MeasurementSide preserves directional semantics, including the source and
target of a bivariate interaction.
*/
type MeasurementSide string

const (
	SideNone       MeasurementSide = ""
	SideBuy        MeasurementSide = "buy"
	SideSell       MeasurementSide = "sell"
	SideBuyToBuy   MeasurementSide = "buy_to_buy"
	SideSellToBuy  MeasurementSide = "sell_to_buy"
	SideBuyToSell  MeasurementSide = "buy_to_sell"
	SideSellToSell MeasurementSide = "sell_to_sell"
)

/*
MeasurementUnit retains the dimensional meaning of Raw so normalization never
erases what the original value represented.
*/
type MeasurementUnit string

const (
	UnitCount                      MeasurementUnit = "count"
	UnitDimensionless              MeasurementUnit = "dimensionless"
	UnitEventsPerSecond            MeasurementUnit = "events_per_second"
	UnitInverseSecond              MeasurementUnit = "inverse_second"
	UnitNat                        MeasurementUnit = "nat"
	UnitSecond                     MeasurementUnit = "second"
	UnitQuoteCurrency              MeasurementUnit = "quote_currency"
	UnitBaseCurrency               MeasurementUnit = "base_currency"
	UnitQuoteCurrencyPerSecond     MeasurementUnit = "quote_currency_per_second"
	UnitBaseCurrencyPerSecond      MeasurementUnit = "base_currency_per_second"
	UnitInverseQuoteCurrencySecond MeasurementUnit = "inverse_quote_currency_second"
)

/*
MeasurementUncertainty reports an interval the estimator actually calculated.
A nil *MeasurementUncertainty on Measurement stays explicit about "no
interval" and must not be read as zero uncertainty.
*/
type MeasurementUncertainty struct {
	Lower      float64 `json:"lower,omitempty"`
	Upper      float64 `json:"upper,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
	Method     string  `json:"method,omitempty"`
}

/*
ValidityState distinguishes a usable numerical estimate from a provisional or
failed one without inventing replacement values.
*/
type ValidityState string

const (
	ValidityValid       ValidityState = "valid"
	ValidityProvisional ValidityState = "provisional"
	ValidityInvalid     ValidityState = "invalid"
)

/*
MeasurementReadiness states the strongest use supported by the evidence. Each
level includes the levels before it; forecast readiness therefore requires a
validated model rather than merely a successful fit.
*/
type MeasurementReadiness string

const (
	ReadinessObservation MeasurementReadiness = "observation"
	ReadinessIntensity   MeasurementReadiness = "intensity"
	ReadinessModel       MeasurementReadiness = "model"
	ReadinessForecast    MeasurementReadiness = "forecast"
)

/*
MeasurementValidity carries estimator readiness and any reason a value cannot
yet be used at a stronger layer.
*/
type MeasurementValidity struct {
	State     ValidityState        `json:"state"`
	Readiness MeasurementReadiness `json:"readiness"`
	Reason    string               `json:"reason,omitempty"`
}

/*
ObservationValidity derives observation-layer validity from how many
corroborating events contributed to the current window. A lone event stays
provisional so downstream logic and UI can distinguish thin batches from
multi-event corroboration while ReadinessObservation remains unchanged.
*/
func ObservationValidity(evidenceCount int) MeasurementValidity {
	validity := MeasurementValidity{
		Readiness: ReadinessObservation,
	}

	if evidenceCount <= 0 {
		validity.State = ValidityInvalid
		validity.Reason = "no observation evidence"

		return validity
	}

	if evidenceCount == 1 {
		validity.State = ValidityProvisional
		validity.Reason = "single observation in window"

		return validity
	}

	validity.State = ValidityValid

	return validity
}

/*
ScaleType identifies the baseline or observation epoch that gives a normalized
or estimated value its local scale.
*/
type ScaleType string

const (
	ScaleObservationWindow ScaleType = "observation_window"
)

/*
ScaleReference identifies the exact data interval used to establish scale so
measurements from different adaptive epochs are not silently compared.
*/
type ScaleReference struct {
	Kind    ScaleType `json:"kind"`
	From    time.Time `json:"from"`
	Through time.Time `json:"through"`
}

/*
StreamType identifies the source of a measurement.
*/
type StreamType string

const (
	Correlation StreamType = "correlation"
	Covariance  StreamType = "covariance"
	CVD         StreamType = "cvd"
	DepthFlow   StreamType = "depth_flow"
	Exhaust     StreamType = "exhaust"
	Fluid       StreamType = "fluid"
	Hawkes      StreamType = "hawkes"
	LeadLag     StreamType = "lead_lag"
	Liquidity   StreamType = "liquidity"
	PumpDump    StreamType = "pump_dump"
	Resonance   StreamType = "resonance"
	Sentiment   StreamType = "sentiment"
	Toxicity    StreamType = "toxicity"
)

/*
Measurement is one immutable numerical observation emitted by a signal. New
signals use the typed fields above the compatibility block; the legacy fields
remain only while older signals are migrated to the same contract.
*/
type Measurement struct {
	Source       SourceType              `json:"source"`
	Metric       MetricType              `json:"metric,omitempty"`
	Subject      SubjectType             `json:"subject,omitempty"`
	Stream       StreamType              `json:"stream,omitempty"`
	Symbol       string                  `json:"symbol" validate:"required"`
	Side         MeasurementSide         `json:"side,omitempty"`
	At           time.Time               `json:"at" validate:"required"`
	ObservedFrom time.Time               `json:"observedFrom,omitempty"`
	Horizon      time.Duration           `json:"horizon,omitempty" validate:"nonnegative"`
	Unit         MeasurementUnit         `json:"unit,omitempty"`
	Raw          float64                 `json:"raw" validate:"finite"`
	Normalized   *float64                `json:"normalized" validate:"finite"`
	Maturity     float64                 `json:"maturity,omitempty" validate:"finite"`
	Uncertainty  *MeasurementUncertainty `json:"uncertainty"`
	Validity     MeasurementValidity     `json:"validity"`
	Scale        ScaleReference          `json:"scale"`
}

/*
CanonicalizeProvenance folds every declared timestamp into one forward evidence
window so graph validation reflects the full span instead of rejecting mixed
producer fields.
*/
func (measurement *Measurement) CanonicalizeProvenance() {
	stamps := make([]time.Time, 0, 4)

	for _, stamp := range []time.Time{
		measurement.At,
		measurement.ObservedFrom,
		measurement.Scale.From,
		measurement.Scale.Through,
	} {
		if stamp.IsZero() {
			continue
		}

		stamps = append(stamps, stamp)
	}

	if len(stamps) == 0 {
		return
	}

	from := stamps[0]
	through := stamps[0]

	for _, stamp := range stamps[1:] {
		if stamp.Before(from) {
			from = stamp
		}

		if stamp.After(through) {
			through = stamp
		}
	}

	measurement.ObservedFrom = from
	measurement.At = through
	measurement.Horizon = through.Sub(from)

	if measurement.Scale.Kind != "" ||
		!measurement.Scale.From.IsZero() ||
		!measurement.Scale.Through.IsZero() {
		if measurement.Scale.Kind == "" {
			measurement.Scale.Kind = ScaleObservationWindow
		}

		measurement.Scale.From = from
		measurement.Scale.Through = through
	}
}

/*
ValidateStruct enforces the cross-field time ordering that tags cannot express.
A measurement may omit an explicit scale interval, but any interval it does
resolve must run forward.
*/
func (measurement *Measurement) ValidateStruct() error {
	if measurement == nil {
		return errors.New("measurement required")
	}

	measurement.CanonicalizeProvenance()

	from, through := measurement.Interval()
	scaleBackwards := !measurement.Scale.From.IsZero() &&
		!measurement.Scale.Through.IsZero() &&
		measurement.Scale.Through.Before(measurement.Scale.From)

	if scaleBackwards || through.Before(from) {
		return errors.New("evidence interval ends before it starts")
	}

	return nil
}

/*
Interval resolves the evidence interval once using the measurement's declared
provenance, then its scale, then its observation time. Relationship composition
uses this canonical policy instead of reconstructing it at every call site.
*/
func (measurement Measurement) Interval() (time.Time, time.Time) {
	stamps := make([]time.Time, 0, 4)

	for _, stamp := range []time.Time{
		measurement.At,
		measurement.ObservedFrom,
		measurement.Scale.From,
		measurement.Scale.Through,
	} {
		if stamp.IsZero() {
			continue
		}

		stamps = append(stamps, stamp)
	}

	if len(stamps) == 0 {
		return time.Time{}, time.Time{}
	}

	from := stamps[0]
	through := stamps[0]

	for _, stamp := range stamps[1:] {
		if stamp.Before(from) {
			from = stamp
		}

		if stamp.After(through) {
			through = stamp
		}
	}

	return from, through
}
