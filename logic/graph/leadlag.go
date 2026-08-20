package graph

import (
	"fmt"
	"math"

	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

func (compiler *measurementCompiler) addLeadLagEdges(
	symbol *types.Symbol,
	graph *types.Graph,
	index *measurementIndex,
) error {
	for _, measurement := range index.bySource[string(types.SourceLeadLag)] {
		if measurement.Symbol != symbol.Symbol || measurement.Peer == "" {
			continue
		}

		if err := compiler.relateLeadLag(measurement, graph); err != nil {
			return err
		}
	}

	return nil
}

func (compiler *measurementCompiler) relateLeadLag(
	measurement *nmtypes.Measurement,
	graph *types.Graph,
) error {
	localPrice, err := priceNode(measurement, graph)

	if err != nil {
		// The measurement node this pass needs is still queued behind the
		// streaming cursor. Skip the relation; the next pass retries with
		// more of the queue observed.
		return nil
	}

	peerPrice, err := peerPriceNode(measurement, graph)

	if err != nil {
		return nil
	}

	supportKey := types.MetricKey(types.MetricSampleCount, types.SideNone)
	support, supportFound := measurement.Metrics[supportKey]

	if !supportFound {
		return nil
	}

	if support.Raw <= 0 {
		compiler.addSymmetricRelation(
			measurement,
			localPrice,
			peerPrice,
			types.RelationIncomparableWith,
			1,
			"lead-lag comparison has no resolved return support",
			graph,
			types.MetricSampleCount,
		)

		return nil
	}

	compiler.addTemporalRelation(measurement, localPrice, peerPrice, graph)

	if err := compiler.addCorrelationRelations(
		measurement, localPrice, peerPrice, graph,
	); err != nil {
		return err
	}

	return nil
}

func peerPriceNode(measurement *nmtypes.Measurement, graph *types.Graph) (*types.Node, error) {
	metricKey := types.MetricKey(types.MetricPeerLastPrice, types.SideNone)
	node := graph.Nodes[measurementNodeID(measurement, metricKey)]

	if node == nil {
		return nil, fmt.Errorf(
			"lead-lag peer price node for %s required", measurement.Peer,
		)
	}

	return node, nil
}

func priceNode(measurement *nmtypes.Measurement, graph *types.Graph) (*types.Node, error) {
	metricKey := types.MetricKey(types.MetricLastPrice, types.SideNone)
	node := graph.Nodes[measurementNodeID(measurement, metricKey)]

	if node == nil {
		return nil, fmt.Errorf("lead-lag price node for %s required", measurement.Symbol)
	}

	return node, nil
}

func normalizedMetric(
	measurement *nmtypes.Measurement,
	metric types.MetricType,
) (float64, error) {
	metricKey := types.MetricKey(metric, types.SideNone)
	sample, found := measurement.Metrics[metricKey]

	if !found || sample.Normalized == nil {
		return 0, fmt.Errorf(
			"lead-lag metric %s for %s required", metric, measurement.Symbol,
		)
	}

	return *sample.Normalized, nil
}

func (compiler *measurementCompiler) addTemporalRelation(
	measurement *nmtypes.Measurement,
	localPrice *types.Node,
	peerPrice *types.Node,
	graph *types.Graph,
) {
	older := localPrice
	newer := peerPrice

	if peerPrice.At.Before(localPrice.At) {
		older = peerPrice
		newer = localPrice
	}

	if older.Horizon <= 0 {
		return
	}

	age := newer.At.Sub(older.At)

	if age <= older.Horizon {
		return
	}

	graph.AddEdge(&types.Edge{
		From:         older.ID,
		To:           newer.ID,
		Relation:     types.RelationStaleRelativeTo,
		Weight:       float64(age) / float64(older.Horizon),
		Evidence:     []string{measurement.ID, older.MeasurementID, newer.MeasurementID},
		ObservedFrom: older.ObservedFrom,
		Horizon:      older.Horizon,
		At:           newer.At,
		Reason:       "older price observation exceeds its own evidence horizon",
	})
}

func (compiler *measurementCompiler) addCorrelationRelations(
	measurement *nmtypes.Measurement,
	localPrice *types.Node,
	peerPrice *types.Node,
	graph *types.Graph,
) error {
	// Each correlation family is independent evidence. A family that is not
	// yet observed is skipped; observed families still wire this pass.
	inefficient, inefficientErr := normalizedMetric(
		measurement, types.MetricInefficient,
	)
	synchronized, _ := normalizedMetric(measurement, types.MetricSync)
	decoupled, _ := normalizedMetric(measurement, types.MetricDecoupled)
	direction, directionErr := normalizedMetric(
		measurement, types.MetricSignedLagDirection,
	)

	if inefficientErr == nil && inefficient > 0 &&
		directionErr == nil && math.Abs(direction) == 1 {
		leader := peerPrice
		follower := localPrice

		if direction < 0 {
			leader = localPrice
			follower = peerPrice
		}

		compiler.addDirectedPair(
			measurement, leader, follower, inefficient, graph,
		)
	}

	if synchronized > 0 {
		compiler.addSymmetricRelation(
			measurement, localPrice, peerPrice, types.RelationRedundantWith,
			synchronized, "contemporaneous price paths carry synchronized evidence", graph,
			types.MetricSync,
			types.MetricSignedContempCorrelation,
			types.MetricSampleCount,
		)
	}

	if decoupled > 0 {
		compiler.addSymmetricRelation(
			measurement, localPrice, peerPrice, types.RelationIndependentOf,
			decoupled, "price paths carry decoupled correlation evidence", graph,
			types.MetricDecoupled,
			types.MetricSignedLagCorrelation,
			types.MetricSampleCount,
		)
	}

	return nil
}

func (compiler *measurementCompiler) addDirectedPair(
	measurement *nmtypes.Measurement,
	leader *types.Node,
	follower *types.Node,
	weight float64,
	graph *types.Graph,
) {
	evidence := leadLagEvidence(
		measurement,
		types.MetricInefficient,
		types.MetricSignedLagDirection,
		types.MetricSignedLagCorrelation,
		types.MetricSampleCount,
	)
	graph.AddEdge(&types.Edge{
		From: leader.ID, To: follower.ID, Relation: types.RelationLeads,
		Weight: weight, Evidence: evidence,
		ObservedFrom: measurement.ObservedFrom, Horizon: measurement.Horizon,
		At: measurement.At, Reason: "lagged price correlation identifies temporal leader",
	})
	graph.AddEdge(&types.Edge{
		From: follower.ID, To: leader.ID, Relation: types.RelationLags,
		Weight: weight, Evidence: evidence,
		ObservedFrom: measurement.ObservedFrom, Horizon: measurement.Horizon,
		At: measurement.At, Reason: "inverse of measured temporal lead",
	})
}

func (compiler *measurementCompiler) addSymmetricRelation(
	measurement *nmtypes.Measurement,
	left *types.Node,
	right *types.Node,
	relation types.RelationType,
	weight float64,
	reason string,
	graph *types.Graph,
	metrics ...types.MetricType,
) {
	evidence := leadLagEvidence(measurement, metrics...)
	for _, endpoints := range [][2]*types.Node{{left, right}, {right, left}} {
		graph.AddEdge(&types.Edge{
			From: endpoints[0].ID, To: endpoints[1].ID, Relation: relation,
			Weight: weight, Evidence: evidence,
			ObservedFrom: measurement.ObservedFrom, Horizon: measurement.Horizon,
			At: measurement.At, Reason: reason,
		})
	}
}

func leadLagEvidence(
	measurement *nmtypes.Measurement,
	metrics ...types.MetricType,
) []string {
	evidence := make([]string, 1, len(metrics)+1)
	evidence[0] = measurement.ID

	for _, metric := range metrics {
		metricKey := types.MetricKey(metric, types.SideNone)
		evidence = append(
			evidence,
			measurementReference(measurement.Source, metricKey),
		)
	}

	return evidence
}
