package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/* instrumentConn records market subscriptions after serving the instrument snapshot. */
type instrumentConn struct {
	*mockConn
	marketSubscriptions int
}

func (conn *instrumentConn) MarkReady() {}

func (conn *instrumentConn) SubInstrument(callback chan any) {
	callback <- &kraken.Instrument{Data: kraken.InstrumentData{
		Pairs: []kraken.InstrumentPair{{
			Symbol: "BTC/USD",
			Base:   "BTC",
			Quote:  "USD",
			Status: "online",
		}},
	}}
}

func (conn *instrumentConn) SubTicker([]string) {
	conn.marketSubscriptions++
}

func (conn *instrumentConn) SubBook([]string) {
	conn.marketSubscriptions++
}

func (conn *instrumentConn) SubTrades([]string) {
	conn.marketSubscriptions++
}

func (conn *instrumentConn) SubL3([]string) {
	conn.marketSubscriptions++
}

func TestInstrumentNewInstrument(t *testing.T) {
	Convey("Given an instrument snapshot", t, func() {
		viper.Set("market.quote_currency", "USD")
		Reset(viper.Reset)

		conn := &instrumentConn{mockConn: newMockConn()}
		api := websocket.NewAPI(t.Context(), conn, conn)
		instrument := newTestInstrument(t, api)

		Convey("It should remain pending without starting market flow", func() {
			So(instrument.Status(), ShouldEqual, types.PENDING)
			So(instrument.Symbols(), ShouldResemble, []string{"BTC/USD"})
			So(conn.marketSubscriptions, ShouldEqual, 0)
		})
	})
}

func TestInstrumentSubscribe(t *testing.T) {
	Convey("Given a constructed instrument registry", t, func() {
		viper.Set("market.quote_currency", "USD")
		viper.Set("market.subscribe.batch", 1)
		viper.Set("market.subscribe.pace", 0)
		Reset(viper.Reset)

		conn := &instrumentConn{mockConn: newMockConn()}
		api := websocket.NewAPI(t.Context(), conn, conn)
		instrument := newTestInstrument(t, api)

		Convey("When subscriptions are explicitly started", func() {
			err := instrument.Subscribe()

			Convey("It should start every market stream and become ready", func() {
				So(err, ShouldBeNil)
				// Trades, Ticker, L3 — no SubBook: no full order book is
				// maintained anymore, so Subscribe never issues one.
				So(conn.marketSubscriptions, ShouldEqual, 3)
				So(instrument.Status(), ShouldEqual, types.READY)
			})
		})
	})
}

/*
newTestInstrument builds an Instrument directly from an api and a Price,
matching how boot wires the two dependencies.
*/
func newTestInstrument(t testing.TB, api *websocket.API) *Instrument {
	t.Helper()

	return NewInstrument(api, newTestPrice(t, api))
}
