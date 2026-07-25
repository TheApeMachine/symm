package broker

import (
	"encoding/json"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
	. "github.com/smartystreets/goconvey/convey"
)

func TestViewPublish(t *testing.T) {
	Convey("Given a balance with quote cash and an open holding", t, func() {
		ui := make(chan []byte, 1)
		balance := NewBalance(nil, nil, ui, config.Fixture().Market)
		balance.quote = "USD"
		balance.data["USD"] = &kraken.BalanceData{
			Asset:   "USD",
			Balance: decimal.NewFromFloat64(1000),
		}
		balance.StoreHolding(&types.Holding{
			Symbol: "BTC/USD",
			Asset:  "BTC",
			Qty:    decimal.NewFromFloat64(0.5),
			Status: types.OPEN,
		})
		So(balance.Publish(), ShouldBeNil)

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

func TestViewFrame(t *testing.T) {
	Convey("Given an open lot and a closed lot", t, func() {
		balance := NewBalance(nil, nil, make(chan []byte, 1), config.Fixture().Market)
		balance.quote = "USD"
		balance.data["USD"] = &kraken.BalanceData{
			Asset:   "USD",
			Balance: decimal.NewFromFloat64(500),
		}
		balance.StoreHolding(&types.Holding{
			Symbol: "AAA/USD",
			Qty:    decimal.NewFromFloat64(1),
			Status: types.OPEN,
		})
		balance.StoreHolding(&types.Holding{
			Symbol: "BBB/USD",
			Qty:    decimal.NewFromFloat64(0),
			Status: types.CLOSED,
		})

		Convey("Frame includes both lots so the audit rail can paint closed inventory", func() {
			payload, err := balance.Frame()
			So(err, ShouldBeNil)
			So(string(payload), ShouldContainSubstring, `"AAA/USD"`)
			So(string(payload), ShouldContainSubstring, `"BBB/USD"`)
			So(string(payload), ShouldContainSubstring, `"closed"`)
		})
	})
}

func BenchmarkViewFrame(b *testing.B) {
	balance := NewBalance(nil, nil, make(chan []byte, 1), config.Fixture().Market)
	balance.quote = "USD"
	balance.data["USD"] = &kraken.BalanceData{
		Asset:   "USD",
		Balance: decimal.NewFromFloat64(1000),
	}
	balance.StoreHolding(&types.Holding{
		Symbol: "BTC/USD",
		Qty:    decimal.NewFromFloat64(1),
		Status: types.OPEN,
	})
	b.ReportAllocs()

	for b.Loop() {
		_, _ = balance.Frame()
	}
}
