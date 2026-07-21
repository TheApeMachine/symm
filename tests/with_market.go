package tests

import . "github.com/smartystreets/goconvey/convey"

/*
WithMarket decorates a test closure with the requested fixture state sequence.
*/
func WithMarket(
	market *Market,
	states []MarketState,
	afterStep func() error,
	test func(),
) func() {
	return func() {
		for _, state := range states {
			So(market.Transition(state, afterStep), ShouldBeNil)
		}

		test()
	}
}
