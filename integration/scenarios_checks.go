package integration

import (
	"fmt"

	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func auditVerdictMatches(row AuditRow, want reasoning.ActionType) bool {
	switch typed := row.Verdict.(type) {
	case string:
		return typed == want.String()
	case float64:
		return reasoning.ActionType(typed) == want
	default:
		return false
	}
}

func checkMeasurementSource(
	id, name string,
	source types.SourceType,
	symbol string,
) ScenarioCheck {
	return checkMeasurementSourceAllowZeroLast(id, name, source, symbol, false)
}

func checkMeasurementSourceAllowZeroLast(
	id, name string,
	source types.SourceType,
	symbol string,
	allowZeroLast bool,
) ScenarioCheck {
	return ScenarioCheck{
		ID:   id,
		Name: name,
		Evaluate: func(snapshot TapeSnapshot, _ error) (bool, string, map[string]any) {
			reading := snapshot.latestBySource(source)
			pass := reading.Source == source && reading.Symbol == symbol

			if pass && !allowZeroLast {
				pass = reading.Last > 0
			}

			return pass, fmt.Sprintf("source=%s symbol=%s last=%.4f", reading.Source, reading.Symbol, reading.Last), map[string]any{
				"categories": snapshot.categoriesForSource(source),
			}
		},
	}
}

/*
checkSignalCategoryFixture requires the probe symbol's latest measurement from the
source to equal the expected category under the synthetic tape. Observed categories
are reported for post-run evaluation; passing only one of several possible labels
is not sufficient.
*/
func checkSignalCategoryFixture(
	id, name string,
	probe SignalCategoryProbe,
) ScenarioCheck {
	return ScenarioCheck{
		ID:   id,
		Name: name,
		Evaluate: func(snapshot TapeSnapshot, _ error) (bool, string, map[string]any) {
			reading := snapshot.latestBySourceSymbol(probe.Source, probe.Symbol)
			observed := snapshot.categoriesForSource(probe.Source)

			contextMap := map[string]any{
				"expected":  probe.Category,
				"observed":  observed,
				"fixture":   probe.Fixture,
				"condition": probe.Condition,
				"symbol":    probe.Symbol,
				"source":    probe.Source,
				"publish_n": snapshot.countBySource(probe.Source),
			}

			if snapshot.countBySource(probe.Source) == 0 {
				return false, "source never published a measurement", contextMap
			}

			if reading.Source != probe.Source || reading.Symbol != probe.Symbol {
				return false, "no measurement for probe symbol", contextMap
			}

			if reading.Category != probe.Category {
				return false, fmt.Sprintf(
					"latest=%s want=%s",
					reading.Category,
					probe.Category,
				), contextMap
			}

			if !snapshot.hasCategory(probe.Source, probe.Category) {
				return false, "expected category never observed during run", contextMap
			}

			return true, fmt.Sprintf("category=%s snr=%.4f", reading.Category, reading.SNR), contextMap
		},
	}
}

func checkCategoryObserved(
	id, name string,
	source types.SourceType,
	categories ...types.CategoryType,
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

func checkActionType(
	id, name string,
	actionType reasoning.ActionType,
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

func checkSourcesObserved(id, name string, sources ...types.SourceType) ScenarioCheck {
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

func checkAuditPlaybookWalk(id, name string, symbol string, action reasoning.ActionType) ScenarioCheck {
	return ScenarioCheck{
		ID:   id,
		Name: name,
		Evaluate: func(snapshot TapeSnapshot, _ error) (bool, string, map[string]any) {
			for _, row := range snapshot.AuditRows {
				if row.AuditEvent != "playbook_walk" || row.Symbol != symbol {
					continue
				}

				if auditVerdictMatches(row, action) {
					return true, fmt.Sprintf("audit verdict=%v", row.Verdict), map[string]any{
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
) []types.Measurement {
	return perspectives.FixturePlaybookEntryMeasurements(symbol, last)
}

func playbookMedianDepthExitMeasurements(
	symbol string,
	last float64,
) []types.Measurement {
	return perspectives.FixturePlaybookExitMeasurements(symbol, last)
}
