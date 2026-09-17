package correlation

import (
	"errors"
	"iter"
	"math"
	"slices"
	"strconv"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/temporal"
)

/*
Pairs owns every symbol's price path and measures the arrival's path against
every retained peer: one dependence estimate and one Fisher significance per
pair, every measured pair retained, and every defined pair with support
admitted to the measurement's Peers. When several peers qualify, the
lexicographically last one is the selected pair, so selection is
deterministic. All selected-pair facts are written where they are computed.
*/
type Pairs struct {
	*core.PrimitiveError

	paths     map[string]core.Primitive
	retained  map[string]PathReading
	pairwise  core.Primitive
	fisher    core.Primitive
	relations core.Primitive
}

/*
NewPairs composes the pair stage over the supplied causal estimator, so the
algo dependency is injected at composition instead of imported here.
*/
func NewPairs(estimator core.Primitive) *Pairs {
	return &Pairs{
		PrimitiveError: core.NewPrimitiveError(),
		paths:          make(map[string]core.Primitive),
		retained:       make(map[string]PathReading),
		pairwise:       NewDependence(estimator),
		fisher:         NewFisher(),
		relations:      NewRelations(),
	}
}

func (pairs *Pairs) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			measurement := *(**data.Measurement[float64])(arriving)

			if measurement.Err != nil {
				if !yield(arriving) {
					return
				}

				continue
			}

			last := measurement.Metrics["last_price"].Raw

			if last <= 0 {
				measurement.Metrics["observation_count"] = measurement.Metrics["observation_count"].Write(0)

				if !yield(arriving) {
					return
				}

				continue
			}

			path := pairs.paths[measurement.Label]

			if path == nil {
				path = NewPath(adaptive.NewWindow())
				pairs.paths[measurement.Label] = path
			}

			price := temporal.Price{At: measurement.At.UnixNano(), Value: last}
			var focal PathReading

			for out := range path.Next(sequence.NewOne(unsafe.Pointer(&price)).Next(nil)) {
				focal = *(*PathReading)(out)
			}

			if err := path.Error(); err != nil {
				measurement.Err = errors.Join(measurement.Err, err)
				pairs.Error(err)

				if !yield(arriving) {
					return
				}

				continue
			}

			measurement.Metrics["observation_count"] = measurement.Metrics["observation_count"].Write(focal.Count)

			if !focal.Accepted {
				measurement.Provenance = map[string]string{"event_time_state": "regressed"}

				if !yield(arriving) {
					return
				}

				continue
			}

			pairs.retained[measurement.Label] = focal

			var (
				selected     DependenceReading
				significance FisherReading
				selection    string
			)

			measurement.Peers = measurement.Peers[:0]

			peerSymbols := make([]string, 0, len(pairs.retained))

			for candidateSymbol := range pairs.retained {
				if candidateSymbol != measurement.Label {
					peerSymbols = append(peerSymbols, candidateSymbol)
				}
			}

			slices.Sort(peerSymbols)

			for _, symbol := range peerSymbols {
				peer := pairs.retained[symbol]
				input := LagProfileInput{Left: focal.Observations, Right: peer.Observations}
				var dependence DependenceReading

				for out := range pairs.pairwise.Next(sequence.NewOne(unsafe.Pointer(&input)).Next(nil)) {
					dependence = *(*DependenceReading)(out)
				}

				if err := pairs.pairwise.Error(); err != nil {
					measurement.Err = errors.Join(measurement.Err, err)
					pairs.Error(err)

					break
				}

				sample := FisherSample{Correlation: dependence.Correlation, Support: dependence.Support}
				var significanceOfPair FisherReading

				for out := range pairs.fisher.Next(sequence.NewOne(unsafe.Pointer(&sample)).Next(nil)) {
					significanceOfPair = *(*FisherReading)(out)
				}

				leftSymbol, rightSymbol := measurement.Label, symbol

				if rightSymbol < leftSymbol {
					leftSymbol, rightSymbol = rightSymbol, leftSymbol
				}

				relation := Relation{
					Left:          leftSymbol,
					Right:         rightSymbol,
					Support:       dependence.Support,
					Defined:       dependence.Defined,
					At:            time.Unix(0, min(price.At, peer.To)),
					FisherDefined: significanceOfPair.Defined,
				}

				if relation.Defined {
					relation.Signed = dependence.Correlation
					relation.Absolute = math.Abs(relation.Signed)
				}

				if relation.FisherDefined {
					relation.PValue = significanceOfPair.PValue
					relation.StandardError = significanceOfPair.StandardError
				}

				for range pairs.relations.Next(sequence.NewOne(unsafe.Pointer(&relation)).Next(nil)) {
				}

				minSupport := core.Unit + core.Unit

				if !dependence.Defined || dependence.Support < minSupport {
					continue
				}

				measurement.Peers = append(measurement.Peers, &data.Measurement[float64]{
					Label: symbol,
					Metrics: map[string]data.Metric[float64]{
						"signed_correlation": {
							Label: "signed_correlation",
							Raw:   dependence.Correlation,
						},
					},
					Metadata: map[string]string{
						"support":          strconv.FormatFloat(dependence.Support, 'f', -1, 64),
						"peer_energy_rate": strconv.FormatFloat(dependence.RightEnergyRate, 'f', -1, 64),
					},
				})

				selected, significance, selection = dependence, significanceOfPair, symbol
			}

			if len(measurement.Peers) > 0 {
				measurement.Provenance = map[string]string{
					"peer":                       selection,
					"pair_diagnostics_selection": "last_defined_peer_lexicographic",
				}

				measurement.Metrics["signed_correlation"] = measurement.Metrics["signed_correlation"].Write(selected.Correlation)
				measurement.Metrics["absolute_correlation"] = measurement.Metrics["absolute_correlation"].Write(math.Abs(selected.Correlation))
				measurement.Metrics["covariance"] = measurement.Metrics["covariance"].Write(selected.Covariance)
				measurement.Metrics["return_energy:reference"] = measurement.Metrics["return_energy:reference"].Write(selected.RightEnergy)
				measurement.Metrics["return_energy:measured"] = measurement.Metrics["return_energy:measured"].Write(selected.LeftEnergy)
				measurement.Metrics["return_energy_rate:reference"] = measurement.Metrics["return_energy_rate:reference"].Write(selected.RightEnergyRate)
				measurement.Metrics["return_energy_rate:measured"] = measurement.Metrics["return_energy_rate:measured"].Write(selected.LeftEnergyRate)
				measurement.Metrics["overlap_density"] = measurement.Metrics["overlap_density"].Write(selected.OverlapDensity)
				measurement.Metrics["supported_return_count:measured"] = measurement.Metrics["supported_return_count:measured"].Write(selected.LeftReturns)
				measurement.Metrics["supported_return_count:reference"] = measurement.Metrics["supported_return_count:reference"].Write(selected.RightReturns)
				measurement.Metrics["overlap_pair_count"] = measurement.Metrics["overlap_pair_count"].Write(selected.Support)
				measurement.Metrics["shared_time"] = measurement.Metrics["shared_time"].Write(selected.SharedTime)

				if significance.Defined {
					measurement.Metrics["correlation_p_value"] = measurement.Metrics["correlation_p_value"].Write(significance.PValue)
					measurement.Metrics["correlation_standard_error_fisher"] = measurement.Metrics["correlation_standard_error_fisher"].Write(significance.StandardError)
				}
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
