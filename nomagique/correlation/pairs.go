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
drive pushes one payload pointer through one primitive and returns the
answer the primitive yielded.
*/
func drive[From, To any](op core.Primitive, payload *From) To {
	var answer To

	for out := range op.Next(transport.NewOne(unsafe.Pointer(payload)).Next(nil)) {
		answer = *(*To)(out)
	}

	return answer
}

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

			if m.Err != nil {
				if !yield(arriving) {
					return
				}

				continue
			}

			last := m.GetMetric("last_price").Raw

			if last == 0 {
				m.SetMetric("observation_count", data.NewMetric(
					"observation_count",
					data.UnitCount,
					data.TimescaleRollingWindow,
					0,
					1,
				).Write(0))

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

			m.SetMetric("observation_count", data.NewMetric(
				"observation_count",
				data.UnitCount,
				data.TimescaleRollingWindow,
				0,
				math.Max(focal.Count, 1),
			).Write(focal.Count))

			if !focal.Accepted {
				m.SetProvenance("event_time_state", "regressed")

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
				var (
					dependence DependenceReading
					err        error
				)

				if fast, ok := op.pairwise.(interface {
					Compute(*LagProfileInput) (DependenceReading, error)
				}); ok {
					dependence, err = fast.Compute(&input)
				} else {
					dependence = drive[LagProfileInput, DependenceReading](op.pairwise, &input)
					err = op.pairwise.Error()
				}

				if err != nil {
					m.Err = errors.Join(m.Err, err)
					op.Error(err)

					break
				}

				sample := FisherSample{Correlation: dependence.Correlation, Support: dependence.Support}
				var significanceOfPair FisherReading

				if fast, ok := op.fisher.(interface {
					Compute(*FisherSample) FisherReading
				}); ok {
					significanceOfPair = fast.Compute(&sample)
				} else {
					significanceOfPair = drive[FisherSample, FisherReading](op.fisher, &sample)
				}

				retain(op, m.Label, symbol, &peer, dependence, significanceOfPair,
					time.Unix(0, min(price.At, peer.To)))

				if !dependence.Defined || dependence.Support < 2 {
					continue
				}

				peerMeas := data.NewMeasurement("correlation")
				peerMeas.Label = symbol
				peerMeas.SetMetric("signed_correlation", data.NewMetric(
					"signed_correlation",
					data.UnitCorrelation,
					data.TimescaleInstantaneous,
					0,
					1,
				).Write(dependence.Correlation))
				peerMeas.SetMetadata("support", strconv.FormatFloat(dependence.Support, 'f', -1, 64))
				peerMeas.SetMetadata("peer_energy_rate", strconv.FormatFloat(dependence.RightEnergyRate, 'f', -1, 64))
				m.Peers = append(m.Peers, peerMeas)

				selected, significance, selection = dependence, significanceOfPair, symbol
			}

			if len(m.Peers) > 0 {
				m.SetProvenance("peer", selection)
				m.SetProvenance("pair_diagnostics_selection", "last_defined_peer_lexicographic")

				m.SetMetric("signed_correlation", data.NewMetric(
					"signed_correlation",
					data.UnitCorrelation,
					data.TimescaleInstantaneous,
					0,
					1,
				).Write(selected.Correlation))
				m.SetMetric("absolute_correlation", data.NewMetric(
					"absolute_correlation",
					data.UnitCorrelation,
					data.TimescaleInstantaneous,
					0,
					1,
				).Write(math.Abs(selected.Correlation)))
				covScale := math.Max(math.Sqrt(selected.LeftEnergyRate*selected.RightEnergyRate), 1e-6)
				m.SetMetric("covariance", data.NewMetric(
					"covariance",
					data.UnitVariance,
					data.TimescaleRollingWindow,
					0,
					covScale,
				).Write(selected.Covariance))
				m.SetMetric("return_energy:reference", data.NewMetric(
					"return_energy:reference",
					data.UnitRatio,
					data.TimescaleRollingWindow,
					0,
					math.Max(selected.RightEnergy, 1e-6),
				).Write(selected.RightEnergy))
				m.SetMetric("return_energy:measured", data.NewMetric(
					"return_energy:measured",
					data.UnitRatio,
					data.TimescaleRollingWindow,
					0,
					math.Max(selected.LeftEnergy, 1e-6),
				).Write(selected.LeftEnergy))
				m.SetMetric("return_energy_rate:reference", data.NewMetric(
					"return_energy_rate:reference",
					data.UnitRate,
					data.TimescalePerSecond,
					0,
					math.Max(selected.RightEnergyRate, 1e-6),
				).Write(selected.RightEnergyRate))
				m.SetMetric("return_energy_rate:measured", data.NewMetric(
					"return_energy_rate:measured",
					data.UnitRate,
					data.TimescalePerSecond,
					0,
					math.Max(selected.LeftEnergyRate, 1e-6),
				).Write(selected.LeftEnergyRate))
				m.SetMetric("overlap_density", data.NewMetric(
					"overlap_density",
					data.UnitRatio,
					data.TimescaleRollingWindow,
					0.5,
					0.5,
				).Write(selected.OverlapDensity))
				m.SetMetric("supported_return_count:measured", data.NewMetric(
					"supported_return_count:measured",
					data.UnitCount,
					data.TimescaleRollingWindow,
					0,
					math.Max(selected.LeftReturns, 1),
				).Write(selected.LeftReturns))
				m.SetMetric("supported_return_count:reference", data.NewMetric(
					"supported_return_count:reference",
					data.UnitCount,
					data.TimescaleRollingWindow,
					0,
					math.Max(selected.RightReturns, 1),
				).Write(selected.RightReturns))
				m.SetMetric("overlap_pair_count", data.NewMetric(
					"overlap_pair_count",
					data.UnitCount,
					data.TimescaleRollingWindow,
					0,
					math.Max(selected.Support, 1),
				).Write(selected.Support))
				m.SetMetric("shared_time", data.NewMetric(
					"shared_time",
					data.UnitDuration,
					data.TimescaleRollingWindow,
					0,
					math.Max(selected.SharedTime, 1e-6),
				).Write(selected.SharedTime))

				if significance.Defined {
					m.SetMetric("correlation_p_value", data.NewMetric(
						"correlation_p_value",
						data.UnitProbability,
						data.TimescaleRollingWindow,
						0.5,
						0.5,
					).Write(significance.PValue))
					m.SetMetric("correlation_standard_error_fisher", data.NewMetric(
						"correlation_standard_error_fisher",
						data.UnitStandardDeviation,
						data.TimescaleRollingWindow,
						0,
						math.Max(significance.StandardError, 1e-6),
					).Write(significance.StandardError))
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
