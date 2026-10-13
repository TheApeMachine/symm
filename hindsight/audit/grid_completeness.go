package audit

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
)

/*
AnalyzeGridCompleteness replays every symbol-tick frame through Grid.Observe,
the production path, and reports the fraction it turns into regions (the
rest are still warming up or have a region without metrics) and the mean
number of metrics each region holds in the observed frames.
*/
func AnalyzeGridCompleteness(
	grid *store.Grid,
	ticks []int64,
	tickMeasurements map[int64][]*data.Measurement,
) GridCompletenessAudit {
	totalFrames := 0
	observedFrames := 0
	var members [store.RegionCount]float64

	for _, tick := range ticks {
		bySymbol := make(map[string][]*data.Measurement)

		for _, measurement := range tickMeasurements[tick] {
			bySymbol[measurement.Label] = append(bySymbol[measurement.Label], measurement)
		}

		for symbol, peers := range bySymbol {
			totalFrames++

			frame := data.NewMeasurement(peers[0].Epoch, symbol, "completeness", peers[0].At, peers[0].From)
			frame.SeqIdx = peers[0].SeqIdx
			frame.Tick = tick
			frame.Peers(peers...)
			frame.Write()

			regions := grid.Observe(frame)

			if regions == nil {
				continue
			}

			observedFrames++

			for _, region := range regions {
				members[region.Key] += float64(region.Counts)
			}
		}
	}

	if observedFrames == 0 {
		return GridCompletenessAudit{
			TotalFrames: totalFrames,
			SummaryText: fmt.Sprintf("Grid Completeness: none of %d frames observed.", totalFrames),
			Status:      VerdictInsufficient,
		}
	}

	meanMembers := make(map[string]float64, store.RegionCount)

	for region := range store.RegionCount {
		meanMembers[fmt.Sprintf("R%02d", region)] = members[region] / float64(observedFrames)
	}

	observedFraction := float64(observedFrames) / float64(totalFrames)

	return GridCompletenessAudit{
		TotalFrames:       totalFrames,
		ObservedFrames:    observedFrames,
		ObservedFraction:  observedFraction,
		MeanRegionMembers: meanMembers,
		SummaryText: fmt.Sprintf(
			"Grid Completeness: %d of %d frames observed (%.1f%%).",
			observedFrames, totalFrames, observedFraction*100,
		),
		Status: VerdictMeasured,
		Passed: true,
	}
}
