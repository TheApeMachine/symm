package correlation

import (
	"context"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/theapemachine/nomagique/algorithm"
	nmcorrelation "github.com/theapemachine/nomagique/correlation"
	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/kraken"
	parent "github.com/theapemachine/symm/signal"
)

// A price path is one reprice per bar. The correlation pipeline holds a
// bounded path per symbol and computes the asynchronous (Hayashi) correlation
// between the focal symbol and each peer, reducing the cohort to one robust
// covariance score.
const (
	maxCorrSamples = 64
	maxCorrPeers   = 16

	// maxInterval guards against a single pathological gap dominating the
	// Hayashi realized variance. It is set to the median reprice spacing in
	// practice; here it is left unbounded so the path itself is the interval.
	maxInterval = 0
)

/*
path is one symbol's retained reprice history.
*/
type path struct {
	samples []nmcorrelation.Sample
}

type Signal struct {
	*runtime.System
	paths *sync.Map
	keys  []string
}

func NewSignal(ctx context.Context) *Signal {
	signal := &Signal{
		paths: &sync.Map{},
		keys: []string{
			"cohort_covariance_score",
		},
	}

	signal.System = runtime.NewSystem(ctx, "correlation", signal)
	return signal
}

/*
Step processes one trade per distinct symbol in the tick. The correlation
pipeline retains cross-symbol peer state, so symbol steps run sequentially;
deduping by symbol avoids redundant pair work when many prints hit the same
symbol in one frame.
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

	untyped, _ := signal.paths.LoadOrStore(symbol, &path{})
	focal := untyped.(*path)

	focal.append(at, price)

	// Collect the peer paths that have enough history to score.
	peers := make([]string, 0)
	signal.paths.Range(func(peerSymbol, peer any) bool {
		p := peer.(*path)
		if peerSymbol != symbol && len(p.samples) >= 2 && len(focal.samples) >= 2 {
			peers = append(peers, peerSymbol.(string))
		}
		return true
	})

	if len(peers) > maxCorrPeers {
		peers = peers[:maxCorrPeers]
	}

	score, defined := signal.cohortScore(focal, peers)

	keys := []string{
		signal.Name(),
		trade.Symbol,
	}

	// The signal carries no layer when there is no evidence yet; a zero that
	// reads as a measured value is worse than an absent metric.
	if defined {
		value.Append([]float64{score}, append(keys, signal.keys[0])...)
	}

	return value
}

/*
cohortScore computes the median absolute asynchronous correlation between the
focal path and each peer path, the same reduction the vendored pipeline's
pairwise stage performed. Returns (score, defined).
*/
func (signal *Signal) cohortScore(focal *path, peers []string) (float64, bool) {
	if len(focal.samples) < 2 || len(peers) == 0 {
		return 0, false
	}

	correlations := make([]float64, 0, len(peers))

	for _, peerSymbol := range peers {
		untyped, _ := signal.paths.Load(peerSymbol)
		peer := untyped.(*path)

		if len(peer.samples) < 2 {
			continue
		}

		// The focal path is the anchor: the Hayashi correlation is symmetric
		// in its intervals, so the left/right assignment is a convention.
		corr, ok := algorithm.HayashiPairCorrelation(
			focal.samples, peer.samples, maxInterval,
		)

		if !ok || math.IsNaN(corr) {
			continue
		}

		correlations = append(correlations, math.Abs(corr))
	}

	if len(correlations) == 0 {
		return 0, false
	}

	// Median is the robust cohort score: a single spurious peer cannot move it.
	sorted := append([]float64(nil), correlations...)
	slices.Sort(sorted)
	middle := len(sorted) / 2

	if len(sorted)%2 == 1 {
		return sorted[middle], true
	}

	return (sorted[middle-1] + sorted[middle]) / 2, true
}

func (p *path) append(at time.Time, value float64) {
	if len(p.samples) > 0 {
		last := p.samples[len(p.samples)-1].At

		if at.Before(last) {
			return
		}

		if at.Equal(last) {
			p.samples[len(p.samples)-1] = nmcorrelation.Sample{At: at, Value: value}
			return
		}
	}

	if len(p.samples) >= maxCorrSamples {
		p.samples = p.samples[1:]
	}

	p.samples = append(p.samples, nmcorrelation.Sample{At: at, Value: value})
}
