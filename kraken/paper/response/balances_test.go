package response

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
)

func TestBalances(t *testing.T) {
	Convey("Given a balances response", t, func() {
		balances := NewBalances()

		Convey("It should send a balances update", func() {
			balances.Send(&qpool.QValue[any]{
				Type: "balances",
				Value: map[string]any{
					"method": "subscribe",
					"params": map[string]any{
						"channel":  "kraken:private",
						"snapshot": true,
					},
				},
			})

			So(balances.model, ShouldNotBeNil)
			So(balances.model.Asset[0].Asset, ShouldEqual, "EUR")
			So(balances.model.Asset[0].Balance, ShouldEqual, 200.0)
			So(balances.model.Asset[0].Wallets[0].Balance, ShouldEqual, 200.0)
		})
	})
}
