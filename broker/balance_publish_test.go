package broker

import (
	"encoding/json"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/config"
)

func TestBalancePublishMarshalsHoldings(t *testing.T) {
	Convey("Given a balance with quote cash and an open holding", t, func() {
		ui := make(chan []byte, 1)
		balance := NewBalance(nil, nil, ui, config.Fixture().Market)
		balance.quote = "USD"
		balance.data["USD"] = &kraken.BalanceData{
			Asset:   "USD",
			Balance: decimal.NewFromFloat64(1000),
		}
		balance.data["BTC"] = &kraken.BalanceData{
			Asset:   "BTC",
			Balance: decimal.NewFromFloat64(0.5),
		}
		qty := decimal.NewFromFloat64(0.5)
		balance.holdings["BTC/USD"] = &types.Holding{
			Symbol: "BTC/USD",
			Asset:  "BTC",
			Qty:    qty,
			Status: types.OPEN,
		}
		balance.Publish()

		Convey("It enqueues a UI Frame with quote balances and holdings", func() {
			So(len(ui), ShouldEqual, 1)
			payload := <-ui
			So(json.Valid(payload), ShouldBeTrue)

			var frame struct {
				Balances []struct {
					Asset string `json:"asset"`
				} `json:"balances"`
				Holdings []struct {
					Symbol string `json:"symbol"`
				} `json:"holdings"`
			}

			So(json.Unmarshal(payload, &frame), ShouldBeNil)
			So(frame.Balances, ShouldHaveLength, 1)
			So(frame.Balances[0].Asset, ShouldEqual, "USD")
			So(frame.Holdings, ShouldHaveLength, 1)
			So(frame.Holdings[0].Symbol, ShouldEqual, "BTC/USD")
			So(string(payload), ShouldContainSubstring, `"available"`)
			So(string(payload), ShouldContainSubstring, `"reserved"`)
		})
	})
}

func TestBalanceSyncWalletSeedsExistingLots(t *testing.T) {
	Convey("Given a wallet snapshot with non-quote inventory", t, func() {
		ui := make(chan []byte, 1)
		balance := NewBalance(nil, nil, ui, config.Fixture().Market)
		balance.quote = "USD"
		frame := []byte(`{
			"channel":"balances","type":"snapshot","sequence":1,
			"data":[
				{"asset":"USD","balance":900},
				{"asset":"ETH","balance":2}
			]
		}`)

		Convey("When BalanceAck ingests the snapshot", func() {
			balance.BalanceAck(frame)

			Convey("It materializes an open holding for the existing lot", func() {
				holding, err := balance.Holding("ETH/USD")
				So(err, ShouldBeNil)
				So(holding.Status, ShouldEqual, types.OPEN)
				So(holding.Qty.Float64(), ShouldEqual, 2.0)
				So(len(ui), ShouldEqual, 1)
				payload := <-ui
				So(string(payload), ShouldContainSubstring, `"ETH/USD"`)
				So(string(payload), ShouldContainSubstring, `"asset":"USD"`)
			})
		})
	})
}

func TestBalanceAckInitializesNilData(t *testing.T) {
	Convey("Given a Balance whose data map was cleared", t, func() {
		ui := make(chan []byte, 1)
		balance := NewBalance(nil, nil, ui, config.Fixture().Market)
		balance.quote = "USD"
		balance.data = nil
		frame := []byte(`{
			"channel":"balances","type":"snapshot","sequence":1,
			"data":[{"asset":"USD","balance":500}]
		}`)

		Convey("When BalanceAck ingests a snapshot", func() {
			So(func() { balance.BalanceAck(frame) }, ShouldNotPanic)

			Convey("It allocates data and stores the snapshot row", func() {
				So(balance.data, ShouldNotBeNil)
				row, err := balance.Get("USD")
				So(err, ShouldBeNil)
				So(row.Balance.Float64(), ShouldEqual, 500.0)
			})
		})
	})
}

func TestBalanceFrameIncludesClosedLotsForAudit(t *testing.T) {
	Convey("Given an open lot and a closed lot", t, func() {
		balance := NewBalance(nil, nil, make(chan []byte, 1), config.Fixture().Market)
		balance.quote = "USD"
		balance.data["USD"] = &kraken.BalanceData{
			Asset:   "USD",
			Balance: decimal.NewFromFloat64(500),
		}
		balance.holdings["AAA/USD"] = &types.Holding{
			Symbol: "AAA/USD",
			Qty:    decimal.NewFromFloat64(1),
			Status: types.OPEN,
		}
		balance.holdings["BBB/USD"] = &types.Holding{
			Symbol: "BBB/USD",
			Qty:    decimal.NewFromFloat64(0),
			Status: types.CLOSED,
		}

		Convey("Frame includes both lots so the audit rail can paint closed inventory", func() {
			payload := string(balance.Frame())
			So(payload, ShouldContainSubstring, `"AAA/USD"`)
			So(payload, ShouldContainSubstring, `"BBB/USD"`)
			So(payload, ShouldContainSubstring, `"closed"`)
		})
	})
}

func BenchmarkBalanceFrame(b *testing.B) {
	balance := NewBalance(nil, nil, make(chan []byte, 1), config.Fixture().Market)
	balance.quote = "USD"
	balance.data["USD"] = &kraken.BalanceData{
		Asset:   "USD",
		Balance: decimal.NewFromFloat64(1000),
	}
	balance.holdings["BTC/USD"] = &types.Holding{
		Symbol: "BTC/USD",
		Qty:    decimal.NewFromFloat64(1),
		Status: types.OPEN,
	}
	b.ReportAllocs()

	for b.Loop() {
		_ = balance.Frame()
	}
}
