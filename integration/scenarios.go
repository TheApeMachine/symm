package integration

import (
	"fmt"
	"time"

	"github.com/theapemachine/symm/market/perspectives"
)

/*
ScenarioCheck evaluates one assertion against collected telemetry.
*/
type ScenarioCheck struct {
	ID       string
	Name     string
	Evaluate func(snapshot TapeSnapshot, engineErr error) (bool, string, map[string]any)
}

/*
Scenario is one deterministic replay-backed integration path.
*/
type Scenario struct {
	ID                  string
	Name                string
	BuildCapture        func(*CaptureBuilder)
	DirectMeasurements  []perspectives.Measurement
	SettleDelay         time.Duration
	Checks              []ScenarioCheck
}

func defaultScenarios() []Scenario {
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
			ID:   "signal.cvd_flow",
			Name: "CVD publishes executed-flow measurement from synthetic tape",
			BuildCapture: func(builder *CaptureBuilder) {
				builder.AppendBaselineMarket()
			},
			SettleDelay: 600 * time.Millisecond,
			Checks: []ScenarioCheck{
				checkMeasurementSource(
					"cvd.source",
					"CVD measurement observed",
					perspectives.SourceCVD,
					testSymbolPrimary,
				),
				checkCategoryObserved(
					"cvd.category",
					"CVD assigns a flow category",
					perspectives.SourceCVD,
					perspectives.CategoryAggressiveDrive,
					perspectives.CategoryHiddenAbsorption,
					perspectives.CategoryStochasticBalance,
					perspectives.CategoryVolumeStarvation,
				),
			},
		},
		{
			ID:   "signal.fluid_mechanics",
			Name: "Fluid publishes mechanical perspective from synthetic book",
			BuildCapture: func(builder *CaptureBuilder) {
				builder.AppendBaselineMarket()
			},
			SettleDelay: 600 * time.Millisecond,
			Checks: []ScenarioCheck{
				checkMeasurementSource(
					"fluid.source",
					"Fluid measurement observed",
					perspectives.SourceFluid,
					testSymbolPrimary,
				),
				checkCategoryObserved(
					"fluid.category",
					"Fluid assigns a mechanical category",
					perspectives.SourceFluid,
					perspectives.CategoryLaminar,
					perspectives.CategoryTurbulent,
					perspectives.CategoryInertial,
					perspectives.CategoryViscous,
				),
			},
		},
		{
			ID:   "signal.sentiment_slump",
			Name: "Sentiment classifies systemic slump under weak breadth",
			BuildCapture: func(builder *CaptureBuilder) {
				builder.AppendInstrumentCatalog()
				builder.AppendSentimentSlumpCrossSection()
			},
			SettleDelay: 600 * time.Millisecond,
			Checks: []ScenarioCheck{
				checkCategoryExact(
					"sentiment.category",
					"Sentiment maps weak breadth to systemic slump",
					perspectives.SourceSentiment,
					testSymbolPrimary,
					perspectives.CategorySystemicSlump,
				),
			},
		},
		{
			ID:   "playbook.limit_entry",
			Name: "Perspective tree publishes limit entry on raw",
			BuildCapture: func(builder *CaptureBuilder) {
				builder.AppendInstrumentCatalog()
				builder.AppendTicker(testSymbolPrimary, 50_000, 49_990, 50_010, 0)
				builder.AppendBookSnapshot(testSymbolPrimary, 49_990, 10, 50_010, 10)
			},
			DirectMeasurements: playbookLiquidityVacuumMeasurements(testSymbolPrimary, 50_000),
			SettleDelay:        500 * time.Millisecond,
			Checks: []ScenarioCheck{
				checkActionType(
					"playbook.action",
					"Story publishes limit entry action",
					perspectives.ActionLimit,
					testSymbolPrimary,
				),
			},
		},
		{
			ID:   "execution.wallet_after_entry",
			Name: "Entry action reaches trader and paper wallet stays coherent",
			BuildCapture: func(builder *CaptureBuilder) {
				builder.AppendInstrumentCatalog()
				builder.AppendTicker(testSymbolPrimary, 50_000, 49_990, 50_010, 0)
				builder.AppendBookSnapshot(testSymbolPrimary, 49_990, 20, 50_010, 20)
				builder.AppendSellTrade(testSymbolPrimary, 50_010, 0.001)
			},
			DirectMeasurements: playbookLiquidityVacuumMeasurements(testSymbolPrimary, 50_000),
			SettleDelay:        900 * time.Millisecond,
			Checks: []ScenarioCheck{
				checkActionType(
					"execution.action",
					"Limit entry action observed on raw",
					perspectives.ActionLimit,
					testSymbolPrimary,
				),
				{
					ID:   "execution.wallet",
					Name: "Paper wallet snapshot published",
					Evaluate: func(snapshot TapeSnapshot, _ error) (bool, string, map[string]any) {
						balance := snapshot.lastWalletBalance()
						pass := len(snapshot.Wallets) > 0 && balance > 0

						return pass, fmt.Sprintf("wallet_frames=%d balance=%.4f", len(snapshot.Wallets), balance), map[string]any{
							"wallet_frames": len(snapshot.Wallets),
							"balance":       balance,
						}
					},
				},
			},
		},
		{
			ID:   "black_swan.crash_recovery",
			Name: "System continues measuring through crash and wide spread",
			BuildCapture: func(builder *CaptureBuilder) {
				builder.AppendInstrumentCatalog()
				builder.AppendBlackSwanCrash()
			},
			SettleDelay: 800 * time.Millisecond,
			Checks: []ScenarioCheck{
				{
					ID:   "black_swan.desk",
					Name: "Desk stays unhalted through crash replay",
					Evaluate: func(snapshot TapeSnapshot, _ error) (bool, string, map[string]any) {
						return snapshot.DeskReady, "desk remained ready", nil
					},
				},
				checkMeasurementSource(
					"black_swan.measurements",
					"Measurements still flow after crash frames",
					perspectives.SourceFluid,
					testSymbolPrimary,
				),
				checkCategoryExact(
					"black_swan.sentiment",
					"Sentiment reads systemic slump on weak breadth",
					perspectives.SourceSentiment,
					testSymbolPrimary,
					perspectives.CategorySystemicSlump,
				),
			},
		},
	}
}

