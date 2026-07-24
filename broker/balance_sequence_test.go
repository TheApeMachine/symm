package broker

import (
	"github.com/theapemachine/symm/config"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func TestBalanceAckSequence(t *testing.T) {
	Convey("Given a balance that accepted snapshot sequence 1", t, func() {
		balance := NewBalance(nil, nil, make(chan []byte, 2), config.Fixture().Market)
		balance.quote = "USD"
		balance.BalanceAck([]byte(`{
			"channel":"balances","type":"snapshot","sequence":1,
			"data":[{"asset":"USD","balance":1000}]
		}`))

		Convey("It accepts the exact next update sequence", func() {
			balance.BalanceAck([]byte(`{
				"channel":"balances","type":"update","sequence":2,
				"data":[{"asset":"USD","balance":900}]
			}`))
			row, err := balance.Get("USD")
			So(err, ShouldBeNil)
			So(row.Balance.Float64(), ShouldEqual, 900.0)
		})

		Convey("It rejects a gapped update without applying it", func() {
			balance.BalanceAck([]byte(`{
				"channel":"balances","type":"update","sequence":4,
				"data":[{"asset":"USD","balance":1}]
			}`))
			row, err := balance.Get("USD")
			So(err, ShouldBeNil)
			So(row.Balance.Float64(), ShouldEqual, 1000.0)
		})
	})
}

func TestLedgerReserveAsset(t *testing.T) {
	Convey("Given a ledger with cash and asset claims", t, func() {
		ledger := NewLedger()
		So(ledger.Reserve("buy", "BTC/USD", decimal.NewFromInt64(50), true), ShouldBeNil)
		So(ledger.ReserveAsset("sell", "BTC", decimal.NewFromFloat64(0.5)), ShouldBeNil)

		Convey("It tracks reserved cash and qty independently", func() {
			So(ledger.ReservedCash().Float64(), ShouldEqual, 50.0)
			So(ledger.ReservedAsset("BTC").Float64(), ShouldEqual, 0.5)
			So(ledger.Commit("buy"), ShouldBeNil)
			So(ledger.ReservedCash().Sign(), ShouldEqual, 0)
			So(ledger.Release("sell"), ShouldBeNil)
			So(ledger.ReservedAsset("BTC").Sign(), ShouldEqual, 0)
		})
	})
}

func TestInstrumentPairClone(t *testing.T) {
	Convey("Given a remembered instrument pair", t, func() {
		instrument := &Instrument{cache: map[string]kraken.InstrumentPair{}}
		qty := decimal.NewFromFloat64(0.0001)
		instrument.Remember(kraken.InstrumentPair{
			Symbol:       "ETH/USD",
			Base:         "ETH",
			Quote:        "USD",
			QtyIncrement: qty,
		})

		Convey("Pair returns an independent decimal copy", func() {
			pair, err := instrument.Pair("ETH/USD")
			So(err, ShouldBeNil)
			pair.QtyIncrement = decimal.NewFromInt64(9)
			So(pair.QtyIncrement.Float64(), ShouldEqual, 9)
			again, err := instrument.Pair("ETH/USD")
			So(err, ShouldBeNil)
			So(again.QtyIncrement.Float64(), ShouldEqual, 0.0001)
		})
	})
}
