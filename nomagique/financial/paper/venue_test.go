package paper_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/financial/paper"
)

func TestVenueApply(t *testing.T) {
	Convey("Given an exchange with an entry resting on BTC/USD", t, func() {
		client := primed("0.001")
		defer client.Release()
		So(commit(client, "BTC/USD", "ENTER", at(1)), ShouldBeNil)

		Convey("When a frame arrives for another symbol", func() {
			account := observe(client, snapshot("ETH/USD"), at(2))

			Convey("Then the entry keeps resting and nothing is spent", func() {
				So(account.Resting(), ShouldEqual, 1)
				So(held(account), ShouldBeEmpty)
				So(number(read(account.Cash())).Cmp(number("1000")), ShouldEqual, 0)
			})
		})

		Convey("When its book stops reconciling with the exchange's checksum", func() {
			broken := []byte(`{"channel":"level3","type":"update","data":[{"symbol":"BTC/USD","checksum":1,"bids":[],"asks":[]}]}`)
			account := observe(client, broken, at(2))

			Convey("Then the book is dropped and the entry waits for a snapshot, not a guess", func() {
				So(account.Books(), ShouldEqual, 0)
				account = observe(client, deepBid("add"), at(3))
				So(account.Resting(), ShouldEqual, 1)
				account = observe(client, snapshot("BTC/USD"), at(4))
				So(account.Resting(), ShouldEqual, 0)
				So(held(account), ShouldResemble, []string{"BTC/USD"})
			})
		})
	})

	Convey("Given a level3 frame without the subscribed depth", t, func() {
		client := paper.Exchange_ServerToClient(paper.NewExchange())
		defer client.Release()
		err := client.Write(context.Background(), func(params paper.Exchange_write_Params) error {
			return first(params.SetCapital("1000"), params.SetTime(at(0)), params.SetFrame(snapshot("BTC/USD")))
		})
		So(err, ShouldBeNil)
		So(client.WaitStreaming(), ShouldNotBeNil)
	})
}

func TestVenueExecute(t *testing.T) {
	Convey("Given a pair whose minimum quantity exceeds what a fifth of the cash buys", t, func() {
		client := primed("2")
		defer client.Release()
		So(commit(client, "BTC/USD", "ENTER", at(1)), ShouldBeNil)
		account := observe(client, deepBid("add"), at(2))

		Convey("Then the exchange refuses the order whole and the cash stays uncommitted", func() {
			So(account.Which(), ShouldEqual, paper.Account_Which_refused)
			So(read(account.Refused().Reason()), ShouldEqual, "below_minimum")
			So(number(read(account.Available())).Cmp(number("1000")), ShouldEqual, 0)
			So(held(account), ShouldBeEmpty)
		})
	})

	Convey("Given a symbol whose instrument rules never arrived", t, func() {
		client := paper.Exchange_ServerToClient(paper.NewExchange())
		defer client.Release()
		observe(client, snapshot("BTC/USD"), at(0))
		So(commit(client, "BTC/USD", "ENTER", at(0)), ShouldBeNil)
		account := observe(client, deepBid("add"), at(1))

		Convey("Then the order resolves as unknown rather than as a fill or a loss", func() {
			So(read(account.Refused().Reason()), ShouldEqual, "unknown_instrument")
			So(number(read(account.Cash())).Cmp(number("1000")), ShouldEqual, 0)
		})
	})
}
