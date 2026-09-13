package correlation

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Fold folds the admitted peers into one cohort summary and derives the
focal-to-cohort relative return energy rate. Without admitted peers there is
nothing to fold and the measurement moves through untouched. Every cohort
fact is written where it is computed.
*/
type Fold struct {
	err    error
	cohort core.Primitive
}

func NewFold() core.Primitive {
	return &Fold{cohort: NewCohort()}
}

func (op *Fold) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement[float64])(arriving)

			if m.Err != nil || len(m.Peers) == 0 {
				if !yield(arriving) {
					return
				}

				continue
			}

			admitted := make([]Peer, len(m.Peers))

			for index, peer := range m.Peers {
				admitted[index] = Peer{
					Correlation: peer.Metrics["signed_correlation"].Raw,
					Support:     peer.Metadata["support"],
					PeerEnergy:  peer.Metadata["peer_energy_rate"],
				}
			}

			summary := fold(op.cohort, admitted)

			if err := op.cohort.Error(); err != nil {
				m.Err = errors.Join(m.Err, err)
				op.Error(err)

				if !yield(arriving) {
					return
				}

				continue
			}

			m.Metrics["cohort_signed_correlation"] = m.Metrics["cohort_signed_correlation"].Write(summary.SignedCorrelation)
			m.Metrics["cohort_absolute_correlation"] = m.Metrics["cohort_absolute_correlation"].Write(summary.AbsoluteCorrelation)
			m.Metrics["cohort_peer_count"] = m.Metrics["cohort_peer_count"].Write(summary.Peers)
			m.Metrics["cohort_effective_peer_count"] = m.Metrics["cohort_effective_peer_count"].Write(summary.EffectivePeers)

			if summary.FisherDefined {
				m.Metrics["cohort_correlation_dispersion"] = m.Metrics["cohort_correlation_dispersion"].Write(summary.Dispersion)
			}

			if summary.PeerEnergyRate > 0 {
				m.Metrics["peer_return_energy_rate"] = m.Metrics["peer_return_energy_rate"].Write(summary.PeerEnergyRate)
				m.Metrics["relative_return_energy"] = m.Metrics["relative_return_energy"].Write(
					m.Metrics["return_energy_rate:measured"].Raw / summary.PeerEnergyRate,
				)
			}

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *Fold) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
History feeds the cohort's signed correlation to the Fisher-space causal
estimator, giving the measurement its baseline, divergence, and z-score.
*/
type History struct {
	err       error
	estimator core.Primitive
}

func NewHistory() core.Primitive {
	return &History{estimator: NewFisherEstimator()}
}

func (op *History) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement[float64])(arriving)

			if m.Err != nil || len(m.Peers) == 0 {
				if !yield(arriving) {
					return
				}

				continue
			}

			signed := m.Metrics["cohort_signed_correlation"].Raw
			view := drive[float64, FisherView](op.estimator, &signed)

			m.Metrics["correlation_baseline"] = m.Metrics["correlation_baseline"].Write(view.Baseline)
			m.Metrics["correlation_divergence"] = m.Metrics["correlation_divergence"].Write(view.Divergence)
			m.Metrics["correlation_zscore"] = m.Metrics["correlation_zscore"].Write(view.ZScore)

			if view.Defined {
				m.Metadata[data.MetadataDivergence] = view.Divergence
				m.Metadata[data.MetadataSupport] = view.Count

				if view.VarianceDefined {
					m.Metadata[data.MetadataNoiseVariance] = view.Variance
				}
			}

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *History) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
Relative tracks the focal-to-cohort relative return energy rate against its
own adaptive baseline.
*/
type Relative struct {
	err      error
	baseline core.Primitive
}

func NewRelative() core.Primitive {
	return &Relative{baseline: adaptive.NewBaseline(adaptive.NewWindow())}
}

func (op *Relative) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement[float64])(arriving)

			if m.Err != nil || len(m.Peers) == 0 {
				if !yield(arriving) {
					return
				}

				continue
			}

			relative := m.Metrics["relative_return_energy"].Raw
			reading := drive[float64, adaptive.BaselineReading](op.baseline, &relative)

			m.Metrics["relative_return_energy_baseline"] = m.Metrics["relative_return_energy_baseline"].Write(reading.Baseline)
			m.Metrics["relative_return_energy_divergence"] = m.Metrics["relative_return_energy_divergence"].Write(reading.Residual)
			m.Metrics["relative_return_energy_zscore"] = m.Metrics["relative_return_energy_zscore"].Write(reading.ZScore)

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *Relative) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
CorrelationVelocity measures how fast the cohort's signed correlation moves.
*/
type CorrelationVelocity struct {
	err      error
	velocity core.Primitive
}

func NewCorrelationVelocity() core.Primitive {
	return &CorrelationVelocity{velocity: temporal.NewVelocity()}
}

func (op *CorrelationVelocity) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement[float64])(arriving)

			if m.Err != nil || len(m.Peers) == 0 {
				if !yield(arriving) {
					return
				}

				continue
			}

			observation := temporal.Observation{
				Value: m.Metrics["cohort_signed_correlation"].Raw,
				At:    m.At.UnixNano(),
			}

			reading := drive[temporal.Observation, temporal.VelocityReading](op.velocity, &observation)

			if reading.Defined {
				m.Metrics["correlation_velocity"] = m.Metrics["correlation_velocity"].Write(reading.Rate)
			}

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *CorrelationVelocity) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
EnergyVelocity measures how fast the relative return energy rate moves.
*/
type EnergyVelocity struct {
	err      error
	velocity core.Primitive
}

func NewEnergyVelocity() core.Primitive {
	return &EnergyVelocity{velocity: temporal.NewVelocity()}
}

func (op *EnergyVelocity) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement[float64])(arriving)

			if m.Err != nil || len(m.Peers) == 0 {
				if !yield(arriving) {
					return
				}

				continue
			}

			observation := temporal.Observation{
				Value: m.Metrics["relative_return_energy"].Raw,
				At:    m.At.UnixNano(),
			}

			reading := drive[temporal.Observation, temporal.VelocityReading](op.velocity, &observation)

			if reading.Defined {
				m.Metrics["relative_return_energy_velocity"] = m.Metrics["relative_return_energy_velocity"].Write(reading.Rate)
			}

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *EnergyVelocity) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
fold pushes one peer run through the cohort reduction and returns the summary.
*/
func fold(op core.Primitive, peers []Peer) CohortSummary {
	var summary CohortSummary

	for out := range op.Next(transport.NewValues(peers...).Next(nil)) {
		summary = *(*CohortSummary)(out)
	}

	return summary
}
