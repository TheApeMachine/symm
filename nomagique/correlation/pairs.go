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

var OutputKeys = []string{
	"last_price",
	"observation_count",
	"signed_correlation",
	"absolute_correlation",
	"cohort_signed_correlation",
	"cohort_absolute_correlation",
	"covariance",
	"return_energy:reference",
	"return_energy:measured",
	"return_energy_rate:reference",
	"return_energy_rate:measured",
	"peer_return_energy_rate",
	"focal_return_energy_rate",
	"supported_return_count:measured",
	"supported_return_count:reference",
	"shared_time",
	"overlap_density",
	"overlap_pair_count",
	"effective_sample_count",
	"correlation_p_value",
	"correlation_standard_error_fisher",
	"cohort_peer_count",
	"cohort_correlation_dispersion",
	"cohort_effective_peer_count",
	"relative_return_energy",
	"relative_cohort_return_energy",
	"correlation_baseline",
	"correlation_divergence",
	"correlation_zscore",
	"correlation_velocity",
	"relative_return_energy_baseline",
	"relative_return_energy_divergence",
	"relative_return_energy_zscore",
	"relative_return_energy_velocity",
	"historical_path_distance",
	"historical_path_percentile",
}

/*
Frame carries calculation context and metrics across correlation primitives.
*/
type Frame struct {
	Symbol  string
	At      int64
	Peers   []Peer
	Metrics map[string]float64
}

/*
Relation is one retained pairwise correlation fact.
*/
type Relation struct {
	Left, Right               string
	Signed, Absolute, Support float64
	PValue, StandardError     float64
	Defined, FisherDefined    bool
	At                        time.Time
}

/*
Relations retains measured pair measurements.
*/
type Relations struct {
	err   error
	pairs map[[2]string]Relation
}

func NewRelations() core.Primitive {
	return &Relations{}
}

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
Pairs measures the arrival's path against every retained peer.
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
			frame := (*Frame)(arriving)
			if frame.Metrics == nil {
				frame.Metrics = make(map[string]float64)
			}

			last := frame.Metrics["last_price"]
			if last <= 0 {
				frame.Metrics["observation_count"] = 0

				if !yield(arriving) {
					return
				}

				continue
			}

			path := op.paths[frame.Symbol]
			if path == nil {
				path = NewPath(op.window())
				op.paths[frame.Symbol] = path
			}

			price := temporal.Price{At: frame.At, Value: last}
			focal := data.To[temporal.Price, PathReading](path, &price)

			if err := path.Error(); err != nil {
				op.Error(err)

				if !yield(arriving) {
					return
				}

				continue
			}

			frame.Metrics["observation_count"] = focal.Count

			if !focal.Accepted {
				if !yield(arriving) {
					return
				}

				continue
			}

			op.retained[frame.Symbol] = focal

			var (
				selected     DependenceReading
				significance FisherReading
			)

			frame.Peers = frame.Peers[:0]

			for _, peerSymbol := range peers(op.retained, frame.Symbol) {
				peer := op.retained[peerSymbol]

				if len(peer.Observations) < 2 {
					continue
				}

				input := LagProfileInput{Left: focal.Observations, Right: peer.Observations}
				dependence := data.To[LagProfileInput, DependenceReading](op.pairwise, &input)

				if err := op.pairwise.Error(); err != nil {
					op.Error(err)
					break
				}

				sample := FisherSample{Correlation: dependence.Correlation, Support: dependence.Support}
				significanceOfPair := data.To[FisherSample, FisherReading](op.fisher, &sample)

				retain(op, frame.Symbol, peerSymbol, dependence, significanceOfPair,
					time.Unix(0, min(price.At, peer.To)))

				if !dependence.Defined || dependence.Support < 2 {
					continue
				}

				frame.Peers = append(frame.Peers, Peer{
					Correlation: dependence.Correlation,
					Support:     dependence.Support,
					PeerEnergy:  dependence.RightEnergyRate,
				})

				selected = dependence
				significance = significanceOfPair
			}

			if len(frame.Peers) > 0 {
				frame.Metrics["signed_correlation"] = selected.Correlation
				frame.Metrics["absolute_correlation"] = math.Abs(selected.Correlation)
				frame.Metrics["covariance"] = selected.Covariance
				frame.Metrics["return_energy:reference"] = selected.RightEnergy
				frame.Metrics["return_energy:measured"] = selected.LeftEnergy
				frame.Metrics["return_energy_rate:reference"] = selected.RightEnergyRate
				frame.Metrics["return_energy_rate:measured"] = selected.LeftEnergyRate
				frame.Metrics["focal_return_energy_rate"] = selected.LeftEnergyRate

				if selected.RightEnergyRate > 0 {
					frame.Metrics["relative_return_energy"] = selected.LeftEnergyRate / selected.RightEnergyRate
				}

				frame.Metrics["overlap_density"] = selected.OverlapDensity
				frame.Metrics["supported_return_count:measured"] = selected.LeftReturns
				frame.Metrics["supported_return_count:reference"] = selected.RightReturns
				frame.Metrics["overlap_pair_count"] = selected.Support
				frame.Metrics["effective_sample_count"] = selected.Support
				frame.Metrics["shared_time"] = selected.SharedTime

				if significance.Defined {
					frame.Metrics["correlation_p_value"] = significance.PValue
					frame.Metrics["correlation_standard_error_fisher"] = significance.StandardError
				}
			}

			if !yield(arriving) {
				return
			}
		}
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
