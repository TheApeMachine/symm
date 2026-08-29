package graph

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/network"
	"github.com/theapemachine/symm/nomagique/relation"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/types"
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
It preserves metric identity, signal source, symbol, peer, unit, timescale, and
model epoch. Whole signal packages are never collapsed into one node.
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
CandidateEdge reports one structurally scheduled edge and its lifecycle state.
*/
type CandidateEdge struct {
	Type   EdgeType
	Source relation.Coordinate
	Target relation.Coordinate
	State  CandidateState
}

/*
edgeKey identifies one directed (Source, Target) edge of one type. It is the
composite identity under which an edge and its history are stored.
*/
type edgeKey struct {
	edgeType EdgeType
	source   relation.Coordinate
	target   relation.Coordinate
}

/*
edgeData is the storage payload for one directed edge: its lifecycle state plus
the chronological history of Relation measurements. Retention is chronological
only, bounded by the graph's history capacity.
*/
type edgeData struct {
	state   CandidateState
	history []*InfluenceEdge
}

/*
InfluenceGraph is a time-indexed observational relation graph over Measurement
coordinates. It stores measured predictive structure; it is not a Structural
Causal Model and never promotes Influence to causal truth automatically.

It is backed by the lock-free generic network.Graph, so node and edge updates
are lock-free and the graph is updated in place as measurements stream in, never
rebuilt from scratch.
*/
type InfluenceGraph struct {
	epoch           uint64
	schemaVersion   uint64
	planVersion     uint64
	historyCapacity int

	edgeMutexes [64]sync.Mutex
	nodes       *network.Graph[relation.Coordinate, InfluenceNode, struct{}]
	edges       *network.Graph[edgeKey, edgeData, struct{}]
}

func (influenceGraph *InfluenceGraph) edgeLock(key edgeKey) *sync.Mutex {
	hash := uint64(key.edgeType)

	for index := 0; index < len(key.source.Symbol); index++ {
		hash = hash*31 + uint64(key.source.Symbol[index])
	}

	for index := 0; index < len(key.target.Symbol); index++ {
		hash = hash*31 + uint64(key.target.Symbol[index])
	}

	return &influenceGraph.edgeMutexes[hash%uint64(len(influenceGraph.edgeMutexes))]
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
		nodes: network.NewGraph[relation.Coordinate, InfluenceNode, struct{}](
			func(left relation.Coordinate, right relation.Coordinate) bool {
				// CompareCoordinate is the allocation-free field-wise order;
				// no rendered identity string is ever materialized here.
				return relation.CompareCoordinate(left, right) < 0
			},
		),
		edges: network.NewGraph[edgeKey, edgeData, struct{}](
			edgeKeyOrder,
		),
	}
}

/*
edgeKeyOrder is the strict weak order over edge keys: the two coordinates are
compared field-wise via the allocation-free CompareCoordinate primitive (source
first, then target), and ties break on the edge type. No string is materialized.
*/
func edgeKeyOrder(left edgeKey, right edgeKey) bool {
	if cmp := relation.CompareCoordinate(left.source, right.source); cmp != 0 {
		return cmp < 0
	}

	if cmp := relation.CompareCoordinate(left.target, right.target); cmp != 0 {
		return cmp < 0
	}

	return left.edgeType < right.edgeType
}

/*
getEdge returns the stored edgeData for a key and whether it exists.
*/
func (influenceGraph *InfluenceGraph) getEdge(key edgeKey) (edgeData, bool) {
	node, found := influenceGraph.edges.Node(key)

	if !found {
		return edgeData{}, false
	}

	return node.Data, true
}

/*
setEdge stores the edgeData under its key.
*/
func (influenceGraph *InfluenceGraph) setEdge(key edgeKey, data edgeData) {
	influenceGraph.edges.SetNode(network.Node[edgeKey, edgeData]{ID: key, Data: data})
}

