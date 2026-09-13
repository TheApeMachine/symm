package data

import (
	"fmt"
	"iter"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/logic"
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
exist and hold a finite, non-negative value. It rewrites the validated metric
and stamps the support baseline on every arrival, so the measurement carries
this arrival's fact, never the prior one's. A failed classification sets the
measurement's error and still yields.
*/
type MetricGate struct {
	err    error
	label  string
	finite core.Primitive
}

func NewMetricGate(label string) core.Primitive {
	return &MetricGate{label: label, finite: logic.NewFinite()}
}

func (op *MetricGate) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**Measurement[float64])(arriving)

			metric, holds := m.Metrics[op.label]

			if !holds {
				m.Err = fmt.Errorf("%w: metric gate requires %s", core.ErrDomain, op.label)

				if !yield(arriving) {
					return
				}

				continue
			}

			value := metric.Raw

			if m.Metadata == nil {
				m.Metadata = make(map[string]float64, 1)
			}

			m.Metadata[MetadataSupport] = 0

			finite := drive[float64, bool](op.finite, &value)

			if err := op.finite.Error(); err != nil {
				m.Err = err

				if !yield(arriving) {
					return
				}

				continue
			}

			if !finite || value < 0 {
				m.Err = fmt.Errorf(
					"%w: metric gate requires a finite non-negative %s",
					core.ErrDomain, op.label,
				)

				if !yield(arriving) {
					return
				}

				continue
			}

			m.Metrics[op.label] = metric.Write(value)

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
