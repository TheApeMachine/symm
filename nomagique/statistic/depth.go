package statistic

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/types"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

/*
ExtractDepth is a lifter primitive: it reads two touch quantities from the
generic input vocabulary — the executable size resting on the bid side
(alpha quantity) and on the ask side (beta quantity) — plus the observable
price each one is a quantity of, and emits the executable two-sided depth
into the shared Quantity slot.

Because it consumes and produces only the generic nomagique input contract, any
upstream preset that emits an alpha quantity, a beta quantity, and the two
prices can feed this stage; nothing about the quote's provenance leaks into
the numeric contract.
*/
func ExtractDepth(input *types.Frame) {
	bidPrice, hasBidPrice := input.Get(nmtypes.AlphaPrice)
	askPrice, hasAskPrice := input.Get(nmtypes.BetaPrice)
	bidQty, hasBidQty := input.Get(nmtypes.AlphaQuantity)
	askQty, hasAskQty := input.Get(nmtypes.BetaQuantity)

	if !hasBidPrice || !hasAskPrice || !hasBidQty || !hasAskQty {
		input.Err = fmt.Errorf(
			"statistic: extract depth requires both touch prices and both touch quantities",
		)

		return
	}

	depth := 0.0

	if bidPrice > 0 && askPrice > bidPrice && bidQty > 0 && askQty > 0 {
		depth = math.Min(bidQty, askQty) * (bidPrice + askPrice) / 2
	}

	input.Put(nmtypes.Quantity, depth)
}
