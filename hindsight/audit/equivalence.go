package audit

import (
	"context"
	"fmt"
	"math"
	"os"
	"runtime"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
)

/*
AnalyzeEquivalence audits the audit itself. It replays identical market measurements
through both the production runtime path (Grid.Observe, peer assembly, and state transitions)
and the audit inspection path, verifying bit-for-bit fidelity.

Any discrepancy between production and audit constitutes a measurement fidelity breach.

This is NOT_A_TEST: both "paths" build the same frame from the same stored
measurements and call the same Grid.Observe and Grid.RegionScores, so they can
only differ if those functions are nondeterministic. Its mismatch counts are
kept as a determinism observation, not reported as a pass.
*/
func AnalyzeEquivalence(
	ctx context.Context,
	grid *store.Grid,
	ticks []int64,
	tickMeasurements map[int64][]*data.Measurement,
) EquivalenceAudit {
	if grid == nil || len(ticks) == 0 || len(tickMeasurements) == 0 {
		return EquivalenceAudit{
			SummaryText: "Insufficient data for production-vs-audit equivalence audit.",
			Passed:      false,
		}
	}

	execPath, err := os.Executable()
	if err != nil {
		execPath = "unknown"
	}

	totalTokensChecked := 0
	tokenMismatches := 0
	metricMismatches := 0
	discrepancies := make([]EquivalenceDiscrepancy, 0)

	for _, tick := range ticks {
		select {
		case <-ctx.Done():
		default:
		}

		measGroup := tickMeasurements[tick]
		if len(measGroup) == 0 {
			continue
		}

		bySymbol := make(map[string][]*data.Measurement)
		for _, m := range measGroup {
			if m != nil {
				bySymbol[m.Label] = append(bySymbol[m.Label], m)
			}
		}

		for sym, symMeas := range bySymbol {
			if len(symMeas) == 0 {
				continue
			}

			// Production execution path:
			prodFrame := data.NewMeasurement(
				symMeas[0].Epoch,
				sym,
				"prod_equivalence",
				symMeas[0].SeqIdx,
				tick,
			)
			prodFrame.At = symMeas[0].At
			prodFrame.From = symMeas[0].From
			prodFrame.Peers(symMeas...)
			prodFrame.Write()
			prodToken := string(grid.Observe(prodFrame))
			prodBrightness := grid.RegionScores(prodFrame).Brightness

			// Audit inspection path (simulating independent re-evaluation):
			auditFrame := data.NewMeasurement(
				symMeas[0].Epoch,
				sym,
				"audit_equivalence",
				symMeas[0].SeqIdx,
				tick,
			)
			auditFrame.At = symMeas[0].At
			auditFrame.From = symMeas[0].From
			auditFrame.Peers(symMeas...)
			auditFrame.Write()
			auditToken := string(grid.Observe(auditFrame))
			auditBrightness := grid.RegionScores(auditFrame).Brightness

			totalTokensChecked++

			if prodToken != auditToken {
				tokenMismatches++
				discrepancies = append(discrepancies, EquivalenceDiscrepancy{
					Tick:       tick,
					Symbol:     sym,
					Source:     "grid",
					Metric:     "winning_token",
					AuditVal:   0,
					ProdVal:    0,
					Difference: 1,
				})
			}

			for reg := uint8(1); reg <= 12; reg++ {
				diff := math.Abs(prodBrightness[reg] - auditBrightness[reg])
				if diff > 1e-9 {
					metricMismatches++
					discrepancies = append(discrepancies, EquivalenceDiscrepancy{
						Tick:       tick,
						Symbol:     sym,
						Source:     "grid",
						Metric:     fmt.Sprintf("region_%02d_brightness", reg),
						AuditVal:   auditBrightness[reg],
						ProdVal:    prodBrightness[reg],
						Difference: diff,
					})
				}
			}
		}
	}

	deterministic := tokenMismatches == 0 && metricMismatches == 0

	summary := fmt.Sprintf(
		"NOT_A_TEST (one code path compared with itself). Determinism replay: %d ticks replayed (%d tokens verified across %s runtime). "+
			"Token mismatches=%d, Metric/brightness mismatches=%d. Divergence=%t.",
		len(ticks), totalTokensChecked, runtime.Version(),
		tokenMismatches, metricMismatches, !deterministic,
	)

	return EquivalenceAudit{
		ExecutablePath:     execPath,
		TotalTicksReplayed: len(ticks),
		TotalTokensChecked: totalTokensChecked,
		TokenMismatches:    tokenMismatches,
		MetricMismatches:   metricMismatches,
		Discrepancies:      discrepancies,
		SummaryText:        summary,
		Deterministic:      deterministic,
		Status:             VerdictNotATest,
		Passed:             false,
	}
}
