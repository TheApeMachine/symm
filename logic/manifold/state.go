package manifold

import (
	"math"
	"time"

	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
)

/*
State is the typed per-symbol field readout consumed by resonance, causal,
and strategy layers.
*/
type State struct {
	FieldSnapshot
	Source               string         `json:"source"`
	Symbol               string         `json:"symbol"`
	At                   time.Time      `json:"at"`
	Epoch                uint64         `json:"epoch"`
	ScaleVersion         uint64         `json:"scaleVersion"`
	Ready                bool           `json:"ready"`
	InvalidReason        InvalidReason  `json:"invalidReason"`
	BestBid              float64        `json:"bestBid"`
	BestAsk              float64        `json:"bestAsk"`
	MidPrice             float64        `json:"midPrice"`
	VisibleMass          float64        `json:"visibleMass"`
	ConservationResidual float64        `json:"conservationResidual"`
	ConservationBound    float64        `json:"conservationBound"`
	BidTouchDensity      float64        `json:"bidTouchDensity"`
	AskTouchDensity      float64        `json:"askTouchDensity"`
	StressAnisotropy     float64        `json:"stressAnisotropy"`
	DeltaT               float64        `json:"deltaT"`
	Subdivisions         int            `json:"subdivisions"`
	PriceScale           float64        `json:"priceScale"`
	SizeScale            float64        `json:"sizeScale"`
	PressureTensor       PressureTensor `json:"pressureTensor"`
	OscillatorCount      int            `json:"oscillatorCount"`
	pmanifold.Reading
}

func (state State) IsFinite() bool {
	return state.Ready && state.Reading.IsFinite()
}

func (state State) GasReady() bool {
	if !state.IsFinite() || !state.gatesFinite() {
		return false
	}

	if state.DeltaT <= 0 || state.Subdivisions <= 0 {
		return false
	}

	if state.VisibleMass <= 0 {
		return false
	}

	if state.ConservationBound < 0 ||
		math.Abs(state.ConservationResidual) > state.ConservationBound {
		return false
	}

	if state.PriceScale <= 0 || state.SizeScale <= 0 {
		return false
	}

	return true
}

func (state State) gatesFinite() bool {
	values := [...]float64{
		state.VisibleMass,
		state.ConservationResidual,
		state.ConservationBound,
		state.DeltaT,
		state.PriceScale,
		state.SizeScale,
	}

	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
	}

	return true
}

func (state State) Duration() time.Duration {
	return time.Duration(state.DeltaT * float64(time.Second))
}

func (state State) HasSpread() bool {
	return state.BestBid > 0 && state.BestAsk >= state.BestBid && state.MidPrice > 0
}

func (state State) SpreadReturn() float64 {
	if !state.HasSpread() {
		return 0
	}

	return (state.BestAsk - state.BestBid) / state.MidPrice
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

	totalMass := 0.0
	touchMass := 0.0

	for _, order := range orders {
		totalMass += order.Quantity

		if order.Side != side {
			continue
		}

		if math.Abs(order.Coordinate.Price) <= touchBand {
			touchMass += order.Quantity
		}
	}

	if totalMass <= 0 {
		return 0
	}

	return touchMass / totalMass
}
