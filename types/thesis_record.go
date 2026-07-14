package types

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/theapemachine/errnie"
)

const thesisRecordSchemaVersion = 1

/*
thesisRecord is the durable, versioned portion of a Thesis. Only uiHub and
Signals remain outside the record because they are runtime connections rather
than lifecycle evidence.
*/
type thesisRecord struct {
	SchemaVersion uint                      `json:"schemaVersion"`
	Checkpoint    int64                     `json:"checkpoint"`
	Tick          int64                     `json:"tick"`
	CrossSection  *thesisCrossSectionRecord `json:"crossSection"`
	Measurements  []*Measurement            `json:"measurements"`
	Graphs        []GraphFrame              `json:"graphs"`
	Forecasts     []Forecasts               `json:"forecasts"`
	Decisions     []Decision                `json:"decisions"`
	TradeJournal  []TradeObservation        `json:"tradeJournal"`
	Lifecycle     map[string]string         `json:"lifecycle"`
	Findings      []Finding                 `json:"findings"`
	Hypotheses    []Hypothesis              `json:"hypotheses"`
	Categories    []Category                `json:"categories"`
	Manifold      []any                     `json:"manifold"`
	Resonance     []any                     `json:"resonance"`
	Causal        []any                     `json:"causal"`
}

/*
thesisCrossSectionRecord persists peer metrics without leaking the derived
symbol index into the durable schema.
*/
type thesisCrossSectionRecord struct {
	Metrics []SymbolMetric `json:"metrics"`
}

/*
MarshalBinary serializes the durable Thesis case record as deterministic,
versioned JSON while excluding state owned by the running process.
*/
func (thesis *Thesis) MarshalBinary() ([]byte, error) {
	if thesis == nil {
		return nil, errnie.Err(errnie.Validation, "Thesis is required", nil)
	}

	thesis.mu.RLock()
	defer thesis.mu.RUnlock()

	record := &thesisRecord{}
	record.Checkpoint = thesis.checkpoint.Load()

	if err := record.capture(thesis); err != nil {
		return nil, err
	}

	payload, err := json.Marshal(record)

	if err != nil {
		return nil, errnie.Err(
			errnie.Validation, "marshal Thesis record", err,
		)
	}

	return payload, nil
}

/*
MarshalCheckpoint timestamps and serializes one state while holding the proven
Thesis read boundary, making the timestamp order match the snapshot order.
*/
func (thesis *Thesis) MarshalCheckpoint() ([]byte, int64, error) {
	if thesis == nil {
		return nil, 0, errnie.Err(errnie.Validation, "Thesis is required", nil)
	}

	thesis.mu.RLock()
	defer thesis.mu.RUnlock()

	timestamp := time.Now().UnixNano()

	for {
		previous := thesis.checkpoint.Load()

		if timestamp <= previous {
			timestamp = previous + 1
		}

		if thesis.checkpoint.CompareAndSwap(previous, timestamp) {
			break
		}
	}

	record := &thesisRecord{Checkpoint: timestamp}

	if err := record.capture(thesis); err != nil {
		return nil, 0, err
	}

	payload, err := json.Marshal(record)

	if err != nil {
		return nil, 0, errnie.Err(
			errnie.Validation, "marshal Thesis checkpoint", err,
		)
	}

	return payload, timestamp, nil
}

/*
RestoreThesis reconstructs a durable Thesis case, its derived indexes, and its
Gonum topology while injecting only the runtime UI hub for the new process.
*/
func RestoreThesis(payload []byte, uiHub chan<- []byte) (*Thesis, error) {
	record := &thesisRecord{}

	if err := json.Unmarshal(payload, record); err != nil {
		return nil, errnie.Err(
			errnie.Validation, "unmarshal Thesis record", err,
		)
	}

	return record.restore(uiHub)
}

