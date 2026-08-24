package graph

import (
	"fmt"
	"sync"
	"time"

	"github.com/theapemachine/symm/nomagique/relation"
)

/*
EdgeType distinguishes directed temporal Influence from contemporaneous
Association. A zero-lag correlation is Association and is never published as
directed Influence.
*/
type EdgeType uint8

const (
	// EdgeInfluence means past Source improved prediction of later Target
	// under the stated Relation model.
	EdgeInfluence EdgeType = iota
	// EdgeAssociation means contemporaneous dependence; it is not directed
	// temporal Influence.
	EdgeAssociation
)

func (edgeType EdgeType) String() string {
	switch edgeType {
	case EdgeInfluence:
		return "influence"
	case EdgeAssociation:
		return "association"
	default:
		return "unknown"
	}
}

/*
CandidateState represents the estimation lifecycle of one planned edge.
Candidate-but-unavailable is not "no relationship".
*/
type CandidateState uint8

const (
	// CandidateScheduled means the edge is structurally scheduled for estimation.
	CandidateScheduled CandidateState = iota
	// CandidateEstimated means the edge currently has a valid Relation.
	CandidateEstimated
	// CandidateUnavailable means the candidate exists but its estimator is
	// currently undefined.
	CandidateUnavailable
)

func (state CandidateState) String() string {
	switch state {
	case CandidateScheduled:
		return "candidate"
	case CandidateEstimated:
		return "estimated"
	case CandidateUnavailable:
		return "unavailable"
	default:
		return "unknown"
	}
}

/*
InfluenceNode is one Measurement coordinate identity in the Influence Graph.
It preserves metric identity, signal source, symbol, peer, unit, timescale,
and model epoch. Whole signal packages are never collapsed into one node.
*/
type InfluenceNode struct {
	Coordinate relation.Coordinate
}

/*
InfluenceEdge is one valid Relation measurement. It carries the full Relation
result — lag, coefficient, PredictiveGain, coefficient uncertainty, Maturity,
time interval, and epoch/provenance — never a reduced Weight + Confidence.
*/
type InfluenceEdge struct {
	Type   EdgeType
	Source relation.Coordinate
	Target relation.Coordinate
	// Result is the underlying Relation measurement; nil only for
	// Association edges without a Relation estimate.
	Result *relation.InfluenceResult
	From   time.Time
	At     time.Time
	Epoch  uint64
}

/*
edgeKey identifies one directed (Source, Target) edge of one type.
*/
type edgeKey struct {
	edgeType EdgeType
	source   relation.Coordinate
	target   relation.Coordinate
}

/*
edgeHistory is the chronological record of one edge. Current values never
erase historical edge state when relation dynamics are required; retention is
chronological only.
*/
type edgeHistory struct {
	edges []*InfluenceEdge
}

/*
CandidateEdge reports one structurally scheduled edge and its lifecycle state.
*/
type CandidateEdge struct {
	Type   EdgeType
	Source relation.Coordinate
	Target relation.Coordinate
	State  CandidateState
}

/*
InfluenceGraph is a time-indexed observational relation graph over Measurement
coordinates. It stores measured predictive structure; it is not a Structural
Causal Model and never promotes Influence to causal truth automatically.
*/
type InfluenceGraph struct {
	mu              sync.RWMutex
	epoch           uint64
	schemaVersion   uint64
	planVersion     uint64
	historyCapacity int
	nodes           map[relation.Coordinate]InfluenceNode
	edges           map[edgeKey]*edgeHistory
	candidates      map[edgeKey]CandidateState
}

/*
NewInfluenceGraph builds an empty graph for one model epoch. historyCapacity
bounds the per-edge retained history; it is infrastructure provenance.
*/
func NewInfluenceGraph(epoch uint64, schemaVersion uint64, planVersion uint64, historyCapacity int) *InfluenceGraph {
	if historyCapacity < 1 {
		historyCapacity = 1
	}

	return &InfluenceGraph{
		epoch:           epoch,
		schemaVersion:   schemaVersion,
		planVersion:     planVersion,
		historyCapacity: historyCapacity,
		nodes:           make(map[relation.Coordinate]InfluenceNode),
		edges:           make(map[edgeKey]*edgeHistory),
		candidates:      make(map[edgeKey]CandidateState),
	}
}

func (influenceGraph *InfluenceGraph) lock() {
	influenceGraph.mu.Lock()
}

func (influenceGraph *InfluenceGraph) unlock() {
	influenceGraph.mu.Unlock()
}

/*
Epoch returns the graph's model epoch.
*/
func (influenceGraph *InfluenceGraph) Epoch() uint64 {
	if influenceGraph == nil {
		return 0
	}

	influenceGraph.lock()
	defer influenceGraph.unlock()

	return influenceGraph.epoch
}

/*
SchemaVersion returns the node schema version of this graph snapshot.
*/
func (influenceGraph *InfluenceGraph) SchemaVersion() uint64 {
	if influenceGraph == nil {
		return 0
	}

	influenceGraph.lock()
	defer influenceGraph.unlock()

	return influenceGraph.schemaVersion
}

