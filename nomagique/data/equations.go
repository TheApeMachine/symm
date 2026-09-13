package data

import (
	"errors"
	"fmt"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
drive evaluates one scalar payload through one primitive.
*/
func drive[From, To any](op core.Primitive, payload *From) To {
	var answer To

	for out := range op.Next(transport.NewOne(unsafe.Pointer(payload)).Next(nil)) {
		answer = *(*To)(out)
	}

	return answer
}

/*
driveAll evaluates one payload through one primitive and reports whether the
primitive yielded a fact at all, so an undefined answer stays unwritten.
*/
func driveAll[From, To any](op core.Primitive, payload *From) (To, bool) {
	var answer To
	answered := false

	for out := range op.Next(transport.NewOne(unsafe.Pointer(payload)).Next(nil)) {
		answer = *(*To)(out)
		answered = true
	}

	return answer, answered
}

/*
Equation is one declared binding over a measurement's facts: the named input
facts are lifted onto the wire, the binding's primitive transforms them, and
the answer is written to the named output fact. A second input name selects
the binary ([2]float64) wire shape; a single input name feeds the value
directly. The binding owns no mathematics and no state.
*/
type Equation struct {
	Output string
	Op     core.Primitive
	Left   string
	Right  string
}

/*
Equations applies declared fact equations to each arriving measurement in
declaration order. It owns no state: every fact it reads and writes belongs
to the measurement, and every transformation is the binding's own primitive.
An operation that yields no fact leaves its output unwritten, so undefined
stays unwritten.
*/
type Equations struct {
	err      error
	bindings []Equation
}

/*
NewEquations creates the fact-equation stage from declared bindings.
*/
func NewEquations(bindings ...Equation) core.Primitive {
	return &Equations{bindings: bindings}
}

func (op *Equations) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**Measurement[float64])(arriving)

			if m.Err != nil {
				if !yield(arriving) {
					return
				}

				continue
			}

			for _, binding := range op.bindings {
				if !op.apply(m, binding) {
					break
				}
			}

			if !yield(arriving) {
				return
			}
		}
	}
}

/*
apply evaluates one binding against the measurement. It reports whether the
measurement may carry on to further bindings: a binding naming a fact the
measurement does not hold fails the measurement.
*/
func (op *Equations) apply(m *Measurement[float64], binding Equation) bool {
	left, holds := m.Metrics[binding.Left]

	if !holds {
		m.Err = fmt.Errorf("%w: equations: %s is not a declared metric", core.ErrDomain, binding.Left)

		return false
	}

	if binding.Right == "" {
		value := left.Raw
		answer, answered := driveAll[float64, float64](binding.Op, &value)

		if answered {
			m.Metrics[binding.Output] = m.Metrics[binding.Output].Write(answer)
		}

		return true
	}

	right, holds := m.Metrics[binding.Right]

	if !holds {
		m.Err = fmt.Errorf("%w: equations: %s is not a declared metric", core.ErrDomain, binding.Right)

		return false
	}

	pair := [2]float64{left.Raw, right.Raw}
	answer, answered := driveAll[[2]float64, float64](binding.Op, &pair)

	if answered {
		m.Metrics[binding.Output] = m.Metrics[binding.Output].Write(answer)
	}

	return true
}

func (op *Equations) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
