package types

/*
LongStance types each category hypothesis by its bearing on a long entry, the
same way CategoryAffinity types each metric by its bearing on a category. A
category appears here only when its market meaning has an unambiguous
directional reading for opening a long; regime-descriptive categories with no
inherent direction (laminar, inertial, dense neutrality, …) are intentionally
absent so structure never masquerades as direction.
*/
var LongStance = map[CategoryType]bool{
	// Favors a long entry: genuine initiative flow, sound liquidity, or an
	// igniting move with organic participation.
	VerticalIgnition:  true,
	CoiledCompression: true,
	OrganicTrend:      true,
	Organic:           true,
	RiskOnSurge:       true,
	AggressiveDrive:   true,
	HardSupport:       true,
	RobustLiquidity:   true,
	HiddenAbsorption:  true,

	// Opposes a long entry: dishonest or vanishing liquidity, exhausted or
	// collapsing moves, and broad risk-off pressure.
	SpoofTrap:          false,
	ToxicBluff:         false,
	LiquidityVacuum:    false,
	BookThinning:       false,
	ExtremeScarcity:    false,
	VolumeStarvation:   false,
	Exhaustion:         false,
	ThermalExhaustion:  false,
	FadedExhaustion:    false,
	MechanicalCollapse: false,
	FragileExpansion:   false,
	ActiveReversal:     false,
	SystemicSlump:      false,
	AnchorStall:        false,
}

/*
longEntryBlockers identifies established market structures that contradict the
mechanics of opening a long regardless of simultaneous initiative evidence.
Deception, absent liquidity, collapse, and active reversal cannot be canceled
out by counting unrelated bullish phenomena.
*/
var longEntryBlockers = map[CategoryType]struct{}{
	SpoofTrap:          {},
	ToxicBluff:         {},
	LiquidityVacuum:    {},
	MechanicalCollapse: {},
	ActiveReversal:     {},
}

/*
EntryEvidence is the active category reading bearing on a long entry. Vetoes
are a subset of Opposes whose market meaning is structurally incompatible with
opening risk rather than merely unfavorable context.
*/
type EntryEvidence struct {
	Favors  []string
	Opposes []string
	Vetoes  []string
}

/*
LongEntryEvidence reports the active category hypotheses bearing on a long entry.
A category is active when its measurement evidence nets supportive — strictly
more Supports than Contradicts edges point at its node — so a single contested
reading never counts as an established phenomenon.
*/
func (evidenceGraph *Graph) LongEntryEvidence() EntryEvidence {
	supports := make(map[string]int)
	contradicts := make(map[string]int)
	reading := EntryEvidence{}

	for _, edge := range evidenceGraph.edges {
		node := evidenceGraph.nodes[edge.To]

		if node == nil || node.Kind != NodeCategory {
			continue
		}

		switch edge.Type {
		case Supports:
			supports[string(node.Category)]++
		case Contradicts:
			contradicts[string(node.Category)]++
		}
	}

	for _, category := range CategoryOrder {
		stance, directional := LongStance[category]

		if !directional {
			continue
		}

		name := string(category)

		if supports[name] <= contradicts[name] {
			continue
		}

		if stance {
			reading.Favors = append(reading.Favors, name)
			continue
		}

		reading.Opposes = append(reading.Opposes, name)

		if _, vetoes := longEntryBlockers[category]; vetoes {
			reading.Vetoes = append(reading.Vetoes, name)
		}
	}

	return reading
}
