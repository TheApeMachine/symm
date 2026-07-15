package orderack_test

import (
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	orderackfixture "github.com/theapemachine/symm/tests/fixtures/orderack"
)

func TestOrderAckFrame(t *testing.T) {
	Convey("Given order acknowledgement options", t, func() {
		for _, testCase := range []struct {
			caseName string
			options  orderackfixture.Options
			reqID    int
			orderID  string
		}{
			{
				caseName: "matching request identity",
				options:  orderackfixture.Options{ReqID: 7, OrderID: "right", Success: true},
				reqID:    7,
				orderID:  "right",
			},
			{
				caseName: "foreign request identity",
				options:  orderackfixture.Options{ReqID: 8, OrderID: "wrong", Success: true},
				reqID:    8,
				orderID:  "wrong",
			},
		} {
			Convey(testCase.caseName, func() {
				payload := orderackfixture.Frame(testCase.options)

				So(string(payload), ShouldContainSubstring, fmt.Sprintf(`"req_id":%d`, testCase.reqID))
				So(string(payload), ShouldContainSubstring, testCase.orderID)
			})
		}
	})
}

func TestOrderAckFixtureGenerate(t *testing.T) {
	Convey("Given an ordered acknowledgement sequence", t, func() {
		fixture := orderackfixture.NewFixture(
			orderackfixture.Options{ReqID: 8, OrderID: "wrong", Success: true},
			orderackfixture.Options{ReqID: 7, OrderID: "right", Success: true},
		)
		count := 0

		for range fixture.Generate() {
			count++
		}

		Convey("Then every configured acknowledgement is emitted in order", func() {
			So(count, ShouldEqual, 2)
		})
	})
}
