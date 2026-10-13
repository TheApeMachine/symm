package audit

import (
	"fmt"
	"math"
	"math/rand"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
)

/*
AnalyzeGridStability compares how often each region is the brightest
(through Grid.Observe, the production path) in the first and second half of
the tape, with a null that reassigns ticks to the two halves at random.
Stationarity of the market is an observation, not a property the pipeline
must have, so the stage claims nothing.
*/
func AnalyzeGridStability(
	grid *store.Grid,
	ticks []int64,
	tickMeasurements map[int64][]*data.Measurement,
	permutations int,
	significance float64,
) Stage3GridStability {
	if len(ticks) < 2 {
		return Stage3GridStability{
			SummaryText: "Fewer than two ticks: no halves to compare.",
			Status:      VerdictInsufficient,
		}
	}

	if permutations <= 0 {
		return Stage3GridStability{
			SummaryText: "permutations must be strictly positive",
			Status:      "INVALID_PARAMETER",
		}
	}

	errnie.Info("[audit] [5/7 Stage 3] Comparing region excitation of the two tape halves...")
	half := len(ticks) / 2
	regionsA := tickRegions(grid, ticks[:half], tickMeasurements)
	regionsB := tickRegions(grid, ticks[half:], tickMeasurements)
	jsd, tvd := computeDistributionDivergence(regionDistribution(regionsA), regionDistribution(regionsB))

	pool := append(append([][]uint8(nil), regionsA...), regionsB...)
	rng := rand.New(rand.NewSource(1791))
	nullJSD := make([]float64, 0, permutations)

	for range permutations {
		rng.Shuffle(len(pool), func(left, right int) { pool[left], pool[right] = pool[right], pool[left] })
		value, _ := computeDistributionDivergence(
			regionDistribution(pool[:len(regionsA)]), regionDistribution(pool[len(regionsA):]),
		)
		nullJSD = append(nullJSD, value)
	}

	pValue := upperPValue(jsd, nullJSD)

	return Stage3GridStability{
		DistributionJSD:    jsd,
		DistributionTVD:    tvd,
		StationarityPValue: pValue,
		IsStationary:       pValue > significance,
		SummaryText: fmt.Sprintf(
			"Cross-half excitation stationarity: JSD=%.3f bits, TVD=%.3f, p=%.3f vs random tick reassignment (stationary: %t).",
			jsd, tvd, pValue, pValue > significance,
		),
		Status: VerdictMeasured,
	}
}

/*
tickRegions returns, per tick, the regions the grid lit for each symbol's
frame at that tick, so distributions over any subset of ticks can be formed
without replaying the grid again.
*/
func tickRegions(
	grid *store.Grid,
	ticks []int64,
	tickMeasurements map[int64][]*data.Measurement,
) [][]uint8 {
	result := make([][]uint8, 0, len(ticks))

	for _, tick := range ticks {
		var lit []uint8
		bySymbol := make(map[string][]*data.Measurement)

		for _, m := range tickMeasurements[tick] {
			if m != nil {
				bySymbol[m.Label] = append(bySymbol[m.Label], m)
			}
		}

		for sym, symMeas := range bySymbol {
			frame := data.NewMeasurement(symMeas[0].Epoch, sym, "stability", symMeas[0].At, symMeas[0].From)
			frame.SeqIdx = symMeas[0].SeqIdx
			frame.Tick = tick
			frame.Peers(symMeas...)
			frame.Write()

			if regions := grid.Observe(frame); regions != nil {
				lit = append(lit, regions[0].Key)
			}
		}

		result = append(result, lit)
	}

	return result
}

/*
regionDistribution is the normalized region histogram of the given ticks.
*/
func regionDistribution(regions [][]uint8) [12]float64 {
	var counts [12]float64
	total := 0.0

	for _, lit := range regions {
		for _, reg := range lit {
			if reg < 12 {
				counts[reg]++
				total++
			}
		}
	}

	if total > 0 {
		for r := range 12 {
			counts[r] /= total
		}
	}

	return counts
}

func computeDistributionDivergence(distA, distB [12]float64) (float64, float64) {
	jsd := 0.0
	tvd := 0.0

	for r := range 12 {
		p := distA[r]
		q := distB[r]
		tvd += math.Abs(p - q)

		m := 0.5 * (p + q)
		if m > 0 {
			if p > 0 {
				jsd += 0.5 * p * math.Log2(p/m)
			}
			if q > 0 {
				jsd += 0.5 * q * math.Log2(q/m)
			}
		}
	}

	tvd *= 0.5
	return jsd, tvd
}
