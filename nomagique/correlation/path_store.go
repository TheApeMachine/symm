package correlation

import (
	"iter"
	"slices"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

type PathStore struct {
	*core.PrimitiveError
	paths   map[string][][2]float64
	current string
}

func NewPathStore() *PathStore {
	return &PathStore{
		PrimitiveError: core.NewPrimitiveError(),
		paths:          make(map[string][][2]float64),
	}
}

func (ps *PathStore) SetCurrent(label string) {
	ps.current = label
}

func (ps *PathStore) Current() string {
	return ps.current
}

func (ps *PathStore) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				continue
			}

			mapping := *(*map[string][][2]float64)(arriving)

			if len(mapping) == 0 {
				if !yield(unsafe.Pointer(&ps.paths)) {
					return
				}
				continue
			}

			for key, val := range mapping {
				ps.paths[key] = val
			}

			if !yield(unsafe.Pointer(&ps.paths)) {
				return
			}
		}
	}
}

func (ps *PathStore) Peers(label string) []string {
	peers := make([]string, 0, len(ps.paths))

	for key := range ps.paths {
		if key != label {
			peers = append(peers, key)
		}
	}

	slices.Sort(peers)
	return peers
}