/*
maxCoordinate is a sentinel coordinate that sorts after every real coordinate
under relation.CompareCoordinate. It is the full-walk upper bound for Range.
*/
func maxCoordinate() relation.Coordinate {
	return relation.Coordinate{
		Symbol:    "\xff",
		Source:    "\xff",
		Metric:    "\xff",
		Side:      "\xff",
		Peer:      "\xff",
		Unit:      nmtypes.Unit(0xff),
		Timescale: nmtypes.Timescale(0xff),
		Epoch:     ^uint64(0),
	}
}

/*
maxEdgeKey is a sentinel edge key that sorts after every real key.
*/
func maxEdgeKey() edgeKey {
	return edgeKey{
		edgeType: EdgeType(0xff),
		source:   maxCoordinate(),
		target:   maxCoordinate(),
	}
}

/*
Epoch returns the graph's model epoch.
*/
func (influenceGraph *InfluenceGraph) Epoch() uint64 {
	if influenceGraph == nil {
		return 0
	}

	return influenceGraph.epoch
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

	if edge.Epoch != influenceGraph.epoch {
		return fmt.Errorf(
			"graph: edge epoch %d is incompatible with graph epoch %d",
			edge.Epoch, influenceGraph.epoch,
		)
	}

	influenceGraph.nodes.SetNode(network.Node[relation.Coordinate, InfluenceNode]{
		ID:   edge.Source,
		Data: InfluenceNode{Coordinate: edge.Source},
	})
	influenceGraph.nodes.SetNode(network.Node[relation.Coordinate, InfluenceNode]{
		ID:   edge.Target,
		Data: InfluenceNode{Coordinate: edge.Target},
	})

	key := edgeKey{edgeType: edge.Type, source: edge.Source, target: edge.Target}
	edgeMutex := influenceGraph.edgeLock(key)
	edgeMutex.Lock()
	defer edgeMutex.Unlock()

	current, _ := influenceGraph.getEdge(key)

	history := current.history
	history = append(history, edge)

	if len(history) > influenceGraph.historyCapacity {
		history = history[len(history)-influenceGraph.historyCapacity:]
	}

	influenceGraph.setEdge(key, edgeData{state: CandidateEstimated, history: history})

	return nil
}

/*
RegisterCandidate marks a structurally scheduled edge as a Candidate. It is how
the graph represents planned Relations before estimation.
*/
func (influenceGraph *InfluenceGraph) RegisterCandidate(edgeType EdgeType, source relation.Coordinate, target relation.Coordinate, epoch uint64) error {
	if influenceGraph == nil {
		return fmt.Errorf("graph: influence graph required")
	}

	if epoch != influenceGraph.epoch {
		return fmt.Errorf(
			"graph: candidate epoch %d is incompatible with graph epoch %d",
			epoch, influenceGraph.epoch,
		)
	}

	key := edgeKey{edgeType: edgeType, source: source, target: target}
	edgeMutex := influenceGraph.edgeLock(key)
	edgeMutex.Lock()
	defer edgeMutex.Unlock()

	if _, found := influenceGraph.getEdge(key); !found {
		influenceGraph.setEdge(key, edgeData{state: CandidateScheduled})
	}

	return nil
}

/*
SetUnavailable marks an existing candidate as unavailable because its estimator
is currently undefined. Unavailable is not "no relationship" and is not a
measured zero.
*/
func (influenceGraph *InfluenceGraph) SetUnavailable(edgeType EdgeType, source relation.Coordinate, target relation.Coordinate, epoch uint64) error {
	if influenceGraph == nil {
		return fmt.Errorf("graph: influence graph required")
	}

	if epoch != influenceGraph.epoch {
		return fmt.Errorf(
			"graph: candidate epoch %d is incompatible with graph epoch %d",
			epoch, influenceGraph.epoch,
		)
	}

	key := edgeKey{edgeType: edgeType, source: source, target: target}
	edgeMutex := influenceGraph.edgeLock(key)
	edgeMutex.Lock()
	defer edgeMutex.Unlock()

	current, found := influenceGraph.getEdge(key)

	if !found {
		return fmt.Errorf(
			"graph: cannot mark unavailable a candidate that was never registered (%s -> %s)",
			source.ID(), target.ID(),
		)
	}

	current.state = CandidateUnavailable
	influenceGraph.setEdge(key, current)

	return nil
}

