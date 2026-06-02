package integration

import (
	"fmt"

	"github.com/theapemachine/symm/market/perspectives"
)

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
				"expected":   categories,
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

func checkSourcesObserved(id, name string, sources ...perspectives.SourceType) ScenarioCheck {
	return ScenarioCheck{
		ID:   id,
		Name: name,
		Evaluate: func(snapshot TapeSnapshot, _ error) (bool, string, map[string]any) {
			missing := make([]string, 0)

			for _, source := range sources {
				if snapshot.countBySource(source) == 0 {
					missing = append(missing, source.String())
				}
			}

			pass := len(missing) == 0

			return pass, fmt.Sprintf("missing=%v", missing), map[string]any{
				"counts": snapshot.countsBySource(),
			}
		},
	}
}

func checkInventory(
	id, name string,
	baseAsset string,
	minQty float64,
) ScenarioCheck {
	return ScenarioCheck{
		ID:   id,
		Name: name,
		Evaluate: func(snapshot TapeSnapshot, _ error) (bool, string, map[string]any) {
			qty := snapshot.lastInventory(baseAsset)
			pass := qty >= minQty

			return pass, fmt.Sprintf("%s=%.8f", baseAsset, qty), map[string]any{
				"inventory": snapshot.lastInventoryMap(),
			}
		},
	}
}

func checkFillEvent(id, name string, symbol string) ScenarioCheck {
	return ScenarioCheck{
		ID:   id,
		Name: name,
		Evaluate: func(snapshot TapeSnapshot, _ error) (bool, string, map[string]any) {
			for _, fill := range snapshot.Fills {
				if fill.Symbol == symbol && fill.Qty > 0 {
					return true, fmt.Sprintf("qty=%.8f price=%.4f", fill.Qty, fill.Price), nil
				}
			}

			return false, "no fill ui event for symbol", map[string]any{
				"fills": len(snapshot.Fills),
			}
		},
	}
}

func checkAuditPlaybookWalk(id, name string, symbol string, action perspectives.ActionType) ScenarioCheck {
	return ScenarioCheck{
		ID:   id,
		Name: name,
		Evaluate: func(snapshot TapeSnapshot, _ error) (bool, string, map[string]any) {
			want := string(action)

			for _, row := range snapshot.AuditRows {
				if row.AuditEvent != "playbook_walk" || row.Symbol != symbol {
					continue
				}

				if row.Verdict == want {
					return true, fmt.Sprintf("audit verdict=%s", row.Verdict), map[string]any{
						"block_reason": row.BlockReason,
					}
				}
			}

			return false, "playbook walk audit row not found", map[string]any{
				"audit_rows": len(snapshot.AuditRows),
			}
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

func playbookMedianDepthExitMeasurements(
	symbol string,
	last float64,
) []perspectives.Measurement {
	return []perspectives.Measurement{
		{Symbol: symbol, Category: perspectives.CategoryMedianDepth, SNR: 1.5, Last: last},
	}
}
