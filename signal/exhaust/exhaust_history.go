package exhaust

import (
	"math"
	"sort"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/statutil"
)

func measurementPrefix(symbol string) []byte {
	return []byte("measurement/" + symbol + "/" + string(logic.SourceExhaustion) + "/")
}

func (signal *Signal) fadeSamples(symbol string) []float64 {
	fadeHistory, _ := signal.tradeHistory(symbol)

	return fadeHistory
}

func (signal *Signal) peakPressureFade(symbol string) float64 {
	fades, _ := signal.tradeHistory(symbol)

	if len(fades) == 0 {
		return 0
	}

	peak := 0.0

	for _, fade := range fades {
		if fade > peak {
			peak = fade
		}
	}

	return peak
}

/*
bookHistory rebuilds the per-row depth-drop, spread-widen and imbalance-flip
series plus the latest touch state from book measurement rows in the tree alone.
The window depth derives from observed timestamps (statutil.WindowDepth), never a
fixed count; the first observation has no prior and seeds its own baseline.
*/
func (signal *Signal) bookHistory(symbol string) (
	depthDrops, spreadWidens, imbalanceFlips []float64,
	prevDepth, prevSpread, prevImbalance float64,
) {
	if signal.tree == nil {
		return nil, nil, nil, 0, 0, 0
	}

	type bookSample struct {
		stamp     float64
		depth     float64
		spread    float64
		imbalance float64
	}

	var (
		stamps  []float64
		samples = make([]bookSample, 0)
	)

	for prior := range signal.tree.Seek(measurementPrefix(symbol)) {
		depth := datura.Peek[float64](prior, "depth")

		if depth <= 0 {
			continue
		}

		stamp := datura.Peek[float64](prior, "timestamp")
		samples = append(samples, bookSample{
			stamp:     stamp,
			depth:     depth,
			spread:    datura.Peek[float64](prior, "spread"),
			imbalance: datura.Peek[float64](prior, "imbalance"),
		})
		stamps = append(stamps, stamp)
	}

	sort.Slice(samples, func(left, right int) bool {
		return samples[left].stamp < samples[right].stamp
	})

	lastDepth, lastSpread, lastImbalance := 0.0, 0.0, 0.0

	for _, sample := range samples {
		if lastDepth > 0 && sample.depth < lastDepth {
			depthDrops = append(depthDrops, (lastDepth-sample.depth)/lastDepth)
		}

		if lastSpread > 0 && sample.spread > lastSpread {
			spreadWidens = append(spreadWidens, (sample.spread-lastSpread)/lastSpread)
		}

		if lastImbalance != 0 || sample.imbalance != 0 {
			imbalanceFlips = append(imbalanceFlips, math.Abs(sample.imbalance-lastImbalance))
		}

		lastDepth = sample.depth
		lastSpread = sample.spread
		lastImbalance = sample.imbalance
		prevDepth = sample.depth
		prevSpread = sample.spread
		prevImbalance = sample.imbalance
	}

	keep := statutil.WindowDepth(stamps)

	return statutil.Tail(depthDrops, keep),
		statutil.Tail(spreadWidens, keep),
		statutil.Tail(imbalanceFlips, keep),
		prevDepth,
		prevSpread,
		prevImbalance
}

/*
tradeHistory rebuilds the pressure-fade baseline and the latest running pressure
from trade measurement rows in the tree alone. Book rows (depth>0) are skipped so
the pressure series stays a clean trade-only stream.
*/
func (signal *Signal) tradeHistory(symbol string) (fadeHistory []float64, prevPressure float64) {
	if signal.tree == nil {
		return nil, 0
	}

	type tradeSample struct {
		stamp    float64
		fade     float64
		pressure float64
	}

	var (
		stamps  []float64
		samples = make([]tradeSample, 0)
	)

	for prior := range signal.tree.Seek(measurementPrefix(symbol)) {
		if datura.Peek[float64](prior, "depth") > 0 {
			continue
		}

		stamp := datura.Peek[float64](prior, "timestamp")
		samples = append(samples, tradeSample{
			stamp:    stamp,
			fade:     datura.Peek[float64](prior, "pressureFade"),
			pressure: datura.Peek[float64](prior, "pressure"),
		})
		stamps = append(stamps, stamp)
	}

	sort.Slice(samples, func(left, right int) bool {
		return samples[left].stamp < samples[right].stamp
	})

	for _, sample := range samples {
		fadeHistory = append(fadeHistory, sample.fade)
		prevPressure = sample.pressure
	}

	return statutil.Tail(fadeHistory, statutil.WindowDepth(stamps)), prevPressure
}
