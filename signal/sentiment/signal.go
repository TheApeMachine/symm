package sentiment

import (
	"context"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/runtime"
	parent "github.com/theapemachine/symm/signal"
)

type causalEstimator struct {
	count float64
	mean  float64
	m2    float64
}

func (ce *causalEstimator) Step(value float64) (
	hasBaseline bool, baseline, residual float64, hasZ bool, zScore float64,
) {
	priorCount := ce.count
	priorMean := ce.mean
	priorM2 := ce.m2

	ce.count++
	delta := value - ce.mean
	ce.mean += delta / ce.count
	ce.m2 += delta * (value - ce.mean)

	if priorCount == 0 {
		return false, 0, 0, false, 0
	}

	residual = value - priorMean
	scale, scorable := core.PriorScale(priorCount, priorM2, value, priorMean)
	if !scorable {
		return true, priorMean, residual, false, 0
	}

	return true, priorMean, residual, true, residual / scale
}

type observation struct {
	at    float64
	price float64
}

type memberState struct {
	observations []observation
	spacings     []float64
	lastPrice    float64
	lastChangeAt float64
}

func (mem *memberState) asOf(t float64) (observation, bool) {
	for index := len(mem.observations) - 1; index >= 0; index-- {
		if mem.observations[index].at <= t {
			return mem.observations[index], true
		}
	}
	return observation{}, false
}

func (mem *memberState) retain() {
	if len(mem.observations) > 256 {
		mem.observations = mem.observations[len(mem.observations)-256:]
	}
}

func median(values []float64) float64 {
	sorted := slices.Clone(values)
	slices.Sort(sorted)
	mid := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[mid]
	}
	return (sorted[mid-1] + sorted[mid]) / 2
}

type focalState struct {
	breadthEstimator causalEstimator
	medianEstimator  causalEstimator
	hasPrevBreadth   bool
	prevBreadth      float64
	historyPoints    [][2]float64
	historyDistances []float64
}

type Signal struct {
	*runtime.System
	mu      sync.Mutex
	members map[string]*memberState
	focals  map[string]*focalState
	keys    []string
}

