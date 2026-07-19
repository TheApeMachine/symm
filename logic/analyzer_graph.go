package logic

import (
	"math"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/types"
)

/*
composeGraphs inserts measurements into per-symbol evidence graphs and drops
graphs that produce no edges after Compose.
*/
func (analyzer *Analyzer) composeGraphs(thesis *types.Thesis) {
	graphsStarted := time.Now()

	for _, measurement := range thesis.Measurements {
		if measurement == nil {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"analyzer received a nil measurement",
				nil,
			))

			continue
		}

		value, found := thesis.Graphs.Load(measurement.Symbol)

		if !found {
			value = types.NewGraph(measurement.Symbol)
			thesis.Graphs.Store(measurement.Symbol, value)
		}

		evidenceGraph := value.(*types.Graph)

		if err := evidenceGraph.AddNode(measurement); err != nil {
			errnie.Error(err)
			continue
		}
	}

	thesis.Graphs.Range(func(_, value any) bool {
		value.(*types.Graph).Compose()
		return true
	})

	analyzer.relateLeadLag(thesis)
	analyzer.relateCausal(thesis)

	thesis.Graphs.Range(func(key, value any) bool {
		if len(value.(*types.Graph).Edges()) == 0 {
			thesis.Graphs.Delete(key)
		}

		return true
	})

	errnie.Error(audit.Phase(analyzer.recorder, thesis.Tick, "graphs", map[string]any{
		"measurements": len(thesis.Measurements),
		"ns":           time.Since(graphsStarted).Nanoseconds(),
	}))
}

/*
relateLeadLag draws directed Leads/Lags edges between an anchor and its
followers. A follower's signed_lag_direction node names its Peer (the anchor)
and carries +1 (anchor leads) or -1 (follower leads). Because the graph is
symbol-local, the anchor's counterpart node is borrowed into the follower graph
as a referenced peer so the directed edge has two real endpoints.
*/
func (analyzer *Analyzer) relateLeadLag(thesis *types.Thesis) {
	thesis.Graphs.Range(func(_, value any) bool {
		follower, ok := value.(*types.Graph)

		if !ok || follower == nil {
			return true
		}

		for _, node := range follower.Nodes() {
			if node.Kind != types.NodeMeasurement ||
				node.Measurement.Metric != types.MetricSignedLagDirection ||
				node.Measurement.Peer == "" ||
				node.Measurement.Normalized == nil {
				continue
			}

			analyzer.relateLeadLagEdge(thesis, follower, node)
		}

		return true
	})
}

/*
relateLeadLagEdge stages the anchor counterpart and draws the directed edge for
one follower direction reading.
*/
func (analyzer *Analyzer) relateLeadLagEdge(
	thesis *types.Thesis,
	follower *types.Graph,
	node *types.Node,
) {
	peerValue, found := thesis.Graphs.Load(node.Measurement.Peer)

	if !found {
		return
	}

	anchorGraph, ok := peerValue.(*types.Graph)

	if !ok || anchorGraph == nil {
		return
	}

	anchorNode := leadLagAnchorNode(anchorGraph)

	if anchorNode == nil {
		return
	}

	anchorKey := follower.StagePeerNode(anchorNode.Measurement)
	observedFrom, _ := node.Measurement.Interval()
	at := node.Measurement.At

	// +1: anchor leads the follower; -1: follower leads the anchor.
	if *node.Measurement.Normalized > 0 {
		follower.Relate(anchorKey, node.Key, types.Leads, at, observedFrom)
		follower.Relate(node.Key, anchorKey, types.Lags, at, observedFrom)

		return
	}

	follower.Relate(node.Key, anchorKey, types.Leads, at, observedFrom)
	follower.Relate(anchorKey, node.Key, types.Lags, at, observedFrom)
}

/*
relateCausal projects each ready causal hypothesis onto its symbol's graph as a
directed Conditions edge from the treatment concept to the outcome concept. Only
hypotheses that are ready and carry a finite, non-zero interventional expectation
are drawn, so a hypothesis with no established effect lights no edge.
*/
func (analyzer *Analyzer) relateCausal(thesis *types.Thesis) {
	for index := range thesis.Hypotheses {
		hypothesis := thesis.Hypotheses[index]

		if !hypothesis.Ready || hypothesis.Symbol == "" ||
			hypothesis.Treatment == "" || hypothesis.Outcome == "" {
			continue
		}

		if !isFiniteNonZero(hypothesis.DoExpectation) &&
			!isFiniteNonZero(hypothesis.Uplift) {
			continue
		}

		value, found := thesis.Graphs.Load(hypothesis.Symbol)

		if !found {
			continue
		}

		evidenceGraph, ok := value.(*types.Graph)

		if !ok || evidenceGraph == nil {
			continue
		}

		evidenceGraph.RelateConditions(
			hypothesis.Treatment, hypothesis.Outcome,
			hypothesis.At, hypothesis.At,
		)
	}
}

/*
isFiniteNonZero reports whether a causal effect estimate is usable evidence.
*/
func isFiniteNonZero(value float64) bool {
	return value != 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

/*
leadLagAnchorNode returns the anchor's own lead-lag reference node: its signed
direction node when present, otherwise its strength node, so the borrowed peer
is a real anchor observation rather than an invented placeholder.
*/
func leadLagAnchorNode(anchorGraph *types.Graph) *types.Node {
	var strength *types.Node

	for _, node := range anchorGraph.Nodes() {
		if node.Kind != types.NodeMeasurement ||
			node.Measurement.Source != types.SourceLeadLag {
			continue
		}

		if node.Measurement.Metric == types.MetricSignedLagDirection {
			return node
		}

		if node.Measurement.Metric == types.MetricStrength {
			strength = node
		}
	}

	return strength
}
