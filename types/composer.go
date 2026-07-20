package types

import (
	"sort"
	"time"

	"github.com/theapemachine/errnie"
)

/*
EvidenceComposer turns one Graph's bounded observations into market evidence.
*/
type EvidenceComposer struct {
	graph        *Graph
	observations map[observable]*observationWindow
}

/*
observable identifies the market meaning shared by successive measurements.
*/
type observable struct {
	metric  MetricType
	subject SubjectType
	side    MeasurementSide
}

/*
observationWindow retains the newest reading and its direct predecessor.
*/
type observationWindow struct {
	older *Node
	newer *Node
}

/*
newEvidenceComposer binds evidence responsibilities to one graph.
*/
func newEvidenceComposer(graph *Graph) *EvidenceComposer {
	return &EvidenceComposer{
		graph:        graph,
		observations: make(map[observable]*observationWindow),
	}
}

/*
AddMeasurements validates a batch and avoids allocating obsolete observations.
*/
func (composer *EvidenceComposer) AddMeasurements(measurements []*Measurement) error {
	type candidates struct {
		older *Measurement
		newer *Measurement
	}

	windows := make(map[observable]candidates)
	untyped := make([]*Measurement, 0)
	after := func(candidate, current *Measurement) bool {
		return candidate.At.After(current.At) ||
			(candidate.At.Equal(current.At) &&
				MeasurementKey(candidate) > MeasurementKey(current))
	}

	for _, measurement := range measurements {
		if measurement == nil {
			return errnie.Validate((*Measurement)(nil))
		}

		if err := errnie.Validate(measurement); err != nil {
			return err
		}

		if measurement.Symbol != composer.graph.Symbol {
			return errnie.Err(
				errnie.Validation, "measurement symbol does not match graph", nil,
			).With("graph", composer.graph.Symbol, "measurement", measurement.Symbol)
		}

		if measurement.Metric == "" || measurement.Subject == "" {
			untyped = append(untyped, measurement)
			continue
		}

		key := observable{
			metric:  measurement.Metric,
			subject: measurement.Subject,
			side:    measurement.Side,
		}
		window := windows[key]

		if window.newer == nil || after(measurement, window.newer) {
			window.older = window.newer
			window.newer = measurement
			windows[key] = window
			continue
		}

		if window.older == nil || after(measurement, window.older) {
			window.older = measurement
			windows[key] = window
		}
	}

	for _, measurement := range untyped {
		if err := composer.graph.AddNode(measurement); err != nil {
			return err
		}
	}

	for _, window := range windows {
		if window.older != nil {
			if err := composer.graph.AddNode(window.older); err != nil {
				return err
			}
		}

		if err := composer.graph.AddNode(window.newer); err != nil {
			return err
		}
	}

	return nil
}

/*
stage retains two nodes because live relationships cannot use older readings.
*/
func (composer *EvidenceComposer) stage(node *Node) {
	measurement := node.Measurement

	if measurement.Metric == "" || measurement.Subject == "" {
		composer.graph.nodes[node.Key] = node
		return
	}

	key := observable{
		metric:  measurement.Metric,
		subject: measurement.Subject,
		side:    measurement.Side,
	}
	window := composer.observations[key]

	if window == nil {
		window = &observationWindow{newer: node}
		composer.observations[key] = window
		composer.graph.nodes[node.Key] = node
		return
	}

	newest := window.newer == nil ||
		measurement.At.After(window.newer.Measurement.At) ||
		(measurement.At.Equal(window.newer.Measurement.At) && node.Key > window.newer.Key)

	if newest {
		if window.older != nil {
			delete(composer.graph.nodes, window.older.Key)
		}

		window.older = window.newer
		window.newer = node
		composer.graph.nodes[node.Key] = node
		return
	}

	older := window.older
	tooOld := older != nil && (measurement.At.Before(older.Measurement.At) ||
		(measurement.At.Equal(older.Measurement.At) && node.Key <= older.Key))

	if tooOld {
		return
	}

	if window.older != nil {
		delete(composer.graph.nodes, window.older.Key)
	}

	window.older = node
	composer.graph.nodes[node.Key] = node
}

/*
RestoreNode reinstates one wire node and rejects empty or duplicate identities.
*/
func (composer *EvidenceComposer) RestoreNode(
	key string,
	kind NodeKind,
	category CategoryType,
	measurement Measurement,
) error {
	if key == "" {
		return errnie.Err(errnie.Validation, "graph: restored node key is required", nil)
	}

	if _, exists := composer.graph.nodes[key]; exists {
		return errnie.Err(
			errnie.Conflict, "graph: restored node key already exists", nil,
		).With("key", key)
	}

	if kind == "" {
		kind = NodeMeasurement
	}

	composer.graph.nodes[key] = &Node{
		Key:         key,
		Kind:        kind,
		Category:    category,
		Measurement: measurement,
	}

	if composer.graph.At.IsZero() || measurement.At.After(composer.graph.At) {
		composer.graph.At = measurement.At
	}

	return nil
}

