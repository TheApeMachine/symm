package paper

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
)

const accountSchedule = `{"XBTUSD":{"wsname":"BTC/USD","quote":"USD","cost_decimals":5,"fees":[[0,0.40],[10000,0.35]]},` +
	`"ETHEUR":{"wsname":"ETH/EUR","quote":"EUR","cost_decimals":5,"fees":[[0,0.40]]}}`

func dollars(text string) *decimal.Decimal {
	value, err := decimal.NewFromString(text)
	So(err, ShouldBeNil)
	return value
}

func opened() (*account, terms) {
	wallet := newAccount()
	So(wallet.open("1000", []byte(accountSchedule)), ShouldBeNil)
	return wallet, terms{currency: "USD", fraction: dollars("0.2")}
}

func refusedWith(wallet *account) string {
	reported, found := wallet.report()
	So(found, ShouldBeTrue)
	So(reported.refused, ShouldNotBeNil)
	return reported.refused.reason
}

func TestAccountDecide(t *testing.T) {
	Convey("Given an account opened with 1000 USD", t, func() {
		wallet, standing := opened()

		Convey("When it decides to ENTER while flat", func() {
			placed, err := wallet.decide("BTC/USD", actionEnter, "2026-09-23T10:00:00Z", standing)
			So(err, ShouldBeNil)

			Convey("Then it spends a fifth of the cash, taker fee included, at the pair's cost precision", func() {
				So(placed.side, ShouldEqual, sideBuy)
				So(placed.amount.Cmp(dollars("199.20318")), ShouldEqual, 0)
			})

			Convey("Then a second ENTER on the same symbol is refused while the first is in flight", func() {
				again, err := wallet.decide("BTC/USD", actionEnter, "2026-09-23T10:00:01Z", standing)
				So(err, ShouldBeNil)
				So(again, ShouldBeNil)
				So(refusedWith(wallet), ShouldEqual, reasonPending)
				So(wallet.available().Cmp(dollars("800")), ShouldEqual, 0)
			})

			Convey("Then another symbol is sized on what is left", func() {
				wallet.pairs["ETH/USD"] = wallet.pairs["BTC/USD"]
				other, err := wallet.decide("ETH/USD", actionEnter, "2026-09-23T10:00:01Z", standing)
				So(err, ShouldBeNil)
				// 160 / 1.004
				So(other.amount.Cmp(dollars("159.36254")), ShouldEqual, 0)
			})
		})

		Convey("When it decides to EXIT while flat", func() {
			placed, err := wallet.decide("BTC/USD", actionExit, "2026-09-23T10:00:00Z", standing)
			So(err, ShouldBeNil)
			So(placed, ShouldBeNil)
			So(refusedWith(wallet), ShouldEqual, reasonFlat)
		})

		Convey("When it decides to ENTER a pair quoted in another currency", func() {
			placed, err := wallet.decide("ETH/EUR", actionEnter, "2026-09-23T10:00:00Z", standing)
			So(err, ShouldBeNil)
			So(placed, ShouldBeNil)
			So(refusedWith(wallet), ShouldEqual, reasonCurrency)
		})

		Convey("When it decides to ENTER a pair the exchange does not quote", func() {
			_, err := wallet.decide("DOGE/USD", actionEnter, "2026-09-23T10:00:00Z", standing)
			So(err, ShouldBeNil)
			So(refusedWith(wallet), ShouldEqual, reasonUnpriced)
		})

		Convey("When the decision is not an action", func() {
			_, err := wallet.decide("BTC/USD", "BUY", "2026-09-23T10:00:00Z", standing)
			So(err, ShouldNotBeNil)
		})
	})

	Convey("Given an account whose trailing volume has reached the next fee tier", t, func() {
		wallet, standing := opened()
		So(wallet.advance("2026-09-23T10:00:00Z"), ShouldBeNil)
		wallet.volume = append(wallet.volume, traded{at: wallet.now, volume: dollars("10000")})

		Convey("Then its orders are sized on the lower fee", func() {
			placed, err := wallet.decide("BTC/USD", actionEnter, "2026-09-23T10:00:01Z", standing)
			So(err, ShouldBeNil)
			// 200 / 1.0035
			So(placed.amount.Cmp(dollars("199.30244")), ShouldEqual, 0)
		})

		Convey("Then volume older than the fee window stops counting", func() {
			placed, err := wallet.decide("BTC/USD", actionEnter, "2026-10-23T10:00:00Z", standing)
			So(err, ShouldBeNil)
			So(placed.amount.Cmp(dollars("199.20318")), ShouldEqual, 0)
		})
	})
}

