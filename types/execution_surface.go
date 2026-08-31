package types

import (
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
)

/*
ExecutionSurface is the compact, immutable executable-liquidation fact a
PositionGuardian consumes after one committed L3 frame. It never retains the
execution state: every field is derived synchronously while the continuously-
resident execution state is safely readable, then the reducer read lock is
released. It carries no history and no snapshots; one open position holds
exactly one current surface.
*/
type ExecutionSurface struct {
	Symbol string
	At     time.Time

	// SellableQty is the position's actual current sellable inventory, not the
	// intended, original-allocation, or pre-fill quantity.
	SellableQty   *decimal.Decimal
	BestBid       *decimal.Decimal
	ExecutableQty *decimal.Decimal
	// ExecutableVWAP is the full-lot liquidation-equivalent GROSS price (raw
	// filled VWAP in price coordinate), comparable to the stoploss's gross
	// break-even geometry. The fee-net proceeds are ExecutableValue.
	ExecutableVWAP *decimal.Decimal
	// ExecutableValue is the fee-net liquidation proceeds in dollar/economic
	// coordinate (gross proceeds minus the sell fee). It is never divided into
	// a "fee-net price".
	ExecutableValue  *decimal.Decimal
	FloorCoverageQty *decimal.Decimal

	// BookComplete reports whether a valid, non-crossed authoritative bid side
	// was readable. FullyExecutable reports whether the complete SellableQty
	// is executable from the visible valid depth; when false the position does
	// not possess a full-lot executable mark and protection must own the event.
	BookComplete    bool
	FullyExecutable bool
}
