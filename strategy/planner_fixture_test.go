package strategy

import (
	"context"
	"sync"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

// fixtureMu serializes viper reads/writes across parallel Decide fixtures.
var fixtureMu sync.Mutex

/*
testPlanner builds a Decide Planner with Desk slot ceilings, Balance cash, and
Price fee tiers so unit tests exercise ranking without transport wiring.
*/
func testPlanner(signals ...types.Signal) *Planner {
	fixtureMu.Lock()
	defer fixtureMu.Unlock()

	previousNormal := viper.GetInt("trading.slots.normal")
	previousReserved := viper.GetInt("trading.slots.reserved")
	previousQuote := viper.GetString("market.quote_currency")
	previousFraction := viper.GetFloat64("trading.allocation.max_fraction")

	viper.Set("trading.slots.normal", 2)
	viper.Set("trading.slots.reserved", 2)
	viper.Set("market.quote_currency", "USD")
	viper.Set("trading.allocation.max_fraction", 0.2)

	balance := broker.NewBalance(nil, nil, nil)
	balance.BalanceAck([]byte(
		`{"channel":"balances","type":"snapshot","sequence":1,"data":[{` +
			`"asset":"USD","balance":"1000","available":"1000","reserved":"0"}]}`,
	))
	desk := broker.NewDesk(nil, nil, nil, balance)
	price := broker.NewPrice(nil)
	instrument := broker.NewInstrument(nil, price, nil)
	rememberTestFees(price)
	rememberTestPairs(instrument, price)
	allocator := NewAllocator(context.Background(), balance, instrument, price)
	desk.SetSlots(2, 2)

	planner := NewPlanner(
		context.Background(),
		nil,
		nil,
		desk,
		instrument,
		price,
		balance,
		signals,
		nil,
		allocator,
		nil,
	)

	viper.Set("trading.slots.normal", previousNormal)
	viper.Set("trading.slots.reserved", previousReserved)
	viper.Set("market.quote_currency", previousQuote)
	viper.Set("trading.allocation.max_fraction", previousFraction)

	return planner
}

/*
testPlannerSlots builds a Decide Planner with explicit slot ceilings.
*/
func testPlannerSlots(normal, reserved int, available float64) *Planner {
	fixtureMu.Lock()
	defer fixtureMu.Unlock()

	previousNormal := viper.GetInt("trading.slots.normal")
	previousReserved := viper.GetInt("trading.slots.reserved")
	previousQuote := viper.GetString("market.quote_currency")
	previousFraction := viper.GetFloat64("trading.allocation.max_fraction")

	viper.Set("trading.slots.normal", normal)
	viper.Set("trading.slots.reserved", reserved)
	viper.Set("market.quote_currency", "USD")
	viper.Set("trading.allocation.max_fraction", 0.2)

	balance := broker.NewBalance(nil, nil, nil)
	balance.BalanceAck([]byte(
		`{"channel":"balances","type":"snapshot","sequence":1,"data":[{` +
			`"asset":"USD","balance":"` + decimal.NewFromFloat64(available).String() +
			`","available":"` + decimal.NewFromFloat64(available).String() +
			`","reserved":"0"}]}`,
	))
	desk := broker.NewDesk(nil, nil, nil, balance)
	price := broker.NewPrice(nil)
	instrument := broker.NewInstrument(nil, price, nil)
	rememberTestFees(price)
	rememberTestPairs(instrument, price)
	allocator := NewAllocator(context.Background(), balance, instrument, price)
	desk.SetSlots(normal, reserved)

	planner := NewPlanner(
		context.Background(),
		nil,
		nil,
		desk,
		instrument,
		price,
		balance,
		nil,
		nil,
		allocator,
		nil,
	)

	viper.Set("trading.slots.normal", previousNormal)
	viper.Set("trading.slots.reserved", previousReserved)
	viper.Set("market.quote_currency", previousQuote)
	viper.Set("trading.allocation.max_fraction", previousFraction)

	return planner
}

func rememberTestFees(price *broker.Price) {
	for _, symbol := range testSymbols() {
		_ = price.RememberFee(symbol, kraken.TradeVolumeFee{
			Fee: decimal.NewFromFloat64(0.1),
		})
	}
}

func testSymbols() []string {
	return []string{
		"FAT/USD", "XRP/USD", "WEAK/USD", "KEEP/USD", "NEXT/USD",
		"OXT/USD", "ZZZ/USD", "COLD/USD", "MEH/USD", "CCC/USD",
		"LOW/USD", "HIGH/USD", "HOLD/USD", "AAA/USD", "BBB/USD",
	}
}

func rememberTestPairs(instrument *broker.Instrument, price *broker.Price) {
	for _, symbol := range testSymbols() {
		base := symbol[:len(symbol)-4]
		instrument.Remember(&kraken.InstrumentPair{
			Symbol: symbol, Base: base, Quote: "USD", Status: "online",
			QtyMin: 0.0001, QtyIncrement: 0.0001, QtyPrecision: 4,
			CostMin: decimal.NewFromFloat64(0.01), CostPrecision: 8,
		})
		price.TickerAck([]byte(
			`{"channel":"ticker","type":"update","data":[{` +
				`"symbol":"` + symbol + `","last":"1","bid":"1","ask":"1"}]}`,
		))
	}
}

/*
seedOpenLot puts a thesis holding onto Balance so Desk.OpenPositions counts it.
Wallet qty is seeded so Remember-style authority stays honest under test.
*/
func seedOpenLot(planner *Planner, holding *types.Holding) {
	if planner == nil || planner.balance == nil || holding == nil {
		return
	}

	planner.balance.Seed(holding)
}

/*
testHolding builds an open pointer lot for Thesis.Holdings fixtures.
*/
func testHolding(symbol string, qty, mark float64) *types.Holding {
	return &types.Holding{
		Symbol: symbol,
		Qty:    decimal.NewFromFloat64(qty),
		Mark:   decimal.NewFromFloat64(mark),
		Status: types.OPEN,
	}
}
