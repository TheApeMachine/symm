package associative

import (
	"fmt"
	"iter"
	"sort"
	"strings"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/geometry"
	"github.com/theapemachine/symm/nomagique/probability"
)

/*
Region extracts the dominant attractor basin signature from the segmented associative
grid edges using Argmax over accumulated basin authorities. The signature is derived
from the member node topology of the winning basin, ensuring stability across reconstructions.
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
		if region.Error() != nil || in == nil {
			return
		}

		basinAuthority := make(map[int]float64)
		basinMembers := make(map[int]map[string]struct{})

		for arriving := range in {
			if arriving == nil {
				continue
			}

			edge := (*geometry.Edge)(arriving)
			basinAuthority[edge.Basin[0]] += edge.Authority[0]
			basinAuthority[edge.Basin[1]] += edge.Authority[1]

			if _, exists := basinMembers[edge.Basin[0]]; !exists {
				basinMembers[edge.Basin[0]] = make(map[string]struct{})
			}

			if edge.Left != nil {
				var coordinates []int
				for coordPtr := range edge.Left.Next(nil) {
					if coordPtr != nil {
						coordinates = append(coordinates, *(*int)(coordPtr))
					}
				}

				if len(coordinates) == 2 {
					basinMembers[edge.Basin[0]][fmt.Sprintf("%d,%d", coordinates[0], coordinates[1])] = struct{}{}
				}
			}

			if _, exists := basinMembers[edge.Basin[1]]; !exists {
				basinMembers[edge.Basin[1]] = make(map[string]struct{})
			}

			if edge.Right != nil {
				var coordinates []int
				for coordPtr := range edge.Right.Next(nil) {
					if coordPtr != nil {
						coordinates = append(coordinates, *(*int)(coordPtr))
					}
				}

				if len(coordinates) == 2 {
					basinMembers[edge.Basin[1]][fmt.Sprintf("%d,%d", coordinates[0], coordinates[1])] = struct{}{}
				}
			}
		}

		if len(basinAuthority) == 0 {
			return
		}

		basinIDs := make([]int, 0, len(basinAuthority))
		for basinID := range basinAuthority {
			basinIDs = append(basinIDs, basinID)
		}
		sort.Ints(basinIDs)

		argmax := probability.NewArgmax()
		argmaxIn := func(yieldVal func(unsafe.Pointer) bool) {
			for _, basinID := range basinIDs {
				val := basinAuthority[basinID]
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

		members := make([]string, 0, len(basinMembers[winningBasin]))
		for memberName := range basinMembers[winningBasin] {
			members = append(members, memberName)
		}
		sort.Strings(members)

		if len(members) > 0 {
			region.out = []byte(strings.Join(members, ";"))
		}

		if len(members) == 0 {
			region.out = fmt.Appendf(nil, "b%d", winningBasin)
		}

		yield(unsafe.Pointer(&region.out))
	}
}
