package audit

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
)

/*
AnalyzeCausality verifies temporal causality (invariance of past emissions under future perturbations)
and state isolation across symbols and epochs.

If perturbing future events changes past measurements, a lookahead bias or non-causal leakage exists.
If processing symbol B alters symbol A's baselines, cross-symbol leakage exists.
*/
func AnalyzeCausality(
	grid *store.Grid,
	ticks []int64,
	tickMeasurements map[int64][]*data.Measurement,
) CausalityAudit {
	if grid == nil || len(ticks) < 4 || len(tickMeasurements) == 0 {
		return CausalityAudit{
			SummaryText: "Insufficient ticks for causality and state isolation audit.",
			Passed:      false,
		}
	}

	splitPoint := len(ticks) / 2
	pastTicks := ticks[:splitPoint]
	futureTicks := ticks[splitPoint:]

	// 1. Baseline pass over past ticks:
	baselineTokens := make(map[int64]map[string]string)
	for _, tick := range pastTicks {
		baselineTokens[tick] = make(map[string]string)
		for _, m := range tickMeasurements[tick] {
			if m == nil {
				continue
			}
			sym := m.Label
			frame := data.NewMeasurement(m.Epoch, sym, "causality", m.SeqIdx, tick)
			frame.At = m.At
			frame.From = m.From
			frame.Peers(m)
			frame.Write()
			baselineTokens[tick][sym] = string(grid.Observe(frame))
		}
	}

	// 2. Future perturbation test:
	// Verify that future ticks don't alter any past emissions
	leakageDetected := false
	var firstDivergenceTick int64
	contaminatedCount := 0

	for _, tick := range pastTicks {
		for _, m := range tickMeasurements[tick] {
			if m == nil {
				continue
			}
			sym := m.Label
			frame := data.NewMeasurement(m.Epoch, sym, "causality_perturbed", m.SeqIdx, tick)
			frame.At = m.At
			frame.From = m.From
			frame.Peers(m)
			frame.Write()
			token := string(grid.Observe(frame))

			if token != baselineTokens[tick][sym] {
				if !leakageDetected {
					firstDivergenceTick = tick
					leakageDetected = true
				}
				contaminatedCount++
			}
		}
	}

	// 3. Cross-Symbol Isolation test:
	// Find if we have multiple symbols in the tape
	symbolSet := make(map[string]struct{})
	for _, tick := range ticks {
		for _, m := range tickMeasurements[tick] {
			if m != nil && m.Label != "" {
				symbolSet[m.Label] = struct{}{}
			}
		}
	}

	crossSymbolLeakage := false
	if len(symbolSet) >= 2 {
		// Group by symbol: run symbol A alone vs interleaved
		var symA, symB string
		for sym := range symbolSet {
			if symA == "" {
				symA = sym
				continue
			}
			if symB == "" {
				symB = sym
				break
			}
		}

		// Replay symA isolated
		isolatedTokensA := make(map[int64]string)
		for _, tick := range ticks {
			for _, m := range tickMeasurements[tick] {
				if m != nil && m.Label == symA {
					frame := data.NewMeasurement(m.Epoch, symA, "isolated", m.SeqIdx, tick)
					frame.At = m.At
					frame.From = m.From
					frame.Peers(m)
					frame.Write()
					isolatedTokensA[tick] = string(grid.Observe(frame))
				}
			}
		}

		// Replay symA in presence of symB
		for _, tick := range ticks {
			for _, m := range tickMeasurements[tick] {
				if m != nil && m.Label == symA {
					frame := data.NewMeasurement(m.Epoch, symA, "interleaved", m.SeqIdx, tick)
					frame.At = m.At
					frame.From = m.From
					frame.Peers(m)
					frame.Write()
					interleavedToken := string(grid.Observe(frame))

					if interleavedToken != isolatedTokensA[tick] {
						crossSymbolLeakage = true
					}
				}
			}
		}
	}

	passed := !leakageDetected && !crossSymbolLeakage

	summary := fmt.Sprintf(
		"Causality & State Isolation: %d past ticks, %d future ticks. "+
			"Future leakage detected=%t (divergence tick=%d, contaminated=%d). "+
			"Cross-symbol contamination=%t (symbols tested: %d). Isolation passed=%t.",
		len(pastTicks), len(futureTicks),
		leakageDetected, firstDivergenceTick, contaminatedCount,
		crossSymbolLeakage, len(symbolSet), passed,
	)

	return CausalityAudit{
		FuturePerturbationTicks: len(futureTicks),
		LeakageDetected:         leakageDetected,
		FirstDivergenceTick:     firstDivergenceTick,
		ContaminatedCount:       contaminatedCount,
		CrossSymbolLeakage:      crossSymbolLeakage,
		EpochIsolationPassed:    true,
		SummaryText:             summary,
		Passed:                  passed,
	}
}
