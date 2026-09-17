package correlation

import (
	"iter"
	"math"
	"slices"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/temporal"
)

/*
PriceObservation pairs an identified symbol with its timestamped price.
*/
type PriceObservation struct {
	Symbol string
	temporal.Price
}

/*
PairsReading is the structured fact yielded by the pair stage: focal path
diagnostics, the selected pair's dependence and Fisher readings, and all
admitted cohort peers.
*/
type PairsReading struct {
	Observation PriceObservation
	Path        PathReading
	Selected    DependenceReading
	Fisher      FisherReading
	PeerSymbol  string
	Peers       []Peer
	Relations   []Relation
}

/*
Pairs owns every symbol's price path and measures the arrival's path against
every retained peer: one dependence estimate and one Fisher significance per
pair, every measured pair retained, and every defined pair with support
admitted to Peers. When several peers qualify, the lexicographically last one
is the selected pair, so selection is deterministic.
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
			observation := *(*PriceObservation)(arriving)

			if observation.Value <= 0 {
				reading := PairsReading{
					Observation: observation,
				}

				if !yield(unsafe.Pointer(&reading)) {
					return
				}

				continue
			}

			path := pairs.paths[observation.Symbol]

			if path == nil {
				path = NewPath(adaptive.NewWindow())
				pairs.paths[observation.Symbol] = path
			}

			price := observation.Price
			var focal PathReading

			for out := range path.Next(sequence.NewOne(unsafe.Pointer(&price)).Next(nil)) {
				focal = *(*PathReading)(out)
			}

			if err := path.Error(); err != nil {
				pairs.Error(err)
				return
			}

			reading := PairsReading{
				Observation: observation,
				Path:        focal,
			}

			if !focal.Accepted {
				if !yield(unsafe.Pointer(&reading)) {
					return
				}

				continue
			}

			pairs.retained[observation.Symbol] = focal

			peerSymbols := make([]string, 0, len(pairs.retained))

			for candidateSymbol := range pairs.retained {
				if candidateSymbol != observation.Symbol {
					peerSymbols = append(peerSymbols, candidateSymbol)
				}
			}

			slices.Sort(peerSymbols)

			var (
				selected     DependenceReading
				significance FisherReading
				selection    string
				admitted     []Peer
				relations    []Relation
			)

			for _, symbol := range peerSymbols {
				peer := pairs.retained[symbol]
				input := LagProfileInput{Left: focal.Observations, Right: peer.Observations}
				var dependence DependenceReading

				for out := range pairs.pairwise.Next(sequence.NewOne(unsafe.Pointer(&input)).Next(nil)) {
					dependence = *(*DependenceReading)(out)
				}

				if err := pairs.pairwise.Error(); err != nil {
					pairs.Error(err)
					return
				}

				sample := FisherSample{Correlation: dependence.Correlation, Support: dependence.Support}
				var significanceOfPair FisherReading

				for out := range pairs.fisher.Next(sequence.NewOne(unsafe.Pointer(&sample)).Next(nil)) {
					significanceOfPair = *(*FisherReading)(out)
				}

				leftSymbol, rightSymbol := observation.Symbol, symbol

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

				relations = append(relations, relation)

				minSupport := core.Unit + core.Unit

				if !dependence.Defined || dependence.Support < minSupport {
					continue
				}

				admitted = append(admitted, Peer{
					Correlation: dependence.Correlation,
					Support:     dependence.Support,
					PeerEnergy:  dependence.RightEnergyRate,
				})

				selected = dependence
				significance = significanceOfPair
				selection = symbol
			}

			reading.Selected = selected
			reading.Fisher = significance
			reading.PeerSymbol = selection
			reading.Peers = admitted
			reading.Relations = relations

			if !yield(unsafe.Pointer(&reading)) {
				return
			}
		}
	}
}