/*
Relation returns the current Influence edge between source and target, or nil.
*/
func (influenceGraph *InfluenceGraph) Relation(source relation.Coordinate, target relation.Coordinate) *InfluenceEdge {
	return influenceGraph.currentEdge(EdgeInfluence, source, target)
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

	data, found := influenceGraph.getEdge(edgeKey{edgeType: edgeType, source: source, target: target})

	if !found || len(data.history) == 0 {
		return nil
	}

	return data.history[len(data.history)-1]
}

/*
Incoming returns the current Influence edges whose target is the coordinate.
*/
func (influenceGraph *InfluenceGraph) Incoming(target relation.Coordinate) []*InfluenceEdge {
	return influenceGraph.currentEdges(func(edge *InfluenceEdge) bool {
		return edge.Target == target && edge.Type == EdgeInfluence
	})
}

/*
Outgoing returns the current Influence edges whose source is the coordinate.
*/
func (influenceGraph *InfluenceGraph) Outgoing(source relation.Coordinate) []*InfluenceEdge {
	return influenceGraph.currentEdges(func(edge *InfluenceEdge) bool {
		return edge.Source == source && edge.Type == EdgeInfluence
	})
}

/*
History returns the chronological Relation history of one edge, oldest first.
*/
func (influenceGraph *InfluenceGraph) History(source relation.Coordinate, target relation.Coordinate) []*InfluenceEdge {
	return influenceGraph.history(EdgeInfluence, source, target)
}

/*
HistoryOf returns the chronological history of one typed edge, oldest first.
*/
func (influenceGraph *InfluenceGraph) HistoryOf(edgeType EdgeType, source relation.Coordinate, target relation.Coordinate) []*InfluenceEdge {
	return influenceGraph.history(edgeType, source, target)
}

func (influenceGraph *InfluenceGraph) history(edgeType EdgeType, source relation.Coordinate, target relation.Coordinate) []*InfluenceEdge {
	if influenceGraph == nil {
		return nil
	}

	data, found := influenceGraph.getEdge(edgeKey{edgeType: edgeType, source: source, target: target})

	if !found {
		return nil
	}

	return append([]*InfluenceEdge(nil), data.history...)
}

/*
compareCandidateEdges orders CandidateEdge values by the graph's canonical
edgeKey order (source, target, type) built on the allocation-free
CompareCoordinate primitive. No identity string is ever materialized.
*/
func compareCandidateEdges(left CandidateEdge, right CandidateEdge) int {
	if cmp := relation.CompareCoordinate(left.Source, right.Source); cmp != 0 {
		return cmp
	}

	if cmp := relation.CompareCoordinate(left.Target, right.Target); cmp != 0 {
		return cmp
	}

	if left.Type < right.Type {
		return -1
	}

	if left.Type > right.Type {
		return 1
	}

	return 0
}

/*
compareInfluenceEdges orders InfluenceEdge pointers by the graph's canonical
edgeKey order (source, target, type) built on the allocation-free
CompareCoordinate primitive. No identity string is ever materialized.
*/
func compareInfluenceEdges(left *InfluenceEdge, right *InfluenceEdge) int {
	if cmp := relation.CompareCoordinate(left.Source, right.Source); cmp != 0 {
		return cmp
	}

	if cmp := relation.CompareCoordinate(left.Target, right.Target); cmp != 0 {
		return cmp
	}

	if left.Type < right.Type {
		return -1
	}

	if left.Type > right.Type {
		return 1
	}

	return 0
}

/*
Candidates returns every structurally scheduled edge with its lifecycle state.
*/
func (influenceGraph *InfluenceGraph) Candidates() []CandidateEdge {
	if influenceGraph == nil {
		return nil
	}

	var candidates []CandidateEdge

	// A single Range pass reads each node's ID and Data together; the
	// previous edgeKeys()+getEdge() form walked the skip list once to
	// collect every key, then performed one more O(log N) findNode search
	// per key to re-fetch data already visited during that walk.
	influenceGraph.edges.Range(edgeKey{}, maxEdgeKey(), func(node network.Node[edgeKey, edgeData]) {
		candidates = append(candidates, CandidateEdge{
			Type:   node.ID.edgeType,
			Source: node.ID.source,
			Target: node.ID.target,
			State:  node.Data.state,
		})
	})

	slices.SortFunc(candidates, compareCandidateEdges)

	return candidates
}

