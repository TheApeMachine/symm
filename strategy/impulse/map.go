package impulse

import (
	"cmp"
	"fmt"
	"iter"
	"slices"
	"unsafe"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/geometry"
)

/*
Map is the application stage composing owner reads, movement concordance,
coordinate relaxation and watershed regions. Workspace dependency barriers
protect its borrowed publications. One consumer owns each Map, including
replay; a historical run never writes the live Map.
*/
type Map struct {
	*core.PrimitiveError
	Markets  map[string]*Market
	Current  *Market
	active   []*Market
	sequence int64
	Invalid  int
}

func NewMap() *Map {
	return &Map{PrimitiveError: core.NewPrimitiveError(), Markets: make(map[string]*Market)}
}

/*
Step processes one complete workspace sequence. Market/source/key ordering is
canonical; map iteration and consumer scheduling cannot order the reductions.
Results are borrowed until the next Step. At/From are display facts only.
*/
func (impulseMap *Map) Step(input *data.Measurement[float64]) error {
	if input.SeqIdx <= impulseMap.sequence {
		return errnie.Error(errnie.Err(errnie.Conflict, "impulse: input sequence must increase", nil))
	}

	impulseMap.active = impulseMap.active[:0]
	impulseMap.Invalid = 0
	observations := input.Peers

	if len(observations) == 0 {
		observations = []*data.Measurement[float64]{input}
	}

	for _, observation := range observations {
		if observation == nil || observation.Source == "training" {
			continue
		}

		if observation.Err != nil {
			errnie.Error(errnie.Err(errnie.Validation, "impulse: rejected observation from "+observation.Provenance["owner"], observation.Err))
			impulseMap.Invalid++
		}

		if observation.Label == "" || observation.SeqIdx != input.SeqIdx {
			return errnie.Error(errnie.Err(errnie.Validation, fmt.Sprintf("impulse: producer %q source %q symbol %q sequence %d, expected %d", observation.Provenance["owner"], observation.Source, observation.Label, observation.SeqIdx, input.SeqIdx), nil))
		}

		market := impulseMap.Markets[observation.Label]

		if market == nil {
			market = newMarket(observation.Label)
			impulseMap.Markets[observation.Label] = market
		}

		if market.Sequence != input.SeqIdx {
			market.Sequence = input.SeqIdx
			market.volumeIncrement = 0
			market.valid = true
			impulseMap.active = append(impulseMap.active, market)
		}

		if observation.Err != nil {
			market.valid = false
		}

		if err := market.Bind(observation); err != nil {
			return err
		}
	}

	slices.SortFunc(impulseMap.active, func(left, right *Market) int { return cmp.Compare(left.Symbol, right.Symbol) })

	for _, market := range impulseMap.active {
		market.prepare()

		for _, cell := range market.Cells {
			cell.Observe(input.SeqIdx)
		}

		if market.volumeIncrement > 0 {
			for index, edge := range market.edges {
				left, right := market.Cells[edge.Left], market.Cells[edge.Right]
				reading := market.pairs[index].Update(left.Movement, right.Movement, market.volumeIncrement)
				market.edges[index].Strength = reading.Strength
			}
			geometry.Relaxation{}.Step(market.points, market.edges)
			geometry.Watershed{}.Step(market.points, market.edges)
		}

		if err := market.light(); err != nil {
			return err
		}
	}

	impulseMap.sequence = input.SeqIdx
	return nil
}

func (impulseMap *Map) Next(input iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range input {
			if err := impulseMap.Step((*data.Measurement[float64])(arriving)); err != nil {
				impulseMap.Error(err)
				return
			}

			for _, market := range impulseMap.active {
				impulseMap.Current = market

				if !yield(unsafe.Pointer(&market.Impulse)) {
					return
				}
			}
		}
	}
}
