package category

import (
	"time"

	"github.com/theapemachine/symm/types"
)

/*
Relation is a typed directed edge between category nodes. Weights strengthen
when the relationship is observed again; evidence refs and At keep the cut that
last justified the edge.
*/
type Relation struct {
	Symbol   string             `json:"symbol"`
	From     types.CategoryType `json:"from"`
	To       types.CategoryType `json:"to"`
	Type     RelationType       `json:"type"`
	Weight   float64            `json:"weight"`
	Evidence []string           `json:"evidence"`
	At       time.Time          `json:"at"`
}

/*
RelationType is the thesis.md edge vocabulary for category→category links.
*/
type RelationType string

const (
	Supports         RelationType = "supports"
	Contradicts      RelationType = "contradicts"
	Conditions       RelationType = "conditions"
	Leads            RelationType = "leads"
	Lags             RelationType = "lags"
	RedundantWith    RelationType = "redundant_with"
	IndependentOf    RelationType = "independent_of"
	StaleRelativeTo  RelationType = "stale_relative_to"
	IncomparableWith RelationType = "incomparable_with"
)

func categoryID(cat types.CategoryType) uint8 {
	switch cat {
	case types.ForecastEdge:
		return 1
	case types.PhysicalField:
		return 2
	case types.Laminar:
		return 3
	case types.Turbulent:
		return 4
	case types.Inertial:
		return 5
	case types.Viscous:
		return 6
	case types.Frenzy:
		return 7
	case types.Saturation:
		return 8
	case types.Organic:
		return 9
	case types.Exhaustion:
		return 10
	case types.HiddenAbsorption:
		return 11
	case types.AggressiveDrive:
		return 12
	case types.StochasticBalance:
		return 13
	case types.VolumeStarvation:
		return 14
	case types.LoadedImbalance:
		return 15
	case types.SpoofTrap:
		return 16
	case types.BookThinning:
		return 17
	case types.DenseNeutrality:
		return 18
	case types.InefficientLag:
		return 19
	case types.SynchronizedDrift:
		return 20
	case types.DecoupledMove:
		return 21
	case types.AnchorStall:
		return 22
	case types.VerticalIgnition:
		return 23
	case types.CoiledCompression:
		return 24
	case types.OrganicTrend:
		return 25
	case types.FadedExhaustion:
		return 26
	case types.ExtremeScarcity:
		return 27
	case types.MedianDepth:
		return 28
	case types.RobustLiquidity:
		return 29
	case types.RiskOnSurge:
		return 30
	case types.DivergentMove:
		return 31
	case types.SystemicSlump:
		return 32
	case types.LiquidityVacuum:
		return 33
	case types.ToxicBluff:
		return 34
	case types.HardSupport:
		return 35
	case types.SystemicHerd:
		return 36
	case types.DecoupledAlpha:
		return 37
	case types.StochasticNoise:
		return 38
	case types.DivergentStress:
		return 39
	case types.EndogenousAlpha:
		return 40
	case types.SystemicBeta:
		return 41
	case types.LiquidityShock:
		return 42
	case types.CausalNoise:
		return 43
	case types.MechanicalCollapse:
		return 44
	case types.ThermalExhaustion:
		return 45
	case types.FragileExpansion:
		return 46
	case types.ActiveReversal:
		return 47
	case types.LaminarResonance:
		return 48
	case types.TurbulentResonance:
		return 49
	case types.Equilibrium:
		return 50
	default:
		return 0
	}
}

func relationID(rel RelationType) uint8 {
	switch rel {
	case Supports:
		return 1
	case Contradicts:
		return 2
	case Conditions:
		return 3
	case Leads:
		return 4
	case Lags:
		return 5
	case RedundantWith:
		return 6
	case IndependentOf:
		return 7
	case StaleRelativeTo:
		return 8
	case IncomparableWith:
		return 9
	default:
		return 0
	}
}

/*
edgeKey uniquely identifies one typed directed edge on one symbol.
*/
type edgeKey struct {
	symbol string
	from   uint8
	to     uint8
	kind   uint8
}

func makeEdgeKey(symbol string, from, to types.CategoryType, kind RelationType) edgeKey {
	return edgeKey{
		symbol: symbol,
		from:   categoryID(from),
		to:     categoryID(to),
		kind:   relationID(kind),
	}
}

/*
nodeKey uniquely identifies one category node on one symbol.
*/
type nodeKey struct {
	symbol string
	kind   uint8
}

func makeNodeKey(symbol string, kind types.CategoryType) nodeKey {
	return nodeKey{
		symbol: symbol,
		kind:   categoryID(kind),
	}
}

/*
Node holds the latest composed activation for a category on a symbol.
*/
type Node struct {
	Symbol    string             `json:"symbol"`
	Type      types.CategoryType `json:"type"`
	Strength  float64            `json:"strength"`
	Freshness float64            `json:"freshness"`
	At        time.Time          `json:"at"`
}
