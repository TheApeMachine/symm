package correlation

import (
	"errors"
	"iter"
	"sort"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/temporal"
)

/*
Focal owns every symbol's price path. The symbol the arrival names advances
its path; every other symbol contributes its most recent accepted reading as
a peer. The paths are Path primitives composed with an adaptive retention
window; this stage only owns which path belongs to which symbol.
*/
type Focal struct {
	err    error
	paths  map[string]core.Primitive
	window func() core.Primitive
	Path   map[string]PathReading
}

func NewFocal() core.Primitive {
	return &Focal{
		paths:  make(map[string]core.Primitive),
		window: adaptive.NewWindow,
		Path:   make(map[string]PathReading),
	}
}

func (op *Focal) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			reading := (*Reading)(arriving)

			if reading.State != StateTraded {
				if !yield(arriving) {
					return
				}

				continue
			}

			path := op.paths[reading.Symbol]

			if path == nil {
				path = NewPath(op.window())
				op.paths[reading.Symbol] = path
			}

			focal := drive[temporal.Price, PathReading](path, &reading.Price)

			if focal.Accepted {
				op.Path[reading.Symbol] = focal
			}

			reading.Focal = focal
			reading.Peers = peers(op.Path, reading.Symbol)

			if !yield(arriving) {
				return
			}
		}
	}
}

/*
peers snapshots the other symbols' retained readings in lexicographic order,
so pair selection stays deterministic.
*/
func peers(retained map[string]PathReading, focal string) []PeerReading {
	symbols := make([]string, 0, len(retained))

	for symbol := range retained {
		if symbol != focal {
			symbols = append(symbols, symbol)
		}
	}

	sort.Strings(symbols)
	peerReadings := make([]PeerReading, 0, len(symbols))

	for _, symbol := range symbols {
		peerReadings = append(peerReadings, PeerReading{Symbol: symbol, Reading: retained[symbol]})
	}

	return peerReadings
}

func (op *Focal) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