func NewSignal(ctx context.Context) *Signal {
	signal := &Signal{
		members: make(map[string]*memberState),
		focals:  make(map[string]*focalState),
		keys: []string{
			"breadth",
			"same_direction_peer_count",
			"opposite_direction_peer_count",
			"breadth_velocity",
			"historical_path_distance",
			"historical_path_percentile",
		},
	}

	signal.System = runtime.NewSystem(ctx, "sentiment", signal)
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

	atNano := float64(value.Timestamp)
	symbol := trade.Symbol

	signal.mu.Lock()
	defer signal.mu.Unlock()

	mem, found := signal.members[symbol]
	if !found {
		mem = &memberState{
			lastPrice:    price,
			lastChangeAt: atNano,
		}
		signal.members[symbol] = mem
	}

	if len(mem.observations) == 0 {
		mem.lastPrice = price
		mem.lastChangeAt = atNano
	}

	if last := len(mem.observations) - 1; last >= 0 && atNano > mem.observations[last].at && price != mem.lastPrice {
		spacing := (atNano - mem.lastChangeAt) * 1e-9
		if spacing > 0 {
			mem.spacings = append(mem.spacings, spacing)
			if len(mem.spacings) > 128 {
				mem.spacings = mem.spacings[len(mem.spacings)-128:]
			}
		}
		mem.lastPrice = price
		mem.lastChangeAt = atNano
	}

	mem.observations = append(mem.observations, observation{at: atNano, price: price})
	mem.retain()

	var cadences []float64
	for _, member := range signal.members {
		if len(member.spacings) > 0 {
			cadences = append(cadences, median(member.spacings))
		}
	}

	if len(cadences) == 0 {
		// Nothing is computable yet: the tick carries no layer for it
		// rather than zeros that read as a measured value.
		return value
	}

	cohortHorizon := median(cadences)
	focalHorizon := cohortHorizon
	if len(mem.spacings) > 0 {
		focalHorizon = median(mem.spacings)
	}

	horizon := math.Max(cohortHorizon, focalHorizon)
	horizonNano := horizon * 1e9
	fromNano := atNano - horizonNano

	type memberReturn struct {
		symbol string
		ret    float64
	}
	var valid []memberReturn

	for memberSymbol, member := range signal.members {
		atObs, hasAt := member.asOf(atNano)
		fromObs, hasFrom := member.asOf(fromNano)

		if !hasAt || !hasFrom || atObs.price <= 0 || fromObs.price <= 0 {
			continue
		}
		if atNano-atObs.at > float64(15*time.Minute) {
			continue
		}

		valid = append(valid, memberReturn{
			symbol: memberSymbol,
			ret:    math.Log(atObs.price / fromObs.price),
		})
	}

	validCount := float64(len(valid))
	if validCount == 0 {
		// Nothing is computable yet: the tick carries no layer for it
		// rather than zeros that read as a measured value.
		return value
	}

	advanceCount := 0.0
	declineCount := 0.0
	var returns []float64

	for _, validReturn := range valid {
		returns = append(returns, validReturn.ret)
		if validReturn.ret > 0 {
			advanceCount++
		} else if validReturn.ret < 0 {
			declineCount++
		}
	}

	breadth := (advanceCount - declineCount) / validCount
	slices.Sort(returns)
	medRet := returns[len(returns)/2]

	focalReturn := 0.0
	for _, validReturn := range valid {
		if validReturn.symbol == symbol {
			focalReturn = validReturn.ret
			break
		}
	}

	sameDirPeer := 0.0
	oppDirPeer := 0.0
	for _, validReturn := range valid {
		if validReturn.symbol == symbol || validReturn.ret == 0 {
			continue
		}
		if (focalReturn > 0 && validReturn.ret > 0) || (focalReturn < 0 && validReturn.ret < 0) {
			sameDirPeer++
		} else if (focalReturn > 0 && validReturn.ret < 0) || (focalReturn < 0 && validReturn.ret > 0) {
			oppDirPeer++
		}
	}

	focal, found := signal.focals[symbol]
	if !found {
		focal = &focalState{}
		signal.focals[symbol] = focal
	}

	_, _, _, hasBZ, bZScore := focal.breadthEstimator.Step(breadth)
	var breadthVelocity float64
	if focal.hasPrevBreadth {
		breadthVelocity = breadth - focal.prevBreadth
	}
	focal.prevBreadth = breadth
	focal.hasPrevBreadth = true

	_, _, _, hasMZ, mZScore := focal.medianEstimator.Step(medRet)
	var histDistance, histPercentile float64
	if hasBZ && hasMZ {
		histDistance, histPercentile = focal.historyPath([2]float64{bZScore, mZScore})
	}

	keys := []string{
		signal.Name(),
		trade.Symbol,
	}

	return value.Append([]float64{
		breadth,
		sameDirPeer,
		oppDirPeer,
		breadthVelocity,
		histDistance,
		histPercentile,
	}, append(keys, signal.keys...)...)
}

func (focal *focalState) historyPath(target [2]float64) (minDist float64, percentile float64) {
	if len(focal.historyPoints) > 0 {
		minDist = math.Inf(1)
		for _, point := range focal.historyPoints {
			dx := target[0] - point[0]
			dy := target[1] - point[1]
			minDist = math.Min(minDist, math.Sqrt(dx*dx+dy*dy))
		}

		if len(focal.historyDistances) > 0 {
			below := 0
			for _, past := range focal.historyDistances {
				if past <= minDist {
					below++
				}
			}
			percentile = float64(below) / float64(len(focal.historyDistances))
		}

		focal.historyDistances = append(focal.historyDistances, minDist)
		if len(focal.historyDistances) > 256 {
			focal.historyDistances = focal.historyDistances[len(focal.historyDistances)-256:]
		}
	}

	focal.historyPoints = append(focal.historyPoints, target)
	if len(focal.historyPoints) > 256 {
		focal.historyPoints = focal.historyPoints[len(focal.historyPoints)-256:]
	}

	return minDist, percentile
}
