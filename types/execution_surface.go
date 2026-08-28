package types

import (
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
)

/*
ExecutionSurface is the compact, immutable executable-liquidation fact a
PositionGuardian consumes after one committed L3 book frame. It never retains
the book: every field is derived synchronously while the authoritative book is
safely readable, then the book pointer is released. It carries no history and
no snapshots; one open position holds exactly one current surface.
*/
type ExecutionSurface struct {
	Symbol string
	At     time.Time

	// SellableQty is the position's actual current sellable inventory, not the
	// intended, original-allocation, or pre-fill quantity.
	SellableQty      *decimal.Decimal
	BestBid          *decimal.Decimal
	ExecutableQty    *decimal.Decimal
	ExecutableVWAP   *decimal.Decimal
	ExecutableValue  *decimal.Decimal
	FloorCoverageQty *decimal.Decimal

	// BookComplete reports whether a valid, non-crossed authoritative bid side
	// was readable. FullyExecutable reports whether the complete SellableQty
	// is executable from the visible valid depth; when false the position does
	// not possess a full-lot executable mark and protection must own the event.
	BookComplete    bool
	FullyExecutable bool
}
