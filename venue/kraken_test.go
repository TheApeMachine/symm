package venue

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewKraken(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When NewKraken is called", func() {
			venue := NewKraken("kraken")

			Convey("All wire decoders are constructed", func() {
				So(venue, ShouldNotBeNil)
				So(venue.balance, ShouldNotBeNil)
				So(venue.tradeBalance, ShouldNotBeNil)
				So(venue.tradesHistory, ShouldNotBeNil)
				So(venue.level3, ShouldNotBeNil)
				So(venue.trade, ShouldNotBeNil)
				So(venue.execution, ShouldNotBeNil)
				So(venue.order, ShouldNotBeNil)
				So(venue.ticker, ShouldNotBeNil)
			})
		})
	})
}

func TestBalance(t *testing.T) {
	Convey("Setup", t, func() {
		venue := NewKraken("kraken")

		Convey("When Balance is called", func() {
			So(venue.Balance(), ShouldEqual, venue.balance)
		})
	})
}

func TestTradeBalance(t *testing.T) {
	Convey("Setup", t, func() {
		venue := NewKraken("kraken")

		Convey("When TradeBalance is called", func() {
			So(venue.TradeBalance(), ShouldEqual, venue.tradeBalance)
		})
	})
}

func TestTradesHistory(t *testing.T) {
	Convey("Setup", t, func() {
		venue := NewKraken("kraken")

		Convey("When TradesHistory is called", func() {
			So(venue.TradesHistory(), ShouldEqual, venue.tradesHistory)
		})
	})
}

func TestLevel3(t *testing.T) {
	Convey("Setup", t, func() {
		venue := NewKraken("kraken")

		Convey("When Level3 is called", func() {
			So(venue.Level3(), ShouldEqual, venue.level3)
		})
	})
}

func TestTrade(t *testing.T) {
	Convey("Setup", t, func() {
		venue := NewKraken("kraken")

		Convey("When Trade is called", func() {
			So(venue.Trade(), ShouldEqual, venue.trade)
		})
	})
}

func TestExecution(t *testing.T) {
	Convey("Setup", t, func() {
		venue := NewKraken("kraken")

		Convey("When Execution is called", func() {
			So(venue.Execution(), ShouldEqual, venue.execution)
		})
	})
}

func TestOrder(t *testing.T) {
	Convey("Setup", t, func() {
		venue := NewKraken("kraken")

		Convey("When Order is called", func() {
			So(venue.Order(), ShouldEqual, venue.order)
		})
	})
}

func TestBook(t *testing.T) {
	Convey("Setup", t, func() {
		venue := NewKraken("kraken")

		Convey("When Book is called", func() {
			So(venue.Book(), ShouldBeNil)
		})
	})
}

func TestTicker(t *testing.T) {
	Convey("Setup", t, func() {
		venue := NewKraken("kraken")

		Convey("When Ticker is called", func() {
			So(venue.Ticker(), ShouldEqual, venue.ticker)
		})
	})
}

func TestRead(t *testing.T) {
	Convey("Setup", t, func() {
		venue := NewKraken("kraken")

		Convey("When the frame is not JSON", func() {
			Convey("Read returns nil", func() {
				So(venue.Read([]byte(`{`)), ShouldBeNil)
			})
		})

		Convey("When the frame has an unknown channel", func() {
			Convey("Read returns nil", func() {
				So(venue.Read([]byte(`{"channel":"heartbeat"}`)), ShouldBeNil)
			})
		})

		Convey("When the frame is a balances channel", func() {
			got := venue.Read([]byte(`{"channel":"balances","data":[]}`))

			Convey("Read returns the shared balance decoder", func() {
				So(got, ShouldEqual, venue.balance)
			})
		})

		Convey("When the frame is a trade channel", func() {
			got := venue.Read([]byte(`{"channel":"trade","data":[]}`))

			Convey("Read returns the shared trade decoder", func() {
				So(got, ShouldEqual, venue.trade)
			})
		})

		Convey("When the frame is an executions channel", func() {
			got := venue.Read([]byte(`{"channel":"executions","data":[]}`))

			Convey("Read returns the shared execution decoder", func() {
				So(got, ShouldEqual, venue.execution)
			})
		})

		Convey("When the frame is an add_order method", func() {
			got := venue.Read([]byte(`{"method":"add_order","result":{}}`))

			Convey("Read returns the shared order decoder", func() {
				So(got, ShouldEqual, venue.order)
			})
		})
	})
}
