package correlation

import (
	"context"
	"fmt"
	"iter"
	"math"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/signal/dist"
	"github.com/theapemachine/symm/statutil"
)

/*
Signal: The "Herd Behavior" Perspective
See the package docs and DECISION.md for details.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	tree   *dmt.Tree
}

type priceTick struct {
	Timestamp int64
	Price     float64
}

type returnInterval struct {
	start int64
	end   int64
	ret   float64
}

func NewSignal(ctx context.Context, tree *dmt.Tree) *Signal {
	ctx, cancel := context.WithCancel(ctx)
	return &Signal{ctx: ctx, cancel: cancel, tree: tree}
}

func (signal *Signal) IngestRoles() []string {
	return []string{"ticker"}
}

func (signal *Signal) Measure(
	datapoint *datura.Artifact,
	crossSection *market.CrossSection,
) iter.Seq[*datura.Artifact] {
	return func(yield func(*datura.Artifact) bool) {
		if signal == nil || datapoint == nil || crossSection == nil {
			return
		}
		if datura.Peek[string](datapoint, "channel") != "ticker" {
			return
		}

		currentStamp := datapoint.Timestamp()
		window := crossSection.MinBarsRequired()
		symbols := crossSection.Symbols()
		ticksCache := make(map[string][]priceTick, len(symbols))

		for _, symbol := range symbols {
			ticksCache[symbol] = signal.getTickerTicks(symbol, currentStamp)
		}

		for rowIndex := 0; ; rowIndex++ {
			row, rowErr := market.SymbolFromTicker(datapoint, rowIndex)
			if rowErr != nil {
				break
			}
			ticks := ticksCache[row.Name]
			hasCurrent := false
			if len(ticks) > 0 && ticks[len(ticks)-1].Timestamp == currentStamp {
				hasCurrent = true
			}
			if !hasCurrent {
				ticksCache[row.Name] = append(ticks, priceTick{Timestamp: currentStamp, Price: row.Price})
			}
		}

		for rowIndex := 0; ; rowIndex++ {
			row, rowErr := market.SymbolFromTicker(datapoint, rowIndex)
			if rowErr != nil {
				return
			}

			csCorr, csEnergy, csPeerCorrs, csPeerEnergyMedian, ok := crossSection.SymbolPeerStats(row.Name, window)
			if !ok {
				continue
			}

			targetTicks := ticksCache[row.Name]
			var stamps []float64
			for _, t := range targetTicks {
				stamps = append(stamps, float64(t.Timestamp))
			}

			cadenceVal := statutil.MedianCadence(stamps)
			if cadenceVal <= 0 {
				cadenceVal = float64(10 * time.Second)
			}
			gridInterval := int64(cadenceVal)
			windowStart := currentStamp - int64(time.Duration(window)*time.Duration(gridInterval))

			allCorrelations := make(map[string]float64)
			for _, symA := range symbols {
				retA := alignToGrid(ticksCache[symA], gridInterval, windowStart, currentStamp)
				var corrs []float64
				for _, symB := range symbols {
					if symA == symB {
						continue
					}
					retB := alignToGrid(ticksCache[symB], gridInterval, windowStart, currentStamp)
					if corr, ok := pearsonCorrelation(retA, retB); ok {
						corrs = append(corrs, corr)
					} else {
						if corrHY, okHY := hayashiYoshidaCorrelation(ticksCache[symA], ticksCache[symB]); okHY {
							corrs = append(corrs, corrHY)
						}
					}
				}
				if len(corrs) > 0 {
					allCorrelations[symA] = statutil.Median(corrs)
				} else {
					allCorrelations[symA] = 0.0
				}
			}

			correlation := csCorr
			if c, found := allCorrelations[row.Name]; found && len(symbols) >= 2 {
				correlation = c
			}

			energy := csEnergy
			peerEnergyMedian := csPeerEnergyMedian

			peerCorrelations := make([]float64, 0, len(symbols))
			for _, symY := range symbols {
				if symY == row.Name {
					continue
				}
				if c, found := allCorrelations[symY]; found {
					peerCorrelations = append(peerCorrelations, c)
				}
			}
			if len(peerCorrelations) == 0 {
				peerCorrelations = csPeerCorrs
			}

			decoupling := math.Max(0, 1-math.Abs(correlation))
			relativeEnergy := energy / (energy + peerEnergyMedian)

			herdGate := 0.0
			if len(peerCorrelations) > 0 {
				percentile := peerHerdingPercentile(peerCorrelations)
				if gate, gateErr := peerQuantile(peerCorrelations, percentile); gateErr != nil {
					errnie.Error(errnie.Err(errnie.Validation, "correlation: herd gate quantile", gateErr))
				} else {
					herdGate = gate
				}
			}

			herd := math.Max(0, correlation-herdGate) * energy
			herdAligned := math.Max(0, correlation) * (1 - decoupling) * energy
			if herdAligned > herd {
				herd = herdAligned
			}
			herd *= 1 - decoupling*decoupling

			alpha := decoupling * decoupling * relativeEnergy * energy * (1 + relativeEnergy)
			noise := decoupling * (1 - decoupling) * (1 - relativeEnergy) / (1 + energy)
			stress := math.Max(0, -correlation) * energy

			shares := []dist.Share{
				{Key: "herdScore", Category: logic.CategorySystemicHerd, Mass: herd},
				{Key: "alphaScore", Category: logic.CategoryDecoupledAlpha, Mass: alpha},
				{Key: "noiseScore", Category: logic.CategoryStochasticNoise, Mass: noise},
				{Key: "stressScore", Category: logic.CategoryDivergentStress, Mass: stress},
			}

			measurement := datura.Acquire("correlation", datura.APPJSON)
			measurement.WithRole("measurement")
			measurement.WithScope(row.Name)
			errnie.Error(measurement.SetOrigin(string(logic.SourceCorrelation)))
			measurement.SetTimestamp(datapoint.Timestamp())
			measurement.MergeOutput("correlation", correlation)
			measurement.MergeOutput("energy", energy)

			history := signal.history(row.Name, currentStamp)
			peakScore := 0.0
			if len(history) >= 2 {
				median := statutil.Median(history)
				mad := statutil.MedianAbsoluteDeviation(history, median)
				if mad > 0 {
					peakScore = math.Max(0, (correlation-median)/mad)
				} else {
					_, std := meanStdDev(history)
					if std > 0 {
						peakScore = math.Max(0, (correlation-median)/std)
					}
				}
			}
			measurement.MergeOutput("peakScore", peakScore)

			confidence := dist.Write(measurement, shares)
			if confidence <= 0 {
				measurement.Release()
				continue
			}

			if !yield(measurement) {
				return
			}
		}
	}
}

func getPriceFromTicker(artifact *datura.Artifact, symbol string) (float64, bool) {
	for rowIndex := 0; ; rowIndex++ {
		sym := datura.Peek[string](artifact, "data", rowIndex, "symbol")
		if sym == "" {
			return 0, false
		}
		if sym == symbol {
			if last := datura.Peek[float64](artifact, "data", rowIndex, "last"); last > 0 {
				return last, true
			}
		}
	}
}

func (signal *Signal) getTickerTicks(symbol string, currentStamp int64) []priceTick {
	if signal.tree == nil {
		return nil
	}

	windowStart := currentStamp - int64(12*time.Hour)
	var ticks []priceTick

	for _, seekKey := range dailyPrefixes("ticker", symbol, windowStart, currentStamp) {
		for prior := range signal.tree.Seek(seekKey) {
			stamp := prior.Timestamp()

			if stamp < windowStart {
				continue
			}

			if stamp > currentStamp {
				break
			}

			if price, ok := getPriceFromTicker(prior, symbol); ok {
				ticks = append(ticks, priceTick{Timestamp: stamp, Price: price})
			}
		}
	}

	if len(ticks) > 50 {
		ticks = ticks[len(ticks)-50:]
	}

	return ticks
}

func alignToGrid(ticks []priceTick, gridInterval int64, windowStart, currentStamp int64) []float64 {
	if len(ticks) == 0 {
		return nil
	}
	numIntervals := int((currentStamp - windowStart) / gridInterval)
	if numIntervals <= 0 {
		return nil
	}
	prices := make([]float64, numIntervals)
	tickIdx := 0
	lastPrice := ticks[0].Price
	for j := 0; j < numIntervals; j++ {
		cutoff := windowStart + int64(j+1)*gridInterval
		for tickIdx < len(ticks) && ticks[tickIdx].Timestamp <= cutoff {
			lastPrice = ticks[tickIdx].Price
			tickIdx++
		}
		prices[j] = lastPrice
	}
	returns := make([]float64, numIntervals-1)
	for j := 1; j < numIntervals; j++ {
		if prices[j-1] > 0 && prices[j] > 0 {
			returns[j-1] = math.Log(prices[j] / prices[j-1])
		}
	}
	return returns
}

func hayashiYoshidaCorrelation(ticksA, ticksB []priceTick) (float64, bool) {
	if len(ticksA) < 2 || len(ticksB) < 2 {
		return 0, false
	}
	returnsA := make([]returnInterval, 0, len(ticksA)-1)
	for i := 1; i < len(ticksA); i++ {
		if ticksA[i-1].Price > 0 && ticksA[i].Price > 0 {
			returnsA = append(returnsA, returnInterval{
				start: ticksA[i-1].Timestamp,
				end:   ticksA[i].Timestamp,
				ret:   math.Log(ticksA[i].Price / ticksA[i-1].Price),
			})
		}
	}
	returnsB := make([]returnInterval, 0, len(ticksB)-1)
	for j := 1; j < len(ticksB); j++ {
		if ticksB[j-1].Price > 0 && ticksB[j].Price > 0 {
			returnsB = append(returnsB, returnInterval{
				start: ticksB[j-1].Timestamp,
				end:   ticksB[j].Timestamp,
				ret:   math.Log(ticksB[j].Price / ticksB[j-1].Price),
			})
		}
	}
	if len(returnsA) == 0 || len(returnsB) == 0 {
		return 0, false
	}

	cov := 0.0
	for _, ra := range returnsA {
		for _, rb := range returnsB {
			overlapStart := ra.start
			if rb.start > overlapStart {
				overlapStart = rb.start
			}
			overlapEnd := ra.end
			if rb.end < overlapEnd {
				overlapEnd = rb.end
			}
			if overlapStart < overlapEnd {
				cov += ra.ret * rb.ret
			}
		}
	}

	var qvA, qvB float64
	for _, ra := range returnsA {
		qvA += ra.ret * ra.ret
	}
	for _, rb := range returnsB {
		qvB += rb.ret * rb.ret
	}
	if qvA <= 0 || qvB <= 0 {
		return 0, false
	}

	corr := cov / math.Sqrt(qvA*qvB)
	if math.IsNaN(corr) || math.IsInf(corr, 0) {
		return 0, false
	}
	return corr, true
}

func meanStdDev(values []float64) (mean float64, std float64) {
	if len(values) == 0 {
		return 0, 0
	}
	for _, value := range values {
		mean += value
	}
	mean /= float64(len(values))
	for _, value := range values {
		delta := value - mean
		std += delta * delta
	}
	std = math.Sqrt(std / float64(len(values)))
	return mean, std
}

func pearsonCorrelation(left, right []float64) (float64, bool) {
	if len(left) != len(right) || len(left) < 2 {
		return 0, false
	}
	meanLeft, stdLeft := meanStdDev(left)
	meanRight, stdRight := meanStdDev(right)
	if stdLeft <= 0 || stdRight <= 0 {
		return 0, false
	}
	covariance := 0.0
	for index := range left {
		covariance += (left[index] - meanLeft) * (right[index] - meanRight)
	}
	covariance /= float64(len(left))
	corr := covariance / (stdLeft * stdRight)
	if math.IsNaN(corr) || math.IsInf(corr, 0) {
		return 0, false
	}
	return corr, true
}

func (signal *Signal) history(symbol string, currentStamp int64) []float64 {
	if signal.tree == nil {
		return nil
	}

	windowStart := currentStamp - int64(12*time.Hour)
	var correlations []float64
	var stamps []float64
	rolePrefix := "measurement/" + symbol + "/" + string(logic.SourceCorrelation)

	for _, seekKey := range dailyPrefixes(rolePrefix, "", windowStart, currentStamp) {
		for prior := range signal.tree.Seek(seekKey) {
			stamp := datura.Peek[float64](prior, "timestamp")
			if stamp == 0 {
				stamp = float64(prior.Timestamp())
			}

			if int64(stamp) < windowStart {
				continue
			}

			if int64(stamp) > currentStamp {
				break
			}

			correlation := datura.Peek[float64](prior, "output", "correlation")
			correlations = append(correlations, correlation)
			stamps = append(stamps, stamp)
		}
	}

	keep := statutil.WindowDepth(stamps)
	return statutil.Tail(correlations, keep)
}

func dailyPrefixes(role string, symbol string, startNano, endNano int64) [][]byte {
	start := time.Unix(0, startNano).UTC().Truncate(24 * time.Hour)
	end := time.Unix(0, endNano).UTC().Truncate(24 * time.Hour)

	if end.Before(start) {
		end = start
	}

	prefixes := make([][]byte, 0, int(end.Sub(start)/(24*time.Hour))+1)

	for cursor := start; !cursor.After(end); cursor = cursor.AddDate(0, 0, 1) {
		dayStr := cursor.Format("2006/01/02")
		if symbol == "" {
			prefixes = append(prefixes, []byte(role+"/"+dayStr+"/"))
		} else {
			prefixes = append(prefixes, []byte(role+"/"+symbol+"/"+dayStr+"/"))
			prefixes = append(prefixes, []byte(role+"/"+symbol+"/kraken/"+dayStr+"/"))
		}
	}

	return prefixes
}

func peerHerdingPercentile(peerCorrelations []float64) float64 {
	lower, upper, err := statutil.Quartiles(peerCorrelations)
	if err != nil {
		return 0.5
	}
	median := statutil.Median(peerCorrelations)
	span := upper - lower
	if span <= 0 && median == 0 {
		return 0.5
	}
	return span / (math.Abs(median) + span)
}

func peerQuantile(values []float64, percentile float64) (float64, error) {
	filtered := make([]float64, 0, len(values))
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			continue
		}
		filtered = append(filtered, value)
	}
	if len(filtered) == 0 {
		return 0, fmt.Errorf("correlation: peer quantile requires finite samples")
	}
	return statutil.Quantile(percentile, filtered)
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
