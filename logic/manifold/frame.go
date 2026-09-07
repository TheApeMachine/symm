package manifold

import (
	"fmt"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// frames retains one log-price/log-quantity coordinate system per symbol.
// The caller serializes access. A probe reads the observed frame and never
// trains it; otherwise an invented candidate would become market evidence.
type frames struct {
	span    float64
	symbols map[string]*coordinateFrame
}
type coordinateFrame struct {
	price, quantity, probe core.Primitive
	lastPrice              map[string]core.Primitive
}

func newFrames(span float64) *frames {
	return &frames{span: span, symbols: make(map[string]*coordinateFrame)}
}

func frameAxis(moments core.Primitive, span float64) core.Primitive {
	return transport.NewPipe(equation.NewCausalResidual(moments),
		store.NewRecord(transport.NewPipe(), transport.NewPipe(
			equation.NewProduct[float64](store.NewConstant(core.From(.5)), equation.NewSum[float64](store.NewConstant(core.From(1.0)),
				transport.NewPipe(equation.NewRatio[float64](store.NewGet("zscore"), store.NewConstant(core.From(span))), calculus.NewTanh(transport.NewIO(core.From(0.0)))))), store.NewKey("position"))))
}
func (owner *frames) frame(symbol string) *coordinateFrame {
	frame := owner.symbols[symbol]
	if frame == nil {
		query := store.NewRecord(transport.NewPipe(),
			transport.NewPipe(store.NewGet("count"), store.NewKey("prior_count")),
			transport.NewPipe(store.NewGet("mean"), store.NewKey("prior_mean")),
			transport.NewPipe(store.NewGet("m2"), store.NewKey("prior_m2")))
		frame = &coordinateFrame{price: frameAxis(algo.NewWelford(), owner.span), quantity: frameAxis(algo.NewWelford(), owner.span), probe: frameAxis(query, owner.span)}
		owner.symbols[symbol] = frame
	}
	return frame
}
func (owner *frames) place(symbol string, price, quantity float64) (x, y, priceDeviation, quantityDeviation float64, err error) {
	frame := owner.frame(symbol)
	p, err := transport.Evaluate[map[string]core.Primitive](frame.price, core.From(price))
	if err != nil {
		return 0, 0, 0, 0, err
	}
	q, err := transport.Evaluate[map[string]core.Primitive](frame.quantity, core.From(quantity))
	if err != nil {
		return 0, 0, 0, 0, err
	}
	pd, qd := core.NewDecoder(p), core.NewDecoder(q)
	x, y = core.Decode[float64](pd, "position"), core.Decode[float64](qd, "position")
	priceDeviation, quantityDeviation = core.Decode[float64](pd, "zscore"), core.Decode[float64](qd, "zscore")
	if pd.Error() != nil {
		return 0, 0, 0, 0, pd.Error()
	}
	if qd.Error() != nil {
		return 0, 0, 0, 0, qd.Error()
	}
	frame.lastPrice = p
	return x, y, priceDeviation, quantityDeviation, nil
}
func (owner *frames) placePrice(symbol string, price float64) (position, deviation float64, err error) {
	frame := owner.symbols[symbol]
	if frame == nil || frame.lastPrice == nil {
		return 0, 0, fmt.Errorf("manifold: no observed price frame for %q", symbol)
	}
	fields := make(map[string]core.Primitive, len(frame.lastPrice))
	for key, value := range frame.lastPrice {
		fields[key] = value
	}
	fields["value"] = core.From(price)
	out, err := transport.Evaluate[map[string]core.Primitive](frame.probe, core.From(fields))
	if err != nil {
		return 0, 0, err
	}
	d := core.NewDecoder(out)
	position, deviation = core.Decode[float64](d, "position"), core.Decode[float64](d, "zscore")
	return position, deviation, d.Error()
}
