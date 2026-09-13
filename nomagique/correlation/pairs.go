package correlation

import (
	"errors"
	"iter"
	"math"
	"sort"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Relation is the measured pair before cohort folding. Support counts overlapping
return pairs, not independent samples. EffectiveSupport and Authority are explicitly unavailable (nil):
the Hayashi estimator does not estimate independence-adjusted sample size.
FisherDefined and PValue retain the existing independent-return approximation;
consumers must not mistake that approximation for calibrated authority.
At is the older endpoint of the two paths, so stale peers remain visible.
*/
type Relation struct {
	EffectiveSupport, Authority *float64
	Left, Right                 string
	Signed, Absolute, Support   float64
	PValue, StandardError       float64
	Defined, FisherDefined      bool
	At                          time.Time
}

/*
Relations retains measured pair measurements. It stores data and answers
nothing else; consumers query it like any other store.
*/
type Relations struct {
	err   error
	pairs map[[2]string]Relation
}

func NewRelations() core.Primitive {
	return &Relations{}
}

/*
Next receives *Relation payloads and retains each under its ordered symbol
pair.
*/
func (op *Relations) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			relation := (*Relation)(arriving)

			if op.pairs == nil {
				op.pairs = make(map[[2]string]Relation)
			}

			op.pairs[[2]string{relation.Left, relation.Right}] = *relation

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *Relations) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
Pairs owns every symbol's price path and measures the arrival's path against
every retained peer: one dependence estimate and one Fisher significance per
pair, every measured pair retained, and every defined pair with support
admitted to the measurement's Peers. When several peers qualify, the
lexicographically last one is the selected pair, so selection is
deterministic. All selected-pair facts are written where they are computed.
*/
type Pairs struct {
	err       error
	paths     map[string]core.Primitive
	window    func() core.Primitive
	retained  map[string]PathReading
	pairwise  core.Primitive
	fisher    core.Primitive
	relations core.Primitive
}

/*
NewPairs composes the pair stage over the supplied causal estimator, so the
algo dependency is injected at composition instead of imported here.
*/
func NewPairs(estimator core.Primitive) core.Primitive {
	return &Pairs{
		paths:     make(map[string]core.Primitive),
		window:    adaptive.NewWindow,
		retained:  make(map[string]PathReading),
		pairwise:  NewDependence(estimator),
		fisher:    NewFisher(),
		relations: NewRelations(),
	}
}

