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

/*
emit writes name's baseline, divergence, and z-score as far as the estimator
defines them, and returns the z-score with its definedness.
*/
func (ce *causalEstimator) emit(out map[string]float64, name string, value float64) (float64, bool) {
	hasBaseline, baseline, residual, hasZ, zScore := ce.Step(value)

	if hasBaseline {
		out[name+"_baseline"] = baseline
		out[name+"_divergence"] = residual
	}

	if hasZ {
		out[name+"_zscore"] = zScore
	}

	return zScore, hasZ
}

type observation struct {
	at    float64
	price float64
}

type memberState struct {
	observations []observation
	spacings     []float64
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
	hasPrevMedian    bool
	prevMedian       float64
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

	price := priceEntry.Metric.Raw
	if price <= 0 {
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

	output := map[string]float64{
		"cohort_member_count": float64(len(signal.members)),
	}

	// The horizon is the cohort's median cadence; without one there is no
	// common horizon and no cross-section.
	var cadences []float64
	for _, m := range signal.members {
		if len(m.spacings) > 0 {
			cadences = append(cadences, median(m.spacings))
		}
	}

	if len(cadences) == 0 {
		return prior.Next(signal.Name(), output)
	}

	horizon := median(cadences)
	horizonNano := horizon * 1e9
	fromNano := atNano - horizonNano
	output["cohort_horizon_seconds"] = horizon

	type memberReturn struct {
		symbol  string
		ret     float64
		fromAge float64
	}
	var valid []memberReturn

	// Every member is cut at the same endpoints (T-H, T], each endpoint the
	// as-of price no older than H.
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

	cohortMemberCount := float64(len(signal.members))
	validCount := float64(len(valid))
	output["valid_member_count"] = validCount
	output["excluded_member_count"] = cohortMemberCount - validCount

	if validCount == 0 {
		res := prior.Next(signal.Name(), output)
		res.From = time.Unix(0, int64(fromNano)).UTC()
		return res
	}

	for _, vr := range valid {
		if vr.symbol == symbol {
			output["return"] = vr.ret
			output["absolute_return"] = math.Abs(vr.ret)
			output["from_age_seconds"] = vr.fromAge
			break
		}
	}

	advanceCount := 0.0
	declineCount := 0.0
	unchangedCount := 0.0

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
			continue
		}

		unchangedCount++
	}

	output["advance_count"] = advanceCount
	output["decline_count"] = declineCount
	output["unchanged_count"] = unchangedCount
	output["advance_fraction"] = advanceCount / validCount
	output["decline_fraction"] = declineCount / validCount
	output["unchanged_fraction"] = unchangedCount / validCount
	output["directional_participation"] = (advanceCount + declineCount) / validCount
	breadth := (advanceCount - declineCount) / validCount
	output["breadth"] = breadth

	if dirMoving := advanceCount + declineCount; dirMoving > 0 {
		output["directional_agreement"] = math.Max(advanceCount, declineCount) / dirMoving
		output["directional_consensus"] = math.Abs(advanceCount-declineCount) / dirMoving
	}

	slices.Sort(returns)
	slices.Sort(absReturns)

	medRet := returns[len(returns)/2]
	medAbsRet := absReturns[len(absReturns)/2]

	sumAbs := 0.0
	sumSq := 0.0

	for _, r := range returns {
		sumAbs += math.Abs(r)
		sumSq += r * r
	}

	var retDevs []float64
	var magDevs []float64

	for _, r := range returns {
		retDevs = append(retDevs, math.Abs(r-medRet))
		magDevs = append(magDevs, math.Abs(math.Abs(r)-medAbsRet))
	}

	slices.Sort(retDevs)
	slices.Sort(magDevs)

	output["median_return"] = medRet
	output["median_absolute_return"] = medAbsRet
	output["mean_absolute_return"] = sumAbs / validCount
	output["rms_return"] = math.Sqrt(sumSq / validCount)
	output["return_mad"] = retDevs[len(retDevs)/2]
	output["magnitude_mad"] = magDevs[len(magDevs)/2]
	output["return_interquartile_range"] = returns[int(float64(len(returns))*0.75)] -
		returns[int(float64(len(returns))*0.25)]

	largestAbs := absReturns[len(absReturns)-1]
	tieCount := 0.0
	largestSigned := 0.0
	largestSymbol := ""

	for _, vr := range valid {
		if math.Abs(vr.ret) == largestAbs {
			tieCount++
			largestSigned = vr.ret
			largestSymbol = vr.symbol
		}
	}

	output["largest_move_tie_count"] = tieCount
	output["largest_absolute_return"] = largestAbs
	output["largest_signed_return"] = largestSigned

	if sumAbs > 0 {
		output["largest_move_share"] = largestAbs / sumAbs
	}

	var peerAbsReturns []float64
	sameDirPeer := 0.0
	oppDirPeer := 0.0
	zeroDirPeer := 0.0

	for _, vr := range valid {
		if vr.symbol == largestSymbol {
			continue
		}

		peerAbsReturns = append(peerAbsReturns, math.Abs(vr.ret))

		if vr.ret == 0 {
			zeroDirPeer++
			continue
		}

		if (largestSigned > 0 && vr.ret > 0) || (largestSigned < 0 && vr.ret < 0) {
			sameDirPeer++
			continue
		}

		oppDirPeer++
	}

	if peerCount := float64(len(peerAbsReturns)); peerCount > 0 {
		slices.Sort(peerAbsReturns)
		peerMedAbs := peerAbsReturns[len(peerAbsReturns)/2]

		var peerDevs []float64
		for _, pr := range peerAbsReturns {
			peerDevs = append(peerDevs, math.Abs(pr-peerMedAbs))
		}
		slices.Sort(peerDevs)
		peerMagMAD := peerDevs[len(peerDevs)/2]
		largestExcess := largestAbs - peerMedAbs

		output["peer_median_absolute_return"] = peerMedAbs
		output["peer_magnitude_mad"] = peerMagMAD
		output["largest_move_excess"] = largestExcess
		output["same_direction_peer_count"] = sameDirPeer
		output["opposite_direction_peer_count"] = oppDirPeer
		output["zero_return_peer_count"] = zeroDirPeer
		output["same_direction_peer_fraction"] = sameDirPeer / peerCount
		output["opposite_direction_peer_fraction"] = oppDirPeer / peerCount
		output["zero_return_peer_fraction"] = zeroDirPeer / peerCount

		if peerMedAbs > 0 {
			output["largest_move_ratio"] = largestAbs / peerMedAbs
		}

		if peerMagMAD > 0 {
			output["largest_move_mad_excess"] = largestExcess / peerMagMAD
		}
	}

	bZScore, hasBZ := signal.breadthEstimator.emit(output, "breadth", breadth)

	if signal.hasPrevBreadth {
		output["breadth_velocity"] = breadth - signal.prevBreadth
	}

	signal.prevBreadth = breadth
	signal.hasPrevBreadth = true

	mZScore, hasMZ := signal.medianEstimator.emit(output, "median_return", medRet)

	if signal.hasPrevMedian {
		output["median_return_velocity"] = medRet - signal.prevMedian
	}

	signal.prevMedian = medRet
	signal.hasPrevMedian = true

	if hasBZ && hasMZ {
		signal.historyPath(output, [2]float64{bZScore, mZScore})
	}

	res := prior.Next(signal.Name(), output)
	res.From = time.Unix(0, int64(fromNano)).UTC()

	return res
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