func playbookLiquidityVacuumMeasurements(
	symbol string,
	last float64,
) []perspectives.Measurement {
	return []perspectives.Measurement{
		{Symbol: symbol, Category: perspectives.CategoryVolumeStarvation, SNR: 1.0, Last: last},
		{Symbol: symbol, Category: perspectives.CategoryLiquidityVacuum, SNR: 1.011867, Last: last},
		{Symbol: symbol, Category: perspectives.CategoryLiquidityVacuum, SNR: 1.0, Last: last},
	}
}

func checkMeasurementSource(
	id, name string,
	source perspectives.SourceType,
	symbol string,
) ScenarioCheck {
	return ScenarioCheck{
		ID:   id,
		Name: name,
		Evaluate: func(snapshot TapeSnapshot, _ error) (bool, string, map[string]any) {
			reading := snapshot.latestBySource(source)
			pass := reading.Source == source && reading.Symbol == symbol && reading.Last > 0

			return pass, fmt.Sprintf("source=%s symbol=%s last=%.4f", reading.Source, reading.Symbol, reading.Last), map[string]any{
				"categories": snapshot.categoriesForSource(source),
			}
		},
	}
}

func checkCategoryObserved(
	id, name string,
	source perspectives.SourceType,
	categories ...perspectives.CategoryType,
) ScenarioCheck {
	return ScenarioCheck{
		ID:   id,
		Name: name,
		Evaluate: func(snapshot TapeSnapshot, _ error) (bool, string, map[string]any) {
			for _, category := range categories {
				if snapshot.hasCategory(source, category) {
					return true, fmt.Sprintf("matched %s", category), map[string]any{
						"categories": snapshot.categoriesForSource(source),
					}
				}
			}

			return false, "no expected category observed", map[string]any{
				"categories": snapshot.categoriesForSource(source),
				"expected":     categories,
			}
		},
	}
}

func checkCategoryExact(
	id, name string,
	source perspectives.SourceType,
	symbol string,
	category perspectives.CategoryType,
) ScenarioCheck {
	return ScenarioCheck{
		ID:   id,
		Name: name,
		Evaluate: func(snapshot TapeSnapshot, _ error) (bool, string, map[string]any) {
			reading := snapshot.latestBySource(source)
			pass := reading.Symbol == symbol && reading.Category == category

			return pass, fmt.Sprintf("category=%s", reading.Category), map[string]any{
				"symbol": reading.Symbol,
				"snr":    reading.SNR,
			}
		},
	}
}

func checkActionType(
	id, name string,
	actionType perspectives.ActionType,
	symbol string,
) ScenarioCheck {
	return ScenarioCheck{
		ID:   id,
		Name: name,
		Evaluate: func(snapshot TapeSnapshot, _ error) (bool, string, map[string]any) {
			for _, action := range snapshot.Actions {
				if action.Type == actionType && action.Symbol == symbol {
					return true, fmt.Sprintf("action=%s qty=%.8f", action.Type, action.Quantity), nil
				}
			}

			return false, "expected action not observed on raw", map[string]any{
				"actions": len(snapshot.Actions),
			}
		},
	}
}

func buildCapture(scenario Scenario) *CaptureBuilder {
	builder := NewCaptureBuilder(time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC))

	if scenario.BuildCapture != nil {
		scenario.BuildCapture(builder)
	}

	return builder
}
