package statistic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/geometry"
)

/*
Observation is one cell's dimensionless movement and measured authority as a Primitive.
When stepped without input, it yields movement, authority, and weight scalar addresses.
*/
type Observation[T any] struct {
	*core.PrimitiveError
	Address   T
	Movement  float64
	Authority float64
	Weight    float64
}

func NewObservation[T any](address T, movement, authority, weight float64) *Observation[T] {
	return &Observation[T]{
		PrimitiveError: core.NewPrimitiveError(),
		Address:        address,
		Movement:       movement,
		Authority:      authority,
		Weight:         weight,
	}
}

func (observation *Observation[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if !yield(unsafe.Pointer(&observation.Movement)) {
			return
		}

		if !yield(unsafe.Pointer(&observation.Authority)) {
			return
		}

		yield(unsafe.Pointer(&observation.Weight))
	}
}

/*
Sympathy evaluates paired movements across registered metrics to find which
metrics cluster sympathetically. It yields geometry.Edge primitives for each pair.
*/
type Sympathy[T core.Ordered[T]] struct {
	*core.PrimitiveError

	addresses   []T
	authorities []float64
	movements   []float64
	pairs       []Concordance
}

func NewSympathy[T core.Ordered[T]]() *Sympathy[T] {
	return &Sympathy[T]{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (sympathy *Sympathy[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			return
		}

		for arriving := range in {
			if sympathy.Error() != nil {
				return
			}

			if arriving == nil {
				continue
			}

			observation := (*Observation[T])(arriving)

			if observation == nil || any(observation.Address) == nil {
				continue
			}

			targetIndex := -1

			for index, heldAddress := range sympathy.addresses {
				if any(heldAddress) == nil {
					continue
				}

				if !heldAddress.Less(observation.Address) && !observation.Address.Less(heldAddress) {
					targetIndex = index
					break
				}
			}

			if targetIndex < 0 {
				targetIndex = len(sympathy.addresses)
				sympathy.addresses = append(sympathy.addresses, observation.Address)
				sympathy.authorities = append(sympathy.authorities, observation.Authority)
				sympathy.movements = append(sympathy.movements, observation.Movement)

				for peerIndex := 0; peerIndex < targetIndex; peerIndex++ {
					sympathy.pairs = append(sympathy.pairs, Concordance{})
				}
			}

			sympathy.authorities[targetIndex] = observation.Authority
			sympathy.movements[targetIndex] = observation.Movement

			clockWeight := observation.Weight
			if clockWeight <= 0 {
				clockWeight = 1.0
			}

			pairIdx := 0
			for i := 0; i < len(sympathy.addresses); i++ {
				for j := i + 1; j < len(sympathy.addresses); j++ {
					leftMov := sympathy.movements[i]
					rightMov := sympathy.movements[j]

					reading := sympathy.pairs[pairIdx].Update(leftMov, rightMov, clockWeight)
					pairIdx++

					leftPrim, _ := any(sympathy.addresses[i]).(core.Primitive)
					rightPrim, _ := any(sympathy.addresses[j]).(core.Primitive)

					weight := geometry.NewWeight(reading.Strength, reading.Orientation)
					edge := geometry.NewEdge(leftPrim, rightPrim, weight)
					edge.Authority = [2]float64{sympathy.authorities[i], sympathy.authorities[j]}

					if !yield(unsafe.Pointer(edge)) {
						return
					}
				}
			}
		}
	}
}
