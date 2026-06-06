package integration

import (
	"fmt"
	"time"

	"github.com/theapemachine/symm/market/perspectives/types"
)

func infraScenarios() []Scenario {
	return []Scenario{
		{
			ID:   "infra.replay_raw_bus",
			Name: "Replay websocket feeds the raw bus",
			BuildCapture: func(builder *CaptureBuilder) {
				builder.AppendBaselineMarket()
			},
			SettleDelay: 400 * time.Millisecond,
			Checks: []ScenarioCheck{
				{
					ID:   "raw.frames",
					Name: "Raw bus received replay frames",
					Evaluate: func(snapshot TapeSnapshot, _ error) (bool, string, map[string]any) {
						pass := snapshot.RawFrames > 0

						return pass, fmt.Sprintf("raw_frames=%d", snapshot.RawFrames), map[string]any{
							"raw_frames": snapshot.RawFrames,
						}
					},
				},
				{
					ID:   "desk.ready",
					Name: "Paper desk reports ready after balance handshake",
					Evaluate: func(snapshot TapeSnapshot, _ error) (bool, string, map[string]any) {
						return snapshot.DeskReady, fmt.Sprintf("desk_ready=%v", snapshot.DeskReady), nil
					},
				},
			},
		},
		{
			ID:   "infra.fast_signals_manifest",
			Name: "Baseline replay activates fast-path measurement sources",
			BuildCapture: func(builder *CaptureBuilder) {
				builder.AppendInstrumentCatalog()
				builder.AppendBaselineMarket()
				builder.AppendTradeBurst(testSymbolPrimary, 128, 100, 1.5, "alternate")
				builder.AppendDepthflowTape(testSymbolPrimary)
				builder.AppendLiquidityCrossSection()
				builder.AppendPumpLift(testSymbolPrimary, 28)
				builder.AppendBookThinning(testSymbolPrimary, 12)
				builder.AppendCausalCrossSection()
				builder.AppendToxicityCancelWall(testSymbolPrimary, 100)
				builder.AppendSentimentSlumpCrossSection()
			},
			SettleDelay: 2 * time.Second,
			Checks: []ScenarioCheck{
				checkSourcesObserved(
					"signals.manifest",
					"Fast-path sources published at least one measurement",
					types.SourceCVD,
					types.SourceFluid,
					types.SourceHawkes,
					types.SourceDepthFlow,
					types.SourceSentiment,
					types.SourceLiquidity,
					types.SourceExhaustion,
					types.SourcePumpDump,
					types.SourceToxicity,
					types.SourceCausal,
				),
			},
		},
	}
}
