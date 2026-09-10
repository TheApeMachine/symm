package manifold

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/transport"
)

type frames struct {
	span    float64
	symbols map[string]*coordinateFrame
}

type coordinateFrame struct {
	price, quantity *axis
	lastPrice       equation.Moments
}

type axis struct {
	moments  *equation.Welford
	residual *equation.CausalResidual
	span     float64
}

func newFrames(span float64) *frames {
	return &frames{span: span, symbols: make(map[string]*coordinateFrame)}
}

func newAxis(span float64) *axis {
	return &axis{moments: equation.NewWelford(), residual: equation.NewCausalResidual(), span: span}
}

func (axis *axis) observe(value float64) (position, zscore float64, moments equation.Moments, err error) {
	reading, err := transport.Evaluate(axis.residual, axis.moments.Next(transport.Values(value)))
	if err != nil {
		return 0, 0, equation.Moments{}, err
	}
	return 0.5 * (1 + math.Tanh(reading.ZScore/axis.span)), reading.ZScore, reading.Moments, nil
}

func (axis *axis) probe(prior equation.Moments, value float64) (position, zscore float64, err error) {
	reading, err := transport.Evaluate(axis.residual, transport.Values(equation.MomentReading{Prior: prior, Value: value}))
	if err != nil {
		return 0, 0, err
	}
	return 0.5 * (1 + math.Tanh(reading.ZScore/axis.span)), reading.ZScore, nil
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
