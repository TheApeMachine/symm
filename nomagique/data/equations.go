package data

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
Equation is a pure Value closure that evaluates an equation over a measurement's metrics.
*/
type Equation = types.Value[*Measurement[float64], *Measurement[float64]]

/*
NewUnaryEquation returns a Value closure that reads a left metric from a measurement,
transforms its raw scalar via op, and writes the result into the output metric.
No structs, pure Value closure.
*/
func NewUnaryEquation(output, left string, op types.Value[float64, float64]) Equation {
	return func(m *Measurement[float64]) *Measurement[float64] {
		if m == nil || m.Err != nil || op == nil {
			return m
		}

		leftMetric, holds := m.Metrics[left]
		if !holds {
			m.Err = fmt.Errorf("equations: %s is not a declared metric", left)
			return m
		}

		m.Metrics[output] = m.Metrics[output].Write(op(leftMetric.Raw))
		return m
	}
}

/*
NewBinaryEquation returns a Value closure that reads left and right metrics from a measurement,
transforms the [2]float64 pair via op, and writes the result into the output metric.
No structs, pure Value closure.
*/
func NewBinaryEquation(output, left, right string, op types.Value[[2]float64, float64]) Equation {
	return func(m *Measurement[float64]) *Measurement[float64] {
		if m == nil || m.Err != nil || op == nil {
			return m
		}

		leftMetric, holds := m.Metrics[left]
		if !holds {
			m.Err = fmt.Errorf("equations: %s is not a declared metric", left)
			return m
		}

		rightMetric, holds := m.Metrics[right]
		if !holds {
			m.Err = fmt.Errorf("equations: %s is not a declared metric", right)
			return m
		}

		m.Metrics[output] = m.Metrics[output].Write(op([2]float64{leftMetric.Raw, rightMetric.Raw}))
		return m
	}
}

/*
NewEquations composes multiple equation Value closures into a single measurement pipeline.
No structs, pure Value composition.
*/
func NewEquations(equations ...Equation) Equation {
	return func(m *Measurement[float64]) *Measurement[float64] {
		for _, eq := range equations {
			if m == nil || m.Err != nil {
				return m
			}
			m = eq(m)
		}
		return m
	}
}
