package tests

import . "github.com/smartystreets/goconvey/convey"

/*
Expectation binds one actual value to a GoConvey assertion and optional args.
*/
type Expectation struct {
	Actual   any
	Assert   Assertion
	Expected []any
}

/*
AssertExpectations applies table-driven expectations through GoConvey matchers.
*/
func AssertExpectations(expectations ...Expectation) {
	for _, expectation := range expectations {
		if len(expectation.Expected) == 0 {
			So(expectation.Actual, expectation.Assert)

			continue
		}

		So(expectation.Actual, expectation.Assert, expectation.Expected...)
	}
}
