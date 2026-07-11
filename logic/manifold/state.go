package manifold

import (
	"math"
	"time"

	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
)

const conservationTolerance = 1e-9

/*
State is the typed per-symbol field readout consumed by resonance, causal,
and strategy layers.
*/
type State struct {
	At                   time.Time
	Epoch                uint64
	ScaleVersion         uint64
	Ready                bool
	InvalidReason        InvalidReason
	VisibleMass          float64
	ConservationResidual float64
	BidTouchDensity      float64
	AskTouchDensity      float64
	StressAnisotropy     float64
	DeltaT               float64
	Subdivisions         int
	PriceScale           float64
	SizeScale            float64
	PressureTensor       PressureTensor
	OscillatorCount      int
	pmanifold.Reading
}

func (state State) IsFinite() bool {
	return state.Ready && state.Reading.IsFinite()
}

func (state State) GasReady() bool {
	if !state.IsFinite() {
		return false
	}

	if state.DeltaT <= 0 || state.Subdivisions <= 0 {
		return false
	}

	if state.VisibleMass <= 0 {
		return false
	}

	if math.Abs(state.ConservationResidual) > conservationTolerance {
		return false
	}

	if state.PriceScale <= 0 || state.SizeScale <= 0 {
		return false
	}

	return true
}

func ReadingFromEvidence(snapshot any) (pmanifold.Reading, bool) {
	state, ok := StateFromEvidence(snapshot)

	if !ok || !state.IsFinite() {
		return pmanifold.Reading{}, false
	}

	return state.Reading, true
}

func StateFromEvidence(snapshot any) (State, bool) {
	state, ok := snapshot.(State)

	if !ok {
		return State{}, false
	}

	return state, true
}

func stressAnisotropy(tensor PressureTensor) float64 {
	mean := tensor.IsotropicScalar()

	if mean <= 0 {
		return 0
	}

	maxAxis := math.Max(tensor.XX, math.Max(tensor.YY, tensor.ZZ))
	minAxis := math.Min(tensor.XX, math.Min(tensor.YY, tensor.ZZ))

	return (maxAxis - minAxis) / mean
}

func touchMassDensity(orders []*PhysicalOrder, side OrderSide, touchBand float64) float64 {
	if len(orders) == 0 || touchBand <= 0 {
		return 0
	}

	sideMass := 0.0
	touchMass := 0.0

	for _, order := range orders {
		if order.Side != side {
			continue
		}

		sideMass += order.Quantity

		if math.Abs(order.Coordinate.Price) <= touchBand {
			touchMass += order.Quantity
		}
	}

	if sideMass <= 0 {
		return 0
	}

	return touchMass / sideMass
}
