package graph

import (
	"fmt"
	"iter"
	"math"
	"sort"
	"strings"

	"github.com/theapemachine/nomagique/probability"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

/*
measurementIndex resolves the exact metric samples named by category and
relationship artifacts without reconstructing identity from node labels.
*/
type measurementIndex struct {
	byReference map[string][]*types.Node
	bySource    map[string][]*nmtypes.Measurement
}

/*
measurementCompiler materializes signal measurements and the relationships
whose evidence is already declared by typed measurement artifacts.
*/
type measurementCompiler struct{}

func newMeasurementCompiler() *measurementCompiler {
	return &measurementCompiler{}
}

func (compiler *measurementCompiler) addNodes(
	symbol string,
	measurements iter.Seq[*nmtypes.Measurement],
	graph *types.Graph,
) (*measurementIndex, error) {
	index := &measurementIndex{
		byReference: make(map[string][]*types.Node),
		bySource:    make(map[string][]*nmtypes.Measurement),
	}
	for measurement := range measurements {
		if measurement.Symbol != symbol {
			return nil, fmt.Errorf(
				"measurement symbol %s does not match graph symbol %s",
				measurement.Symbol,
				symbol,
			)
		}

		if err := compiler.addMeasurement(measurement, graph, index); err != nil {
			return nil, err
		}
	}

	for _, node := range graph.Nodes {
		if node.Kind != types.KindMeasurement {
			continue
		}

		reference := measurementReference(
			node.Source,
			types.MetricKey(node.Metric, node.Side),
		)
		index.byReference[reference] = append(index.byReference[reference], node)
	}

	return index, nil
}

func (compiler *measurementCompiler) addMeasurement(
	measurement *nmtypes.Measurement,
	graph *types.Graph,
	index *measurementIndex,
) error {
	if measurement.ID == "" || measurement.Source == "" ||
		measurement.Symbol == "" {
		return fmt.Errorf("identified measurement required")
	}

	if string(measurement.Source) == string(types.SourceLeadLag) && measurement.Peer != "" {
		_, peerPriceFound := measurement.Metrics[types.MetricKey(
			types.MetricPeerLastPrice,
			types.SideNone,
		)]

		if !peerPriceFound || measurement.PeerAt.IsZero() ||
			measurement.PeerObservedFrom.IsZero() ||
			measurement.PeerObservedFrom.After(measurement.PeerAt) {
			return fmt.Errorf("complete lead-lag peer observation required")
		}
	}

	index.bySource[measurement.Source] = append(
		index.bySource[measurement.Source], measurement,
	)

	keys := make([]string, 0, len(measurement.Metrics))

	for key := range measurement.Metrics {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	for _, metricKey := range keys {
		sample := measurement.Metrics[metricKey]
		metric, side := types.ParseMetricKey(metricKey)

		if !compiler.graphMetric(measurement.Source, metric, side, sample) {
			continue
		}

		node := measurementNode(measurement, metricKey, metric, side, sample)
		graph.AddNode(node)
	}

	return nil
}

func (compiler *measurementCompiler) graphMetric(
	source string,
	metric types.MetricType,
	side types.MeasurementSide,
	sample *nmtypes.Metric[float64],
) bool {
	if source == string(types.SourceLeadLag) &&
		(metric == types.MetricLastPrice || metric == types.MetricPeerLastPrice) {
		return true
	}

	if sample.Normalized == nil || *sample.Normalized <= 0 {
		return false
	}

	groups, known := types.SignalMetricGroups[types.SourceType(source)]

	if !known {
		return false
	}

	_, known = groups[types.MetricKey(metric, side)]
	return known
}

func measurementNode(
	measurement *nmtypes.Measurement,
	metricKey string,
	metric types.MetricType,
	side types.MeasurementSide,
	sample *nmtypes.Metric[float64],
) *types.Node {
	node := &types.Node{
		ID:            measurementNodeID(measurement, metricKey),
		Symbol:        measurement.Symbol,
		Peer:          measurement.Peer,
		Source:        measurement.Source,
		MeasurementID: measurement.ID,
		Metric:        metric,
		Side:          side,
		Kind:          types.KindMeasurement,
		Value:         sample.Raw,
		Normalized:    cloneFloat(sample.Normalized),
		Maturity:      measurement.Maturity,
		Unit:          types.MeasurementUnit(sample.Unit.String()),
		ObservedFrom:  measurement.ObservedFrom,
		Horizon:       measurement.Horizon,
		At:            measurement.At,
	}

	if sample.Normalized != nil && *sample.Normalized > 0 {
		node.Confidence = measurementConfidence(*sample.Normalized)
	}

	if metric != types.MetricPeerLastPrice {
		return node
	}

	node.Symbol = measurement.Peer
	node.Peer = measurement.Symbol
	node.At = measurement.PeerAt
	node.ObservedFrom = measurement.PeerObservedFrom
	node.Horizon = measurement.PeerAt.Sub(measurement.PeerObservedFrom)

	return node
}

func measurementNodeID(
	measurement *nmtypes.Measurement,
	metricKey string,
) string {
	if metricKey == types.MetricKey(types.MetricPeerLastPrice, types.SideNone) {
		return "meas:" + measurement.Peer + ":" + measurement.Source + ":" +
			measurement.Symbol + ":" + metricKey
	}

	nodeID := "meas:" + measurement.Symbol + ":" + measurement.Source + ":"

	if measurement.Peer != "" {
		nodeID += measurement.Peer + ":"
	}

	return nodeID + metricKey
}

func measurementReference(source string, metricKey string) string {
	return source + ":" + metricKey
}

/*
measurementConfidence maps a normalized measurement strength to an open unit
interval using the same magnitude margin as every other graph mass. The caller
already requires a strictly positive normalized value, so zero and negative
inputs never reach this helper. NaN or infinity are left to fail loudly.
*/
func measurementConfidence(normalized float64) float64 {
	weight, err := probability.MagnitudeMargin(normalized)

	if err != nil {
		panic("graph: invalid normalized measurement strength: " + err.Error())
	}

	if weight >= 1 {
		return math.Nextafter(1, 0)
	}

	return weight
}

func cloneFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}

	cloned := *value

	return &cloned
}

