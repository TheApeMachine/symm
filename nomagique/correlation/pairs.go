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
	"covariance_score",
	"absolute_covariance_score",
	"cohort_covariance_score",
	"cohort_absolute_covariance_score",
	"covariance",
	"covariance_standard_error",
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
	"covariance_p_value",
	"cohort_peer_count",
	"cohort_covariance_score_dispersion",
	"cohort_effective_peer_count",
	"relative_return_energy",
	"relative_cohort_return_energy",
	"covariance_score_baseline",
	"covariance_score_divergence",
	"covariance_score_zscore",
	"covariance_score_velocity",
	"relative_return_energy_baseline",
	"relative_return_energy_divergence",
	"relative_return_energy_zscore",
	"relative_return_energy_velocity",
	"historical_path_distance",
	"historical_path_percentile",
}

/*
Frame carries calculation context and metrics across correlation primitives.
Reference is the peer the single-pair metrics describe: the admitted peer with
the most overlapping returns (support), ties going to the first symbol in
sorted order. It is empty when no peer is admitted.
*/
type Frame struct {
	Symbol    string
	At        int64
	Peers     []Peer
	Reference string
	Metrics   map[string]float64
}

/*
Relation is one retained pairwise dependence fact: the Hayashi-Yoshida
covariance, its standard error, and their score.
*/
type Relation struct {
	Left, Right                   string
	Covariance, StandardError     float64
	Score, AbsoluteScore, Support float64
	PValue                        float64
	Defined, ScoreDefined         bool
	At                            time.Time
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
	relations core.Primitive
}

func NewPairs(estimator core.Primitive) core.Primitive {
	return &Pairs{
		paths:     make(map[string]core.Primitive),
		window:    adaptive.NewWindow,
		retained:  make(map[string]PathReading),
		pairwise:  NewDependence(estimator),
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

			var selected DependenceReading

			frame.Peers = frame.Peers[:0]
			frame.Reference = ""

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

				retain(op, frame.Symbol, peerSymbol, dependence,
					time.Unix(0, min(price.At, peer.To)))

				if !dependence.Defined || !dependence.ScoreDefined || dependence.Support < 2 {
					continue
				}

				frame.Peers = append(frame.Peers, Peer{
					Score:      dependence.Score,
					Support:    dependence.Support,
					PeerEnergy: dependence.RightEnergyRate,
				})

				// The reference is the peer with the most overlapping data;
				// peers arrive in sorted order, so a tie keeps the first.
				if frame.Reference == "" || dependence.Support > selected.Support {
					selected = dependence
					frame.Reference = peerSymbol
				}
			}

			if len(frame.Peers) > 0 {
				frame.Metrics["covariance_score"] = selected.Score
				frame.Metrics["absolute_covariance_score"] = math.Abs(selected.Score)
				frame.Metrics["covariance"] = selected.Covariance
				frame.Metrics["covariance_standard_error"] = selected.StandardError
				frame.Metrics["covariance_p_value"] = scorePValue(selected.Score)
				frame.Metrics["return_energy:reference"] = selected.RightEnergy
				frame.Metrics["return_energy:measured"] = selected.LeftEnergy
				frame.Metrics["return_energy_rate:reference"] = selected.RightEnergyRate
				frame.Metrics["return_energy_rate:measured"] = selected.LeftEnergyRate

				frame.Metrics["overlap_density"] = selected.OverlapDensity
				frame.Metrics["supported_return_count:measured"] = selected.LeftReturns
				frame.Metrics["supported_return_count:reference"] = selected.RightReturns
				frame.Metrics["overlap_pair_count"] = selected.Support
				frame.Metrics["shared_time"] = selected.SharedTime
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

/*
scorePValue is the two-sided normal p-value of a covariance score under the
null of no co-movement that defines its standard error.
*/
func scorePValue(score float64) float64 {
	return math.Erfc(math.Abs(score) / math.Sqrt2)
}

func retain(
	op *Pairs, leftSymbol, rightSymbol string,
	dependence DependenceReading, at time.Time,
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
		relation.Covariance = dependence.Covariance
	}

	relation.ScoreDefined = relation.Defined && dependence.ScoreDefined

	if relation.ScoreDefined {
		relation.StandardError = dependence.StandardError
		relation.Score = dependence.Score
		relation.AbsoluteScore = math.Abs(dependence.Score)
		relation.PValue = scorePValue(dependence.Score)
	}

	for range op.relations.Next(transport.NewOne(unsafe.Pointer(&relation)).Next(nil)) {
	}
}
