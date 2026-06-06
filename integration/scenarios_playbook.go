package integration

import (
	"time"

	"github.com/theapemachine/symm/market/perspectives/reasoning"
)

func playbookScenarios() []Scenario {
	return []Scenario{
		{
			ID:   "playbook.limit_entry",
			Name: "Perspective tree publishes limit entry on raw",
			BuildCapture: func(builder *CaptureBuilder) {
				builder.AppendInstrumentCatalog()
				builder.AppendTicker(testSymbolPrimary, 50_000, 49_990, 50_010, 0)
				builder.AppendBookSnapshot(testSymbolPrimary, 49_990, 10, 50_010, 10)
			},
			DirectMeasurements: playbookLiquidityVacuumMeasurements(testSymbolPrimary, 50_000),
			SettleDelay:        600 * time.Millisecond,
			Checks: []ScenarioCheck{
				checkActionType("playbook.action", "Story publishes limit entry action",
					reasoning.ActionLimit, testSymbolPrimary),
				checkAuditPlaybookWalk("playbook.audit", "Playbook walk audit records limit verdict",
					testSymbolPrimary, reasoning.ActionLimit),
			},
		},
		{
			ID:   "playbook.settle_holding",
			Name: "Holding observation routes to settle_position",
			BuildCapture: func(builder *CaptureBuilder) {
				builder.AppendInstrumentCatalog()
				builder.AppendTicker(testSymbolPrimary, 50_000, 49_990, 50_010, 0)
				builder.AppendBookSnapshot(testSymbolPrimary, 49_990, 10, 50_010, 10)
			},
			HoldingSymbols:     []string{testSymbolPrimary},
			DirectMeasurements: playbookMedianDepthExitMeasurements(testSymbolPrimary, 50_000),
			SettleDelay:        600 * time.Millisecond,
			Checks: []ScenarioCheck{
				checkActionType("playbook.exit", "Story publishes settle_position while holding",
					reasoning.ActionSettlePosition, testSymbolPrimary),
				checkAuditPlaybookWalk("playbook.exit_audit", "Playbook walk audit records settle verdict",
					testSymbolPrimary, reasoning.ActionSettlePosition),
			},
		},
	}
}