/*
Nodes returns every coordinate node with retained data, in coordinate order.
*/
func (influenceGraph *InfluenceGraph) Nodes() []InfluenceNode {
	if influenceGraph == nil {
		return nil
	}

	result := make([]InfluenceNode, 0)

	influenceGraph.nodes.Range(relation.Coordinate{}, maxCoordinate(), func(node network.Node[relation.Coordinate, InfluenceNode]) {
		result = append(result, node.Data)
	})

	return result
}

/*
Edges returns every current edge across all types, retaining full Relation
statistics.
*/
func (influenceGraph *InfluenceGraph) Edges() []*InfluenceEdge {
	return influenceGraph.currentEdges(func(edge *InfluenceEdge) bool {
		return true
	})
}

func (influenceGraph *InfluenceGraph) currentEdges(predicate func(*InfluenceEdge) bool) []*InfluenceEdge {
	if influenceGraph == nil {
		return nil
	}

	var edges []*InfluenceEdge

	// See Candidates: one Range pass replaces the previous
	// edgeKeys()+getEdge() double walk.
	influenceGraph.edges.Range(edgeKey{}, maxEdgeKey(), func(node network.Node[edgeKey, edgeData]) {
		if len(node.Data.history) == 0 {
			return
		}

		edge := node.Data.history[len(node.Data.history)-1]

		if edge.Type != node.ID.edgeType {
			return
		}

		if predicate(edge) {
			edges = append(edges, edge)
		}
	})

	slices.SortFunc(edges, compareInfluenceEdges)

	return edges
}

/*
NodeCount returns the number of coordinate nodes.
*/
func (influenceGraph *InfluenceGraph) NodeCount() int {
	if influenceGraph == nil {
		return 0
	}

	return influenceGraph.nodes.Len()
}

/*
EdgeCount returns the number of current (latest-state) edges across all types.
*/
func (influenceGraph *InfluenceGraph) EdgeCount() int {
	if influenceGraph == nil {
		return 0
	}

	return influenceGraph.edges.Len()
}

/*
Solver is the Influence Graph stage of the observational pipeline. It owns the
coordinate store, the Relation estimator, the candidate relation plan, and the
Influence Graph, and it processes one Measurement at a time via Step. Step is an
incrementing iterator: it appends observations, evaluates the candidate plan,
and updates the Influence Graph in place — never rebuilding it from scratch. It
is a pure domain processor: no UI frames, wire projections, or diagnostic timing
live here, exactly as signals carry none.
*/
type Solver struct {
	ctx           context.Context
	cancel        context.CancelFunc
	err           error
	status        *runtime.Status
	epoch         uint64
	store         *relation.ObservationStore
	estimator     *relation.InfluenceEstimator
	influence     *InfluenceGraph
	plans         []*relation.RelationPlan
	interval      time.Duration
	lastRun       sync.Map
	compiledPlans sync.Map
	ObserveModule func(string, time.Duration)
}

/*
GraphUpdate is the domain event the solver publishes after each Measurement
advances the Influence Graph. It carries only the symbol and as-of time;
consumers read the shared Influence Graph for the data itself. Defined in
types so types.Envelope can carry it without an import cycle back to graph.
*/
type GraphUpdate = types.GraphUpdate

/*
SolverOption configures one graph-solver property at construction.
*/
type SolverOption func(*Solver)

/*
WithInterval overrides the per-symbol Relation refresh cadence. A non-positive
value disables the cadence gate (every observation re-estimates, matching the
solver's own due() semantics). The default is 100ms.
*/
func WithInterval(interval time.Duration) SolverOption {
	return func(solver *Solver) {
		solver.interval = interval
	}
}

