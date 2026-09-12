package manifold

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/transport"
)

type frames struct {
	span    float64
	symbols map[string]*coordinateFrame
}

type coordinateFrame struct {
	price, quantity *axis
	lastPrice       statistic.Moments
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

func (axis *axis) observe(value float64) (position, zscore float64, moments statistic.Moments, err error) {
	var reading statistic.MomentReading

	for out := range axis.moments.Next(transport.NewValues(value).Next(nil)) {
		reading = *(*statistic.MomentReading)(out)
	}

	if err := axis.moments.Error(); err != nil {
		return 0, 0, statistic.Moments{}, err
	}

	result, err := evaluateCausalResidual(axis.residual, reading)
	if err != nil {
		return 0, 0, statistic.Moments{}, err
	}

	return 0.5 * (1 + math.Tanh(result.ZScore/axis.span)), result.ZScore, reading.Moments, nil
}

func (axis *axis) probe(prior statistic.Moments, value float64) (position, zscore float64, err error) {
	result, err := evaluateCausalResidual(axis.residual, statistic.MomentReading{Prior: prior, Value: value})
	if err != nil {
		return 0, 0, err
	}

	return 0.5 * (1 + math.Tanh(result.ZScore/axis.span)), result.ZScore, nil
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
	if held == nil || held.lastPrice.Count == 0 {
		return 0, 0, fmt.Errorf("manifold: no observed price frame for %q", symbol)
	}
	return held.price.probe(held.lastPrice, price)
}

/*
evaluateCausalResidual drives one moment reading through the causal residual
primitive.
*/
func evaluateCausalResidual(
	residual core.Primitive,
	reading statistic.MomentReading,
) (statistic.CausalResidualResult, error) {
	var result statistic.CausalResidualResult

	for out := range residual.Next(transport.NewValues(reading).Next(nil)) {
		result = *(*statistic.CausalResidualResult)(out)
	}

	if err := residual.Error(); err != nil {
		return statistic.CausalResidualResult{}, err
	}

	return result, nil
}