/*
StagePeerNode borrows another symbol's measurement as a real edge endpoint.
*/
func (composer *EvidenceComposer) StagePeerNode(measurement Measurement) string {
	key := MeasurementKey(&measurement)

	if _, exists := composer.graph.nodes[key]; exists {
		return key
	}

	composer.graph.nodes[key] = &Node{
		Key:         key,
		Kind:        NodeMeasurement,
		Measurement: measurement,
	}

	return key
}

/*
CategoryEvidence returns supporting, opposing, and absent affinity metrics.
*/
func (composer *EvidenceComposer) CategoryEvidence(category CategoryType) (
	supporting, opposing, missing []string,
) {
	categoryKey := CategoryKey(category)
	present := make(map[MetricType]struct{})

	for _, edge := range composer.graph.edges {
		if edge.To != categoryKey {
			continue
		}

		switch edge.Type {
		case Supports:
			supporting = append(supporting, edge.From)
		case Contradicts:
			opposing = append(opposing, edge.From)
		default:
			continue
		}

		if node := composer.graph.nodes[edge.From]; node != nil {
			present[node.Measurement.Metric] = struct{}{}
		}
	}

	for metric, affinity := range CategoryAffinity {
		if _, seen := present[metric]; !seen && affinity.Lists(category) {
			missing = append(missing, string(metric))
		}
	}

	sort.Strings(missing)

	return supporting, opposing, missing
}

/*
RelateConditions stages causal concepts and records their directed claim.
*/
func (composer *EvidenceComposer) RelateConditions(
	treatment, outcome string,
	at, observedFrom time.Time,
) bool {
	if treatment == "" || outcome == "" {
		return false
	}

	return composer.graph.Relate(
		composer.ensureConcept(treatment, at),
		composer.ensureConcept(outcome, at),
		Conditions,
		at,
		observedFrom,
	)
}

/*
composeCategories relates current valid readings to their category hypotheses.
*/
func (composer *EvidenceComposer) composeCategories() {
	for _, node := range composer.graph.nodes {
		if node.Kind != NodeMeasurement || !composer.latest(node) {
			continue
		}

		measurement := node.Measurement

		if measurement.Validity.State != ValidityValid ||
			!categoryEvidenceActive(measurement.Normalized) {
			continue
		}

		affinity, ok := AffinityFor(measurement.Metric)

		if !ok {
			continue
		}

		observedFrom, _ := measurement.Interval()

		for _, category := range affinity.Supports {
			composer.graph.Relate(
				node.Key, composer.ensureCategory(category, measurement.At),
				Supports, measurement.At, observedFrom,
			)
		}

		for _, category := range affinity.Opposes {
			composer.graph.Relate(
				node.Key, composer.ensureCategory(category, measurement.At),
				Contradicts, measurement.At, observedFrom,
			)
		}
	}
}

/*
latest reports whether node is the current observation for its typed meaning.
*/
func (composer *EvidenceComposer) latest(node *Node) bool {
	measurement := node.Measurement

	if measurement.Metric == "" || measurement.Subject == "" {
		return true
	}

	window := composer.observations[observable{
		metric:  measurement.Metric,
		subject: measurement.Subject,
		side:    measurement.Side,
	}]

	return window != nil && window.newer == node
}

/*
ensureCategory stages one synthetic category hypothesis node.
*/
func (composer *EvidenceComposer) ensureCategory(category CategoryType, at time.Time) string {
	key := CategoryKey(category)

	if _, exists := composer.graph.nodes[key]; exists {
		return key
	}

	composer.graph.nodes[key] = &Node{
		Key:      key,
		Kind:     NodeCategory,
		Category: category,
		Measurement: Measurement{
			Source: SourceCategory,
			Metric: MetricType(category),
			Symbol: composer.graph.Symbol,
			At:     at,
		},
	}

	return key
}

/*
ensureConcept stages one named causal variable node.
*/
func (composer *EvidenceComposer) ensureConcept(name string, at time.Time) string {
	key := ConceptKey(name)

	if _, exists := composer.graph.nodes[key]; exists {
		return key
	}

	composer.graph.nodes[key] = &Node{
		Key:  key,
		Kind: NodeConcept,
		Measurement: Measurement{
			Source: SourceCausal,
			Metric: MetricType(name),
			Symbol: composer.graph.Symbol,
			At:     at,
		},
	}

	return key
}
