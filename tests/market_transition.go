package tests

import "fmt"

/*
Express runs a transitioned regime through its complete ignition decay.
*/
func (market *Market) Express(symbol string) error {
	generator, ok := market.generators[symbol]

	if !ok {
		return fmt.Errorf("market: cannot express unknown symbol %q", symbol)
	}

	for !generator.IgnitionSpent() {
		market.Tick()
	}

	return nil
}

/*
ExpressAll advances one timeline until every selected ignition is spent.
*/
func (market *Market) ExpressAll(symbols []string) error {
	for _, symbol := range symbols {
		if _, known := market.generators[symbol]; !known {
			return fmt.Errorf("market: cannot express unknown symbol %q", symbol)
		}
	}

	for {
		pending := false

		for _, symbol := range symbols {
			if !market.generators[symbol].IgnitionSpent() {
				pending = true
				break
			}
		}

		if !pending {
			return nil
		}

		market.Tick()
	}
}

/*
Flatten runs the market until the driven desk carries no lot for the symbol.
*/
func (market *Market) Flatten(symbol string) error {
	if market.stack == nil {
		return fmt.Errorf(
			"market: %s cannot be run flat without a driven stack", symbol,
		)
	}

	for range market.Config.FlattenTickLimit {
		if market.stack.Holding(symbol) == 0 {
			return nil
		}

		market.Tick()
	}

	return fmt.Errorf(
		"market: %s was still held after %d ticks",
		symbol, market.Config.FlattenTickLimit,
	)
}