/*
capture projects one live Thesis onto the durable schema and orders graph
frames so equivalent case records always produce identical bytes.
*/
func (record *thesisRecord) capture(thesis *Thesis) error {
	if thesis == nil {
		return errnie.Err(errnie.Validation, "marshal nil Thesis", nil)
	}

	if thesis.CrossSection == nil || thesis.CrossSection.Metrics == nil {
		return errnie.Err(errnie.Validation, "marshal Thesis without cross-section metrics", nil)
	}

	if thesis.Measurements == nil || thesis.Graphs == nil || thesis.Forecasts == nil ||
		thesis.Decisions == nil || thesis.TradeJournal == nil || thesis.Lifecycle == nil ||
		thesis.Findings == nil || thesis.Hypotheses == nil || thesis.Categories == nil ||
		thesis.Manifold == nil || thesis.Resonance == nil || thesis.Causal == nil {
		return errnie.Err(errnie.Validation, "marshal Thesis with incomplete durable state", nil)
	}

	record.SchemaVersion = thesisRecordSchemaVersion
	record.Tick = thesis.Tick
	record.CrossSection = &thesisCrossSectionRecord{
		Metrics: slices.Clone(thesis.CrossSection.Metrics),
	}
	record.Measurements = slices.Clone(thesis.Measurements)
	record.Forecasts = slices.Clone(thesis.Forecasts)
	record.Decisions = slices.Clone(thesis.Decisions)
	record.TradeJournal = slices.Clone(thesis.TradeJournal)
	record.Lifecycle = maps.Clone(thesis.Lifecycle)
	record.Findings = slices.Clone(thesis.Findings)
	record.Hypotheses = slices.Clone(thesis.Hypotheses)
	record.Categories = slices.Clone(thesis.Categories)
	record.Manifold = slices.Clone(thesis.Manifold)
	record.Resonance = slices.Clone(thesis.Resonance)
	record.Causal = slices.Clone(thesis.Causal)

	symbols := make([]string, 0, len(thesis.Graphs))

	for symbol := range thesis.Graphs {
		symbols = append(symbols, symbol)
	}

	slices.Sort(symbols)
	record.Graphs = make([]GraphFrame, 0, len(symbols))

	for _, symbol := range symbols {
		evidenceGraph := thesis.Graphs[symbol]

		if evidenceGraph == nil || symbol == "" || evidenceGraph.Symbol != symbol {
			return errnie.Err(
				errnie.Validation, "marshal Thesis with invalid graph identity", nil,
			).With("key", symbol)
		}

		frame := evidenceGraph.Frame()
		frame.order()
		record.Graphs = append(record.Graphs, frame)
	}

	return nil
}

/*
restore validates the durable schema before constructing the recovered domain
state and newly initialized runtime state.
*/
func (record *thesisRecord) restore(uiHub chan<- []byte) (*Thesis, error) {
	if err := record.validate(); err != nil {
		return nil, err
	}

	crossSection := &CrossSection{Metrics: slices.Clone(record.CrossSection.Metrics)}

	if err := crossSection.rebuild(); err != nil {
		return nil, err
	}

	graphs := make(map[string]*Graph, len(record.Graphs))

	for _, frame := range record.Graphs {
		if _, exists := graphs[frame.Symbol]; exists {
			return nil, errnie.Err(
				errnie.Validation, "restore Thesis with duplicate graph symbol", nil,
			).With("symbol", frame.Symbol)
		}

		evidenceGraph, err := frame.restore()

		if err != nil {
			return nil, err
		}

		graphs[frame.Symbol] = evidenceGraph
	}

	restored := NewThesis(uiHub)
	restored.checkpoint.Store(record.Checkpoint)
	restored.Tick = record.Tick
	restored.CrossSection = crossSection
	restored.Measurements = record.Measurements
	restored.Graphs = graphs
	restored.Forecasts = record.Forecasts
	restored.Decisions = record.Decisions
	restored.TradeJournal = record.TradeJournal
	restored.Lifecycle = record.Lifecycle
	restored.Findings = record.Findings
	restored.Hypotheses = record.Hypotheses
	restored.Categories = record.Categories
	restored.Manifold = record.Manifold
	restored.Resonance = record.Resonance
	restored.Causal = record.Causal
	return restored, nil
}

/*
validate rejects records that cannot distinguish persisted empty state from a
missing field, and prevents incompatible schema versions from being inferred.
*/
func (record *thesisRecord) validate() error {
	if record.SchemaVersion != thesisRecordSchemaVersion {
		return errnie.Err(
			errnie.Validation, "unsupported Thesis record schema version", nil,
		).With("version", record.SchemaVersion)
	}

	if record.CrossSection == nil || record.CrossSection.Metrics == nil {
		return errnie.Err(errnie.Validation, "Thesis record cross-section is missing", nil)
	}

	if record.Measurements == nil || record.Graphs == nil || record.Forecasts == nil ||
		record.Decisions == nil || record.TradeJournal == nil || record.Lifecycle == nil ||
		record.Findings == nil || record.Hypotheses == nil || record.Categories == nil ||
		record.Manifold == nil || record.Resonance == nil || record.Causal == nil {
		return errnie.Err(errnie.Validation, "Thesis record durable state is incomplete", nil)
	}

	return nil
}

