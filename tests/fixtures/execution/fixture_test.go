package execution_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	executionfixture "github.com/theapemachine/symm/tests/fixtures/execution"
)

func TestExecutionFrame(t *testing.T) {
	Convey("Given named execution scenarios", t, func() {
		for _, testCase := range []struct {
			caseName string
			options  executionfixture.Options
			orderID  string
			execID   string
		}{
			{caseName: "buy fill", options: executionfixture.BuyFill(), orderID: "order-1", execID: "fill-2"},
			{caseName: "exit fill", options: executionfixture.ExitFill(), orderID: "exit-1", execID: "exit-fill"},
			{caseName: "reduce fill", options: executionfixture.ReduceFill(), orderID: "reduce-1", execID: "reduce-fill"},
		} {
			Convey(testCase.caseName, func() {
				payload := executionfixture.Frame(testCase.options)

				So(string(payload), ShouldContainSubstring, testCase.orderID)
				So(string(payload), ShouldContainSubstring, testCase.execID)
				So(string(payload), ShouldContainSubstring, `"channel":"executions"`)
			})
		}
	})
}
