package associative

import (
	"fmt"
	"iter"
	"sort"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/geometry"
	"github.com/theapemachine/symm/nomagique/probability"
)

/*
Region extracts the dominant attractor basin token from the segmented associative
grid edges using Argmax over accumulated basin authorities.
*/
type Region struct {
	*core.PrimitiveError
	out []byte
}

func NewRegion() *Region {
	return &Region{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (region *Region) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if region.Error() != nil {
			return
		}

		basinAuthority := make(map[int]float64)

		for arriving := range in {
			if arriving == nil {
				continue
			}

			edge := (*geometry.Edge)(arriving)
			basinAuthority[edge.Basin[0]] += edge.Authority[0]
			basinAuthority[edge.Basin[1]] += edge.Authority[1]
		}

		if len(basinAuthority) == 0 {
			return
		}

		basinIDs := make([]int, 0, len(basinAuthority))
		for b := range basinAuthority {
			basinIDs = append(basinIDs, b)
		}
		sort.Ints(basinIDs)

		argmax := probability.NewArgmax()
		argmaxIn := func(yieldVal func(unsafe.Pointer) bool) {
			for _, b := range basinIDs {
				val := basinAuthority[b]
				if !yieldVal(unsafe.Pointer(&val)) {
					return
				}
			}
		}

		var winningBasin int
		for out := range argmax.Next(argmaxIn) {
			res := (*probability.ArgmaxResult)(out)
			winningBasin = basinIDs[res.Index]
		}

		region.out = fmt.Appendf(nil, "r%d", winningBasin)
		yield(unsafe.Pointer(&region.out))
	}
}
