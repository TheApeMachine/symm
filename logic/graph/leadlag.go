package graph

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/types"
)

func (compiler *measurementCompiler) addLeadLagEdges(
	symbol *types.Symbol,
	graph *Graph,
	index *measurementIndex,
) error {
	for _, measurement := range index.bySource[types.SourceLeadLag] {
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
	measurement *types.Measurement,
	graph *Graph,
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
			RelationIncomparableWith,
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

func peerPriceNode(measurement *types.Measurement, graph *Graph) (*Node, error) {
	metricKey := types.MetricKey(types.MetricPeerLastPrice, types.SideNone)
	node := graph.Nodes[measurementNodeID(*measurement, metricKey)]

	if node == nil {
		return nil, fmt.Errorf(
			"lead-lag peer price node for %s required", measurement.Peer,
		)
	}

	return node, nil
}

func priceNode(measurement *types.Measurement, graph *Graph) (*Node, error) {
	metricKey := types.MetricKey(types.MetricLastPrice, types.SideNone)
	node := graph.Nodes[measurementNodeID(*measurement, metricKey)]

	if node == nil {
		return nil, fmt.Errorf("lead-lag price node for %s required", measurement.Symbol)
	}

	return node, nil
}

func normalizedMetric(
	measurement *types.Measurement,
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
	measurement *types.Measurement,
	localPrice *Node,
	peerPrice *Node,
	graph *Graph,
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

	graph.AddEdge(&Edge{
		From:         older.ID,
		To:           newer.ID,
		Relation:     RelationStaleRelativeTo,
		Weight:       float64(age) / float64(older.Horizon),
		Evidence:     []string{measurement.ID, older.MeasurementID, newer.MeasurementID},
		ObservedFrom: older.ObservedFrom,
		Horizon:      older.Horizon,
		At:           newer.At,
		Reason:       "older price observation exceeds its own evidence horizon",
	})
}

func (compiler *measurementCompiler) addCorrelationRelations(
	measurement *types.Measurement,
	localPrice *Node,
	peerPrice *Node,
	graph *Graph,
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
			measurement, localPrice, peerPrice, RelationRedundantWith,
			synchronized, "contemporaneous price paths carry synchronized evidence", graph,
			types.MetricSync,
			types.MetricSignedContempCorrelation,
			types.MetricSampleCount,
		)
	}

	if decoupled > 0 {
		compiler.addSymmetricRelation(
			measurement, localPrice, peerPrice, RelationIndependentOf,
			decoupled, "price paths carry decoupled correlation evidence", graph,
			types.MetricDecoupled,
			types.MetricSignedCorrelation,
			types.MetricSampleCount,
		)
	}

	return nil
}

func (compiler *measurementCompiler) addDirectedPair(
	measurement *types.Measurement,
	leader *Node,
	follower *Node,
	weight float64,
	graph *Graph,
) {
	evidence := leadLagEvidence(
		measurement,
		types.MetricInefficient,
		types.MetricSignedLagDirection,
		types.MetricSignedLagCorrelation,
		types.MetricSampleCount,
	)
	graph.AddEdge(&Edge{
		From: leader.ID, To: follower.ID, Relation: RelationLeads,
		Weight: weight, Evidence: evidence,
		ObservedFrom: measurement.ObservedFrom, Horizon: measurement.Horizon,
		At: measurement.At, Reason: "lagged price correlation identifies temporal leader",
	})
	graph.AddEdge(&Edge{
		From: follower.ID, To: leader.ID, Relation: RelationLags,
		Weight: weight, Evidence: evidence,
		ObservedFrom: measurement.ObservedFrom, Horizon: measurement.Horizon,
		At: measurement.At, Reason: "inverse of measured temporal lead",
	})
}

func (compiler *measurementCompiler) addSymmetricRelation(
	measurement *types.Measurement,
	left *Node,
	right *Node,
	relation RelationType,
	weight float64,
	reason string,
	graph *Graph,
	metrics ...types.MetricType,
) {
	evidence := leadLagEvidence(measurement, metrics...)
	for _, endpoints := range [][2]*Node{{left, right}, {right, left}} {
		graph.AddEdge(&Edge{
			From: endpoints[0].ID, To: endpoints[1].ID, Relation: relation,
			Weight: weight, Evidence: evidence,
			ObservedFrom: measurement.ObservedFrom, Horizon: measurement.Horizon,
			At: measurement.At, Reason: reason,
		})
	}
}

func leadLagEvidence(
	measurement *types.Measurement,
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