/*
NewSolver builds the graph stage. historyCapacity bounds each coordinate's
retained observations (infrastructure provenance). schemaVersion and the plans'
versions identify the graph snapshot. opts override the default Relation refresh
cadence.
*/
func NewSolver(
	ctx context.Context,
	epoch uint64,
	historyCapacity int,
	plans []*relation.RelationPlan,
	schemaVersion uint64,
	opts ...SolverOption,
) *Solver {
	ctx, cancel := context.WithCancel(ctx)

	solver := &Solver{
		ctx:       ctx,
		cancel:    cancel,
		status:    runtime.NewStatus(),
		epoch:     epoch,
		store:     relation.NewObservationStore(historyCapacity),
		estimator: relation.NewInfluenceEstimator("prequential-linear-v1"),
		influence: NewInfluenceGraph(epoch, schemaVersion, planVersion(plans), 64),
		plans:     plans,
		interval:  100 * time.Millisecond,
	}

	for _, opt := range opts {
		opt(solver)
	}

	return solver
}

/*
Name returns the stage's subscription identity.
*/
func (solver *Solver) Name() string { return "graph" }

/*
Error returns the subscription step failure, if any.
*/
func (solver *Solver) Error() error { return solver.err }

/*
due reports whether the Relation refresh interval has elapsed for a symbol.
*/
func (solver *Solver) due(symbol string, at time.Time) bool {
	if solver.interval <= 0 {
		return true
	}

	lastVal, ran := solver.lastRun.Load(symbol)

	if !ran {
		solver.lastRun.Store(symbol, at)
		return true
	}

	last, ok := lastVal.(time.Time)

	if !ok || at.Sub(last) >= solver.interval {
		solver.lastRun.Store(symbol, at)
		return true
	}

	return false
}

/*
Step folds every signal measurement populated on this envelope into the
coordinate store, re-estimating the planned Relations for its symbol when due
and updating the Influence Graph in place, then writes the last resulting
GraphUpdate back onto the envelope.
*/
func (solver *Solver) Step(envelope *types.Envelope) *types.Envelope {
	measurements := []*data.Measurement[float64]{
		envelope.Correlation,
		envelope.LeadLag,
		envelope.Liquidity,
		envelope.Sentiment,
		envelope.CVD,
		envelope.DepthFlow,
		envelope.Morphology,
	}

	for _, measurement := range measurements {
		if measurement == nil {
			continue
		}

		if update := solver.StepMeasurement(measurement.ToTypesMeasurement()); update != nil {
			envelope.GraphUpdate = update
		}
	}

	return envelope
}

/*
StepMeasurement appends one Measurement to the coordinate store, re-estimates
the planned Relations for its symbol when due, and updates the Influence Graph
in place. The workspace delivers values for one symbol in order, so the store
and graph advance as data becomes available.
*/
func (solver *Solver) StepMeasurement(measurement *nmtypes.Measurement) *GraphUpdate {
	if solver == nil {
		return nil
	}

	started := time.Now()
	defer func() {
		if solver.ObserveModule != nil {
			solver.ObserveModule("graph", time.Since(started))
		}
	}()

	observations := relation.AppendMeasurement(measurement, solver.epoch)

	if len(observations) == 0 {
		return nil
	}

	solver.store.AppendObservations(observations)

	if solver.due(measurement.Symbol, measurement.At) {
		solver.estimate(measurement.Symbol)
	}

	return &GraphUpdate{
		Symbol: measurement.Symbol,
		At:     measurement.At,
	}
}

/*
Store exposes the observational coordinate store.
*/
func (solver *Solver) Store() *relation.ObservationStore {
	if solver == nil {
		return nil
	}

	return solver.store
}

/*
Graph exposes the Influence Graph.
*/
func (solver *Solver) Graph() *InfluenceGraph {
	if solver == nil {
		return nil
	}

	return solver.influence
}

/*
Close cleans up the solver.
*/
func (solver *Solver) Close() error {
	if solver == nil {
		return nil
	}

	solver.cancel()
	return nil
}

/*
SharedObservationStore and SharedInfluenceGraph are the shared-object names the
graph stage registers its authoritative coordinate store and Influence Graph
under. Downstream stages read them through the workspace.
*/
const (
	SharedObservationStore = "relation_store"
	SharedInfluenceGraph   = "influence_graph"
)

