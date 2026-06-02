package integration

import (
	"fmt"
	"time"

	"github.com/theapemachine/symm/market/perspectives"
)

func stressScenarios() []Scenario {
	return []Scenario{
		{
			ID:   "black_swan.crash_recovery",
			Name: "System continues measuring through crash and wide spread",
			BuildCapture: func(builder *CaptureBuilder) {
				builder.AppendInstrumentCatalog()
				builder.AppendBlackSwanCrash()
			},
			SettleDelay: 1200 * time.Millisecond,
			Checks: []ScenarioCheck{
				{
					ID:   "black_swan.desk",
					Name: "Desk stays unhalted through crash replay",
					Evaluate: func(snapshot TapeSnapshot, _ error) (bool, string, map[string]any) {
						return snapshot.DeskReady, "desk remained ready", nil
					},
				},
				checkMeasurementSource("black_swan.fluid", "Fluid still publishes after crash",
					perspectives.SourceFluid, testSymbolPrimary),
				checkMeasurementSource("black_swan.cvd", "CVD still publishes after crash",
					perspectives.SourceCVD, testSymbolPrimary),
				checkCategoryObserved("black_swan.sentiment", "Sentiment classifies stress after crash",
					perspectives.SourceSentiment,
					perspectives.CategorySystemicSlump,
					perspectives.CategoryDivergentMove,
					perspectives.CategoryRiskOnSurge,
				),
				{
					ID:   "black_swan.spread",
					Name: "Wide-spread book frames were ingested",
					Evaluate: func(snapshot TapeSnapshot, _ error) (bool, string, map[string]any) {
						pass := snapshot.RawFrames >= 8

						return pass, fmt.Sprintf("raw_frames=%d", snapshot.RawFrames), nil
					},
				},
			},
		},
		{
			ID:   "black_swan.post_crash_entry_gate",
			Name: "Playbook still evaluates entries after crash without tripping halt",
			BuildCapture: func(builder *CaptureBuilder) {
				builder.AppendInstrumentCatalog()
				builder.AppendBlackSwanCrash()
				builder.AppendTicker(testSymbolPrimary, 60, 58, 62, -10)
				builder.AppendBookSnapshot(testSymbolPrimary, 58, 15, 62, 15)
			},
			DirectMeasurements: playbookLiquidityVacuumMeasurements(testSymbolPrimary, 60),
			SettleDelay:        1200 * time.Millisecond,
			Checks: []ScenarioCheck{
				{
					ID:   "black_swan.halt",
					Name: "Desk remains ready for post-crash playbook walk",
					Evaluate: func(snapshot TapeSnapshot, _ error) (bool, string, map[string]any) {
						return snapshot.DeskReady, "desk ready after crash", nil
					},
				},
				{
					ID:   "black_swan.playbook_audit",
					Name: "Playbook walk audit emitted after crash",
					Evaluate: func(snapshot TapeSnapshot, _ error) (bool, string, map[string]any) {
						for _, row := range snapshot.AuditRows {
							if row.AuditEvent == "playbook_walk" && row.Symbol == testSymbolPrimary {
								return true, "playbook_walk logged", map[string]any{
									"verdict":      row.Verdict,
									"block_reason": row.BlockReason,
								}
							}
						}

						return false, "no playbook_walk audit after crash", map[string]any{
							"audit_rows": len(snapshot.AuditRows),
						}
					},
				},
			},
		},
	}
}
