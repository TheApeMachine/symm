package compiler

import (
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Registry maps the string types from the JSON graph to factory functions
that instantiate their `Value[any, any]` wrappers. 
This allows us to dynamically wire generic closures at runtime.
*/
var Registry = map[string]func() types.Value[any, any]{
	// Arithmetic Atoms (Stateless wrapped as constructors for consistency)
	"arithmetic.Multiply": func() types.Value[any, any] {
		return func(in any) any {
			return arithmetic.Multiply(in.([2]float64))
		}
	},
	"arithmetic.Divide": func() types.Value[any, any] {
		return func(in any) any {
			return arithmetic.Divide(in.([2]float64))
		}
	},
	
	// Stateful Arithmetic Atoms
	"arithmetic.Sum": func() types.Value[any, any] {
		sum := arithmetic.NewSum() // Initialize state once
		return func(in any) any {
			return sum(in.(float64)) // Execute in hot path
		}
	},

	// Data Atoms
	"data.Extract": func() types.Value[any, any] {
		extract := data.NewExtract("value") // Simplified for runtime demo
		return func(in any) any {
			return extract(in.(map[string]any))
		}
	},

	// Calculus Atoms
	"calculus.Tanh": func() types.Value[any, any] {
		return func(in any) any {
			return calculus.Tanh(in.(float64))
		}
	},

	// Temporal Atoms
	"temporal.Rate": func() types.Value[any, any] {
		vel := temporal.NewVelocity()
		return func(in any) any {
			return vel(in.([2]float64))
		}
	},

	// Statistical Atoms
	"statistic.ResidualBaseline": func() types.Value[any, any] {
		baseline := statistic.NewResidualBaseline()
		return func(in any) any {
			return baseline(in.(float64))
		}
	},
	"statistic.ResidualDivergence": func() types.Value[any, any] {
		divergence := statistic.NewResidualDivergence()
		return func(in any) any {
			return divergence(in.(float64))
		}
	},
	"statistic.ResidualZScore": func() types.Value[any, any] {
		zscore := statistic.NewZScore()
		return func(in any) any {
			return zscore(in.(float64))
		}
	},
}

/*
Primitives returns a UI-compatible definition list of available primitive nodes.
*/
func Primitives() (any, error) {
	res := make(map[string]map[string]any)
	for k := range Registry {
		res[k] = map[string]any{
			"type":    k,
			"label":   k,
			"inputs":  []any{},
			"outputs": []any{},
		}
	}
	return res, nil
}

