package user

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/public"
)

func TestDecodeBalances(t *testing.T) {
	Convey("Given a balances snapshot envelope", t, func() {
		message := &public.SocketMessage{
			Channel: public.BalancesChannel,
			Type:    balanceSnapshot,
			Data: []byte(`[
				{
					"asset":"EUR",
					"asset_class":"currency",
					"balance":200,
					"wallets":[{"type":"spot","id":"main","balance":200}]
				}
			]`),
		}

		Convey("It should decode Kraken field names without renaming", func() {
			rows, err := DecodeBalances(message)

			So(err, ShouldBeNil)
			So(len(rows), ShouldEqual, 1)
			So(rows[0].Asset, ShouldEqual, "EUR")
			So(rows[0].Balance, ShouldEqual, 200)
			So(rows[0].Wallets[0].ID, ShouldEqual, "main")
			So(rows[0].IsSnapshot(), ShouldBeTrue)
		})
	})
}
