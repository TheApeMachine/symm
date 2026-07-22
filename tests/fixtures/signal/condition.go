package signal

import "time"

/*
bookCondition expresses semantic liquidity regimes directly against the
authoritative fixture book so every generated Kraken feed observes one cause.
*/
func (signal *Signal) bookCondition(
	state State,
	selected map[string]bool,
) []Step {
	actions := make([]Action, 0, len(selected)*2)

	for _, symbol := range signal.symbols {
		if !selected[symbol] {
			continue
		}

		market := signal.markets[symbol]

		switch state {
		case ThinLiquidity:
			ask := market.asks[touchIndex(market.asks, "sell")]
			actions = append(actions, Action{
				Kind:   Trade,
				Symbol: symbol,
				Side:   "buy",
				Qty:    ask.Qty - idleVolume,
			})
		case LoadedLiquidity:
			wall := 0.0

			for _, orders := range [][]Order{market.bids, market.asks} {
				for _, order := range orders {
					wall += order.Qty
				}
			}

			actions = append(actions, Action{
				Kind:   Refill,
				Symbol: symbol,
				Side:   "buy",
				Qty:    wall * initialOrderQuantity / idleVolume,
			})
		case LiquidityRetreat:
			bid := market.bids[touchIndex(market.bids, "buy")]
			actions = append(actions,
				Action{
					Kind:   Add,
					Symbol: symbol,
					Side:   "buy",
					Price:  bid.Price,
					Qty:    idleVolume,
				},
				Action{
					Kind:   Trade,
					Symbol: symbol,
					Side:   "buy",
					Qty:    idleVolume,
				},
				Action{
					Kind:    Cancel,
					Symbol:  symbol,
					OrderID: bid.ID,
				},
			)
		}
	}

	steps := []Step{{
		Advance: time.Second,
		Actions: actions,
	}}

	if state != LoadedLiquidity {
		return steps
	}

	for range idleObservations + 1 {
		refills := make([]Action, 0, len(selected))

		for _, symbol := range signal.symbols {
			if selected[symbol] {
				refills = append(refills, Action{
					Kind:   Refill,
					Symbol: symbol,
					Side:   "buy",
					Qty:    idleVolume,
				})
			}
		}

		steps = append(steps, Step{
			Advance: time.Second,
			Actions: refills,
		})
	}

	return steps
}

/*
isBookCondition identifies regimes whose causal shape is a direct book action
rather than a generated directional price leg.
*/
func (state State) isBookCondition() bool {
	return state >= ThinLiquidity && state <= LiquidityRetreat
}
