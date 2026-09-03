package nomagique

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/types"
)

// Re-exported type aliases from domain subpackages so that the root package
// provides top-level access to the full nomagique v2 ecosystem.
type (
	// Core carrier and topological algebra (from nomagique/types)
	Sum     = types.Sum
	Product = types.Product

	// Temporal primitives (from nomagique/temporal)
	Decay    = temporal.Decay
	Governor = temporal.Governor

	// Stateful memory store (from nomagique/store)
	Store     = store.Store
	StoreType = store.StoreType

	// Statistical primitives (from nomagique/statistic)
	Standardize = statistic.Standardize

	// Equations (from nomagique/equation)
	Standardizer = equation.Standardizer

	// Calculus transfer functions (from nomagique/calculus)
	Exponential = calculus.ExponentialNode
	Linear      = calculus.LinearNode

	// Routing and blending logic (from nomagique/logic)
	Pick            = logic.Pick
	Mix             = logic.Mix
	VolatilityBlend = logic.VolatilityBlend
)

const (
	DynamicRing = store.DynamicRing
)

var (
	// Reductions
	Average      = statistic.Average
	Median       = statistic.MedianReduction
	LinearSlope  = calculus.LinearSlope
	Min          = calculus.Min
	Max          = calculus.Max
	SumReduction = calculus.SumReduction
)
