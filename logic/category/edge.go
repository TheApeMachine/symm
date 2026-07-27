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

/*
edgeKey uniquely identifies one typed directed edge on one symbol.
*/
type edgeKey struct {
	symbol string
	from   types.CategoryType
	to     types.CategoryType
	kind   RelationType
}

/*
nodeKey uniquely identifies one category node on one symbol.
*/
type nodeKey struct {
	symbol string
	kind   types.CategoryType
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
