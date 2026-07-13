package types

import (
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
)

/*
SubjectType identifies the market object or model component described by a
metric. Metrics with the same unit are not necessarily comparable when their
subjects differ.
*/
type SubjectType string

const (
	SubjectTradeArrivals SubjectType = "trade_arrivals"
	SubjectHawkesProcess SubjectType = "hawkes_process"
	SubjectHawkesKernel  SubjectType = "hawkes_kernel"
	SubjectHawkesFit     SubjectType = "hawkes_fit"
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
	UnitCount           MeasurementUnit = "count"
	UnitDimensionless   MeasurementUnit = "dimensionless"
	UnitEventsPerSecond MeasurementUnit = "events_per_second"
	UnitInverseSecond   MeasurementUnit = "inverse_second"
	UnitNat             MeasurementUnit = "nat"
	UnitSecond          MeasurementUnit = "second"
)

/*
OptionalValue makes a missing normalized representation distinct from the
valid normalized value zero.
*/
type OptionalValue struct {
	Value     float64 `json:"value"`
	Available bool    `json:"available"`
}

/*
MeasurementUncertainty reports an interval only when the estimator calculated
one. An unavailable interval stays explicit and must not be read as zero
uncertainty.
*/
type MeasurementUncertainty struct {
	Available  bool    `json:"available"`
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
	DepthFlow   StreamType = "depth_flow"
	Exhaust     StreamType = "exhaust"
	Fluid       StreamType = "fluid"
	Hawkes      StreamType = "hawkes"
	LeadLag     StreamType = "lead_lag"
	Liquidity   StreamType = "liquidity"
	PumpDump    StreamType = "pump_dump"
	Sentiment   StreamType = "sentiment"
	Toxicity    StreamType = "toxicity"
)

/*
Measurement is one immutable numerical observation emitted by a signal. New
signals use the typed fields above the compatibility block; the legacy fields
remain only while older signals are migrated to the same contract.
*/
type Measurement struct {
	Source       SourceType
	Metric       MetricType
	Subject      SubjectType
	Stream       StreamType
	Symbol       string
	Side         MeasurementSide
	At           time.Time
	ObservedFrom time.Time
	Horizon      time.Duration
	Unit         MeasurementUnit
	Raw          float64
	Normalized   float64
	Maturity     float64
	Uncertainty  MeasurementUncertainty
	Validity     MeasurementValidity
	Scale        ScaleReference
}