func (op *Pairs) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement[float64])(arriving)

			if m.Err != nil {
				if !yield(arriving) {
					return
				}

				continue
			}

			last := m.Metrics["last_price"].Raw

			if last == 0 {
				m.Metrics["observation_count"] = m.Metrics["observation_count"].Write(0)

				if !yield(arriving) {
					return
				}

				continue
			}

			path := op.paths[m.Label]

			if path == nil {
				path = NewPath(op.window())
				op.paths[m.Label] = path
			}

			price := temporal.Price{At: m.At.UnixNano(), Value: last}
			focal := drive[temporal.Price, PathReading](path, &price)

			if err := path.Error(); err != nil {
				m.Err = errors.Join(m.Err, err)
				op.Error(err)

				if !yield(arriving) {
					return
				}

				continue
			}

			m.Metrics["observation_count"] = m.Metrics["observation_count"].Write(focal.Count)

			if !focal.Accepted {
				m.Provenance = map[string]string{"event_time_state": "regressed"}

				if !yield(arriving) {
					return
				}

				continue
			}

			op.retained[m.Label] = focal

			var (
				selected     DependenceReading
				significance FisherReading
				selection    string
			)

			m.Peers = m.Peers[:0]

			for _, symbol := range peers(op.retained, m.Label) {
				peer := op.retained[symbol]

				input := LagProfileInput{Left: focal.Observations, Right: peer.Observations}
				dependence := drive[LagProfileInput, DependenceReading](op.pairwise, &input)

				if err := op.pairwise.Error(); err != nil {
					m.Err = errors.Join(m.Err, err)
					op.Error(err)

					break
				}

				sample := FisherSample{Correlation: dependence.Correlation, Support: dependence.Support}
				significanceOfPair := drive[FisherSample, FisherReading](op.fisher, &sample)

				retain(op, m.Label, symbol, &peer, dependence, significanceOfPair,
					time.Unix(0, min(price.At, peer.To)))

				if !dependence.Defined || dependence.Support < 2 {
					continue
				}

				m.Peers = append(m.Peers, &data.Measurement[float64]{
					Label: symbol,
					Metrics: map[string]data.Metric[float64]{
						"signed_correlation": {
							Label: "signed_correlation",
							Raw:   dependence.Correlation,
						},
					},
					Metadata: map[string]float64{
						"support":          dependence.Support,
						"peer_energy_rate": dependence.RightEnergyRate,
					},
				})

				selected, significance, selection = dependence, significanceOfPair, symbol
			}

			if len(m.Peers) > 0 {
				m.Provenance = map[string]string{
					"peer":                       selection,
					"pair_diagnostics_selection": "last_defined_peer_lexicographic",
				}

				m.Metrics["signed_correlation"] = m.Metrics["signed_correlation"].Write(selected.Correlation)
				m.Metrics["absolute_correlation"] = m.Metrics["absolute_correlation"].Write(math.Abs(selected.Correlation))
				m.Metrics["covariance"] = m.Metrics["covariance"].Write(selected.Covariance)
				m.Metrics["return_energy:reference"] = m.Metrics["return_energy:reference"].Write(selected.RightEnergy)
				m.Metrics["return_energy:measured"] = m.Metrics["return_energy:measured"].Write(selected.LeftEnergy)
				m.Metrics["return_energy_rate:reference"] = m.Metrics["return_energy_rate:reference"].Write(selected.RightEnergyRate)
				m.Metrics["return_energy_rate:measured"] = m.Metrics["return_energy_rate:measured"].Write(selected.LeftEnergyRate)
				m.Metrics["overlap_density"] = m.Metrics["overlap_density"].Write(selected.OverlapDensity)
				m.Metrics["supported_return_count:measured"] = m.Metrics["supported_return_count:measured"].Write(selected.LeftReturns)
				m.Metrics["supported_return_count:reference"] = m.Metrics["supported_return_count:reference"].Write(selected.RightReturns)
				m.Metrics["overlap_pair_count"] = m.Metrics["overlap_pair_count"].Write(selected.Support)
				m.Metrics["shared_time"] = m.Metrics["shared_time"].Write(selected.SharedTime)

				if significance.Defined {
					m.Metrics["correlation_p_value"] = m.Metrics["correlation_p_value"].Write(significance.PValue)
					m.Metrics["correlation_standard_error_fisher"] = m.Metrics["correlation_standard_error_fisher"].Write(significance.StandardError)
				}
			}

			if !yield(arriving) {
				return
			}
		}
	}
}

/*
peers lists the retained symbols other than the focal one in lexicographic
order, so pair selection stays deterministic.
*/
func peers(retained map[string]PathReading, focal string) []string {
	symbols := make([]string, 0, len(retained))

	for symbol := range retained {
		if symbol != focal {
			symbols = append(symbols, symbol)
		}
	}

	sort.Strings(symbols)

	return symbols
}

/*
retain records one measured pair, whether or not it is defined. Undefined
Fisher fields remain explicitly unavailable; NaN is not serialized as though
it were a p-value.
*/
func retain(
	op *Pairs, leftSymbol, rightSymbol string, peer *PathReading,
	dependence DependenceReading, fisher FisherReading, at time.Time,
) {
	left, right := leftSymbol, rightSymbol

	if right < left {
		left, right = right, left
	}

	relation := Relation{
		Left:    left,
		Right:   right,
		Support: dependence.Support,
		Defined: dependence.Defined,
		At:      at,
	}

	if relation.Defined {
		relation.Signed = dependence.Correlation
		relation.Absolute = math.Abs(relation.Signed)
	}

	relation.FisherDefined = fisher.Defined

	if relation.FisherDefined {
		relation.PValue = fisher.PValue
		relation.StandardError = fisher.StandardError
	}

	for range op.relations.Next(transport.NewOne(unsafe.Pointer(&relation)).Next(nil)) {
	}
}

func (op *Pairs) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