/*
planVersion returns the highest relation-plan version, which participates in the
graph snapshot identity.
*/
func planVersion(plans []*relation.RelationPlan) uint64 {
	version := uint64(0)

	for _, plan := range plans {
		if plan != nil && plan.Version > version {
			version = plan.Version
		}
	}

	return version
}

type compiledPlanEntry struct {
	coordinateCount int
	candidates      []relation.CompiledCandidate
}

/*
estimate evaluates every planned pair for one symbol and records the Influence
edges using precompiled candidate topology.
*/
func (solver *Solver) estimate(symbol string) {
	coordinateCount := solver.store.CoordinateCount()

	var candidates []relation.CompiledCandidate

	if cachedValue, found := solver.compiledPlans.Load(symbol); found {
		entry := cachedValue.(compiledPlanEntry)

		if entry.coordinateCount == coordinateCount {
			candidates = entry.candidates
		}
	}

	if candidates == nil {
		candidates = relation.CompilePlansForSymbol(solver.plans, symbol, solver.epoch, solver.store)
		solver.compiledPlans.Store(symbol, compiledPlanEntry{
			coordinateCount: coordinateCount,
			candidates:      candidates,
		})
	}

	for _, candidate := range candidates {
		solver.estimateCandidate(candidate)
	}
}

/*
estimateCandidate estimates one precompiled candidate pair and records the Influence edge.
*/
func (solver *Solver) estimateCandidate(candidate relation.CompiledCandidate) {
	_ = solver.influence.RegisterCandidate(EdgeInfluence, candidate.Source, candidate.Target, solver.epoch)

	if !candidate.ControlsComplete {
		_ = solver.influence.SetUnavailable(EdgeInfluence, candidate.Source, candidate.Target, solver.epoch)
		return
	}

	result, err := solver.estimator.Estimate(solver.store, relation.InfluenceRequest{
		Source:   candidate.Source,
		Target:   candidate.Target,
		Controls: candidate.Controls,
		Lag:      candidate.Lag,
	})

	if err != nil {
		return
	}

	if result.Defined() {
		_ = solver.influence.UpsertEdge(&InfluenceEdge{
			Type:   EdgeInfluence,
			Source: candidate.Source,
			Target: candidate.Target,
			Result: result,
			From:   result.From,
			At:     result.At,
			Epoch:  solver.epoch,
		})

		return
	}

	_ = solver.influence.SetUnavailable(EdgeInfluence, candidate.Source, candidate.Target, solver.epoch)
}

