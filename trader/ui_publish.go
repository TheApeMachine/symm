package trader

import (
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/logic"
)

type fieldSnapshotter interface {
	FieldSnapshot(time.Time) (map[string]any, error)
}

func measurementWireMap(measurement *datura.Artifact) map[string]any {
	if measurement == nil {
		return nil
	}

	wire := map[string]any{}

	if payload := measurement.DecryptPayload(); len(payload) > 0 {
		_ = sonic.Unmarshal(payload, &wire)
	}

	if origin, err := measurement.Origin(); err == nil && origin != "" {
		wire["origin"] = origin
		wire["source"] = origin
	}

	if scope, err := measurement.Scope(); err == nil && scope != "" {
		wire["scope"] = scope
	}

	if role, err := measurement.Role(); err == nil && role != "" {
		wire["role"] = role
	}

	return wire
}

func (crypto *Crypto) publishUIArtifact(payload map[string]any) {
	if crypto == nil || crypto.uiBroadcast == nil || len(payload) == 0 {
		return
	}

	wire, err := sonic.Marshal(payload)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"trader: ui publish marshal failed",
			err,
		))

		return
	}

	artifact := datura.Acquire("trader", datura.APPJSON).
		WithDestination("ui").
		WithPayload(wire)

	frameType, _ := payload["type"].(string)

	if frameType != "" {
		artifact.WithRole(frameType)
	}

	if err := crypto.uiBroadcast.Send(artifact); err != nil {
		errnie.Error(errnie.Err(
			errnie.IO,
			"trader: ui publish send failed",
			err,
		))
	}
}

func (crypto *Crypto) publishDecisionTreeSnapshot() {
	if crypto == nil || crypto.story == nil {
		return
	}

	tree := crypto.story.PlaybookTree()

	if tree == nil || len(tree.Branches) == 0 {
		return
	}

	crypto.publishUIArtifact(map[string]any{
		"type":     "decision_tree",
		"branches": tree.Branches,
	})
}

func (crypto *Crypto) publishDecisionWalk(
	scope string,
	measurements []*datura.Artifact,
) {
	if crypto == nil || crypto.story == nil || scope == "" || len(measurements) == 0 {
		return
	}

	tree := crypto.story.PlaybookTree()

	if tree == nil || len(tree.Branches) == 0 {
		return
	}

	trace := logic.WalkTree(scope, measurements, nil, tree.Branches)

	if len(trace.Steps) == 0 {
		return
	}

	payload, err := sonic.Marshal(trace)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"trader: decision walk marshal failed",
			err,
		))

		return
	}

	var wire map[string]any

	if err := sonic.Unmarshal(payload, &wire); err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"trader: decision walk decode failed",
			err,
		))

		return
	}

	wire["type"] = "decision_walk"
	crypto.publishUIArtifact(wire)
}

func dedupeMeasurementWires(
	readings []map[string]any,
) []map[string]any {
	if len(readings) == 0 {
		return readings
	}

	bySource := make(map[string]map[string]any, len(readings))
	order := make([]string, 0, len(readings))

	for _, wire := range readings {
		source, _ := wire["source"].(string)

		if source == "" {
			if origin, ok := wire["origin"].(string); ok {
				source = origin
			}
		}

		if source == "" {
			continue
		}

		if _, seen := bySource[source]; !seen {
			order = append(order, source)
		}

		bySource[source] = wire
	}

	deduped := make([]map[string]any, 0, len(order))

	for _, source := range order {
		deduped = append(deduped, bySource[source])
	}

	return deduped
}

func measurementConfidence(measurement *datura.Artifact) float64 {
	if measurement == nil {
		return 0
	}

	return datura.Peek[float64](measurement, "output", "confidence")
}

func shouldReplaceMeasurement(
	anchor string,
	scope string,
	existingScope string,
	newConfidence float64,
	oldConfidence float64,
) bool {
	if scope == anchor && existingScope != anchor {
		return true
	}

	if existingScope == anchor && scope != anchor {
		return false
	}

	return newConfidence > oldConfidence
}

/*
collapseMeasurementsForUI keeps one measurement per signal origin for gauge hydration.
Anchor scope wins ties; otherwise the highest-confidence reading is kept.
*/
func collapseMeasurementsForUI(
	anchor string,
	calibrating []*datura.Artifact,
	grouped map[string][]*datura.Artifact,
) ([]*datura.Artifact, map[string][]*datura.Artifact) {
	byOrigin := make(map[string]*datura.Artifact)

	consider := func(measurement *datura.Artifact) {
		if measurement == nil {
			return
		}

		origin, err := measurement.Origin()

		if err != nil || origin == "" {
			return
		}

		scope, _ := measurement.Scope()
		existing, ok := byOrigin[origin]

		if !ok {
			byOrigin[origin] = measurement

			return
		}

		existingScope, _ := existing.Scope()

		if shouldReplaceMeasurement(
			anchor,
			scope,
			existingScope,
			measurementConfidence(measurement),
			measurementConfidence(existing),
		) {
			byOrigin[origin] = measurement
		}
	}

	for _, measurement := range calibrating {
		consider(measurement)
	}

	for _, measurements := range grouped {
		for _, measurement := range measurements {
			consider(measurement)
		}
	}

	uiCalibrating := make([]*datura.Artifact, 0, len(byOrigin))
	uiGrouped := make(map[string][]*datura.Artifact)

	for _, measurement := range byOrigin {
		if datura.Peek[bool](measurement, "calibrating") {
			uiCalibrating = append(uiCalibrating, measurement)

			continue
		}

		scope, _ := measurement.Scope()
		uiGrouped[scope] = append(uiGrouped[scope], measurement)
	}

	return uiCalibrating, uiGrouped
}

func (crypto *Crypto) publishMeasurementState(
	grouped map[string][]*datura.Artifact,
	calibrating []*datura.Artifact,
) {
	if crypto == nil {
		return
	}

	readings := make([]map[string]any, 0)

	for _, measurement := range calibrating {
		wire := measurementWireMap(measurement)

		if len(wire) == 0 {
			continue
		}

		readings = append(readings, wire)
	}

	for _, measurements := range grouped {
		for _, measurement := range measurements {
			wire := measurementWireMap(measurement)

			if len(wire) == 0 {
				continue
			}

			readings = append(readings, wire)
		}
	}

	if len(readings) == 0 {
		return
	}

	readings = dedupeMeasurementWires(readings)

	if len(readings) == 0 {
		return
	}

	crypto.publishUIArtifact(map[string]any{
		"type":                 "state",
		"measurements":         readings,
		"playbook_evaluations": crypto.playbookEvaluations.Load(),
	})
}

func (crypto *Crypto) publishFieldSnapshots(eventAt time.Time) {
	if crypto == nil || crypto.signals == nil {
		return
	}

	for _, payload := range crypto.signals.FieldSnapshots(eventAt) {
		crypto.publishUIArtifact(payload)
	}
}

func (crypto *Crypto) publishStoryTick() {
	if crypto == nil || crypto.uiBroadcast == nil {
		return
	}

	if err := crypto.uiBroadcast.Send(
		datura.Acquire("trader", datura.APPJSON).WithPayload(datura.Map[any]{
			"type":                 "story_tick",
			"story_ticks":          crypto.storyTicks.Load(),
			"playbook_evaluations": crypto.playbookEvaluations.Load(),
		}.Marshal()).WithDestination(
			"ui",
		).WithRole(
			"story",
		).WithScope(
			"trader",
		),
	); err != nil {
		errnie.Error(errnie.Err(
			errnie.IO,
			"trader: story tick publish failed",
			err,
		))
	}
}
