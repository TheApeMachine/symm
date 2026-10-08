package leadlag

import (
	"context"
	"unsafe"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
	nmcorrelation "github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
)

const maxLeadLagSamples = 64


/*
Signal is the asynchronous price-path lead-lag instrument.
*/
type Signal struct {
	*runtime.System
	paths   map[string][]temporal.Price
	leadlag core.Primitive
}

func NewSignal(ctx context.Context) *Signal {
	signal := &Signal{
		paths:   make(map[string][]temporal.Price),
		leadlag: nmcorrelation.NewLeadLag(algo.NewHayashiYoshida()),
	}

	signal.System = runtime.NewSystem(ctx, "leadlag", signal)
	return signal
}

func (signal *Signal) Step(prior *data.Measurement) *data.Measurement {
	if signal.Status() != runtime.READY {
		errnie.Warn(signal.Name() + ": Step called before READY; dropping event")
		return nil
	}

	if prior == nil || prior.Label == "" {
		return nil
	}

	var price float64
	var found bool

	for entry := range prior.Read("price") {
		if entry != nil && entry.Metric != nil {
			price = entry.Metric.Raw
			found = true
			break
		}
	}

	if !found {
		signal.Error(errnie.Err(errnie.Validation, "[leadlag] missing price", nil))
		return nil
	}

	if price <= 0 {
		return nil
	}

	output := make(map[string]float64)
	output["last"] = price
	atNano := prior.At.UnixNano()

	currentPath := signal.paths[prior.Label]

	if len(currentPath) >= maxLeadLagSamples {
		currentPath = currentPath[1:]
	}

	currentPath = append(currentPath, temporal.Price{At: atNano, Value: price})
	signal.paths[prior.Label] = currentPath

	for peerSymbol, peerPath := range signal.paths {
		if peerSymbol == prior.Label || len(peerPath) < 3 || len(currentPath) < 3 {
			continue
		}

		input := nmcorrelation.LagProfileInput{Left: peerPath, Right: currentPath}
		reading := drive[nmcorrelation.LagProfileInput, nmcorrelation.LeadLagReading](signal.leadlag, &input)

		if !reading.Defined {
			continue
		}

		output["reference_symbol@"+peerSymbol] = 1.0
		output["contemporaneous_correlation@"+peerSymbol] = reading.Contemporaneous
		output["best_lag_correlation@"+peerSymbol] = reading.Y
		output["best_lag_seconds@"+peerSymbol] = reading.X
		output["best_lag_index@"+peerSymbol] = reading.LagIndex
		output["absolute_correlation_gain@"+peerSymbol] = reading.AbsoluteGain
		output["lag_search_resolution_seconds@"+peerSymbol] = reading.Spacing * 1e-9
		output["lag_search_span@"+peerSymbol] = reading.Span
		output["lag_fraction@"+peerSymbol] = reading.LagFraction
		output["overlap_pair_count@"+peerSymbol] = reading.Support
		output["search_count@"+peerSymbol] = reading.SearchCount
		output["lag_peak_prominence@"+peerSymbol] = reading.Prominence
		output["lag_peak_curvature@"+peerSymbol] = reading.Curvature
	}

	if err := signal.leadlag.Error(); err != nil {
		signal.Error(err)
		return nil
	}

	return prior.Next(signal.Name(), output)
}

func drive[From, To any](op core.Primitive, payload *From) To {
	var answer To

	for out := range op.Next(transport.NewOne(unsafe.Pointer(payload)).Next(nil)) {
		answer = *(*To)(out)
	}

	return answer
}
