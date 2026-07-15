package tests

import (
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/symm/kraken"
)

/*
TradesScenario names one reusable trade-history shape for broker reconciliation.
*/
type TradesScenario string

const (
	TradesClosedRoundTripCurrentLot TradesScenario = "closed_round_trip_current_lot"
	TradesHydrateBTCAndGALA         TradesScenario = "hydrate_btc_and_gala"
	TradesManagingEntryOrder        TradesScenario = "managing_entry_order"
)

/*
TradesHistory returns one named trade-history fixture for broker position tests.
*/
func TradesHistory(scenario TradesScenario) *kraken.TradesHistory {
	switch scenario {
	case TradesClosedRoundTripCurrentLot:
		currentQuantity, err := decimal.NewFromString("1.0000000000004")

		if err != nil {
			panic(err)
		}

		currentCost, err := decimal.NewFromString("120.000000000048")

		if err != nil {
			panic(err)
		}

		return &kraken.TradesHistory{Result: kraken.TradesHistoryResult{
			Trades: map[string]spot.Trade{
				"closed-buy": {
					Pair: "BTCUSD", Type: "buy", Time: decimal.NewFromInt64(1),
					Price: decimal.NewFromInt64(100), Cost: decimal.NewFromInt64(100),
					Fee: decimal.NewFromInt64(1), Volume: decimal.NewFromInt64(1),
				},
				"closed-sell": {
					Pair: "BTCUSD", Type: "sell", Time: decimal.NewFromInt64(2),
					Price: decimal.NewFromInt64(110), Cost: decimal.NewFromInt64(110),
					Fee: decimal.NewFromInt64(1), Volume: decimal.NewFromInt64(1),
				},
				"current-buy": {
					Pair: "BTCUSD", Type: "buy", Time: decimal.NewFromInt64(3),
					Price: decimal.NewFromInt64(120), Cost: currentCost,
					Fee: decimal.NewFromInt64(1), Volume: currentQuantity,
				},
			},
		}}

	case TradesHydrateBTCAndGALA:
		return &kraken.TradesHistory{Result: kraken.TradesHistoryResult{
			Trades: map[string]spot.Trade{
				"gala-buy": {
					Pair: "GALAUSD", Type: "buy",
					Price:  decimal.NewFromFloat64(0.00232),
					Volume: decimal.NewFromFloat64(13536.853376037476),
				},
				"btc-buy": {
					OrderID: "btc-order",
					Pair:    "BTCUSD",
					Type:    "buy",
					Time:    decimal.NewFromInt64(1_700_000_000),
					Price:   decimal.NewFromFloat64(64129.900),
					Cost:    decimal.NewFromFloat64(6.41299),
					Fee:     decimal.NewFromFloat64(0.016673774),
					Volume:  decimal.NewFromFloat64(0.0001),
					TradeID: decimal.NewFromInt64(1),
				},
			},
		}}

	case TradesManagingEntryOrder:
		return &kraken.TradesHistory{Result: kraken.TradesHistoryResult{
			Trades: map[string]spot.Trade{
				"current-buy": {
					OrderID: "entry-order", Pair: "BTCUSD", Type: "buy",
					Time: decimal.NewFromInt64(3), Price: decimal.NewFromInt64(120),
					Cost: decimal.NewFromInt64(120), Fee: decimal.NewFromInt64(1),
					Volume: decimal.NewFromInt64(1),
				},
			},
		}}
	}

	panic("tests: unknown trades history scenario " + string(scenario))
}

/*
HydrateBalance returns the wallet rows used by broker hydrate tests.
*/
func HydrateBalance() []kraken.BalanceData {
	return []kraken.BalanceData{
		{Asset: "GALA", Balance: *decimal.NewFromFloat64(13536.853376037476)},
		{Asset: "BTC", Balance: *decimal.NewFromFloat64(0.0001)},
	}
}
