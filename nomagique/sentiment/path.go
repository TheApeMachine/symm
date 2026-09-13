package sentiment

import (
	"errors"
	iter "iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
path holds one symbol's retained price-path state: the previous observation's
price and split-second timestamp, and the observation count.
*/
type path struct {
	previousPrice float64
	previousSec   float64
	previousNsec  float64
	count         int
	hasPrice      bool
}

/*
Return owns the per-symbol price paths: it accepts each arrival in timestamp
order, derives the log return against the retained previous price, and stamps
the path's observation count as the measurement's support. A timestamp that
regresses behind the retained one never touches the path; the measurement
moves on carrying zero support and the regression's provenance.
*/
type Return struct {
	err   error
	paths map[string]*path
}

func NewReturn() core.Primitive {
	return &Return{paths: make(map[string]*path)}
}

func (op *Return) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement[float64])(arriving)
			last := m.Metrics["last"].Raw

			if m.Err != nil || last == 0 {
				if !yield(arriving) {
					return
				}

				continue
			}

			state := op.paths[m.Label]

			if state == nil {
				state = &path{}
				op.paths[m.Label] = state
			}

			sec := float64(m.At.Unix())
			nsec := float64(m.At.Nanosecond())

			if state.hasPrice && (sec < state.previousSec || (sec == state.previousSec && nsec < state.previousNsec)) {
				if m.Provenance == nil {
					m.Provenance = make(map[string]string)
				}

				m.Provenance["event_time_state"] = "regressed"

				if !yield(arriving) {
					return
				}

				continue
			}

			previousPrice := state.previousPrice
			hasPrevious := state.hasPrice

			state.count++
			state.previousPrice = last
			state.previousSec = sec
			state.previousNsec = nsec
			state.hasPrice = true

			if hasPrevious && previousPrice > 0 && last > 0 {
				logReturn := math.Log(last / previousPrice)

				m.Metrics["return"] = m.Metrics["return"].Write(logReturn)
				m.Metrics["absolute_return"] = m.Metrics["absolute_return"].Write(math.Abs(logReturn))
			}

			m.Metadata[data.MetadataSupport] = float64(state.count)

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *Return) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
