package correlation

import (
	"errors"
	"iter"
	"strconv"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/temporal"
)

/*
Fold folds the admitted peers into one cohort summary and derives the
focal-to-cohort relative return energy rate. Without admitted peers there is
nothing to fold and the measurement moves through untouched. Every cohort
fact is written where it is computed.
*/
type Fold struct {
	*core.PrimitiveError

	cohort core.Primitive
}

func NewFold() *Fold {
	return &Fold{PrimitiveError: core.NewPrimitiveError(), cohort: NewCohort()}
}

func (fold *Fold) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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
				support, _ := strconv.ParseFloat(peer.Metadata["support"], 64)
				peerEnergy, _ := strconv.ParseFloat(peer.Metadata["peer_energy_rate"], 64)

				admitted[index] = Peer{
					Correlation: peer.Metrics["signed_correlation"].Raw,
					Support:     support,
					PeerEnergy:  peerEnergy,
				}
			}

			summary := calculateFold(fold.cohort, admitted)

			if err := fold.cohort.Error(); err != nil {
				m.Err = errors.Join(m.Err, err)
				fold.Error(err)

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

/*
History feeds the cohort's signed correlation to the Fisher-space causal
estimator, giving the measurement its baseline, divergence, and z-score.
*/
type History struct {
	*core.PrimitiveError

	estimator core.Primitive
}

func NewHistory() *History {
	return &History{PrimitiveError: core.NewPrimitiveError(), estimator: NewFisherEstimator()}
}

func (history *History) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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
			view := drive[float64, FisherView](history.estimator, &signed)

			m.Metrics["correlation_baseline"] = m.Metrics["correlation_baseline"].Write(view.Baseline)
			m.Metrics["correlation_divergence"] = m.Metrics["correlation_divergence"].Write(view.Divergence)
			m.Metrics["correlation_zscore"] = m.Metrics["correlation_zscore"].Write(view.ZScore)

			if view.Defined {
				if m.Metadata == nil {
					m.Metadata = make(map[string]string)
				}

				m.Metadata[data.MetadataDivergence] = strconv.FormatFloat(view.Divergence, 'f', -1, 64)
				m.Metadata[data.MetadataSupport] = strconv.FormatFloat(view.Count, 'f', -1, 64)

				if view.VarianceDefined {
					m.Metadata[data.MetadataNoiseVariance] = strconv.FormatFloat(view.Variance, 'f', -1, 64)
				}
			}

			if !yield(arriving) {
				return
			}
		}
	}
}

/*
Relative tracks the focal-to-cohort relative return energy rate against its
own adaptive baseline.
*/
type Relative struct {
	*core.PrimitiveError

	baseline core.Primitive
}

func NewRelative() *Relative {
	return &Relative{PrimitiveError: core.NewPrimitiveError(), baseline: adaptive.NewBaseline(adaptive.NewWindow())}
}

func (relative *Relative) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement[float64])(arriving)

			if m.Err != nil || len(m.Peers) == 0 {
				if !yield(arriving) {
					return
				}

				continue
			}

			currentRelative := m.Metrics["relative_return_energy"].Raw
			reading := drive[float64, adaptive.BaselineReading](relative.baseline, &currentRelative)

			m.Metrics["relative_return_energy_baseline"] = m.Metrics["relative_return_energy_baseline"].Write(reading.Baseline)
			m.Metrics["relative_return_energy_divergence"] = m.Metrics["relative_return_energy_divergence"].Write(reading.Residual)
			m.Metrics["relative_return_energy_zscore"] = m.Metrics["relative_return_energy_zscore"].Write(reading.ZScore)

			if !yield(arriving) {
				return
			}
		}
	}
}

/*
CorrelationVelocity measures how fast the cohort's signed correlation moves.
*/
type CorrelationVelocity struct {
	*core.PrimitiveError

	velocity core.Primitive
}

func NewCorrelationVelocity() *CorrelationVelocity {
	return &CorrelationVelocity{PrimitiveError: core.NewPrimitiveError(), velocity: temporal.NewVelocity()}
}

func (correlationVelocity *CorrelationVelocity) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			reading := drive[temporal.Observation, temporal.VelocityReading](correlationVelocity.velocity, &observation)

			if reading.Defined {
				m.Metrics["correlation_velocity"] = m.Metrics["correlation_velocity"].Write(reading.Rate)
			}

			if !yield(arriving) {
				return
			}
		}
	}
}

/*
EnergyVelocity measures how fast the relative return energy rate moves.
*/
type EnergyVelocity struct {
	*core.PrimitiveError

	velocity core.Primitive
}

func NewEnergyVelocity() *EnergyVelocity {
	return &EnergyVelocity{PrimitiveError: core.NewPrimitiveError(), velocity: temporal.NewVelocity()}
}

func (energyVelocity *EnergyVelocity) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			reading := drive[temporal.Observation, temporal.VelocityReading](energyVelocity.velocity, &observation)

			if reading.Defined {
				m.Metrics["relative_return_energy_velocity"] = m.Metrics["relative_return_energy_velocity"].Write(reading.Rate)
			}

			if !yield(arriving) {
				return
			}
		}
	}
}

/*
fold pushes one peer run through the cohort reduction and returns the summary.
*/
func calculateFold(op core.Primitive, peers []Peer) CohortSummary {
	var summary CohortSummary

	for out := range op.Next(sequence.NewValues(peers...).Next(nil)) {
		summary = *(*CohortSummary)(out)
	}

	return summary
}
