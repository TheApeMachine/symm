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
	Number    = types.Number
	Node      = types.Node
	Identity  = types.IdentityNode
	Chain     = types.Chain
	Split     = types.Split
	Sum       = types.Sum
	Product   = types.Product
	Reduction = types.Reduction
	Router    = types.Router

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

	// Legacy types
	AtomicStream                = types.AtomicStream
	Frame                       = types.Frame
	KeyedNumber[Key comparable] = types.KeyedNumber[Key]
	Primitive                   = types.Primitive
	Single                      = types.Single
	Stream                      = types.Stream
	Symbol                      = types.Symbol
)

const (
	MaxSlots    = types.MaxSlots
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

	// Legacy functions
	Assign            = types.Assign
	Configure         = types.Configure
	FrameFromNamed    = types.FrameFromNamed
	Fork              = types.Fork
	ForkStrict        = types.ForkStrict
	In                = types.In
	Intern            = types.Intern
	Join              = types.Join
	MustIntern        = types.MustIntern
	NewAtomicStream   = types.NewAtomicStream
	NewSingle         = types.NewSingle
	NewStream         = types.NewStream
	Out               = types.Out
	Pipe              = types.Pipe
	PrimitiveError    = types.PrimitiveError
	Relay             = types.Relay
	RegisteredSymbols = types.RegisteredSymbols
	State             = types.State
	Step              = types.Step
	SymbolName        = types.SymbolName
	Wire              = types.Wire
)

// NewKeyedNumber composes primitives into one isolated numeric unit per key.
func NewKeyedNumber[Key comparable](primitives ...Primitive) *KeyedNumber[Key] {
	return types.NewKeyedNumber[Key](primitives...)
}

// NewKeyedNumberWithInitial provides the initial committed state for newly seen keys.
func NewKeyedNumberWithInitial[Key comparable](
	initial func(Key) Frame,
	primitives ...Primitive,
) *KeyedNumber[Key] {
	return types.NewKeyedNumberWithInitial[Key](initial, primitives...)
}

// NewNumber delegates to NewKeyedNumber for backwards compatibility.
func NewNumber[Key comparable](primitives ...Primitive) *KeyedNumber[Key] {
	return types.NewKeyedNumber[Key](primitives...)
}

// NewNumberWithInitial delegates to NewKeyedNumberWithInitial for backwards compatibility.
func NewNumberWithInitial[Key comparable](
	initial func(Key) Frame,
	primitives ...Primitive,
) *KeyedNumber[Key] {
	return types.NewKeyedNumberWithInitial[Key](initial, primitives...)
}
