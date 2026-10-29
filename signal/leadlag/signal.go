package leadlag

import (
	"context"
	"math"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/theapemachine/nomagique/algorithm"
	nmcorrelation "github.com/theapemachine/nomagique/correlation"
	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/kraken"
	parent "github.com/theapemachine/symm/signal"
)

// One reprice is one bar. CrossLagScore shifts the anchor path in whole-bar
// increments, so the bar interval is the median reprice spacing; deriving it
// from the path keeps the lag in price events rather than an arbitrary clock.
const barIntervalSeconds = 1.0

const (
	maxLeadLagSamples = 64
	maxLeadLagPeers   = 16
)

/*
Signal is the asynchronous price-path lead-lag instrument.
*/
type Signal struct {
	*runtime.System
	paths *sync.Map
	keys  []string
}

func NewSignal(ctx context.Context) *Signal {
	signal := &Signal{
		paths: &sync.Map{},
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

	at := time.Unix(0, int64(value.Timestamp))
	symbol := trade.Symbol

	untypedCurrentPath, _ := signal.paths.LoadOrStore(symbol, []nmcorrelation.Sample{})
	currentPath := untypedCurrentPath.([]nmcorrelation.Sample)

	if len(currentPath) > 0 {
		last := currentPath[len(currentPath)-1].At
		if at.Before(last) {
			return value
		}
		if at.Equal(last) {
			currentPath[len(currentPath)-1] = nmcorrelation.Sample{At: at, Value: price}
			signal.paths.Store(symbol, currentPath)
		}
	}

	if len(currentPath) == 0 || at.After(currentPath[len(currentPath)-1].At) {
		if len(currentPath) >= maxLeadLagSamples {
			currentPath = currentPath[1:]
		}
		currentPath = append(currentPath, nmcorrelation.Sample{At: at, Value: price})
		signal.paths.Store(symbol, currentPath)
	}

	candidateSymbols := make([]string, 0)
	signal.paths.Range(func(peerSymbol, peerPath any) bool {
		if peerSymbol != symbol && len(peerPath.([]nmcorrelation.Sample)) >= 3 && len(currentPath) >= 3 {
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

	// The focal path is the measured series; each peer is searched as the
	// anchor against it. CrossLagScore shifts the anchor in whole-bar steps and
	// reports the best lag (in bars) and its Hayashi correlation. A negative
	// lag means the focal path precedes the peer.
	readings := make([]leadLagReading, 0, len(candidateSymbols))
	for _, peerSymbol := range candidateSymbols {
		untypedPeerPath, _ := signal.paths.Load(peerSymbol)
		peerPath := untypedPeerPath.([]nmcorrelation.Sample)

		lagBars, corr, ok := algorithm.CrossLagScore(peerPath, currentPath, time.Second*barIntervalSeconds)

		if !ok {
			continue
		}

		readings = append(readings, leadLagReading{
			lagSeconds:  float64(lagBars) * barIntervalSeconds,
			corr:        corr,
			leads:       lagBars < 0,
			gain:        math.Abs(corr),
			gainDefined: true,
		})
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
