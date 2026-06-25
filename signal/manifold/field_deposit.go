package manifold

import (
	"fmt"
)

func (field *Field) depositBook(state *UniverseState, activeCarriers int) error {
	if state.tickSize <= 0 {
		return fmt.Errorf("manifold: tick size must be positive for %q", state.symbol)
	}

	if activeCarriers <= 0 {
		return fmt.Errorf("manifold: active carrier count must be positive")
	}

	bids := truncateLevels(state.book.Bids, 1)
	asks := truncateLevels(state.book.Asks, 1)

	for _, level := range bids {
		if depositErr := field.depositBookLevel(
			state, level.Price, level.Qty, activeCarriers, -1,
		); depositErr != nil {
			return depositErr
		}
	}

	for _, level := range asks {
		if depositErr := field.depositBookLevel(
			state, level.Price, level.Qty, activeCarriers, 1,
		); depositErr != nil {
			return depositErr
		}
	}

	return nil
}

func (field *Field) depositBookLevel(
	state *UniverseState,
	price, qty float64,
	activeCarriers int,
	sideSign float64,
) error {
	if qty <= 0 {
		return nil
	}

	offsetTicks := (price - state.midPrice) / state.tickSize
	coords := field.universe.coords(state, offsetTicks)

	rho, rhoErr := field.liquidityRho(state, qty, activeCarriers)

	if rhoErr != nil {
		return rhoErr
	}

	momentum := sideSign * rho

	return field.solver.DepositCell(
		coords.cellX,
		coords.cellY,
		coords.cellZ,
		rho,
		momentum,
		0,
		0,
		rho*field.config.CV,
	)
}
