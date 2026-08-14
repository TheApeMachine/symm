package hawkes

import (
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/hawkes"
)

/*
TradeInput is one precisely timestamped trade arrival. Timestamp is the native
path; UnixNano remains available when an exchange already supplies its event
time as an integer.
*/
type TradeInput struct {
	Symbol    string
	Side      string
	Timestamp time.Time
	UnixNano  int64
}

/*
Input carries a chronological marked stream into Hawkes fitting without
reconstructing exchange timestamps through floating-point seconds.
*/
type Input struct {
	Symbol       string
	ObservedFrom time.Time
	Horizon      time.Time
	Stream       hawkes.ArrivalStream
}

/*
Sample owns per-symbol marked arrival histories for one serial market-data
processor. It derives retention from the observed event-time scale so symbols
with different activity rates do not share a fixed event-count window.
*/
type Sample struct {
	windows map[string]*window
}

type window struct {
	arrivals *hawkes.ArrivalWindow
	origin   time.Time
}

/*
NewSample returns a trade-arrival sampler whose symbol-local histories remain
with one event-processing owner.
*/
func NewSample() *Sample {
	return &Sample{windows: make(map[string]*window)}
}

/*
MeasureArrival appends one marked event and returns the current statistically
bounded stream without sacrificing timestamp precision.
*/
func (sample *Sample) MeasureArrival(
	input TradeInput,
) (Input, bool, error) {
	if input.Symbol == "" || input.Side != "buy" && input.Side != "sell" {
		return Input{}, false, errnie.Error(errnie.Err(
			errnie.Validation,
			"excitation sample: trade requires symbol and buy/sell side",
			nil,
		))
	}

	arrival := input.ArrivalTime()

	if arrival.IsZero() {
		return Input{}, false, errnie.Error(errnie.Err(
			errnie.Validation,
			"excitation sample: timestamp required",
			nil,
		))
	}

	window := sample.window(input.Symbol)

	if window.origin.IsZero() || arrival.Before(window.origin) {
		window.origin = arrival
	}

	if input.Side == "buy" {
		window.arrivals.AppendBuy(arrival)
	}

	if input.Side == "sell" {
		window.arrivals.AppendSell(arrival)
	}

	return window.input(input.Symbol)
}

/*
ArrivalTime returns the native event time without converting it through float
Unix seconds.
*/
func (input TradeInput) ArrivalTime() time.Time {
	if !input.Timestamp.IsZero() {
		return input.Timestamp
	}

	if input.UnixNano != 0 {
		return time.Unix(0, input.UnixNano)
	}

	return time.Time{}
}

func (sample *Sample) window(symbol string) *window {
	current, ok := sample.windows[symbol]

	if ok {
		return current
	}

	current = &window{arrivals: hawkes.NewArrivalWindow(0)}
	sample.windows[symbol] = current

	return current
}

func (window *window) input(symbol string) (Input, bool, error) {
	stream := window.arrivals.Stream()
	observedFrom, horizon, ok := stream.Bounds()

	if !ok {
		return Input{}, false, nil
	}

	context, ready := hawkes.NewObservationContext(stream, horizon)

	if !ready {
		return Input{
			Symbol:       symbol,
			ObservedFrom: window.origin,
			Horizon:      horizon,
			Stream:       stream.WithObservationOrigin(window.origin),
		}, true, nil
	}

	if context.TradeWindow > 0 {
		observedFrom = horizon.Add(-context.TradeWindow)

		if observedFrom.Before(window.origin) {
			observedFrom = window.origin
		}
	}

	window.arrivals.RetainFrom(observedFrom)
	stream = window.arrivals.Stream().WithObservationOrigin(observedFrom)

	return Input{
		Symbol:       symbol,
		ObservedFrom: observedFrom,
		Horizon:      horizon,
		Stream:       stream,
	}, true, nil
}
