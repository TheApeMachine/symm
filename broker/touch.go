package broker

import (
	"fmt"
	"math"

	"github.com/theapemachine/errnie"
)

/*
CrossedTouch is the fault a touch consumer raises when the BookSource yields a
best ask at or below the best bid. Book withholds pending and
checksum-diverging books, so a crossed or locked touch that reaches a reader is
corrupt book state, not a market fact. The error is Internal so the reader's
System closes and the process halts; its Conflict cause and message carry the
symbol and touch, which keeps it distinct from a missing BookSource (a wiring
fault raised at construction).
*/
func CrossedTouch(owner, symbol string, bid, ask float64) error {
	return errnie.Err(
		errnie.Internal,
		fmt.Sprintf("[%s] crossed or locked book touch for %s: bid=%v ask=%v", owner, symbol, bid, ask),
		errnie.Err(errnie.Conflict, "book state is crossed or locked", nil),
	)
}

/*
InvalidTouch is the fault a touch consumer raises when the BookSource yields
a best bid and best ask that are both present but carry a non-finite or
non-positive price or quantity. An absent or one-sided book is a momentary
market state and is dropped by the reader; a present level with impossible
values is corrupt book state, so it halts exactly like CrossedTouch.
*/
func InvalidTouch(owner, symbol string, values map[string]float64) error {
	return errnie.Err(
		errnie.Internal,
		fmt.Sprintf("[%s] non-finite or non-positive book touch for %s: %v", owner, symbol, values),
		errnie.Err(errnie.Validation, "book touch values are invalid", nil),
	)
}

/*
ValidTouchValue reports whether a value read from a present book level is a
positive, finite number.
*/
func ValidTouchValue(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}
