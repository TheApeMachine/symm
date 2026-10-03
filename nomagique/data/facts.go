package data

import (
	"fmt"
	"iter"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
CrossMember is one member's latest retained change facts.
*/
type CrossMember struct {
	Label  string
	Change float64
	At     time.Time
	From   time.Time
}

/*
MetricGate classifies the arrival against one declared metric: the metric must
exist and hold a non-negative value. It rewrites the validated metric on every arrival.
A failed classification sets the measurement's error and still yields.
*/
type MetricGate struct {
	err   error
	label string
}

func NewMetricGate(label string) core.Primitive {
	return &MetricGate{label: label}
}

func (op *MetricGate) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**Measurement[float64])(arriving)

			metric, holds := m.LookupMetric(op.label)

			if !holds {
				m.Err = fmt.Errorf("%w: metric gate requires %s", core.ErrDomain, op.label)

				if !yield(arriving) {
					return
				}

				continue
			}

			value := metric.Raw

			if value < 0 {
				m.Err = fmt.Errorf(
					"%w: metric gate requires a non-negative %s",
					core.ErrDomain, op.label,
				)

				if !yield(arriving) {
					return
				}

				continue
			}

			m.SetMetric(op.label, metric.Write(value))

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *MetricGate) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = fmt.Errorf("%w: %s", err, op.label)
		}
	}

	return op.err
}
