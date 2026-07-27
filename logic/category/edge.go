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

var (
	categoryIDs = map[types.CategoryType]uint8{}
	relationIDs = map[RelationType]uint8{}
)

func init() {
	for idx, cat := range types.CategoryOrder {
		categoryIDs[cat] = uint8(idx + 1)
	}

	relations := []RelationType{
		Supports, Contradicts, Conditions, Leads, Lags,
		RedundantWith, IndependentOf, StaleRelativeTo, IncomparableWith,
	}
	for idx, rel := range relations {
		relationIDs[rel] = uint8(idx + 1)
	}
}

func categoryID(cat types.CategoryType) uint8 {
	return categoryIDs[cat]
}

func relationID(rel RelationType) uint8 {
	return relationIDs[rel]
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
		from:   categoryIDs[from],
		to:     categoryIDs[to],
		kind:   relationIDs[kind],
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
		kind:   categoryIDs[kind],
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
