package leadlag

import (
	"context"
	"slices"
	"strings"
	"sync"
	"unsafe"

	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
	nmcorrelation "github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
	parent "github.com/theapemachine/symm/signal"
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
	paths   *sync.Map
	leadlag core.Primitive
	keys    []string
}

func NewSignal(ctx context.Context) *Signal {
	signal := &Signal{
		paths:   &sync.Map{},
		leadlag: nmcorrelation.NewLeadLag(algo.NewHayashiYoshida()),
		keys: []string{
			"defined_peer_count",
			"best_lag_seconds_median",
			"best_lag_seconds_mad",
			"led_peer_share",
			"covariance_score_gain_median",
		},
	}

	signal.System = runtime.NewSystem(ctx, "leadlag", signal)
	return signal
}

/*
Step processes every trade entry of the tick's frame; per-symbol state is keyed
by the entry's symbol and each entry writes its own (reserved) layer.
*/
func (signal *Signal) Step(value *data.Value) *data.Value {
	var trades []kraken.TradeData
	value.Lookup(&trades, "spot:trade")

	for _, trade := range parent.LastTradePerSymbol(trades) {
		signal.step(value, trade)
	}

	return value
}

func (signal *Signal) step(value *data.Value, trade kraken.TradeData) *data.Value {
	if signal.Status() != runtime.READY || value == nil || len(value.Layers) == 0 {
		return value
	}

	price := trade.Price.Float64()
	if price <= 0 {
		return value
	}

	atNano := int64(value.Timestamp)
	symbol := trade.Symbol

	untypedCurrentPath, _ := signal.paths.LoadOrStore(symbol, []temporal.Price{})
	currentPath := untypedCurrentPath.([]temporal.Price)

	if len(currentPath) > 0 {
		last := currentPath[len(currentPath)-1].At
		if atNano < last {
			return value
		}
		if atNano == last {
			currentPath[len(currentPath)-1] = temporal.Price{At: atNano, Value: price}
			signal.paths.Store(symbol, currentPath)
		}
	}

	if len(currentPath) == 0 || atNano > currentPath[len(currentPath)-1].At {
		if len(currentPath) >= maxLeadLagSamples {
			currentPath = currentPath[1:]
		}
		currentPath = append(currentPath, temporal.Price{At: atNano, Value: price})
		signal.paths.Store(symbol, currentPath)
	}

	candidateSymbols := make([]string, 0)
	signal.paths.Range(func(peerSymbol, peerPath any) bool {
		if peerSymbol != symbol && len(peerPath.([]temporal.Price)) >= 3 && len(currentPath) >= 3 {
			candidateSymbols = append(candidateSymbols, peerSymbol.(string))
		}
		return true
	})

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
		untypedPeerPath, _ := signal.paths.Load(peerSymbol)
		peerPath := untypedPeerPath.([]temporal.Price)
		input := nmcorrelation.LagProfileInput{Left: peerPath, Right: currentPath}
		reading := drive[nmcorrelation.LagProfileInput, nmcorrelation.LeadLagReading](signal.leadlag, &input)
		if reading.Defined {
			readings = append(readings, reading)
		}
	}

	output := summarize(readings)

	keys := []string{
		signal.Name(),
		trade.Symbol,
	}

	return value.Append([]float64{
		output["defined_peer_count"],
		output["best_lag_seconds_median"],
		output["best_lag_seconds_mad"],
		output["led_peer_share"],
		output["covariance_score_gain_median"],
	}, append(keys, signal.keys...)...)
}

func drive[From, To any](op core.Primitive, payload *From) To {
	var answer To
	for out := range op.Next(transport.NewOne(unsafe.Pointer(payload)).Next(nil)) {
		answer = *(*To)(out)
	}
	return answer
}

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
