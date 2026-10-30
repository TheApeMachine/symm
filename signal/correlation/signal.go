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
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/kraken"
	parent "github.com/theapemachine/symm/signal"
)

const (
	maxCorrSamples  = 64
	minBoardSamples = 3
	maxBoardSymbols = 32
	maxBoardPairs   = 256
	maxInterval     = 0
)

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
			"structural_alignment",
		},
	}

	signal.System = runtime.NewSystem(ctx, "correlation", signal)
	return signal
}

/*
Step retains reprices for every symbol, then publishes one board-wide
structural_alignment reading on correlation/aggregate/….
*/
func (signal *Signal) Step(value *data.Value) *data.Value {
	if signal.Status() != runtime.READY || !parent.HasFrame(value) {
		return value
	}

	var trades []kraken.TradeData
	value.Lookup(&trades, "spot:trade")

	for _, trade := range parent.LastTradePerSymbol(trades) {
		signal.ingest(trade, value)
		score, ok := signal.boardAlignment()

		if ok {
			value.Append([]float64{score}, append([]string{
				signal.Name(),
				trade.Symbol,
			}, "structural_alignment")...)

			value.Append([]float64{score}, append([]string{
				signal.Name(),
				"aggregate",
			}, "structural_alignment")...)
		}

		audit.Record.Write(map[string]any{
			"signal":   signal.Name(),
			"trade":    trade,
			"score":    score,
			"ok":       ok,
			"eligible": len(signal.eligibleSymbols()),
		})
	}

	return value
}

func (signal *Signal) ingest(trade kraken.TradeData, value *data.Value) {
	price := trade.Price.Float64()
	if price <= 0 {
		return
	}

	at := time.Unix(0, int64(value.Timestamp))
	symbol := trade.Symbol

	untyped, _ := signal.paths.LoadOrStore(symbol, &path{})
	focal := untyped.(*path)
	focal.append(at, price)
}

/*
boardAlignment is the median absolute Hayashi correlation over eligible
symbol pairs: high means many paths are moving on the same current now.
*/
func (signal *Signal) boardAlignment() (float64, bool) {
	eligible := signal.eligibleSymbols()
	if len(eligible) < 2 {
		return 0, false
	}

	correlations := make([]float64, 0, maxBoardPairs)
	pairs := 0

	for i := 0; i < len(eligible); i++ {
		leftSym := eligible[i]
		left := signal.pathFor(leftSym)
		if left == nil {
			continue
		}

		for j := i + 1; j < len(eligible); j++ {
			if pairs >= maxBoardPairs {
				break
			}

			right := signal.pathFor(eligible[j])
			if right == nil {
				continue
			}

			corr, ok := algorithm.HayashiPairCorrelation(
				left.samples, right.samples, maxInterval,
			)
			if !ok || math.IsNaN(corr) {
				continue
			}

			correlations = append(correlations, math.Abs(corr))
			pairs++
		}

		if pairs >= maxBoardPairs {
			break
		}
	}

	if len(correlations) == 0 {
		return 0, false
	}

	sorted := append([]float64(nil), correlations...)
	slices.Sort(sorted)
	middle := len(sorted) / 2

	if len(sorted)%2 == 1 {
		return sorted[middle], true
	}

	return (sorted[middle-1] + sorted[middle]) / 2, true
}

func (signal *Signal) eligibleSymbols() []string {
	var symbols []string

	signal.paths.Range(func(symbol, untyped any) bool {
		p := untyped.(*path)
		if len(p.samples) >= minBoardSamples {
			symbols = append(symbols, symbol.(string))
		}
		return true
	})

	slices.Sort(symbols)

	if len(symbols) > maxBoardSymbols {
		symbols = symbols[:maxBoardSymbols]
	}

	return symbols
}

func (signal *Signal) pathFor(symbol string) *path {
	untyped, ok := signal.paths.Load(symbol)
	if !ok {
		return nil
	}
	return untyped.(*path)
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