/*
rebuild reconstructs the CrossSection symbol index and rejects ambiguous
metrics rather than choosing one duplicate during recovery.
*/
func (crossSection *CrossSection) rebuild() error {
	crossSection.index = make(map[string]int, len(crossSection.Metrics))

	for index, metric := range crossSection.Metrics {
		if strings.TrimSpace(metric.Symbol) == "" {
			return errnie.Err(errnie.Validation, "restore cross-section with empty symbol", nil)
		}

		if _, exists := crossSection.index[metric.Symbol]; exists {
			return errnie.Err(
				errnie.Validation, "restore cross-section with duplicate symbol", nil,
			).With("symbol", metric.Symbol)
		}

		crossSection.index[metric.Symbol] = index
	}

	return nil
}

/*
order canonicalizes the only slices whose order comes from Gonum iteration;
the JSON encoder supplies deterministic ordering for string-keyed maps.
*/
func (frame *GraphFrame) order() {
	slices.SortFunc(frame.Nodes, func(left, right GraphNodeWire) int {
		return strings.Compare(left.Key, right.Key)
	})
	slices.SortFunc(frame.Edges, func(left, right GraphEdgeWire) int {
		if order := strings.Compare(left.From, right.From); order != 0 {
			return order
		}

		if order := strings.Compare(left.To, right.To); order != 0 {
			return order
		}

		if order := strings.Compare(string(left.Type), string(right.Type)); order != 0 {
			return order
		}

		if order := left.At.Compare(right.At); order != 0 {
			return order
		}

		return left.ObservedFrom.Compare(right.ObservedFrom)
	})
}

/*
restore validates GraphFrame node identities and edge references while
reconstructing the Gonum graph as the sole owner of topology.
*/
func (frame GraphFrame) restore() (*Graph, error) {
	if strings.TrimSpace(frame.Symbol) == "" || frame.Nodes == nil || frame.Edges == nil {
		return nil, errnie.Err(errnie.Validation, "restore invalid Thesis graph frame", nil)
	}

	evidenceGraph := NewGraph(frame.Symbol)
	keys := make(map[string]struct{}, len(frame.Nodes))

	for _, node := range frame.Nodes {
		expected := MeasurementKey(&node.Measurement)

		if node.Key == "" || node.Key != expected {
			return nil, errnie.Err(
				errnie.Validation, "restore Thesis graph with invalid node key", nil,
			).With("symbol", frame.Symbol, "key", node.Key)
		}

		if _, exists := keys[node.Key]; exists {
			return nil, errnie.Err(
				errnie.Validation, "restore Thesis graph with duplicate node key", nil,
			).With("symbol", frame.Symbol, "key", node.Key)
		}

		if err := evidenceGraph.AddNode(&node.Measurement); err != nil {
			return nil, errnie.Err(
				errnie.Validation, "restore Thesis graph node", err,
			).With("symbol", frame.Symbol, "key", node.Key)
		}

		keys[node.Key] = struct{}{}
	}

	for _, edge := range frame.Edges {
		_, fromExists := keys[edge.From]
		_, toExists := keys[edge.To]

		if !fromExists || !toExists || !edge.Type.persistable() {
			return nil, errnie.Err(
				errnie.Validation, "restore Thesis graph with invalid edge", nil,
			).With("symbol", frame.Symbol, "from", edge.From, "to", edge.To)
		}

		if !evidenceGraph.Relate(edge.From, edge.To, edge.Type, edge.At, edge.ObservedFrom) {
			return nil, errnie.Err(
				errnie.Validation, "restore Thesis graph with duplicate edge", nil,
			).With("symbol", frame.Symbol, "from", edge.From, "to", edge.To)
		}
	}

	if !evidenceGraph.At.Equal(frame.At) {
		return nil, errnie.Err(
			errnie.Validation, "restore Thesis graph timestamp does not match topology", nil,
		).With("symbol", frame.Symbol)
	}

	return evidenceGraph, nil
}

/*
persistable reports whether an edge kind belongs to the graph relationship
vocabulary understood by this schema version.
*/
func (edgeType EdgeType) persistable() bool {
	switch edgeType {
	case Supports, Contradicts, Conditions, Leads, Lags, Redundant,
		Independent, Stale, Incomparable:
		return true
	}

	return false
}
