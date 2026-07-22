package broker

import (
	"encoding/json"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBalancePublishMarshalsHoldings(t *testing.T) {
	Convey("Given a balance with no open holdings", t, func() {
		ui := make(chan []byte, 1)
		balance := NewBalance(nil, nil, ui)
		balance.Publish()

		Convey("It enqueues JSON with a holdings array", func() {
			So(len(ui), ShouldEqual, 1)
			payload := <-ui
			So(json.Valid(payload), ShouldBeTrue)
			So(string(payload), ShouldContainSubstring, `"holdings"`)
			So(string(payload), ShouldContainSubstring, `"balances"`)
		})
	})
}
