package manifold

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/statistic"
)

type frames struct {
	span    float64
	symbols map[string]*coordinateFrame
}

type coordinateFrame struct {
	price, quantity *axis
	lastPrice       [3]float64
}

type axis struct {
	moments  core.Primitive
	residual core.Primitive
	span     float64
}

func newFrames(span float64) *frames {
	return &frames{span: span, symbols: make(map[string]*coordinateFrame)}
}

func newAxis(span float64) *axis {
	return &axis{moments: statistic.NewEstimator(), residual: statistic.NewCausalResidual(), span: span}
}

func (axis *axis) observe(value float64) (position, zscore float64, moments [3]float64, err error) {
	var reading *[10]float64

	for out := range axis.moments.Next(data.NewValue(value)) {
		reading = (*[10]float64)(out)
	}

	if err := axis.moments.Error(); err != nil {
		return 0, 0, [3]float64{}, err
	}

	if reading == nil {
		return 0, 0, [3]float64{}, fmt.Errorf("manifold: estimator produced no reading")
	}

	result, err := evaluateCausalResidual(axis.residual, reading)
	if err != nil {
		return 0, 0, [3]float64{}, err
	}

	moments = [3]float64{reading[0], reading[1], reading[2]}
	return 0.5 * (1 + math.Tanh(result[6]/axis.span)), result[6], moments, nil
}

func (axis *axis) probe(prior [3]float64, value float64) (position, zscore float64, err error) {
	// Synthesize an Estimator-shaped reading so CausalResidual can score the
	// probe against the last observed prior without training the estimator.
	reading := [10]float64{
		3: prior[0],
		4: prior[1],
		5: prior[2],
		6: value,
	}
	result, err := evaluateCausalResidual(axis.residual, &reading)
	if err != nil {
		return 0, 0, err
	}

	return 0.5 * (1 + math.Tanh(result[6]/axis.span)), result[6], nil
}

func (owner *frames) frame(symbol string) *coordinateFrame {
	held := owner.symbols[symbol]
	if held == nil {
		held = &coordinateFrame{price: newAxis(owner.span), quantity: newAxis(owner.span)}
		owner.symbols[symbol] = held
	}
	return held
}

func (owner *frames) place(symbol string, price, quantity float64) (x, y, priceDeviation, quantityDeviation float64, err error) {
	held := owner.frame(symbol)
	x, priceDeviation, moments, err := held.price.observe(price)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	y, quantityDeviation, _, err = held.quantity.observe(quantity)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	held.lastPrice = moments
	return x, y, priceDeviation, quantityDeviation, nil
}

func (owner *frames) placePrice(symbol string, price float64) (position, deviation float64, err error) {
	held := owner.symbols[symbol]
	if held == nil || held.lastPrice[0] == 0 {
		return 0, 0, fmt.Errorf("manifold: no observed price frame for %q", symbol)
	}
	return held.price.probe(held.lastPrice, price)
}

/*
evaluateCausalResidual drives one Estimator reading through the causal residual
primitive and returns the residual's *[8]float64 facts.
*/
func evaluateCausalResidual(
	residual core.Primitive,
	reading *[10]float64,
) ([8]float64, error) {
	var result [8]float64

	for out := range residual.Next(data.NewValue(*reading)) {
		result = *(*[8]float64)(out)
	}

	if err := residual.Error(); err != nil {
		return [8]float64{}, err
	}

	return result, nil
}
