package sentiment

import (
	"context"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

type causalEstimator struct {
	count float64
	mean  float64
	m2    float64
}

/*
Step scores value against the estimator's state before it, then incorporates
it. The baseline (and residual against it) is defined from the second sample;
the z-score only once core.PriorScale admits the prior dispersion
(enough prior samples, a scale not negligible next to the values).
*/
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
	lastMeasurement *data.Measurement
	observations    []observation
	spacings        []float64
	lastPrice       float64
	lastChangeAt    float64
}

/*
asOf is the zero-order hold of the member's last observation at or before t.
*/
func (mem *memberState) asOf(t float64) (observation, bool) {
	for index := len(mem.observations) - 1; index >= 0; index-- {
		if mem.observations[index].at <= t {
			return mem.observations[index], true
		}
	}

	return observation{}, false
}

/*
retain keeps up to the most recent 256 observations for this member, matching
the retention capacity of other signals while supporting zero-order hold cuts.
*/
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

type Signal struct {
	*runtime.System
	mu               sync.Mutex
	members          map[string]*memberState
	breadthEstimator causalEstimator
	medianEstimator  causalEstimator
	hasPrevBreadth   bool
	prevBreadth      float64
	historyPoints    [][2]float64
	historyDistances []float64
}

func NewSignal(ctx context.Context) *Signal {
	signal := &Signal{
		members: make(map[string]*memberState),
	}

	signal.System = runtime.NewSystem(ctx, "sentiment", signal)
	return signal
}

func (signal *Signal) Step(prior *data.Measurement) *data.Measurement {
	if signal.Status() != runtime.READY || prior == nil || prior.Label == "" {
		return nil
	}

	priceEntry := data.Pull(prior.Read("price"))
	if priceEntry == nil || priceEntry.Metric == nil {
		return nil
	}

	price := priceEntry.Metric.Raw()
	if price <= 0 {
		return nil
	}

	atNano := float64(prior.At.UnixNano())
	symbol := prior.Label

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

	output := map[string]float64{}

	var cadences []float64
	for _, member := range signal.members {
		if len(member.spacings) > 0 {
			cadences = append(cadences, median(member.spacings))
		}
	}

	emit := func(from time.Time) *data.Measurement {
		if mem.lastMeasurement == nil {
			mem.lastMeasurement = data.NewMeasurement(prior.Epoch, prior.Label, signal.Name(), prior.At, prior.From)
		}

		res := mem.lastMeasurement.Next(prior, output)
		mem.lastMeasurement = res

		if !from.IsZero() {
			res.From = from
		}

		return res
	}

	if len(cadences) == 0 {
		return emit(time.Time{})
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
		symbol  string
		ret     float64
		fromAge float64
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
			symbol:  memberSymbol,
			ret:     math.Log(atObs.price / fromObs.price),
			fromAge: (fromNano - fromObs.at) * 1e-9,
		})
	}

	validCount := float64(len(valid))

	if validCount == 0 {
		return emit(time.Unix(0, int64(fromNano)).UTC())
	}

	advanceCount := 0.0
	declineCount := 0.0

	var returns []float64
	var absReturns []float64

	for _, validReturn := range valid {
		returns = append(returns, validReturn.ret)
		absReturns = append(absReturns, math.Abs(validReturn.ret))

		if validReturn.ret > 0 {
			advanceCount++
			continue
		}

		if validReturn.ret < 0 {
			declineCount++
		}
	}

	breadth := (advanceCount - declineCount) / validCount
	output["breadth"] = breadth

	slices.Sort(returns)
	slices.Sort(absReturns)

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
		if validReturn.symbol == symbol {
			continue
		}

		if validReturn.ret == 0 {
			continue
		}

		if (focalReturn > 0 && validReturn.ret > 0) || (focalReturn < 0 && validReturn.ret < 0) {
			sameDirPeer++
			continue
		}

		if (focalReturn > 0 && validReturn.ret < 0) || (focalReturn < 0 && validReturn.ret > 0) {
			oppDirPeer++
			continue
		}
	}

	output["same_direction_peer_count"] = sameDirPeer
	output["opposite_direction_peer_count"] = oppDirPeer

	_, _, _, hasBZ, bZScore := signal.breadthEstimator.Step(breadth)

	if signal.hasPrevBreadth {
		output["breadth_velocity"] = breadth - signal.prevBreadth
	}

	signal.prevBreadth = breadth
	signal.hasPrevBreadth = true

	_, _, _, hasMZ, mZScore := signal.medianEstimator.Step(medRet)

	if hasBZ && hasMZ {
		signal.historyPath(output, [2]float64{bZScore, mZScore})
	}

	return emit(time.Unix(0, int64(fromNano)).UTC())
}

/*
historyPath scores how far this cut's (breadth, median-return) z-score point
lies from the nearest retained one, and that distance's rank among past
nearest distances.
*/
func (signal *Signal) historyPath(out map[string]float64, target [2]float64) {
	if len(signal.historyPoints) > 0 {
		minDist := math.Inf(1)

		for _, point := range signal.historyPoints {
			dx := target[0] - point[0]
			dy := target[1] - point[1]
			minDist = math.Min(minDist, math.Sqrt(dx*dx+dy*dy))
		}

		out["historical_path_distance"] = minDist

		if len(signal.historyDistances) > 0 {
			below := 0

			for _, past := range signal.historyDistances {
				if past <= minDist {
					below++
				}
			}

			out["historical_path_percentile"] = float64(below) / float64(len(signal.historyDistances))
		}

		signal.historyDistances = append(signal.historyDistances, minDist)

		if len(signal.historyDistances) > 256 {
			signal.historyDistances = signal.historyDistances[len(signal.historyDistances)-256:]
		}
	}

	signal.historyPoints = append(signal.historyPoints, target)

	if len(signal.historyPoints) > 256 {
		signal.historyPoints = signal.historyPoints[len(signal.historyPoints)-256:]
	}
}
