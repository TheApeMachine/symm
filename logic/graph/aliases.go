package graph

import (
	"time"

	"github.com/theapemachine/symm/types"
)

/*
Type aliases re-export the graph domain types that now live in the types
package, so the graph solver keeps reading Graph/Node/Edge and the Kind* /
Relation* / Source* vocabulary without a qualified prefix on every reference.
*/
type (
	RelationType       = types.RelationType
	Kind               = types.Kind
	Node               = types.Node
	Edge               = types.Edge
	Graph              = types.Graph
	OpportunitySummary = types.OpportunitySummary
	OpportunityScore   = types.OpportunityScore
)

/*
NewGraph re-exports the types package graph constructor for strategy callers.
*/
func NewGraph(at time.Time) *Graph {
	return types.NewGraph(at)
}

const (
	KindCategory    = types.KindCategory
	KindCausal      = types.KindCausal
	KindCognition   = types.KindCognition
	KindHypothesis  = types.KindHypothesis
	KindManifold    = types.KindManifold
	KindMeasurement = types.KindMeasurement
	KindPrediction  = types.KindPrediction
	KindResonance   = types.KindResonance
)

const (
	RelationConditions       = types.RelationConditions
	RelationContradicts      = types.RelationContradicts
	RelationIncomparableWith = types.RelationIncomparableWith
	RelationIndependentOf    = types.RelationIndependentOf
	RelationLags             = types.RelationLags
	RelationLeads            = types.RelationLeads
	RelationRedundantWith    = types.RelationRedundantWith
	RelationStaleRelativeTo  = types.RelationStaleRelativeTo
	RelationSupports         = types.RelationSupports
)

const (
	SourceCategory  = types.SourceCategory
	SourceCausal    = types.SourceCausal
	SourceCognition = types.SourceCognition
	SourceGraph     = types.SourceGraph
	SourceManifold  = types.SourceManifold
	SourcePlanner   = types.SourcePlanner
	SourceResonance = types.SourceResonance
)
