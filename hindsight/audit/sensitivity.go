package audit

import (
	"fmt"
	"slices"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
)

/*
AnalyzeGridSensitivity tests the grid's dependence on individual signal families.
It evaluates:
 1. Leave-One-Family-Out (LOFO): the region-distribution shift (JSD, bits)
    when each signal family is removed; above thresholds.DominanceJSD the
    family dominates the grid.
 2. Duplication: the largest shift caused by duplicating any one family;
    above thresholds.DuplicationJSD the grid is not duplication resistant.

Both limits are named configuration, not derived from a null: there is no
sampling distribution for "how much one family may move the grid", so the
report states the values it used.
*/
func AnalyzeGridSensitivity(
	grid *store.Grid,
	ticks []int64,
	tickMeasurements map[int64][]*data.Measurement,
	thresholds Thresholds,
) SensitivityAudit {
	if grid == nil || len(ticks) == 0 || len(tickMeasurements) == 0 {
		return SensitivityAudit{
			SummaryText: "Insufficient data for grid sensitivity audit.",
			Status:      VerdictInsufficient,
		}
	}

	familySet := make(map[string]struct{})
	for _, tick := range ticks {
		for _, m := range tickMeasurements[tick] {
			if m != nil && m.Source != "" {
				familySet[m.Source] = struct{}{}
			}
		}
	}

	families := make([]string, 0, len(familySet))
	for f := range familySet {
		families = append(families, f)
	}

	slices.Sort(families)

	baseDist := computeSampleRegionDistribution(grid, ticks, tickMeasurements, "")
	testedStats := make([]FamilySensitivityStat, 0, len(families))
	maxDuplicationJSD := 0.0
	duplicatedFamily := ""

	for _, fam := range families {
		lofoDist := computeSampleRegionDistribution(grid, ticks, tickMeasurements, fam)
		jsd, _ := computeDistributionDivergence(baseDist, lofoDist)
		dupDist := computeDuplicatedRegionDistribution(grid, ticks, tickMeasurements, fam)
		dupJSD, _ := computeDistributionDivergence(baseDist, dupDist)

		if dupJSD > maxDuplicationJSD || duplicatedFamily == "" {
			maxDuplicationJSD, duplicatedFamily = dupJSD, fam
		}

		testedStats = append(testedStats, FamilySensitivityStat{
			Family:        fam,
			RemovedJSD:    jsd,
			DuplicatedJSD: dupJSD,
			IsDominant:    jsd > thresholds.DominanceJSD,
		})
	}

	return sensitivityReport(testedStats, regionMass(baseDist), maxDuplicationJSD, duplicatedFamily, thresholds)
}

/*
regionMass is 1 when the grid lit any region on the sample, 0 otherwise.
*/
func regionMass(dist [13]float64) float64 {
	total := 0.0

	for _, value := range dist {
		total += value
	}

	return total
}

/*
sensitivityReport turns the per-family shifts into the verdict. A sample on
which the grid lit nothing cannot show dependence either way.
*/
func sensitivityReport(
	stats []FamilySensitivityStat,
	baseMass float64,
	maxDuplicationJSD float64,
	duplicatedFamily string,
	thresholds Thresholds,
) SensitivityAudit {
	dominant := 0

	for _, stat := range stats {
		if stat.IsDominant {
			dominant++
		}
	}

	resistant := maxDuplicationJSD <= thresholds.DuplicationJSD
	verdict := VerdictSupported

	switch {
	case len(stats) < 2 || baseMass == 0:
		verdict = VerdictInsufficient
	case dominant > 0 || !resistant:
		verdict = VerdictNotSupported
	}

	return SensitivityAudit{
		FamiliesTested:       stats,
		DominantFamilies:     dominant,
		MaxDuplicationJSD:    maxDuplicationJSD,
		MaxDuplicationFamily: duplicatedFamily,
		DuplicationResistant: resistant,
		Status:               verdict,
		Passed:               passed(verdict),
		SummaryText: fmt.Sprintf(
			"Grid Sensitivity: %d families evaluated. Dominant (removal shifts regions by > %.2f bits): %d. "+
				"Largest duplication shift %.3f bits (%s; limit %.2f). Verdict %s.",
			len(stats), thresholds.DominanceJSD, dominant, maxDuplicationJSD, duplicatedFamily,
			thresholds.DuplicationJSD, verdict,
		),
	}
}

func computeSampleRegionDistribution(
	grid *store.Grid,
	ticks []int64,
	tickMeasurements map[int64][]*data.Measurement,
	excludeFamily string,
) [13]float64 {
	var counts [13]float64
	total := 0.0

	for _, tick := range ticks {
		measGroup := tickMeasurements[tick]
		if len(measGroup) == 0 {
			continue
		}

		bySymbol := make(map[string][]*data.Measurement)
		for _, m := range measGroup {
			if m != nil {
				if excludeFamily != "" && m.Source == excludeFamily {
					continue
				}
				bySymbol[m.Label] = append(bySymbol[m.Label], m)
			}
		}

		for sym, symMeas := range bySymbol {
			if len(symMeas) == 0 {
				continue
			}
			frame := data.NewMeasurement(
				symMeas[0].Epoch,
				sym,
				"sensitivity",
				symMeas[0].SeqIdx,
				tick,
			)
			frame.At = symMeas[0].At
			frame.From = symMeas[0].From
			frame.Peers(symMeas...)
			frame.Write()
			tokenBytes := grid.Observe(frame)
			if len(tokenBytes) >= 3 && tokenBytes[0] == 'R' {
				var reg uint8
				if tokenBytes[1] == '0' {
					reg = tokenBytes[2] - '0'
				} else {
					reg = 10 + (tokenBytes[2] - '0')
				}
				if reg >= 1 && reg <= 12 {
					counts[reg]++
					total++
				}
			}
		}
	}

	if total > 0 {
		for r := 1; r <= 12; r++ {
			counts[r] /= total
		}
	}

	return counts
}

func computeDuplicatedRegionDistribution(
	grid *store.Grid,
	ticks []int64,
	tickMeasurements map[int64][]*data.Measurement,
	duplicateFamily string,
) [13]float64 {
	var counts [13]float64
	total := 0.0

	for _, tick := range ticks {
		measGroup := tickMeasurements[tick]
		if len(measGroup) == 0 {
			continue
		}

		bySymbol := make(map[string][]*data.Measurement)
		for _, m := range measGroup {
			if m != nil {
				bySymbol[m.Label] = append(bySymbol[m.Label], m)
				if m.Source == duplicateFamily {
					// Duplicate this measurement to simulate metric spam
					bySymbol[m.Label] = append(bySymbol[m.Label], m)
				}
			}
		}

		for sym, symMeas := range bySymbol {
			if len(symMeas) == 0 {
				continue
			}
			frame := data.NewMeasurement(
				symMeas[0].Epoch,
				sym,
				"sensitivity_dup",
				symMeas[0].SeqIdx,
				tick,
			)
			frame.At = symMeas[0].At
			frame.From = symMeas[0].From
			frame.Peers(symMeas...)
			frame.Write()
			tokenBytes := grid.Observe(frame)
			if len(tokenBytes) >= 3 && tokenBytes[0] == 'R' {
				var reg uint8
				if tokenBytes[1] == '0' {
					reg = tokenBytes[2] - '0'
				} else {
					reg = 10 + (tokenBytes[2] - '0')
				}
				if reg >= 1 && reg <= 12 {
					counts[reg]++
					total++
				}
			}
		}
	}

	if total > 0 {
		for r := 1; r <= 12; r++ {
			counts[r] /= total
		}
	}

	return counts
}