/*
PlanVersion returns the relation-plan version of this graph snapshot.
*/
func (influenceGraph *InfluenceGraph) PlanVersion() uint64 {
	if influenceGraph == nil {
		return 0
	}

	influenceGraph.lock()
	defer influenceGraph.unlock()

	return influenceGraph.planVersion
}

/*
UpsertEdge records one Relation measurement: it updates the node set and
appends to the edge's chronological history. It never deletes an edge because
gain, coefficient, SNR, or Maturity is low, and it never merges edges from an
incompatible epoch.
*/
func (influenceGraph *InfluenceGraph) UpsertEdge(edge *InfluenceEdge) error {
	if influenceGraph == nil || edge == nil {
		return fmt.Errorf("graph: influence graph and edge are required")
	}

	influenceGraph.lock()
	defer influenceGraph.unlock()

	if edge.Epoch != influenceGraph.epoch {
		return fmt.Errorf(
			"graph: edge epoch %d is incompatible with graph epoch %d",
			edge.Epoch, influenceGraph.epoch,
		)
	}

	influenceGraph.nodes[edge.Source] = InfluenceNode{Coordinate: edge.Source}
	influenceGraph.nodes[edge.Target] = InfluenceNode{Coordinate: edge.Target}

	key := edgeKey{edgeType: edge.Type, source: edge.Source, target: edge.Target}
	history := influenceGraph.edges[key]

	if history == nil {
		history = &edgeHistory{}
		influenceGraph.edges[key] = history
	}

	history.edges = append(history.edges, edge)

	if len(history.edges) > influenceGraph.historyCapacity {
		history.edges = history.edges[len(history.edges)-influenceGraph.historyCapacity:]
	}

	influenceGraph.candidates[key] = CandidateEstimated

	return nil
}

/*
RegisterCandidate marks a structurally scheduled edge as a Candidate. It is
how the graph represents planned Relations before estimation.
*/
func (influenceGraph *InfluenceGraph) RegisterCandidate(edgeType EdgeType, source relation.Coordinate, target relation.Coordinate, epoch uint64) error {
	if influenceGraph == nil {
		return fmt.Errorf("graph: influence graph required")
	}

	influenceGraph.lock()
	defer influenceGraph.unlock()

	if epoch != influenceGraph.epoch {
		return fmt.Errorf(
			"graph: candidate epoch %d is incompatible with graph epoch %d",
			epoch, influenceGraph.epoch,
		)
	}

	key := edgeKey{edgeType: edgeType, source: source, target: target}

	if _, exists := influenceGraph.candidates[key]; !exists {
		influenceGraph.candidates[key] = CandidateScheduled
	}

	return nil
}

/*
SetUnavailable marks an existing candidate as unavailable because its
estimator is currently undefined. Unavailable is not "no relationship" and is
not a measured zero.
*/
func (influenceGraph *InfluenceGraph) SetUnavailable(edgeType EdgeType, source relation.Coordinate, target relation.Coordinate, epoch uint64) error {
	if influenceGraph == nil {
		return fmt.Errorf("graph: influence graph required")
	}

	influenceGraph.lock()
	defer influenceGraph.unlock()

	if epoch != influenceGraph.epoch {
		return fmt.Errorf(
			"graph: candidate epoch %d is incompatible with graph epoch %d",
			epoch, influenceGraph.epoch,
		)
	}

	key := edgeKey{edgeType: edgeType, source: source, target: target}
	influenceGraph.candidates[key] = CandidateUnavailable

	return nil
}

/*
Edge returns the current Influence edge between source and target, or nil.
*/
func (influenceGraph *InfluenceGraph) Edge(source relation.Coordinate, target relation.Coordinate) *InfluenceEdge {
	return influenceGraph.currentEdge(EdgeInfluence, source, target)
}

/*
CurrentEdge returns the current edge of the given type between source and
target, or nil.
*/
func (influenceGraph *InfluenceGraph) CurrentEdge(edgeType EdgeType, source relation.Coordinate, target relation.Coordinate) *InfluenceEdge {
	return influenceGraph.currentEdge(edgeType, source, target)
}

func (influenceGraph *InfluenceGraph) currentEdge(edgeType EdgeType, source relation.Coordinate, target relation.Coordinate) *InfluenceEdge {
	if influenceGraph == nil {
		return nil
	}

	influenceGraph.lock()
	defer influenceGraph.unlock()

	history := influenceGraph.edges[edgeKey{edgeType: edgeType, source: source, target: target}]

	if history == nil || len(history.edges) == 0 {
		return nil
	}

	return history.edges[len(history.edges)-1]
}

/*
NodeCount returns the number of coordinate nodes.
*/
func (influenceGraph *InfluenceGraph) NodeCount() int {
	if influenceGraph == nil {
		return 0
	}

	influenceGraph.lock()
	defer influenceGraph.unlock()

	return len(influenceGraph.nodes)
}

/*
EdgeCount returns the number of current (latest-state) edges across all types.
*/
func (influenceGraph *InfluenceGraph) EdgeCount() int {
	if influenceGraph == nil {
		return 0
	}

	influenceGraph.lock()
	defer influenceGraph.unlock()

	return len(influenceGraph.edges)
}
