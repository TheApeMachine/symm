package correlation

import (
	"errors"
	"iter"
	"math"
	"sort"
	"strconv"
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
			m := *(**data.Measurement)(arriving)

			if m.Error() != nil {
				if !yield(arriving) {
					return
				}

				continue
			}

			last := m.Value("last_price")

			if last == 0 {
				m.Put("observation_count", 0)

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
				m.SetError(err)
				op.Error(err)

				if !yield(arriving) {
					return
				}

				continue
			}

			m.Put("observation_count", focal.Count)

			if !focal.Accepted {
				m.SetMeta("event_time_state", "regressed")

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

			m.ClearPeers()

			for _, symbol := range peers(op.retained, m.Label) {
				peer := op.retained[symbol]

				input := LagProfileInput{Left: focal.Observations, Right: peer.Observations}
				dependence := drive[LagProfileInput, DependenceReading](op.pairwise, &input)

				if err := op.pairwise.Error(); err != nil {
					m.SetError(err)
					op.Error(err)

					break
				}

				sample := FisherSample{Correlation: dependence.Correlation, Support: dependence.Support}
				significanceOfPair := drive[FisherSample, FisherReading](op.fisher, &sample)

				retain(op, m.Label, symbol, dependence, significanceOfPair,
					time.Unix(0, min(price.At, peer.To)))

				if !dependence.Defined || dependence.Support < 2 {
					continue
				}

				peerMeas := data.NewMeasurement(
					m.Epoch, symbol, m.Source, m.SeqIdx, m.Tick,
					&data.StringEntry{Key: "support", Value: strconv.FormatFloat(dependence.Support, 'f', -1, 64)},
					&data.StringEntry{Key: "peer_energy_rate", Value: strconv.FormatFloat(dependence.RightEnergyRate, 'f', -1, 64)},
				)
				peerMeas.Put("signed_correlation", dependence.Correlation)
				m.AddPeer(peerMeas)

				selected, significance, selection = dependence, significanceOfPair, symbol
			}

			if len(m.Peers()) > 0 {
				m.SetMeta("peer", selection)
				m.SetMeta("pair_diagnostics_selection", "last_defined_peer_lexicographic")

				m.Put("signed_correlation", selected.Correlation)
				m.Put("absolute_correlation", math.Abs(selected.Correlation))
				m.Put("covariance", selected.Covariance)
				m.Put("return_energy:reference", selected.RightEnergy)
				m.Put("return_energy:measured", selected.LeftEnergy)
				m.Put("return_energy_rate:reference", selected.RightEnergyRate)
				m.Put("return_energy_rate:measured", selected.LeftEnergyRate)
				m.Put("overlap_density", selected.OverlapDensity)
				m.Put("supported_return_count:measured", selected.LeftReturns)
				m.Put("supported_return_count:reference", selected.RightReturns)
				m.Put("overlap_pair_count", selected.Support)
				m.Put("shared_time", selected.SharedTime)

				if significance.Defined {
					m.Put("correlation_p_value", significance.PValue)
					m.Put("correlation_standard_error_fisher", significance.StandardError)
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
	op *Pairs, leftSymbol, rightSymbol string,
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
