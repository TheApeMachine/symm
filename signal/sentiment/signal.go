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
retain drops observations older than the oldest one a cut at t with horizon
h could still validate: a start endpoint must be no older than t - 2h.
*/
func (mem *memberState) retain(oldest float64) {
	keep := 0

	for keep < len(mem.observations) && mem.observations[keep].at < oldest {
		keep++
	}

	mem.observations = mem.observations[keep:]
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
		signal.mu.Lock()
		defer signal.mu.Unlock()

		if mem, found := signal.members[prior.Label]; found && mem.lastMeasurement != nil {
			return mem.lastMeasurement
		}

		return nil
	}

	price := priceEntry.Metric.Raw()
	if price <= 0 {
		signal.mu.Lock()
		defer signal.mu.Unlock()

		if mem, found := signal.members[prior.Label]; found && mem.lastMeasurement != nil {
			return mem.lastMeasurement
		}

		return nil
	}

	atNano := float64(prior.At.UnixNano())
	symbol := prior.Label

	signal.mu.Lock()
	defer signal.mu.Unlock()

	mem, found := signal.members[symbol]
	if !found {
		mem = &memberState{}
		signal.members[symbol] = mem
	}

	if last := len(mem.observations) - 1; last >= 0 && atNano > mem.observations[last].at {
		mem.spacings = append(mem.spacings, (atNano-mem.observations[last].at)*1e-9)
		if len(mem.spacings) > 128 {
			mem.spacings = mem.spacings[len(mem.spacings)-128:]
		}
	}

	mem.observations = append(mem.observations, observation{at: atNano, price: price})

	output := map[string]float64{}

	var cadences []float64
	for _, m := range signal.members {
		if len(m.spacings) > 0 {
			cadences = append(cadences, median(m.spacings))
		}
	}

	emit := func(from time.Time) *data.Measurement {
		var res *data.Measurement

		if mem.lastMeasurement == nil {
			res = data.NewFromPrior(prior, signal.Name(), output)
		}

		if res == nil {
			res = mem.lastMeasurement.Next(output)
		}

		res.At = prior.At
		mem.lastMeasurement = res

		if !from.IsZero() {
			res.From = from
		}

		return res
	}

	if len(cadences) == 0 {
		return emit(time.Time{})
	}

	horizon := median(cadences)
	horizonNano := horizon * 1e9
	fromNano := atNano - horizonNano

	type memberReturn struct {
		symbol  string
		ret     float64
		fromAge float64
	}
	var valid []memberReturn

	for s, m := range signal.members {
		m.retain(fromNano - horizonNano)

		atObs, hasAt := m.asOf(atNano)
		fromObs, hasFrom := m.asOf(fromNano)

		if !hasAt || !hasFrom || atNano-atObs.at > horizonNano || fromNano-fromObs.at > horizonNano {
			continue
		}

		valid = append(valid, memberReturn{
			symbol:  s,
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

	for _, vr := range valid {
		returns = append(returns, vr.ret)
		absReturns = append(absReturns, math.Abs(vr.ret))

		if vr.ret > 0 {
			advanceCount++
			continue
		}

		if vr.ret < 0 {
			declineCount++
		}
	}

	breadth := (advanceCount - declineCount) / validCount
	output["breadth"] = breadth

	slices.Sort(returns)
	slices.Sort(absReturns)

	medRet := returns[len(returns)/2]

	largestAbs := absReturns[len(absReturns)-1]
	largestSigned := 0.0
	largestSymbol := ""

	for _, vr := range valid {
		if math.Abs(vr.ret) == largestAbs {
			largestSigned = vr.ret
			largestSymbol = vr.symbol
			break
		}
	}

	sameDirPeer := 0.0
	oppDirPeer := 0.0

	for _, vr := range valid {
		if vr.symbol == largestSymbol {
			continue
		}

		if vr.ret == 0 {
			continue
		}

		if (largestSigned > 0 && vr.ret > 0) || (largestSigned < 0 && vr.ret < 0) {
			sameDirPeer++
			continue
		}

		oppDirPeer++
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
