package broker

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/internal/testconfig"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func TestPreflightGates(t *testing.T) {
	testconfig.Load(t)

	Convey("Given a complete fresh quote", t, func() {
		quote := Quote{
			Symbol:    "BTC/EUR",
			Bid:       99.95,
			Ask:       100,
			Last:      99.975,
			UpdatedAt: time.Now().UTC(),
		}

		Convey("It should accept maker limits", func() {
			request := PreflightRequest{
				Quote:      quote,
				Side:       trading.Buy,
				Quantity:   0.01,
				OrderType:  trading.Limit,
				ActionType: reasoning.ActionLimit,
			}

			So(PreflightGates(request), ShouldBeNil)
		})

		Convey("It should reject incomplete quotes", func() {
			incomplete := Quote{Symbol: "BTC/EUR", Last: 100, UpdatedAt: time.Now().UTC()}
			request := PreflightRequest{
				Quote:      incomplete,
				Side:       trading.Buy,
				Quantity:   0.01,
				OrderType:  trading.Market,
				ActionType: reasoning.ActionMarket,
			}

			So(PreflightGates(request), ShouldNotBeNil)
		})

		Convey("It should reject stale quotes for entries", func() {
			stale := quote
			stale.UpdatedAt = time.Now().UTC().Add(-1 * time.Hour)
			request := PreflightRequest{
				Quote:      stale,
				Side:       trading.Buy,
				Quantity:   0.01,
				OrderType:  trading.Market,
				ActionType: reasoning.ActionMarket,
			}

			So(PreflightGates(request), ShouldNotBeNil)
		})

		Convey("It should bypass stress and slippage gates for fresh exits", func() {
			exitRequest := PreflightRequest{
				Quote:      quote,
				Side:       trading.Sell,
				Quantity:   0.01,
				OrderType:  trading.Market,
				ActionType: reasoning.ActionSettlePosition,
				Stress: SymbolStress{
					ToxicityCategory: types.CategoryToxicBluff,
					ToxicitySNR:      4,
				},
			}

			So(PreflightGates(exitRequest), ShouldBeNil)
		})

		Convey("It should reject stale quotes for exits", func() {
			stale := quote
			stale.UpdatedAt = time.Now().UTC().Add(-1 * time.Hour)
			request := PreflightRequest{
				Quote:      stale,
				Side:       trading.Sell,
				Quantity:   0.01,
				OrderType:  trading.Market,
				ActionType: reasoning.ActionSettlePosition,
				Stress: SymbolStress{
					ToxicityCategory: types.CategoryToxicBluff,
					ToxicitySNR:      4,
				},
			}

			err := PreflightGates(request)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "stale last price for exit")
		})

		Convey("It should tighten entry slippage under hostile stress", func() {
			wideBook := quote
			wideBook.Book.Asks = []market.BookLevel{
				{Price: 100, Qty: 0.001},
				{Price: 101, Qty: 0.02},
			}
			request := PreflightRequest{
				Quote:      wideBook,
				Side:       trading.Buy,
				Quantity:   0.01,
				OrderType:  trading.Market,
				ActionType: reasoning.ActionMarket,
				Stress: SymbolStress{
					ToxicityCategory: types.CategoryToxicBluff,
					ToxicitySNR:      1,
				},
			}

			So(PreflightGates(request), ShouldNotBeNil)
		})

		Convey("It should reject market orders when book depth cannot cover size", func() {
			shallowBook := quote
			shallowBook.Book.Asks = []market.BookLevel{{Price: 100, Qty: 0.001}}
			request := PreflightRequest{
				Quote:      shallowBook,
				Side:       trading.Buy,
				Quantity:   0.01,
				OrderType:  trading.Market,
				ActionType: reasoning.ActionMarket,
			}

			err := PreflightGates(request)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "insufficient book depth")
		})
	})
}

func BenchmarkPreflightGates(b *testing.B) {
	testconfig.MustLoad()

	quote := Quote{
		Symbol:    "BTC/EUR",
		Bid:       99.95,
		Ask:       100,
		Last:      99.975,
		UpdatedAt: time.Now().UTC(),
	}

	request := PreflightRequest{
		Quote:      quote,
		Side:       trading.Buy,
		Quantity:   0.01,
		OrderType:  trading.Limit,
		ActionType: reasoning.ActionLimit,
	}

	for b.Loop() {
		_ = PreflightGates(request)
	}
}
