package sentiment

import (
	"context"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/kraken"
	parent "github.com/theapemachine/symm/signal"
)

// minPrior is the minimum observations a running moment must hold before its
// predictive scale is treated as usable. Below this the centre and spread are
// estimated from too few samples for the predictive correction to be
// meaningful.
const minPrior = 9

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

	variance := priorM2 / float64(priorCount-1)

	if priorCount < minPrior || variance <= 0 {
		return true, priorMean, residual, false, 0
	}

	stddev := math.Sqrt(variance)
	count := float64(priorCount)

	// Predictive scale: estimating the centre and the spread from the same
	// priorCount observations inflates the spread a new sample is drawn from.
	centreInflation := math.Sqrt(1 + 1/count)
	spreadInflation := 1 + 1/math.Sqrt(2*(count-1))

	scale := stddev * centreInflation * spreadInflation

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
	members     *sync.Map
	focals      *sync.Map
	lastAligned *sync.Map
	keys        []string
}

func NewSignal(ctx context.Context) *Signal {
	signal := &Signal{
		members:     &sync.Map{},
		focals:      &sync.Map{},
		lastAligned: &sync.Map{},
		keys: []string{
			"cohort_still_aligned",
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

	signal.appendCohortRecurrence(value)

	return value
}

/*
appendCohortRecurrence publishes the median same-direction peer count across
symbols as the board sentiment oracle (R11), not a single-name reading.
*/
func (signal *Signal) appendCohortRecurrence(value *data.Value) {
	if signal.Status() != runtime.READY || value == nil {
		return
	}

	var scores []float64
	signal.lastAligned.Range(func(_, untyped any) bool {
		if score, ok := untyped.(float64); ok {
			scores = append(scores, score)
		}
		return true
	})

	if len(scores) == 0 {
		return
	}

	slices.Sort(scores)
	median := scores[len(scores)/2]
	if len(scores)%2 == 0 {
		median = (scores[len(scores)/2-1] + scores[len(scores)/2]) / 2
	}

	value.Append([]float64{median}, append([]string{
		signal.Name(),
		"aggregate",
	}, "cohort_still_aligned")...)
}

func (signal *Signal) step(value *data.Value, trade kraken.TradeData) *data.Value {
	if signal.Status() != runtime.READY {
		return value
	}

	price := trade.Price.Float64()
	if price <= 0 {
		return value
	}

	atNano := float64(value.Timestamp)
	symbol := trade.Symbol

	untypedMem, _ := signal.members.LoadOrStore(symbol, &memberState{
		lastPrice:    price,
		lastChangeAt: atNano,
	})

	mem := untypedMem.(*memberState)

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
	signal.members.Range(func(memberSymbol, member any) bool {
		if len(member.(*memberState).spacings) > 0 {
			cadences = append(cadences, median(member.(*memberState).spacings))
		}
		return true
	})

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

	signal.members.Range(func(memberSymbol, untypedMember any) bool {
		member := untypedMember.(*memberState)

		atObs, hasAt := member.asOf(atNano)
		fromObs, hasFrom := member.asOf(fromNano)

		if !hasAt || !hasFrom || atObs.price <= 0 || fromObs.price <= 0 {
			return true
		}
		if atNano-atObs.at > float64(15*time.Minute) {
			return true
		}

		valid = append(valid, memberReturn{
			symbol: memberSymbol.(string),
			ret:    math.Log(atObs.price / fromObs.price),
		})
		return true
	})

	validCount := float64(len(valid))
	if validCount == 0 {
		// Nothing is computable yet: the tick carries no layer for it
		// rather than zeros that read as a measured value.
		return value
	}

	focalReturn := 0.0
	for _, validReturn := range valid {
		if validReturn.symbol == symbol {
			focalReturn = validReturn.ret
			break
		}
	}

	sameDirPeer := 0.0
	for _, validReturn := range valid {
		if validReturn.symbol == symbol || validReturn.ret == 0 {
			continue
		}
		if (focalReturn > 0 && validReturn.ret > 0) || (focalReturn < 0 && validReturn.ret < 0) {
			sameDirPeer++
		}
	}

	keys := []string{
		signal.Name(),
		trade.Symbol,
	}

	signal.lastAligned.Store(symbol, sameDirPeer)

	out := value.Append([]float64{
		sameDirPeer,
	}, append(keys, signal.keys...)...)

	audit.Record.Write(map[string]any{
		"signal":      signal.Name(),
		"trade":       trade,
		"sameDirPeer": sameDirPeer,
	})

	return out
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
