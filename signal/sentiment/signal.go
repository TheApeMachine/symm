package sentiment

import (
	"context"
	"math"
	"slices"
	"sync"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

var outputKeys = []string{
	"cohort_member_count",
	"valid_member_count",
	"excluded_member_count",
	"cohort_horizon_seconds",
	"return",
	"absolute_return",
	"asof_age_seconds",
	"from_age_seconds",
	"advance_count",
	"decline_count",
	"unchanged_count",
	"advance_fraction",
	"decline_fraction",
	"unchanged_fraction",
	"directional_participation",
	"breadth",
	"directional_agreement",
	"directional_consensus",
	"median_return",
	"median_absolute_return",
	"mean_absolute_return",
	"rms_return",
	"return_mad",
	"magnitude_mad",
	"return_interquartile_range",
	"largest_move_tie_count",
	"largest_absolute_return",
	"largest_signed_return",
	"largest_move_share",
	"peer_median_absolute_return",
	"peer_magnitude_mad",
	"largest_move_excess",
	"largest_move_ratio",
	"largest_move_mad_excess",
	"same_direction_peer_count",
	"opposite_direction_peer_count",
	"zero_return_peer_count",
	"same_direction_peer_fraction",
	"opposite_direction_peer_fraction",
	"zero_return_peer_fraction",
	"breadth_baseline",
	"breadth_divergence",
	"breadth_zscore",
	"median_return_baseline",
	"median_return_divergence",
	"median_return_zscore",
	"median_return_velocity",
	"breadth_velocity",
	"historical_path_distance",
	"historical_path_percentile",
}

type causalEstimator struct {
	count float64
	mean  float64
	m2    float64
}

func (ce *causalEstimator) Step(value float64) (bool, float64, float64, float64, float64) {
	priorCount := ce.count
	priorMean := ce.mean
	priorM2 := ce.m2

	ce.count++
	delta := value - ce.mean
	ce.mean += delta / ce.count
	ce.m2 += delta * (value - ce.mean)

	baseline := value
	hasPrior := false
	if priorCount > 0 {
		hasPrior = true
		baseline = priorMean
	}

	var priorVar float64
	if priorCount > 1 {
		priorVar = priorM2 / (priorCount - 1)
	}

	residual := value - baseline
	scale := math.Abs(residual)
	var dispersion float64

	if priorVar > 0 {
		d := math.Sqrt(priorVar)
		if d > 2.220446049250313e-16 {
			scale = d
			dispersion = d
		}
	}

	var zScore float64
	if scale > 0 {
		zScore = residual / scale
	}

	return hasPrior, baseline, dispersion, residual, zScore
}

type memberState struct {
	lastPrice float64
	lastAt    float64
	prevPrice float64
	prevAt    float64
	spacings  []float64
	hasPrev   bool
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

	if mem.hasPrev && atNano > mem.lastAt {
		spacing := (atNano - mem.lastAt) * 1e-9
		mem.spacings = append(mem.spacings, spacing)
		if len(mem.spacings) > 128 {
			mem.spacings = mem.spacings[len(mem.spacings)-128:]
		}
		mem.prevPrice = mem.lastPrice
		mem.prevAt = mem.lastAt
	}
	mem.lastPrice = price
	mem.lastAt = atNano
	mem.hasPrev = true

	var cadences []float64
	for _, m := range signal.members {
		if len(m.spacings) > 0 {
			sorted := slices.Clone(m.spacings)
			slices.Sort(sorted)
			med := sorted[len(sorted)/2]
			cadences = append(cadences, med)
		}
	}

	horizon := 0.1
	if len(cadences) > 0 {
		slices.Sort(cadences)
		horizon = cadences[len(cadences)/2]
	}

	type memberReturn struct {
		symbol  string
		ret     float64
		asofAge float64
		fromAge float64
	}
	var valid []memberReturn

	for s, m := range signal.members {
		if m.prevPrice > 0 && m.lastPrice > 0 && m.prevAt < m.lastAt {
			ret := math.Log(m.lastPrice / m.prevPrice)
			asofAge := (atNano - m.lastAt) * 1e-9
			fromAge := (atNano - m.prevAt) * 1e-9
			valid = append(valid, memberReturn{
				symbol:  s,
				ret:     ret,
				asofAge: asofAge,
				fromAge: fromAge,
			})
		}
	}

	cohortMemberCount := float64(len(signal.members))
	validCount := float64(len(valid))
	excludedCount := cohortMemberCount - validCount

	output := make(map[string]float64, len(outputKeys))

	for _, key := range outputKeys {
		output[key] = 0.0
	}

	if validCount == 0 {
		return prior.Next(signal.Name(), output)
	}

	focalRet := 0.0
	focalAsofAge := 0.0
	focalFromAge := 0.0

	for _, vr := range valid {
		if vr.symbol == symbol {
			focalRet = vr.ret
			focalAsofAge = vr.asofAge
			focalFromAge = vr.fromAge
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

	advFrac := advanceCount / validCount
	decFrac := declineCount / validCount
	unchFrac := unchangedCount / validCount
	dirPart := (advanceCount + declineCount) / validCount
	breadth := (advanceCount - declineCount) / validCount

	dirMoving := advanceCount + declineCount
	dirAgree := 0.0
	dirConsensus := 0.0

	if dirMoving > 0 {
		dirAgree = math.Max(advanceCount, declineCount) / dirMoving
		dirConsensus = math.Abs(advanceCount-declineCount) / dirMoving
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

	meanAbsRet := sumAbs / validCount
	rmsRet := math.Sqrt(sumSq / validCount)

	var retDevs []float64
	var magDevs []float64

	for _, r := range returns {
		retDevs = append(retDevs, math.Abs(r-medRet))
		magDevs = append(magDevs, math.Abs(math.Abs(r)-medAbsRet))
	}

	slices.Sort(retDevs)
	slices.Sort(magDevs)
	retMAD := retDevs[len(retDevs)/2]
	magMAD := magDevs[len(magDevs)/2]

	q25 := returns[int(float64(len(returns))*0.25)]
	q75 := returns[int(float64(len(returns))*0.75)]
	iqr := q75 - q25

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

	largestShare := 0.0

	if sumAbs > 0 {
		largestShare = largestAbs / sumAbs
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

	peerCount := float64(len(peerAbsReturns))
	peerMedAbs := 0.0
	peerMagMAD := 0.0
	largestExcess := 0.0
	largestRatio := 1.0
	largestMADExcess := 0.0
	sameDirFrac := 0.0
	oppDirFrac := 0.0
	zeroDirFrac := 0.0

	if peerCount > 0 {
		slices.Sort(peerAbsReturns)
		peerMedAbs = peerAbsReturns[len(peerAbsReturns)/2]

		var peerDevs []float64
		for _, pr := range peerAbsReturns {
			peerDevs = append(peerDevs, math.Abs(pr-peerMedAbs))
		}
		slices.Sort(peerDevs)
		peerMagMAD = peerDevs[len(peerDevs)/2]

		largestExcess = largestAbs - peerMedAbs

		if peerMedAbs > 0 {
			largestRatio = largestAbs / peerMedAbs
		}

		if peerMagMAD > 0 {
			largestMADExcess = largestExcess / peerMagMAD
		}

		sameDirFrac = sameDirPeer / peerCount
		oppDirFrac = oppDirPeer / peerCount
		zeroDirFrac = zeroDirPeer / peerCount
	}

	bBaseline := 0.0
	bDiv := 0.0
	bZScore := 0.0
	hasBPrior, bb, _, bRes, bZ := signal.breadthEstimator.Step(breadth)

	if hasBPrior {
		bBaseline = bb
		bDiv = bRes
		bZScore = bZ
	}

	bVelocity := 0.0

	if signal.hasPrevBreadth {
		bVelocity = breadth - signal.prevBreadth
	}

	signal.prevBreadth = breadth
	signal.hasPrevBreadth = true

	mBaseline := 0.0
	mDiv := 0.0
	mZScore := 0.0
	hasMPrior, mb, _, mRes, mZ := signal.medianEstimator.Step(medRet)

	if hasMPrior {
		mBaseline = mb
		mDiv = mRes
		mZScore = mZ
	}

	mVelocity := 0.0

	if signal.hasPrevMedian {
		mVelocity = medRet - signal.prevMedian
	}

	signal.prevMedian = medRet
	signal.hasPrevMedian = true

	target := [2]float64{bZScore, mZScore}
	histDist := 0.0
	histPerc := 0.0

	if len(signal.historyPoints) > 0 {
		diffX := target[0] - signal.historyPoints[0][0]
		diffY := target[1] - signal.historyPoints[0][1]
		minDist := math.Sqrt(diffX*diffX + diffY*diffY)

		for i := 1; i < len(signal.historyPoints); i++ {
			dx := target[0] - signal.historyPoints[i][0]
			dy := target[1] - signal.historyPoints[i][1]
			d := math.Sqrt(dx*dx + dy*dy)

			if d < minDist {
				minDist = d
			}
		}

		if len(signal.historyDistances) > 0 {
			below := 0

			for _, pd := range signal.historyDistances {
				if pd <= minDist {
					below++
				}
			}

			histPerc = float64(below) / float64(len(signal.historyDistances))
		}

		histDist = minDist
		signal.historyDistances = append(signal.historyDistances, minDist)

		if len(signal.historyDistances) > 256 {
			signal.historyDistances = signal.historyDistances[len(signal.historyDistances)-256:]
		}
	}

	signal.historyPoints = append(signal.historyPoints, target)

	if len(signal.historyPoints) > 256 {
		signal.historyPoints = signal.historyPoints[len(signal.historyPoints)-256:]
	}

	return prior.Next(signal.Name(), map[string]float64{
		"cohort_member_count":              cohortMemberCount,
		"valid_member_count":               validCount,
		"excluded_member_count":            excludedCount,
		"cohort_horizon_seconds":           horizon,
		"return":                           focalRet,
		"absolute_return":                  math.Abs(focalRet),
		"asof_age_seconds":                 focalAsofAge,
		"from_age_seconds":                 focalFromAge,
		"advance_count":                    advanceCount,
		"decline_count":                    declineCount,
		"unchanged_count":                  unchangedCount,
		"advance_fraction":                 advFrac,
		"decline_fraction":                 decFrac,
		"unchanged_fraction":               unchFrac,
		"directional_participation":        dirPart,
		"breadth":                          breadth,
		"directional_agreement":            dirAgree,
		"directional_conse	nsus":           dirConsensus,
		"median_return":                    medRet,
		"median_absolute_return":           medAbsRet,
		"mean_absolute_return":             meanAbsRet,
		"rms_return":                       rmsRet,
		"return_mad":                       retMAD,
		"magnitude_mad":                    magMAD,
		"return_interquartile_range":       iqr,
		"largest_move_tie_count":           tieCount,
		"largest_absolute_return":          largestAbs,
		"largest_signed_return":            largestSigned,
		"largest_move_share":               largestShare,
		"peer_median_absolute_return":      peerMedAbs,
		"peer_magnitude_mad":               peerMagMAD,
		"largest_move_excess":              largestExcess,
		"largest_move_ratio":               largestRatio,
		"largest_move_mad_excess":          largestMADExcess,
		"same_direction_peer_count":        sameDirPeer,
		"opposite_direction_peer_count":    oppDirPeer,
		"zero_return_peer_count":           zeroDirPeer,
		"same_direction_peer_fraction":     sameDirFrac,
		"opposite_direction_peer_fraction": oppDirFrac,
		"zero_return_peer_fraction":        zeroDirFrac,
		"breadth_baseline":                 bBaseline,
		"breadth_divergence":               bDiv,
		"breadth_zscore":                   bZScore,
		"median_return_baseline":           mBaseline,
		"median_return_divergence":         mDiv,
		"median_return_zscore":             mZScore,
		"median_return_velocity":           mVelocity,
		"breadth_velocity":                 bVelocity,
		"historical_path_distance":         histDist,
		"historical_path_percentile":       histPerc,
	})
}
