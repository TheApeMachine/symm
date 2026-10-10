package leadlag

import (
	"context"
	"maps"
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

	if len(currentPath) > 0 {
		last := currentPath[len(currentPath)-1].At

		if atNano < last {
			return nil
		}

		if atNano == last {
			currentPath[len(currentPath)-1] = temporal.Price{At: atNano, Value: price}
			signal.paths[prior.Label] = currentPath
		}
	}

	if len(currentPath) == 0 || atNano > currentPath[len(currentPath)-1].At {
		if len(currentPath) >= maxLeadLagSamples {
			currentPath = currentPath[1:]
		}

		currentPath = append(currentPath, temporal.Price{At: atNano, Value: price})
		signal.paths[prior.Label] = currentPath
	}

	readings := make([]nmcorrelation.LeadLagReading, 0, len(signal.paths))

	for peerSymbol, peerPath := range signal.paths {
		if peerSymbol == prior.Label || len(peerPath) < 3 || len(currentPath) < 3 {
			continue
		}

		input := nmcorrelation.LagProfileInput{Left: peerPath, Right: currentPath}
		reading := drive[nmcorrelation.LagProfileInput, nmcorrelation.LeadLagReading](signal.leadlag, &input)

		if reading.Defined {
			readings = append(readings, reading)
		}
	}

	maps.Copy(output, summarize(readings))

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
