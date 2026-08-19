package graph

import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/mcts"
	"github.com/theapemachine/symm/types"
)

/*
ReasoningTier separates evidence, physical context, structural variables, and
strategic propositions without changing the evidence graph's native topology.
*/
type ReasoningTier string

const (
	ReasoningTierMeasurement ReasoningTier = "measurement"
	ReasoningTierField       ReasoningTier = "field"
	ReasoningTierSCM         ReasoningTier = "scm"
	ReasoningTierDecision    ReasoningTier = "decision"
)

/*
ReasoningNode is the visual projection of one evidence or derived SCM node.
*/
type ReasoningNode struct {
	ID         string                 `json:"id"`
	Label      string                 `json:"label"`
	Symbol     string                 `json:"symbol,omitempty"`
	Tier       ReasoningTier          `json:"tier"`
	Role       string                 `json:"role,omitempty"`
	Source     string                 `json:"source,omitempty"`
	Value      float64                `json:"value"`
	Confidence float64                `json:"confidence"`
	Derived    bool                   `json:"derived"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

/*
ReasoningLink makes causal role and evidence provenance explicit in the UI.
*/
type ReasoningLink struct {
	From       string  `json:"from"`
	To         string  `json:"to"`
	Relation   string  `json:"relation"`
	Weight     float64 `json:"weight"`
	Confidence float64 `json:"confidence"`
	Derived    bool    `json:"derived"`
}

/*
ReasoningTopology is an inspectable four-tier projection over the market graph.
*/
type ReasoningTopology struct {
	Symbol         string             `json:"symbol,omitempty"`
	Ready          bool               `json:"ready"`
	Reason         string             `json:"reason"`
	ObservedRows   int                `json:"observedRows"`
	MaximumHorizon int                `json:"maximumHorizon"`
	Treatment      string             `json:"treatment"`
	Mediator       string             `json:"mediator"`
	Target         string             `json:"target"`
	Controls       []string           `json:"controls"`
	CurrentState   map[string]float64 `json:"currentState"`
	Nodes          []ReasoningNode    `json:"nodes"`
	Links          []ReasoningLink    `json:"links"`
}

type reasoningAggregate struct {
	weightedValue float64
	confidence    float64
}

func (aggregate *reasoningAggregate) Add(value float64, confidence float64) {
	aggregate.weightedValue += value * confidence
	aggregate.confidence += confidence
}

func (aggregate *reasoningAggregate) Mean() float64 {
	if aggregate.confidence == 0 {
		return 0
	}

	return aggregate.weightedValue / aggregate.confidence
}

type reasoningNodeView struct {
	ID            string
	Symbol        string
	Source        string
	MeasurementID string
	Metric        string
	Value         float64
	Confidence    float64
	Metadata      map[string]interface{}
}

/*
ReasoningFrame compiles graph evidence into named market variables. It is the
simulation state, not an observational causal row with a fabricated outcome.

The archetype is the identifiable opportunity the graph currently expresses;
the physical fields (velocity/energy/causal expectation/spread) are the
trust-attenuated readings the archetype's rollout dynamics consume.
*/
func (graph *Graph) ReasoningFrame() nomagique.Frame {
	frame := nomagique.Frame{}

	now := time.Now().UTC()
	active := graph.ActiveOpportunity(now)

	frame.Put(mcts.SymbolContextConfidence, graph.reasoningConfidence())
	frame.Put(mcts.SymbolTreatment, mcts.ActionWait)
	frame.Put(mcts.SymbolTarget, graph.reasoningTarget())
	frame.Put(mcts.SymbolFlow, graph.reasoningSignal("flow"))
	frame.Put(mcts.SymbolLiquidityImpact, math.Abs(graph.reasoningSignal("liquidity")))
	frame.Put(mcts.SymbolHawkes, graph.reasoningSignal("hawkes"))
	frame.Put(mcts.SymbolCoherence, graph.reasoningSignal("coherence"))
	frame.Put(mcts.SymbolRegime, graph.reasoningSignal("regime"))
	frame.Put(mcts.SymbolSurprise, math.Abs(graph.reasoningSignal("surprise")))
	frame.Put(mcts.SymbolPosition, 0)
	frame.Put(mcts.SymbolHorizon, 0)
	frame.Put(mcts.SymbolMaximumHorizon, float64(graph.reasoningHorizon()))
	frame.Put(mcts.SymbolArchetype, float64(opportunityArchetypeIndex(active.Type)))
	frame.Put(mcts.SymbolVelocity, graph.reasoningSignal("velocity"))
	frame.Put(mcts.SymbolStoredEnergy, graph.reasoningSignal("energy"))
	frame.Put(mcts.SymbolCausalExpectation, graph.reasoningSignal("causal"))
	frame.Put(mcts.SymbolSpread, math.Abs(graph.reasoningSignal("spread")))

	return frame
}

func opportunityArchetypeIndex(archetype types.OpportunityType) int {
	switch archetype {
	case types.OpportunitySuddenPump:
		return 1
	case types.OpportunityCoiledCompression:
		return 2
	case types.OpportunityDailyRiser:
		return 3
	case types.OpportunityInefficientLag:
		return 4
	case types.OpportunityAbsorptionReversal:
		return 5
	default:
		return 0
	}
}

/*
ReasoningHistory returns only explicit observational SCM rows carried in node
metadata. Graph edges and simulated states are never relabeled as observations.
*/
func (graph *Graph) ReasoningHistory() []nomagique.Frame {
	observations := make([]nomagique.Frame, 0)

	for _, node := range graph.reasoningNodeViews() {
		row, found := reasoningRow(node.Metadata["reasoning_row"])

		if !found {
			continue
		}

		frame, err := mcts.RowToFrame(row)

		if err != nil {
			continue
		}

		observations = append(observations, frame)
	}

	return observations
}

/*
ReasoningKey identifies the symbol-level observational stream.
*/
func (graph *Graph) ReasoningKey() string {
	return graph.reasoningSymbol()
}

/*
ApplyReasoningIntervention evolves position and reward over a graph-derived
horizon using the dynamics of the archetype the graph currently expresses.

A sudden pump decays through its Hawkes kernel, a coiled compression converts
stored potential energy into directional velocity, a daily riser rolls under
light damping, an inefficient lag mean-reverts toward its leader, and an
absorption reversal bounces off the absorbed wall. Every step still pays its
crossing cost (spread + impact) and a time penalty, so the search discovers
real opportunity geometry instead of adding monolith scalars.
*/
func (graph *Graph) ApplyReasoningIntervention(
	state nomagique.Frame,
	action float64,
) (nomagique.Frame, error) {
	if err := mcts.ValidateReasoningFrame(state); err != nil {
		return state, err
	}

	position, _ := state.Get(mcts.SymbolPosition)
	nextPosition := position

	switch action {
	case mcts.ActionWait:
	case mcts.ActionEnter:
		if position != 0 {
			return state, fmt.Errorf("graph: enter requires a flat position")
		}

		nextPosition = 1
	case mcts.ActionScale:
		if position == 0 {
			return state, fmt.Errorf("graph: scale requires an open position")
		}

		nextPosition = position + 1
	case mcts.ActionExit:
		if position == 0 {
			return state, fmt.Errorf("graph: exit requires an open position")
		}

		nextPosition = 0
	default:
		return state, fmt.Errorf("graph: unknown intervention %g", action)
	}

	archetype, _ := state.Get(mcts.SymbolArchetype)
	velocity, _ := state.Get(mcts.SymbolVelocity)
	storedEnergy, _ := state.Get(mcts.SymbolStoredEnergy)
	causalExpectation, _ := state.Get(mcts.SymbolCausalExpectation)
	spread, _ := state.Get(mcts.SymbolSpread)
	liquidityImpact, _ := state.Get(mcts.SymbolLiquidityImpact)
	contextConfidence, _ := state.Get(mcts.SymbolContextConfidence)
	currentReward, _ := state.Get(mcts.SymbolTarget)
	currentHorizon, _ := state.Get(mcts.SymbolHorizon)
	nextHorizon := currentHorizon + 1

	crossingCost := math.Abs(spread) + math.Abs(liquidityImpact)
	decayFactor := math.Exp(-0.5 * float64(currentHorizon+1))

	stepReturn := 0.0
	nextVelocity := velocity
	nextEnergy := storedEnergy

	switch int(archetype) {
	case 1: // sudden pump: explosive entry, Hawkes exponential decay
		stepReturn = (velocity*1.5 + confidenceDrive(contextConfidence)) * decayFactor
		stepReturn += 0.3 * causalExpectation
		stepReturn -= crossingCost
		nextVelocity = velocity * decayFactor
		nextEnergy = storedEnergy * 0.7
	case 2: // coiled compression: potential-to-kinetic release
		kineticRelease := math.Sqrt(math.Max(0, storedEnergy)) * 0.05
		stepReturn = kineticRelease - crossingCost
		nextVelocity = velocity + kineticRelease
		nextEnergy = storedEnergy * 0.2
	case 3: // daily riser: light damping trend roll
		damping := 0.95
		nextVelocity = velocity * damping
		stepReturn = nextVelocity - crossingCost*0.5
		nextEnergy = storedEnergy * damping
	case 4: // inefficient lag: leader catch-up mean reversion
		stepReturn = causalExpectation/math.Sqrt(float64(nextHorizon)) - crossingCost
		nextVelocity = 0
		nextEnergy = storedEnergy * 0.5
	case 5: // absorption reversal: counterfactual bounce off the wall
		stepReturn = -velocity - crossingCost
		nextVelocity = -velocity * 0.5
		nextEnergy = storedEnergy * 0.9
	default: // equilibrium/noise
		stepReturn = velocity*0.5 - crossingCost
		nextVelocity = velocity * 0.5
		nextEnergy = storedEnergy * 0.9
	}

	timePenalty := 0.0001 * float64(currentHorizon)
	rewardDelta := nextPosition*stepReturn - timePenalty

	state.Put(mcts.SymbolTreatment, action)
	state.Put(mcts.SymbolPosition, nextPosition)
	state.Put(mcts.SymbolHorizon, nextHorizon)
	state.Put(mcts.SymbolVelocity, nextVelocity)
	state.Put(mcts.SymbolStoredEnergy, nextEnergy)
	state.Put(mcts.SymbolTarget, currentReward+rewardDelta)

	return state, mcts.ValidateReasoningFrame(state)
}

/*
confidenceDrive converts the context-confidence posterior mass into the
directional drive a pump roll adds above its kinematic velocity. It is the
probability mass itself, clamped to the unit interval — not a scaled multiplier
— so a confident reading contributes at most one unit of drive.
*/
func confidenceDrive(contextConfidence float64) float64 {
	if math.IsNaN(contextConfidence) || math.IsInf(contextConfidence, 0) {
		return 0
	}

	return math.Max(0, math.Min(1, contextConfidence))
}

/*
ReasoningTopology builds the UI projection without changing decision evidence.
*/
func (graph *Graph) ReasoningTopology() ReasoningTopology {
	frame := graph.ReasoningFrame()
	symbol := graph.reasoningSymbol()
	treatmentID := "scm:" + symbol + ":treatment:flow"
	mediatorID := "scm:" + symbol + ":mediator:liquidity_impact"
	targetID := "scm:" + symbol + ":target:forward_return"
	ready := graph != nil && graph.ReadyForSearch()
	topology := ReasoningTopology{
		Symbol:         symbol,
		Ready:          ready,
		ObservedRows:   len(graph.ReasoningHistory()),
		MaximumHorizon: graph.reasoningHorizon(),
		Treatment:      treatmentID,
		Mediator:       mediatorID,
		Target:         targetID,
		Controls: []string{
			"context_confidence",
			"hawkes",
			"coherence",
			"regime",
			"surprise",
		},
		CurrentState: mcts.FrameValues(frame),
	}

	if ready {
		topology.Reason = "evidence graph is ready for strategic search"
	} else {
		topology.Reason = "evidence graph lacks a reachable, supported proposition"
	}

	topology.Nodes = graph.reasoningNodes()
	confidence, _ := frame.Get(mcts.SymbolContextConfidence)
	flow, _ := frame.Get(mcts.SymbolFlow)
	liquidity, _ := frame.Get(mcts.SymbolLiquidityImpact)
	target, _ := frame.Get(mcts.SymbolTarget)
	topology.Nodes = append(
		topology.Nodes,
		ReasoningNode{
			ID: treatmentID, Label: "Aggressor flow intervention", Symbol: symbol,
			Tier: ReasoningTierSCM, Role: "treatment", Source: "reasoning",
			Value: flow, Confidence: confidence, Derived: true,
		},
		ReasoningNode{
			ID: mediatorID, Label: "Liquidity impact", Symbol: symbol,
			Tier: ReasoningTierSCM, Role: "mediator", Source: "reasoning",
			Value: liquidity, Confidence: confidence, Derived: true,
		},
		ReasoningNode{
			ID: targetID, Label: "Forward return", Symbol: symbol,
			Tier: ReasoningTierSCM, Role: "target", Source: "reasoning",
			Value: target, Confidence: confidence, Derived: true,
		},
	)
	topology.Links = graph.reasoningLinks(treatmentID, mediatorID, targetID, confidence)
	return topology
}

func (graph *Graph) reasoningNodes() []ReasoningNode {
	views := graph.reasoningNodeViews()
	nodes := make([]ReasoningNode, 0, len(views))

	for _, node := range views {
		nodes = append(nodes, ReasoningNode{
			ID:         node.ID,
			Label:      node.ID,
			Symbol:     node.Symbol,
			Tier:       reasoningTier(node.ID, node.MeasurementID, node.Source),
			Source:     node.Source,
			Value:      node.Value,
			Confidence: node.Confidence,
			Metadata:   cloneReasoningMetadata(node.Metadata),
		})
	}

	return nodes
}

func (graph *Graph) reasoningLinks(
	treatmentID string,
	mediatorID string,
	targetID string,
	confidence float64,
) []ReasoningLink {
	links := []ReasoningLink{
		{
			From: treatmentID, To: mediatorID, Relation: "causes",
			Weight: 1, Confidence: confidence, Derived: true,
		},
		{
			From: mediatorID, To: targetID, Relation: "causes",
			Weight: 1, Confidence: confidence, Derived: true,
		},
	}

	if graph == nil || graph.DecisionTarget == "" {
		return links
	}

	return append(links, ReasoningLink{
		From: targetID, To: graph.DecisionTarget, Relation: "supports",
		Weight: 1, Confidence: confidence, Derived: true,
	})
}

func (graph *Graph) reasoningSignal(role string) float64 {
	aggregate := &reasoningAggregate{}

	for _, node := range graph.reasoningNodeViews() {
		if reasoningRole(node.ID, node.Source, node.Metric) != role {
			continue
		}

		aggregate.Add(node.Value, node.Confidence)
	}

	return aggregate.Mean()
}

func (graph *Graph) reasoningConfidence() float64 {
	confidence := 0.0
	count := 0

	for _, node := range graph.reasoningNodeViews() {
		if node.Confidence <= 0 {
			continue
		}

		confidence += node.Confidence
		count++
	}

	if count == 0 {
		return 0
	}

	return confidence / float64(count)
}

func (graph *Graph) reasoningTarget() float64 {
	if graph == nil || graph.DecisionTarget == "" {
		return 0
	}

	for _, node := range graph.reasoningNodeViews() {
		if node.ID == graph.DecisionTarget {
			return node.Value
		}
	}

	return 0
}

func (graph *Graph) reasoningHorizon() int {
	if graph == nil {
		return 1
	}

	relevantCount := len(graph.relevantNodes())
	horizon := int(math.Ceil(math.Log2(float64(relevantCount + 1))))

	if horizon < 1 {
		return 1
	}

	return horizon
}

func (graph *Graph) reasoningSymbol() string {
	if graph == nil {
		return "market"
	}

	views := graph.reasoningNodeViews()

	for _, node := range views {
		if node.ID == graph.DecisionTarget && node.Symbol != "" {
			return node.Symbol
		}
	}

	for _, node := range views {
		if node.Symbol != "" {
			return node.Symbol
		}
	}

	return "market"
}

func (graph *Graph) reasoningNodeViews() []reasoningNodeView {
	if graph == nil {
		return nil
	}

	views := make([]reasoningNodeView, 0, len(graph.Nodes))

	for nodeID, rawNode := range graph.Nodes {
		view, found := inspectReasoningNode(nodeID, rawNode)

		if !found {
			continue
		}

		views = append(views, view)
	}

	sort.Slice(views, func(leftIndex int, rightIndex int) bool {
		return views[leftIndex].ID < views[rightIndex].ID
	})
	return views
}

func inspectReasoningNode(nodeID string, rawNode interface{}) (reasoningNodeView, bool) {
	value := reflect.ValueOf(rawNode)

	for value.IsValid() && (value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface) {
		if value.IsNil() {
			return reasoningNodeView{}, false
		}

		value = value.Elem()
	}

	if !value.IsValid() || value.Kind() != reflect.Struct {
		return reasoningNodeView{}, false
	}

	view := reasoningNodeView{
		ID:            reasoningStringField(value, "ID"),
		Symbol:        reasoningStringField(value, "Symbol"),
		Source:        reasoningStringField(value, "Source"),
		MeasurementID: reasoningStringField(value, "MeasurementID"),
		Metric:        reasoningStringField(value, "Metric"),
		Value:         reasoningNumberField(value, "Value"),
		Confidence:    reasoningNumberField(value, "Confidence"),
		Metadata:      reasoningMetadataField(value, "Metadata"),
	}

	if view.ID == "" {
		view.ID = nodeID
	}

	return view, true
}

func reasoningStringField(value reflect.Value, name string) string {
	field := value.FieldByName(name)

	if !field.IsValid() || !field.CanInterface() {
		return ""
	}

	return fmt.Sprint(field.Interface())
}

func reasoningNumberField(value reflect.Value, name string) float64 {
	field := value.FieldByName(name)

	if !field.IsValid() {
		return 0
	}

	switch field.Kind() {
	case reflect.Float32, reflect.Float64:
		return field.Convert(reflect.TypeOf(float64(0))).Float()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(field.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(field.Uint())
	default:
		return 0
	}
}

func reasoningMetadataField(value reflect.Value, name string) map[string]interface{} {
	field := value.FieldByName(name)

	if !field.IsValid() || field.Kind() != reflect.Map {
		return nil
	}

	metadata := make(map[string]interface{}, field.Len())
	iterator := field.MapRange()

	for iterator.Next() {
		key := iterator.Key()
		entry := iterator.Value()

		if key.Kind() != reflect.String || !entry.IsValid() || !entry.CanInterface() {
			continue
		}

		metadata[key.String()] = entry.Interface()
	}

	return metadata
}

func cloneReasoningMetadata(metadata map[string]interface{}) map[string]interface{} {
	if metadata == nil {
		return nil
	}

	cloned := make(map[string]interface{}, len(metadata))

	for key, value := range metadata {
		cloned[key] = value
	}

	return cloned
}

func reasoningTier(
	nodeID string,
	measurementID string,
	source string,
) ReasoningTier {
	normalized := strings.ToLower(strings.Join([]string{nodeID, source}, " "))

	if strings.HasPrefix(normalized, "hyp:") || strings.Contains(normalized, "decision") {
		return ReasoningTierDecision
	}

	if strings.HasPrefix(normalized, "scm:") || strings.Contains(normalized, "causal") {
		return ReasoningTierSCM
	}

	if measurementID != "" || strings.HasPrefix(normalized, "measurement:") {
		return ReasoningTierMeasurement
	}

	return ReasoningTierField
}

func reasoningRole(nodeID string, source string, metric string) string {
	normalized := strings.ToLower(strings.Join([]string{nodeID, source, metric}, " "))

	for _, marker := range []string{"ignition", "aggressor", "cvd", "flow", "velocity", "jump_amplitude"} {
		if strings.Contains(normalized, marker) {
			return "flow"
		}
	}

	for _, marker := range []string{"spread", "depth", "impact", "liquidity", "book"} {
		if strings.Contains(normalized, marker) {
			return "liquidity"
		}
	}

	if strings.Contains(normalized, "hawkes") {
		return "hawkes"
	}

	for _, marker := range []string{"coherence", "kuramoto", "phase", "fluid", "manifold"} {
		if strings.Contains(normalized, marker) {
			return "coherence"
		}
	}

	for _, marker := range []string{"regime", "trend", "cognition", "memory"} {
		if strings.Contains(normalized, marker) {
			return "regime"
		}
	}

	for _, marker := range []string{"surprise", "energy", "prediction_error", "passivity", "variance", "dissipation"} {
		if strings.Contains(normalized, marker) {
			return "surprise"
		}
	}

	return "context"
}

func reasoningRow(value interface{}) ([]float64, bool) {
	if row, supported := value.([]float64); supported {
		return append([]float64(nil), row...), true
	}

	reflected := reflect.ValueOf(value)

	if !reflected.IsValid() || reflected.Kind() != reflect.Slice {
		return nil, false
	}

	row := make([]float64, reflected.Len())

	for valueIndex := 0; valueIndex < reflected.Len(); valueIndex++ {
		entry := reflected.Index(valueIndex)

		for entry.IsValid() && entry.Kind() == reflect.Interface {
			entry = entry.Elem()
		}

		if !entry.IsValid() || (entry.Kind() != reflect.Float32 && entry.Kind() != reflect.Float64) {
			return nil, false
		}

		row[valueIndex] = entry.Convert(reflect.TypeOf(float64(0))).Float()
	}

	return row, true
}

func meanValues(values ...float64) float64 {
	if len(values) == 0 {
		return 0
	}

	total := 0.0

	for _, value := range values {
		total += value
	}

	return total / float64(len(values))
}
