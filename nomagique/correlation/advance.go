package correlation

import (
	"errors"
	"iter"
	"strconv"
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
			m := *(**data.Measurement)(arriving)

			if m.Error() != nil || len(m.Peers()) == 0 {
				if !yield(arriving) {
					return
				}

				continue
			}

			admitted := make([]Peer, len(m.Peers()))

			for index, peer := range m.Peers() {
				support, _ := strconv.ParseFloat(peer.Meta("support"), 64)
				peerEnergy, _ := strconv.ParseFloat(peer.Meta("peer_energy_rate"), 64)

				admitted[index] = Peer{
					Correlation: peer.Value("signed_correlation"),
					Support:     support,
					PeerEnergy:  peerEnergy,
				}
			}

			summary := fold(op.cohort, admitted)

			if err := op.cohort.Error(); err != nil {
				m.SetError(err)
				op.Error(err)

				if !yield(arriving) {
					return
				}

				continue
			}

			if summary.Defined {
				m.Put("cohort_signed_correlation", summary.SignedCorrelation)
				m.Put("cohort_absolute_correlation", summary.AbsoluteCorrelation)
				m.Put("cohort_effective_peer_count", summary.EffectivePeers)
			}
			m.Put("cohort_peer_count", summary.Peers)

			if summary.FisherDefined {
				m.Put("cohort_correlation_dispersion", summary.Dispersion)
			}

			if summary.PeerEnergyRate > 0 {
				m.Put("peer_return_energy_rate", summary.PeerEnergyRate)
				m.Put("relative_return_energy", m.Value("return_energy_rate:measured")/summary.PeerEnergyRate)
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
			m := *(**data.Measurement)(arriving)

			if m.Error() != nil || len(m.Peers()) == 0 || m.Value("cohort_peer_count") == 0 {
				if !yield(arriving) {
					return
				}

				continue
			}

			signed := m.Value("cohort_signed_correlation")
			view := drive[float64, FisherView](op.estimator, &signed)

			m.Put("correlation_baseline", view.Baseline)
			m.Put("correlation_divergence", view.Divergence)
			m.Put("correlation_zscore", view.ZScore)

			if view.Defined {
				m.SetMeta(data.MetadataDivergence, strconv.FormatFloat(view.Divergence, 'f', -1, 64))
				m.SetMeta(data.MetadataSupport, strconv.FormatFloat(view.Count, 'f', -1, 64))

				if view.VarianceDefined {
					m.SetMeta(data.MetadataNoiseVariance, strconv.FormatFloat(view.Variance, 'f', -1, 64))
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
			m := *(**data.Measurement)(arriving)

			if m.Error() != nil || len(m.Peers()) == 0 || m.Value("cohort_peer_count") == 0 {
				if !yield(arriving) {
					return
				}

				continue
			}

			relative := m.Value("relative_return_energy")
			reading := drive[float64, adaptive.BaselineReading](op.baseline, &relative)

			m.Put("relative_return_energy_baseline", reading.Baseline)
			m.Put("relative_return_energy_divergence", reading.Residual)
			m.Put("relative_return_energy_zscore", reading.ZScore)

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
			m := *(**data.Measurement)(arriving)

			if m.Error() != nil || len(m.Peers()) == 0 || m.Value("cohort_peer_count") == 0 {
				if !yield(arriving) {
					return
				}

				continue
			}

			observation := temporal.Observation{
				Value: m.Value("cohort_signed_correlation"),
				At:    m.At.UnixNano(),
			}

			reading := drive[temporal.Observation, temporal.VelocityReading](op.velocity, &observation)

			if reading.Defined {
				m.Put("correlation_velocity", reading.Rate)
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
			m := *(**data.Measurement)(arriving)

			if m.Error() != nil || len(m.Peers()) == 0 || m.Value("cohort_peer_count") == 0 {
				if !yield(arriving) {
					return
				}

				continue
			}

			observation := temporal.Observation{
				Value: m.Value("relative_return_energy"),
				At:    m.At.UnixNano(),
			}

			reading := drive[temporal.Observation, temporal.VelocityReading](op.velocity, &observation)

			if reading.Defined {
				m.Put("relative_return_energy_velocity", reading.Rate)
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
