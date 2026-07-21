package tests

/*
WithMarket decorates a test closure with the requested fixture state sequence.
*/
func WithMarket(
	market *Market,
	states []MarketState,
	test func(),
) func() {
	return func() {
		for _, state := range states {
			market.Transition(state)
		}

		test()
	}
}
