package graph

import (
	"fmt"
	"math"
	"strings"

	"github.com/theapemachine/symm/types"
)

/*
measurementIndex resolves the exact metric samples named by category and
relationship artifacts without reconstructing identity from node labels.
*/
type measurementIndex struct {
	byReference map[string][]*Node
	bySource    map[types.SourceType][]*types.Measurement
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
	symbol *types.Symbol,
	graph *Graph,
) (*measurementIndex, error) {
	index := &measurementIndex{
		byReference: make(map[string][]*Node),
		bySource:    make(map[types.SourceType][]*types.Measurement),
	}
	measurements, _, _ := symbol.MeasurementState()

	for _, measurement := range types.FilterLatestSourceEpochs(measurements) {
		if measurement != nil && measurement.Symbol != symbol.Symbol {
			return nil, fmt.Errorf(
				"measurement symbol %s does not match graph symbol %s",
				measurement.Symbol,
				symbol.Symbol,
			)
		}

		if err := compiler.addMeasurement(measurement, graph, index); err != nil {
			return nil, err
		}
	}

	return index, nil
}

func (compiler *measurementCompiler) addMeasurement(
	measurement *types.Measurement,
	graph *Graph,
	index *measurementIndex,
) error {
	if measurement == nil || measurement.ID == "" || measurement.Source == "" ||
		measurement.Symbol == "" || measurement.At.IsZero() || len(measurement.Metrics) == 0 {
		return fmt.Errorf("identified, timestamped measurement metrics required")
	}

	if measurement.Source == types.SourceLeadLag && measurement.Peer != "" {
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

	quality := measurementQuality(measurement)
	index.bySource[measurement.Source] = append(
		index.bySource[measurement.Source], measurement,
	)
	measurement.EachMetric(func(
		metric types.MetricType,
		side types.MeasurementSide,
		sample types.MetricSample,
	) bool {
		metricKey := types.MetricKey(metric, side)
		node := measurementNode(measurement, metricKey, metric, side, sample, quality)
		graph.AddNode(node)
		reference := measurementReference(measurement.Source, metricKey)
		index.byReference[reference] = append(index.byReference[reference], node)
		return true
	})

	return nil
}

func measurementNode(
	measurement *types.Measurement,
	metricKey string,
	metric types.MetricType,
	side types.MeasurementSide,
	sample types.MetricSample,
	quality *float64,
) *Node {
	node := &Node{
		ID:            measurementNodeID(measurement, metricKey),
		Symbol:        measurement.Symbol,
		Peer:          measurement.Peer,
		Source:        string(measurement.Source),
		MeasurementID: measurement.ID,
		Metric:        metric,
		Side:          side,
		Kind:          KindMeasurement,
		Value:         sample.Raw,
		Normalized:    cloneFloat(sample.Normalized),
		Quality:       cloneFloat(quality),
		Maturity:      measurement.Maturity,
		Unit:          sample.Unit,
		ObservedFrom:  measurement.ObservedFrom,
		Horizon:       measurement.Horizon,
		At:            measurement.At,
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

func measurementQuality(measurement *types.Measurement) *float64 {
	sample, found := measurement.Metrics[types.MetricKey(types.MetricSNR, types.SideNone)]

	if !found || sample.Normalized == nil {
		return nil
	}

	return cloneFloat(sample.Normalized)
}

func measurementNodeID(
	measurement *types.Measurement,
	metricKey string,
) string {
	if metricKey == types.MetricKey(types.MetricPeerLastPrice, types.SideNone) {
		return "meas:" + measurement.Peer + ":" + string(measurement.Source) + ":" +
			measurement.Symbol + ":" + metricKey
	}

	nodeID := "meas:" + measurement.Symbol + ":" + string(measurement.Source) + ":"

	if measurement.Peer != "" {
		nodeID += measurement.Peer + ":"
	}

	return nodeID + metricKey
}

func measurementReference(source types.SourceType, metricKey string) string {
	return string(source) + ":" + metricKey
}

func cloneFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}

	cloned := *value

	return &cloned
}

func (compiler *measurementCompiler) addCategoryEdges(
	symbol *types.Symbol,
	graph *Graph,
	index *measurementIndex,
) error {
	stored, found := symbol.Categories.Load(symbol.Symbol)

	if !found {
		return nil
	}

	categories := stored.([]types.Category)

	for _, category := range categories {
		if category.EvidenceRevision != graph.EvidenceRevision ||
			category.Type == types.CategoryTypeNone {
			continue
		}

		if category.Symbol != symbol.Symbol {
			return fmt.Errorf(
				"category symbol %s does not match graph symbol %s",
				category.Symbol,
				symbol.Symbol,
			)
		}

		if err := compiler.addCategoryRelations(
			category, category.Supporting, RelationSupports, graph, index,
		); err != nil {
			return err
		}

		if err := compiler.addCategoryRelations(
			category, category.Opposing, RelationContradicts, graph, index,
		); err != nil {
			return err
		}
	}

	return nil
}

func (compiler *measurementCompiler) addCategoryRelations(
	category types.Category,
	references []string,
	relation RelationType,
	graph *Graph,
	index *measurementIndex,
) error {
	targetID := fmt.Sprintf("cat:%s:%s", category.Symbol, category.Type)

	for _, reference := range references {
		nodes := index.byReference[reference]

		if len(nodes) == 0 {
			return fmt.Errorf("category evidence %s has no measurement node", reference)
		}

		for _, node := range nodes {
			if node.Normalized == nil {
				return fmt.Errorf("category evidence %s has no normalized value", reference)
			}

			weight := math.Abs(*node.Normalized)

			if weight == 0 {
				continue
			}

			graph.AddEdge(&Edge{
				From:         node.ID,
				To:           targetID,
				Relation:     relation,
				Weight:       weight,
				Quality:      cloneFloat(node.Quality),
				Evidence:     []string{node.MeasurementID, reference},
				ObservedFrom: node.ObservedFrom,
				Horizon:      node.Horizon,
				At:           node.At,
				Reason: strings.Join([]string{
					reference, string(relation), string(category.Type),
				}, " "),
			})
		}
	}

	return nil
}
