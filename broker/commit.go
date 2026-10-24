package broker

import (
	"fmt"

	"github.com/theapemachine/errnie"
)

/*
ClosedFill is the priced round trip a commit stores with the labelled ticks.
*/
type ClosedFill struct {
	Epoch                 uint64
	EntryPrice, ExitPrice float64
	EntryAt, ExitAt       int64
}

/*
ClosedFill finds the archived virtual lot opened at entry and closed at exit.
The commit button identifies the lot by those ticks.
*/
func (desk *Desk) ClosedFill(symbol string, entry, exit uint64) (ClosedFill, error) {
	if desk == nil {
		return ClosedFill{}, errnie.Err(errnie.Validation, "[hindsight] desk unavailable", nil)
	}

	seen := map[string]struct{}{}
	var matches []*Position

	add := func(position *Position) {
		if position == nil || !position.Virtual || position.Symbol != symbol || position.ID == "" {
			return
		}

		if position.EntryTick != entry || position.ExitTick != exit {
			return
		}

		if _, ok := seen[position.ID]; ok {
			return
		}

		seen[position.ID] = struct{}{}
		matches = append(matches, position)
	}

	if desk.virtuals != nil {
		for _, position := range desk.virtuals.Closed() {
			add(position)
		}
	}

	if desk.virtualArchive != nil {
		desk.virtualArchive.Range(func(_, value any) bool {
			position, _ := value.(*Position)
			add(position)
			return true
		})
	}

	if len(matches) == 0 {
		return ClosedFill{}, errnie.Err(
			errnie.NotFound,
			fmt.Sprintf("[hindsight] no closed virtual %s from tick %d to %d", symbol, entry, exit),
			nil,
		)
	}

	if len(matches) > 1 {
		return ClosedFill{}, errnie.Err(
			errnie.Validation,
			fmt.Sprintf("[hindsight] %d closed virtuals for %s from tick %d to %d", len(matches), symbol, entry, exit),
			nil,
		)
	}

	return fillOf(matches[0])
}

func fillOf(position *Position) (ClosedFill, error) {
	position.mu.RLock()
	defer position.mu.RUnlock()

	if position.EntryPrice == nil || position.EntryPrice.Sign() <= 0 {
		return ClosedFill{}, errnie.Err(
			errnie.UnprocessableContent,
			"[hindsight] "+position.Symbol+" has no entry price",
			nil,
		)
	}

	exit, ok := exitPrice(position)
	if !ok {
		return ClosedFill{}, errnie.Err(
			errnie.UnprocessableContent,
			"[hindsight] "+position.Symbol+" has no exit price",
			nil,
		)
	}

	fill := ClosedFill{
		Epoch:      position.PolicyEpoch,
		EntryPrice: position.EntryPrice.Float64(),
		ExitPrice:  exit,
	}

	if !position.OpenedAt.IsZero() {
		fill.EntryAt = position.OpenedAt.UnixNano()
	}

	if position.ClosedAt != nil && !position.ClosedAt.IsZero() {
		fill.ExitAt = position.ClosedAt.UnixNano()
	}

	return fill, nil
}

func exitPrice(position *Position) (float64, bool) {
	if position.ExitPrice == nil || position.ExitPrice.Sign() <= 0 {
		return 0, false
	}

	return position.ExitPrice.Float64(), true
}