func (compiler *measurementCompiler) addCategoryEdges(
	categories []types.Category,
	symbol string,
	graph *types.Graph,
	index *measurementIndex,
) error {
	for _, category := range categories {
		if category.Type == types.CategoryTypeNone {
			continue
		}

		if category.Symbol != symbol {
			return fmt.Errorf(
				"category symbol %s does not match graph symbol %s",
				category.Symbol,
				symbol,
			)
		}

		if err := compiler.addCategoryRelations(
			category, category.Supporting, types.RelationSupports, graph, index,
		); err != nil {
			return err
		}

		if err := compiler.addCategoryRelations(
			category, category.Opposing, types.RelationContradicts, graph, index,
		); err != nil {
			return err
		}
	}

	return nil
}

func (compiler *measurementCompiler) addCategoryRelations(
	category types.Category,
	references []string,
	relation types.RelationType,
	graph *types.Graph,
	index *measurementIndex,
) error {
	targetID := fmt.Sprintf("cat:%s:%s", category.Symbol, category.Type)

	for _, reference := range references {
		nodes := index.byReference[reference]

		// A category may reference evidence this pass has not observed yet.
		// Cursors drain under the streaming volume clock, so the measurement
		// node for a reference can legitimately still be queued. Skip the
		// relation and leave the graph honestly incomplete; the next pass
		// observes more of the queue and retries the wiring.
		if len(nodes) == 0 {
			continue
		}

		normalized := false

		for _, node := range nodes {
			if node.Normalized == nil {
				continue
			}

			normalized = true

			weight := math.Abs(*node.Normalized)

			if weight == 0 {
				continue
			}

			graph.AddEdge(&types.Edge{
				From:         node.ID,
				To:           targetID,
				Relation:     relation,
				Weight:       weight,
				Confidence:   category.Confidence,
				Evidence:     []string{node.MeasurementID, reference},
				ObservedFrom: node.ObservedFrom,
				Horizon:      node.Horizon,
				At:           node.At,
				Reason: strings.Join([]string{
					reference, string(relation), string(category.Type),
				}, " "),
			})
		}

		// No normalized evidence either: the reference is not yet measurable.
		// Same contract as the missing node — skip, stay honest, retry next
		// pass.
		if !normalized {
			continue
		}
	}

	return nil
}