/*
InfluenceGraphWire projects the Influence Graph's subgraph for one symbol into
the dashboard GraphFrameT. It is a free projection function, not a method on the
domain type, exactly as ResonanceWire projects a ResonanceArtifact: the domain
graph carries no wire knowledge, and the projection runs only when the boot
side-effect observer needs to render the focused symbol.

The view is read-only and reversible: every node is a Measurement coordinate
identity (never a collapsed signal rollup) stamped with the store's latest
observation — Raw as Value, its magnitude as Strength, Maturity as Confidence,
and the observation instant as At — and a coordinate with no retained
observation renders as its identity alone. The edge set mirrors the backend's
full structural state: a fitted edge carries the measured Relation (signed
coefficient as Weight, Maturity as Confidence, lag/PredictiveGain/estimator as
the reason), while a scheduled-but-unfitted candidate renders as a
state-annotated edge (Derived=true, zero weight/confidence) so the graph is
never empty simply because a Relation has not yet been estimated.
*/
func InfluenceGraphWire(
	store *relation.ObservationStore,
	influenceGraph *InfluenceGraph,
	symbol string,
	at time.Time,
) *wire.GraphFrameT {
	if influenceGraph == nil {
		return nil
	}

	// The frontend must mirror whatever the backend currently holds, and the
	// backend knows more than just successfully fitted relations: it holds a
	// full scheduled candidate set with per-edge lifecycle state. Projecting
	// only defined edges would render an empty graph whenever no Relation
	// has yet reached a fit — a faithful view needs the candidates too.
	//
	// Every stored edgeData node already carries both the candidate state and
	// the measured history together, so the candidate view and the defined
	// view are two projections of the same node, not two separate data sets.
	// A single Range pass produces both instead of walking Candidates() and
	// Edges() (which itself walks the same tree) three times over: once to
	// build a defined-edge lookup map, once for the candidate/edge pairing,
	// and once more for orphan defined-edge endpoints.
	nodes := make([]*wire.GraphNodeT, 0)
	seenNodes := make(map[relation.Coordinate]bool)
	edges := make([]*wire.GraphEdgeT, 0, influenceGraph.edges.Len())

	emitNode := func(coordinate relation.Coordinate) {
		if coordinate.Symbol != symbol || seenNodes[coordinate] {
			return
		}

		seenNodes[coordinate] = true
		nodes = append(nodes, influenceNodeWire(store, coordinate))
	}

	influenceGraph.edges.Range(edgeKey{}, maxEdgeKey(), func(node network.Node[edgeKey, edgeData]) {
		key := node.ID
		data := node.Data

		// Defined-edge endpoints render as nodes even when the pair falls
		// outside the candidate plan (Association edges, or a partner the
		// current plan no longer schedules).
		if key.source.Symbol == symbol {
			emitNode(key.source)
		}

		if key.target.Symbol == symbol {
			emitNode(key.target)
		}

		if key.source.Symbol != symbol || key.target.Symbol != symbol {
			return
		}

		if len(data.history) > 0 {
			latest := data.history[len(data.history)-1]

			if latest.Type == key.edgeType {
				edges = append(edges, influenceEdgeWire(latest))
				return
			}
		}

		edges = append(edges, &wire.GraphEdgeT{
			From:       key.source.ID(),
			To:         key.target.ID(),
			Relation:   key.edgeType.String(),
			Reason:     "state=" + data.state.String(),
			Derived:    data.state != CandidateEstimated,
			At:         at.UnixNano(),
			Confidence: 0,
			Weight:     0,
		})
	})

	return &wire.GraphFrameT{
		At:    at.UnixNano(),
		Nodes: nodes,
		Edges: edges,
	}
}

func influenceNodeWire(store *relation.ObservationStore, coordinate relation.Coordinate) *wire.GraphNodeT {
	node := &wire.GraphNodeT{
		Id:     coordinate.ID(),
		Symbol: coordinate.Symbol,
		Peer:   coordinate.Peer,
		Source: coordinate.Source,
		Metric: coordinate.Metric,
		Side:   coordinate.Side,
		Kind:   "measurement",
		Unit:   coordinate.Unit.String(),
		Metadata: &wire.GraphMetadataT{
			Strings: []*wire.NamedStringT{
				{Name: "timescale", Value: coordinate.Timescale.String()},
			},
			Numbers: []*wire.NamedNumberT{
				{Name: "epoch", Value: float64(coordinate.Epoch)},
			},
		},
	}

	if store == nil {
		return node
	}

	observation, found := store.Latest(coordinate)

	if !found {
		return node
	}

	node.Value = observation.Raw
	node.Strength = math.Abs(observation.Raw)
	node.Confidence = observation.Maturity
	node.At = observation.At.UnixNano()

	return node
}

func influenceEdgeWire(edge *InfluenceEdge) *wire.GraphEdgeT {
	weight := 0.0
	confidence := 0.0
	reason := "influence"

	if edge.Result != nil {
		confidence = edge.Result.Maturity

		if edge.Result.Coefficient != nil {
			weight = *edge.Result.Coefficient
		}

		gain := "undefined"

		if edge.Result.PredictiveGain != nil {
			gain = strconv.FormatFloat(*edge.Result.PredictiveGain, 'f', 4, 64)
		}

		reason = "lag=" + edge.Result.Lag.String() +
			" gain=" + gain +
			" estimator=" + edge.Result.EstimatorVersion
	}

	return &wire.GraphEdgeT{
		From:       edge.Source.ID(),
		To:         edge.Target.ID(),
		Relation:   edge.Type.String(),
		Weight:     weight,
		Confidence: confidence,
		At:         edge.At.UnixNano(),
		Reason:     reason,
	}
}
