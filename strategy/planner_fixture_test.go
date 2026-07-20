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

/*
Fixture builds Decide Planners with Desk slots, Balance cash, and Price fee
tiers so unit tests exercise ranking without transport wiring. One shared
instance serializes viper mutation across parallel tests.
*/
type Fixture struct {
	mu sync.Mutex
}

/*
NewFixture returns a Decide test fixture.
*/
func NewFixture() *Fixture {
	return &Fixture{}
}

// decideFixture is the package fixture for strategy Decide tests.
var decideFixture = NewFixture()

/*
Planner builds a Decide Planner with default slot ceilings and cash.
*/
func (fixture *Fixture) Planner(signals ...types.Signal) *Planner {
	return fixture.Slots(2, 2, 1000, signals...)
}

/*
Slots builds a Decide Planner with explicit slot ceilings and available cash.
*/
func (fixture *Fixture) Slots(
	normal, reserved int,
	available float64,
	signals ...types.Signal,
) *Planner {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()

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
	fixture.fees(price)
	fixture.pairs(instrument, price)
	allocator := NewAllocator(context.Background(), balance, instrument, price)
	desk.SetSlots(normal, reserved)

	planner := NewPlanner(
		context.Background(),
		make(chan []byte, 64),
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
Holding builds an open pointer lot for Thesis.Holdings fixtures.
*/
func (fixture *Fixture) Holding(symbol string, qty, mark float64) *types.Holding {
	return &types.Holding{
		Symbol: symbol,
		Qty:    decimal.NewFromFloat64(qty),
		Mark:   decimal.NewFromFloat64(mark),
		Status: types.OPEN,
	}
}

/*
Seed puts a thesis holding onto Balance so Desk.OpenPositions counts it.
*/
func (fixture *Fixture) Seed(planner *Planner, holding *types.Holding) {
	if err := planner.validate(map[string]any{"holding": holding}); err != nil {
		return
	}

	planner.balance.Seed(holding)
}

func (fixture *Fixture) fees(price *broker.Price) {
	for _, symbol := range fixture.symbols() {
		_ = price.RememberFee(symbol, kraken.TradeVolumeFee{
			Fee: decimal.NewFromFloat64(0.1),
		})
	}
}

func (fixture *Fixture) pairs(instrument *broker.Instrument, price *broker.Price) {
	for _, symbol := range fixture.symbols() {
		base := symbol[:len(symbol)-4]
		instrument.Remember(&kraken.InstrumentPair{
			Symbol: symbol, Base: base, Quote: "USD", Status: "online",
			QtyMin:       decimal.NewFromFloat64(0.0001),
			QtyIncrement: decimal.NewFromFloat64(0.0001), QtyPrecision: 4,
			CostMin: decimal.NewFromFloat64(0.01), CostPrecision: 8,
		})
		price.TickerAck([]byte(
			`{"channel":"ticker","type":"update","data":[{` +
				`"symbol":"` + symbol + `","last":"1","bid":"1","ask":"1"}]}`,
		))
	}
}

func (fixture *Fixture) symbols() []string {
	return []string{
		"FAT/USD", "XRP/USD", "WEAK/USD", "KEEP/USD", "NEXT/USD",
		"OXT/USD", "ZZZ/USD", "COLD/USD", "MEH/USD", "CCC/USD",
		"LOW/USD", "HIGH/USD", "HOLD/USD", "AAA/USD", "BBB/USD",
	}
}
