package types

import (
	"sync"
	"time"
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
MetricSample is one named numeric reading inside a source×symbol Measurement.
*/
type MetricSample struct {
	Raw        float64         `json:"raw"`
	Normalized *float64        `json:"normalized,omitempty"`
	Unit       MeasurementUnit `json:"unit,omitempty"`
}

/*
MeasurementArrival is the bounded marked-event evidence retained by an
incremental arrival-process measurement. It carries only the timestamp and
side the estimator needs; exchange trade payloads remain raw Thesis input.
*/
type MeasurementArrival struct {
	At   time.Time       `json:"at"`
	Side MeasurementSide `json:"side"`
}

/*
Measurement is one source×symbol observation: shared provenance plus a Metrics
map keyed like the UI wire (metric, or metric:side).

Provenance contract (set correctly at emit; never rewritten downstream):
  - At is the as-of / emit instant and is required.
  - ObservedFrom, when set, is the start of the observation window and must
    not be after At. Horizon is At−ObservedFrom when both ends are known.
  - Scale is an optional separate fit/baseline epoch. When both Scale.From and
    Scale.Through are set they must run forward; Scale is not folded into the
    observation interval.
  - Bivariate rows set PeerAt and PeerObservedFrom for the peer observation;
    these timestamps are not inferred from the local interval downstream.
*/
type Measurement struct {
	ID               string                  `json:"id"`
	Source           SourceType              `json:"source"`
	Symbol           string                  `json:"symbol" validate:"required"`
	Tick             int64                   `json:"tick,omitempty"`
	Peer             string                  `json:"peer,omitempty"`
	At               time.Time               `json:"at" validate:"required"`
	ObservedFrom     time.Time               `json:"observedFrom,omitempty"`
	Horizon          time.Duration           `json:"horizon,omitempty" validate:"nonnegative"`
	PeerAt           time.Time               `json:"peerAt,omitempty"`
	PeerObservedFrom time.Time               `json:"peerObservedFrom,omitempty"`
	Maturity         float64                 `json:"maturity,omitempty" validate:"finite"`
	Uncertainty      *MeasurementUncertainty `json:"uncertainty,omitempty"`
	Metrics          map[string]MetricSample `json:"metrics,omitempty"`
	Metadata         map[string]float64      `json:"metadata,omitempty"`
	Arrivals         []MeasurementArrival    `json:"-"`
}

/*
PutMarketValue stores one raw market value used to build this Measurement's
Metrics. It is intentionally numeric-only so this metadata stays a raw market
snapshot rather than becoming a general annotation bag.
*/
func (measurement *Measurement) PutMarketValue(key string, value float64) {
	if measurement == nil || key == "" {
		return
	}

	if measurement.Metadata == nil {
		measurement.Metadata = map[string]float64{}
	}

	measurement.Metadata[key] = value
}

/*
Key returns the stable Thesis storage identity for this source-symbol row.
Signals emit one row per source and symbol, while bivariate rows include Peer so
cross-symbol evidence does not overwrite univariate evidence.
*/
func (measurement *Measurement) Key() string {
	if measurement == nil {
		return ""
	}

	return string(measurement.Source) + ":" + measurement.Symbol + ":" + measurement.Peer
}

/*
Interval returns the observation window: ObservedFrom→At when ObservedFrom is
set, otherwise the point [At, At]. Scale is intentionally excluded.
*/
func (measurement Measurement) Interval() (time.Time, time.Time) {
	if measurement.At.IsZero() && measurement.ObservedFrom.IsZero() {
		return time.Time{}, time.Time{}
	}

	through := measurement.At
	from := measurement.ObservedFrom

	if from.IsZero() {
		from = through
	}

	return from, through
}

/*
ObservationCount returns how many distinct symbols appear in the rows.
*/
func ObservationCount(measurements *sync.Map) int {
	symbols := make(map[string]struct{})

	measurements.Range(func(_, value any) bool {
		measurement, ok := value.(*Measurement)

		if ok && measurement != nil {
			symbols[measurement.Symbol] = struct{}{}
			return true
		}

		rows, ok := value.([]*Measurement)

		if !ok {
			return true
		}

		for _, row := range rows {
			if row == nil {
				continue
			}

			symbols[row.Symbol] = struct{}{}
		}

		return true
	})

	return len(symbols)
}

