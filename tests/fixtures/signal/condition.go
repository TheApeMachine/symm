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
			actions = append(actions,
				Action{Kind: Cancel, Symbol: symbol, OrderID: ask.ID},
				Action{
					Kind:   Add,
					Symbol: symbol,
					Side:   "sell",
					Price:  ask.Price,
					Qty:    idleVolume,
				},
			)
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
				Qty:    wall,
			})
		case LiquidityRetreat:
			bid := market.bids[touchIndex(market.bids, "buy")]
			actions = append(actions,
				Action{
					Kind:    Cancel,
					Symbol:  symbol,
					OrderID: bid.ID,
				},
				Action{
					Kind: WidenSpread, Symbol: symbol, Ticks: 2,
				},
			)
		case SpoofLiquidity:
			bid := market.bids[touchIndex(market.bids, "buy")]
			wall := 0.0

			for _, orders := range [][]Order{market.bids, market.asks} {
				for _, order := range orders {
					wall += order.Qty
				}
			}

			actions = append(actions,
				Action{
					Kind: Refill, Symbol: symbol, Side: "sell",
					Qty: wall,
				},
			)

			for range len(market.bids) + len(market.asks) {
				actions = append(actions, Action{
					Kind:   Add,
					Symbol: symbol,
					Side:   "buy",
					Price:  bid.Price - PriceIncrement,
					Qty:    wall,
				})
			}
		case DepthThinning:
			bid := market.bids[touchIndex(market.bids, "buy")]
			ask := market.asks[touchIndex(market.asks, "sell")]
			wall := 0.0

			for _, orders := range [][]Order{market.bids, market.asks} {
				for _, order := range orders {
					wall += order.Qty
				}
			}

			if bid.Qty < ask.Qty {
				actions = append(actions, Action{
					Kind: Refill, Symbol: symbol, Side: "buy", Qty: ask.Qty - bid.Qty,
				})
			}

			if ask.Qty < bid.Qty {
				actions = append(actions, Action{
					Kind: Refill, Symbol: symbol, Side: "sell", Qty: bid.Qty - ask.Qty,
				})
			}

			for level := bookLevels + 1; level <= bookLevels*2; level++ {
				distance := float64(level) * PriceIncrement
				actions = append(actions,
					Action{
						Kind: Add, Symbol: symbol, Side: "buy",
						Price: bid.Price - distance, Qty: initialOrderQuantity,
					},
					Action{
						Kind: Add, Symbol: symbol, Side: "sell",
						Price: ask.Price + distance, Qty: initialOrderQuantity,
					},
				)
			}

			for level := bookLevels*2 + 1; level <= bookLevels*5; level++ {
				actions = append(actions, Action{
					Kind:   Add,
					Symbol: symbol,
					Side:   "buy",
					Price:  bid.Price - float64(level)*PriceIncrement,
					Qty:    wall,
				})
			}
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
	return state >= ThinLiquidity && state <= DepthThinning
}