func TestAccountSettle(t *testing.T) {
	Convey("Given an account holding a filled entry", t, func() {
		wallet, standing := opened()
		placed, err := wallet.decide("BTC/USD", actionEnter, "2026-09-23T10:00:00Z", standing)
		So(err, ShouldBeNil)
		So(wallet.settle(execution{
			order: placed.id, symbol: "BTC/USD", side: sideBuy, time: "2026-09-23T10:00:01Z",
			quantity: dollars("2"), cost: dollars("199"), unfilled: dollars("0.20318"),
		}), ShouldBeNil)

		Convey("Then cash paid cost plus fee and the unspent reservation is released", func() {
			// 199 + 0.796
			So(wallet.cash.Cmp(dollars("800.204")), ShouldEqual, 0)
			So(wallet.available().Cmp(dollars("800.204")), ShouldEqual, 0)
			So(wallet.held(), ShouldResemble, []string{"BTC/USD"})
		})

		Convey("Then a second ENTER while holding is refused", func() {
			_, err := wallet.decide("BTC/USD", actionEnter, "2026-09-23T10:00:02Z", standing)
			So(err, ShouldBeNil)
			So(refusedWith(wallet), ShouldEqual, reasonHolding)
		})

		Convey("When the exit only partly fills", func() {
			exit, err := wallet.decide("BTC/USD", actionExit, "2026-09-23T10:00:02Z", standing)
			So(err, ShouldBeNil)
			So(exit.amount.Cmp(dollars("2")), ShouldEqual, 0)
			So(wallet.settle(execution{
				order: exit.id, symbol: "BTC/USD", side: sideSell, time: "2026-09-23T10:00:03Z",
				quantity: dollars("0.5"), cost: dollars("50"), unfilled: dollars("1.5"),
			}), ShouldBeNil)

			Convey("Then basis is allocated to what was sold and the rest is still held", func() {
				position := wallet.holdings["BTC/USD"]
				So(position.quantity.Cmp(dollars("1.5")), ShouldEqual, 0)
				// 199.796 * 0.5 / 2 allocated; the remainder kept by subtraction.
				So(position.basis.Cmp(dollars("149.847")), ShouldEqual, 0)
				_, found := wallet.report()
				So(found, ShouldBeFalse)
			})

			Convey("Then selling the rest closes the round trip on everything it made", func() {
				rest, err := wallet.decide("BTC/USD", actionExit, "2026-09-23T10:00:04Z", standing)
				So(err, ShouldBeNil)
				So(wallet.settle(execution{
					order: rest.id, symbol: "BTC/USD", side: sideSell, time: "2026-09-23T10:00:05Z",
					quantity: dollars("1.5"), cost: dollars("160"), unfilled: dollars("0"),
				}), ShouldBeNil)
				reported, found := wallet.report()
				So(found, ShouldBeTrue)
				So(reported.closed, ShouldNotBeNil)
				// proceeds (50 - 0.2) + (160 - 0.64) = 209.16; spent 199.796
				So(reported.closed.proceeds.Cmp(dollars("209.16")), ShouldEqual, 0)
				So(reported.closed.pnl.Cmp(dollars("9.364")), ShouldEqual, 0)
				So(reported.closed.opened, ShouldEqual, "2026-09-23T10:00:01Z")
				So(reported.closed.closed, ShouldEqual, "2026-09-23T10:00:05Z")
				So(wallet.cash.Cmp(dollars("1009.364")), ShouldEqual, 0)
			})
		})

		Convey("When the venue refuses the exit", func() {
			exit, err := wallet.decide("BTC/USD", actionExit, "2026-09-23T10:00:02Z", standing)
			So(err, ShouldBeNil)
			So(wallet.settle(execution{
				order: exit.id, symbol: "BTC/USD", side: sideSell, time: "2026-09-23T10:00:03Z",
				quantity: zero(), cost: zero(), unfilled: dollars("2"), reason: reasonBelowMinimum,
			}), ShouldBeNil)

			Convey("Then the position is still held and the refusal is reported", func() {
				So(refusedWith(wallet), ShouldEqual, reasonBelowMinimum)
				So(wallet.held(), ShouldResemble, []string{"BTC/USD"})
			})
		})
	})

	Convey("Given a fill for an order the account never placed", t, func() {
		wallet, _ := opened()
		err := wallet.settle(execution{order: 7, symbol: "BTC/USD", side: sideBuy, quantity: dollars("1"), cost: dollars("1")})
		So(err, ShouldNotBeNil)
	})
}
