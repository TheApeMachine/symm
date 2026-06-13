package signal

import (
	"sort"
	"time"

	"github.com/theapemachine/symm/logic"
	symmmarket "github.com/theapemachine/symm/market"
)

func entityFusionOrder(
	source logic.SourceType,
	accepted []logic.EntityType,
) []logic.EntityType {
	preferred := fusionPreference(source)
	ordered := make([]logic.EntityType, 0, len(accepted))
	seen := make(map[logic.EntityType]struct{}, len(accepted))

	for _, entityType := range preferred {
		for _, acceptedType := range accepted {
			if acceptedType != entityType {
				continue
			}

			if _, ok := seen[entityType]; ok {
				continue
			}

			ordered = append(ordered, entityType)
			seen[entityType] = struct{}{}
		}
	}

	for _, entityType := range sortedEntityTypes(accepted) {
		if _, ok := seen[entityType]; ok {
			continue
		}

		ordered = append(ordered, entityType)
		seen[entityType] = struct{}{}
	}

	return ordered
}

func fusionPreference(source logic.SourceType) []logic.EntityType {
	switch logic.SourceDecisionClassFor(source) {
	case logic.SourceClassFlow:
		return []logic.EntityType{logic.EntityTrade}
	case logic.SourceClassTouch:
		return []logic.EntityType{
			logic.EntityBook,
			logic.EntityTick,
			logic.EntityTrade,
		}
	default:
		return []logic.EntityType{
			logic.EntityBook,
			logic.EntityTick,
			logic.EntityTrade,
		}
	}
}

func sortedEntityTypes(accepted []logic.EntityType) []logic.EntityType {
	ordered := append([]logic.EntityType(nil), accepted...)

	sort.Slice(ordered, func(leftIndex, rightIndex int) bool {
		return entityRank(ordered[leftIndex]) < entityRank(ordered[rightIndex])
	})

	return ordered
}

func entityRank(entityType logic.EntityType) int {
	switch entityType {
	case logic.EntityBook:
		return 0
	case logic.EntityTick:
		return 1
	case logic.EntityTrade:
		return 2
	default:
		return 3
	}
}

func sortedWarmSymbols(warmed map[string]symmmarket.Signal) []string {
	symbols := make([]string, 0, len(warmed))

	for symbol := range warmed {
		symbols = append(symbols, symbol)
	}

	sort.Strings(symbols)

	return symbols
}

func finalizeMeasurementFrame(
	measurement logic.Measurement,
	eventAt time.Time,
) logic.Measurement {
	registry := symmmarket.ActiveTouchRegistry()
	referenceAt := eventAt

	if referenceAt.IsZero() {
		referenceAt = time.Now().UTC()
	}

	touch, touchReady := loadTouch(registry, measurement.Symbol, referenceAt)

	if touchReady {
		measurement = stampTouch(measurement, touch)
	}

	measurement.DecisionGrade = logic.DecisionGradeFor(
		measurement.Source,
		touchReady,
	)

	if measurement.NoveltySurprise <= 0 && measurement.Surprise > 0 &&
		measurement.Source != logic.SourcePrediction {
		measurement.NoveltySurprise = measurement.Surprise
	}

	if measurement.EdgeSurprise <= 0 && measurement.Source == logic.SourcePrediction {
		measurement.EdgeSurprise = measurement.Surprise
	}

	if measurement.EdgeConfidence <= 0 {
		measurement.EdgeConfidence = measurement.Confidence
	}

	return measurement
}

func loadTouch(
	registry *symmmarket.TouchRegistry,
	symbol string,
	referenceAt time.Time,
) (symmmarket.TouchSnapshot, bool) {
	if registry == nil || symbol == "" {
		return symmmarket.TouchSnapshot{}, false
	}

	return registry.Load(symbol, referenceAt)
}

func stampTouch(
	measurement logic.Measurement,
	touch symmmarket.TouchSnapshot,
) logic.Measurement {
	midpoint, midpointErr := touch.Midpoint()

	if midpointErr == nil && touch.Last <= 0 {
		measurement.Price = midpoint
	}

	spread := touch.Spread()

	if spread > 0 {
		measurement.Spread = spread
	}

	if touch.Last > 0 {
		measurement.Price = touch.Last
	}

	measurement.ObservedAt = touch.ObservedAt

	return measurement
}
