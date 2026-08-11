package graph

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/types"
)

func (compiler *measurementCompiler) addLeadLagEdges(
	thesis *types.Thesis,
	symbol *types.Symbol,
	graph *Graph,
	index *measurementIndex,
) error {
	localMeasurements := append(
		[]*types.Measurement(nil), index.bySource[types.SourceLeadLag]...,
	)

	for _, measurement := range localMeasurements {
		if measurement.Symbol != symbol.Symbol || measurement.Peer == "" {
			continue
		}

		peerMeasurement, err := leadLagMeasurement(thesis, measurement.Peer)

		if err != nil {
			return err
		}

		if err := compiler.addMeasurement(peerMeasurement, graph, index); err != nil {
			return fmt.Errorf("peer %s: %w", measurement.Peer, err)
		}

		if err := compiler.relateLeadLag(measurement, peerMeasurement, graph); err != nil {
			return err
		}
	}

	return nil
}

func leadLagMeasurement(
	thesis *types.Thesis,
	symbol string,
) (*types.Measurement, error) {
	stored, found := thesis.Symbols.Load(symbol)

	if !found {
		return nil, fmt.Errorf("lead-lag peer symbol %s required", symbol)
	}

	peer := stored.(*types.Symbol)

	for _, measurement := range peer.Measurements {
		if measurement != nil && measurement.Source == types.SourceLeadLag &&
			measurement.Symbol == symbol && measurement.Peer == "" {
			return measurement, nil
		}
	}

	return nil, fmt.Errorf("lead-lag anchor measurement %s required", symbol)
}

func (compiler *measurementCompiler) relateLeadLag(
	measurement *types.Measurement,
	peer *types.Measurement,
	graph *Graph,
) error {
	localPrice, err := priceNode(measurement, graph)

	if err != nil {
		return err
	}

	peerPrice, err := priceNode(peer, graph)

	if err != nil {
		return err
	}

	support, err := normalizedMetric(measurement, types.MetricSampleSupport)

	if err != nil {
		return err
	}

	if support == 0 {
		missingSupport := 1 - support
		compiler.addSymmetricRelation(
			measurement,
			localPrice,
			peerPrice,
			RelationIncomparableWith,
			missingSupport,
			"lead-lag comparison has no resolved return support",
			graph,
			types.MetricSampleSupport,
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

func priceNode(measurement *types.Measurement, graph *Graph) (*Node, error) {
	metricKey := types.MetricKey(types.MetricLastPrice, types.SideNone)
	node := graph.Nodes[measurementNodeID(measurement, metricKey)]

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
		Quality:      measurementQuality(measurement),
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
	inefficient, err := normalizedMetric(measurement, types.MetricInefficient)

	if err != nil {
		return err
	}

	synchronized, err := normalizedMetric(measurement, types.MetricSync)

	if err != nil {
		return err
	}

	decoupled, err := normalizedMetric(measurement, types.MetricDecoupled)

	if err != nil {
		return err
	}

	direction, err := normalizedMetric(
		measurement, types.MetricSignedLagDirection,
	)

	if err != nil {
		return err
	}

	if inefficient > 0 && math.Abs(direction) == 1 {
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
			types.MetricSampleSupport,
		)
	}

	if decoupled > 0 {
		compiler.addSymmetricRelation(
			measurement, localPrice, peerPrice, RelationIndependentOf,
			decoupled, "price paths carry decoupled correlation evidence", graph,
			types.MetricDecoupled,
			types.MetricSignedCorrelation,
			types.MetricSampleSupport,
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
		types.MetricSampleSupport,
	)
	quality := measurementQuality(measurement)
	graph.AddEdge(&Edge{
		From: leader.ID, To: follower.ID, Relation: RelationLeads,
		Weight: weight, Quality: cloneFloat(quality), Evidence: evidence,
		ObservedFrom: measurement.ObservedFrom, Horizon: measurement.Horizon,
		At: measurement.At, Reason: "lagged price correlation identifies temporal leader",
	})
	graph.AddEdge(&Edge{
		From: follower.ID, To: leader.ID, Relation: RelationLags,
		Weight: weight, Quality: cloneFloat(quality), Evidence: evidence,
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
	quality := measurementQuality(measurement)

	for _, endpoints := range [][2]*Node{{left, right}, {right, left}} {
		graph.AddEdge(&Edge{
			From: endpoints[0].ID, To: endpoints[1].ID, Relation: relation,
			Weight: weight, Quality: cloneFloat(quality), Evidence: evidence,
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
