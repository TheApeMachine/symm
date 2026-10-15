package leadlag

import (
	"context"
	"slices"
	"strings"
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

const (
	maxLeadLagSamples = 64
	maxLeadLagPeers   = 16
)

/*
Signal is the asynchronous price-path lead-lag instrument.
*/
type Signal struct {
	*runtime.System
	paths   map[string][]temporal.Price
	leadlag core.Primitive
	last    map[string]*data.Measurement
}

func NewSignal(ctx context.Context) *Signal {
	signal := &Signal{
		paths:   make(map[string][]temporal.Price),
		leadlag: nmcorrelation.NewLeadLag(algo.NewHayashiYoshida()),
		last:    make(map[string]*data.Measurement),
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
			price = entry.Metric.Raw()
			found = true
			break
		}
	}

	if !found {
		if prev, ok := signal.last[prior.Label]; ok && prev != nil {
			return prev
		}

		if prior.Source == "spot:trade" {
			signal.Error(errnie.Err(errnie.Validation, "[leadlag] missing price", nil))
		}

		return nil
	}

	if price <= 0 {
		if prev, ok := signal.last[prior.Label]; ok && prev != nil {
			return prev
		}

		return nil
	}

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

	candidateSymbols := make([]string, 0, len(signal.paths))

	for peerSymbol, peerPath := range signal.paths {
		if peerSymbol != prior.Label && len(peerPath) >= 3 && len(currentPath) >= 3 {
			candidateSymbols = append(candidateSymbols, peerSymbol)
		}
	}

	slices.SortFunc(candidateSymbols, func(a, b string) int {
		rankA := leaderRank(a)
		rankB := leaderRank(b)
		if rankA != rankB {
			return rankA - rankB
		}
		return strings.Compare(a, b)
	})

	if len(candidateSymbols) > maxLeadLagPeers {
		candidateSymbols = candidateSymbols[:maxLeadLagPeers]
	}

	readings := make([]nmcorrelation.LeadLagReading, 0, len(candidateSymbols))

	for _, peerSymbol := range candidateSymbols {
		peerPath := signal.paths[peerSymbol]
		input := nmcorrelation.LagProfileInput{Left: peerPath, Right: currentPath}
		reading := drive[nmcorrelation.LagProfileInput, nmcorrelation.LeadLagReading](signal.leadlag, &input)

		if reading.Defined {
			readings = append(readings, reading)
		}
	}

	output := summarize(readings)
	if output == nil {
		output = make(map[string]float64)
	}

	if err := signal.leadlag.Error(); err != nil {
		signal.Error(err)
		return nil
	}

	prev := signal.last[prior.Label]
	if prev == nil {
		prev = data.NewMeasurement(prior.Epoch, prior.Label, signal.Name(), prior.At, prior.From)
	}

	res := prev.Next(prior, output)
	signal.last[prior.Label] = res
	return res
}

func drive[From, To any](op core.Primitive, payload *From) To {
	var answer To

	for out := range op.Next(transport.NewOne(unsafe.Pointer(payload)).Next(nil)) {
		answer = *(*To)(out)
	}

	return answer
}

/*
leaderRank prioritizes anchor market movers (BTC, ETH, SOL) over obscure altcoins
so that leadlag evaluates cross-asset dynamics against true market drivers.
*/
func leaderRank(symbol string) int {
	switch {
	case strings.HasPrefix(symbol, "BTC") || strings.HasPrefix(symbol, "XBT") || strings.HasPrefix(symbol, "XXBT"):
		return 0
	case strings.HasPrefix(symbol, "ETH") || strings.HasPrefix(symbol, "XETH"):
		return 1
	case strings.HasPrefix(symbol, "SOL"):
		return 2
	default:
		return 3
	}
}
