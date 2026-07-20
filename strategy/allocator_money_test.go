package strategy

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
allocatorMoneyEnv wires Balance/Instrument/Price for one symbol enter path.
*/
type allocatorMoneyEnv struct {
	balance    *broker.Balance
	instrument *broker.Instrument
	price      *broker.Price
	allocator  *Allocator
	symbol     string
}

func newAllocatorMoneyEnv(
	t *testing.T,
	symbol, ask string,
	qtyMin float64,
	qtyIncrement float64,
	qtyPrecision int,
	cash float64,
	maxFraction float64,
) *allocatorMoneyEnv {
	t.Helper()

	previousFraction := viper.GetFloat64("trading.allocation.max_fraction")
	previousQuote := viper.GetString("market.quote_currency")
	viper.Set("trading.allocation.max_fraction", maxFraction)
	viper.Set("market.quote_currency", "USD")
	t.Cleanup(func() {
		viper.Set("trading.allocation.max_fraction", previousFraction)
		viper.Set("market.quote_currency", previousQuote)
	})

	balance := broker.NewBalance(nil, nil, nil)
	balance.BalanceAck(fmt.Appendf(nil,
		`{"channel":"balances","type":"snapshot","sequence":1,"data":[{`+
			`"asset":"USD","balance":"%0.8f","available":"%0.8f","reserved":"0"}]}`,
		cash, cash,
	))

	instrument := broker.NewInstrument(nil, nil, nil)
	instrument.Remember(&kraken.InstrumentPair{
		Symbol:         symbol,
		Base:           "BASE",
		Quote:          "USD",
		Status:         "online",
		QtyMin:         decimal.NewFromFloat64(qtyMin),
		QtyIncrement:   decimal.NewFromFloat64(qtyIncrement),
		QtyPrecision:   qtyPrecision,
		CostPrecision:  5,
		PricePrecision: 5,
		CostMin:        decimal.NewFromFloat64(0.5),
	})

	price := broker.NewPrice(nil)
	_ = price.RememberFee(symbol, kraken.TradeVolumeFee{
		Fee: decimal.NewFromFloat64(0.40),
	})
	price.TickerAck(fmt.Appendf(nil,
		`{"channel":"ticker","type":"update","data":[{`+
			`"symbol":"%s","ask":"%s","bid":"%s","last":"%s"}]}`,
		symbol, ask, ask, ask,
	))

	return &allocatorMoneyEnv{
		balance:    balance,
		instrument: instrument,
		price:      price,
		allocator: NewAllocator(
			context.Background(), balance, instrument, price,
		),
		symbol: symbol,
	}
}

func (env *allocatorMoneyEnv) enter(risk float64) *types.Thesis {
	thesis := types.NewThesis(nil, nil)
	thesis.Decisions = append(thesis.Decisions, types.Decision{
		Action:            types.ActionEnter,
		Symbol:            env.symbol,
		AllocationHaircut: risk,
		At:                time.Unix(1, 0).UTC(),
	})
	thesis.Holdings.Store(env.symbol, &types.Holding{
		Symbol: env.symbol,
		Status: types.PENDING,
	})

	return thesis
}

/*
TestAllocateNeverOverspendsBookedSlice is the Allocator money invariant: when
sizing succeeds, ProposedNotional stays inside the risk-adjusted booked budget.
*/
func TestAllocateNeverOverspendsBookedSlice(t *testing.T) {
	// Not parallel: viper max_fraction / quote_currency are process-global.
	cases := []struct {
		name         string
		symbol       string
		ask          string
		qtyMin       float64
		qtyIncrement float64
		qtyPrecision int
		cash         float64
		maxFraction  float64
		risk         float64
	}{
		{
			name: "bill_wallet_slice", symbol: "BILL/USD", ask: "0.02424",
			qtyMin: 100, qtyIncrement: 0.00001, qtyPrecision: 5,
			cash: 118.73, maxFraction: 0.20, risk: 0,
		},
		{
			name: "bill_high_risk_haircut", symbol: "BILL/USD", ask: "0.02424",
			qtyMin: 100, qtyIncrement: 0.00001, qtyPrecision: 5,
			cash: 118.73, maxFraction: 0.20, risk: 0.7,
		},
		{
			name: "btc_slice", symbol: "BTC/USD", ask: "68421.1",
			qtyMin: 0.0001, qtyIncrement: 0.00000001, qtyPrecision: 8,
			cash: 118.73, maxFraction: 0.20, risk: 0.2,
		},
		{
			name: "coarse_integer_lot", symbol: "COARSE/USD", ask: "1.37",
			qtyMin: 1, qtyIncrement: 1, qtyPrecision: 0,
			cash: 500, maxFraction: 0.20, risk: 0.1,
		},
	}

	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			env := newAllocatorMoneyEnv(
				t, testCase.symbol, testCase.ask,
				testCase.qtyMin, testCase.qtyIncrement, testCase.qtyPrecision,
				testCase.cash, testCase.maxFraction,
			)
			thesis := env.enter(testCase.risk)

			if err := env.allocator.Allocate(thesis); err != nil {
				t.Fatalf("Allocate: %v", err)
			}

			decision := thesis.Decisions[0]

			if decision.ProposedQuantity == nil || decision.ProposedNotional == nil {
				// Risk/cash may honestly refuse — that is not overspend.
				if decision.Reason == "" {
					t.Fatal("unsized enter needs a reason")
				}

				return
			}

			pair, err := env.instrument.Pair(testCase.symbol)

			if err != nil {
				t.Fatal(err)
			}

			cost, err := env.price.Taker(pair, decision.ProposedQuantity)

			if err != nil {
				t.Fatal(err)
			}

			if cost.Cmp(decision.ProposedNotional) != 0 {
				t.Fatalf(
					"ProposedNotional %s != Taker %s",
					decision.ProposedNotional, cost,
				)
			}

			slice := decimal.NewFromFloat64(testCase.cash * testCase.maxFraction)
			budget := slice.Sub(env.price.Mul(
				slice, decimal.NewFromFloat64(testCase.risk),
			))

			if decision.ProposedNotional.Cmp(budget) > 0 {
				t.Fatalf(
					"ProposedNotional %s exceeds risk budget %s (slice %s)",
					decision.ProposedNotional, budget, slice,
				)
			}

			if decision.ProposedQuantity.Float64() < testCase.qtyMin {
				t.Fatalf(
					"ProposedQuantity %s below qty_min %g",
					decision.ProposedQuantity, testCase.qtyMin,
				)
			}
		})
	}
}

/*
TestAllocateReleasesOnUnfundableMinimum ensures a too-small wallet slice does
not leave a dangling reservation or a sized holding.
*/
func TestAllocateReleasesOnUnfundableMinimum(t *testing.T) {
	env := newAllocatorMoneyEnv(
		t, "BILL/USD", "0.02424",
		100, 0.00001, 5,
		1.0, 0.20, // slice 0.20 << ~2.43 min lot
	)
	thesis := env.enter(0)

	if err := env.allocator.Allocate(thesis); err != nil {
		t.Fatalf("Allocate: %v", err)
	}

	decision := thesis.Decisions[0]

	if decision.ProposedQuantity != nil || decision.ReservationID != "" {
		t.Fatalf(
			"want unsized enter, got qty=%v reservation=%q reason=%s",
			decision.ProposedQuantity, decision.ReservationID, decision.Reason,
		)
	}

	if _, ok := thesis.Holdings.Load("BILL/USD"); ok {
		t.Fatal("want holding deleted when minimum exceeds slice")
	}

	available, err := env.balance.AvailableCash()

	if err != nil {
		t.Fatal(err)
	}

	minimumAvailable, err := decimal.NewFromString("0.99")

	if err != nil {
		t.Fatal(err)
	}

	if available.Cmp(minimumAvailable) < 0 {
		t.Fatalf("reservation leaked: available=%v", available)
	}
}

/*
TestAllocateRiskCanRefuseButNeverOversize checks the haircut path: high risk
either sizes inside the reduced budget or refuses cleanly.
*/
func TestAllocateRiskCanRefuseButNeverOversize(t *testing.T) {
	for _, risk := range []float64{0, 0.5, 0.9, 0.99, 1.0} {
		risk := risk
		t.Run(fmt.Sprintf("risk_%g", risk), func(t *testing.T) {
			env := newAllocatorMoneyEnv(
				t, "BILL/USD", "0.02424",
				100, 0.00001, 5,
				118.73, 0.20,
			)
			thesis := env.enter(risk)

			if err := env.allocator.Allocate(thesis); err != nil {
				t.Fatalf("Allocate: %v", err)
			}

			decision := thesis.Decisions[0]

			if decision.ProposedNotional == nil {
				return
			}

			slice := decimal.NewFromFloat64(118.73 * 0.20)
			budget := slice.Sub(env.price.Mul(slice, decimal.NewFromFloat64(risk)))

			if budget.Sign() <= 0 {
				t.Fatalf("sized with non-positive risk budget at risk=%g", risk)
			}

			if decision.ProposedNotional.Cmp(budget) > 0 {
				t.Fatalf(
					"risk=%g notional %s > budget %s",
					risk, decision.ProposedNotional, budget,
				)
			}
		})
	}
}

func BenchmarkAllocateBillSlice(b *testing.B) {
	previousFraction := viper.GetFloat64("trading.allocation.max_fraction")
	previousQuote := viper.GetString("market.quote_currency")
	viper.Set("trading.allocation.max_fraction", 0.20)
	viper.Set("market.quote_currency", "USD")
	b.Cleanup(func() {
		viper.Set("trading.allocation.max_fraction", previousFraction)
		viper.Set("market.quote_currency", previousQuote)
	})

	balance := broker.NewBalance(nil, nil, nil)
	balance.BalanceAck([]byte(
		`{"channel":"balances","type":"snapshot","sequence":1,"data":[{` +
			`"asset":"USD","balance":"118.73","available":"118.73","reserved":"0"}]}`,
	))
	instrument := broker.NewInstrument(nil, nil, nil)
	instrument.Remember(&kraken.InstrumentPair{
		Symbol: "BILL/USD", Base: "BILL", Quote: "USD", Status: "online",
		QtyMin:       decimal.NewFromInt64(100),
		QtyIncrement: decimal.NewFromFloat64(0.00001), QtyPrecision: 5,
		CostPrecision: 5, PricePrecision: 5,
		CostMin: decimal.NewFromFloat64(0.5),
	})
	price := broker.NewPrice(nil)
	_ = price.RememberFee("BILL/USD", kraken.TradeVolumeFee{
		Fee: decimal.NewFromFloat64(0.40),
	})
	price.TickerAck([]byte(
		`{"channel":"ticker","type":"update","data":[{` +
			`"symbol":"BILL/USD","ask":"0.02424","bid":"0.02421","last":"0.02437"}]}`,
	))
	allocator := NewAllocator(context.Background(), balance, instrument, price)

	b.ReportAllocs()

	for b.Loop() {
		thesis := types.NewThesis(nil, nil)
		thesis.Decisions = append(thesis.Decisions, types.Decision{
			Action: types.ActionEnter, Symbol: "BILL/USD", AllocationHaircut: 0.2,
			At: time.Unix(1, 0).UTC(),
		})
		thesis.Holdings.Store("BILL/USD", &types.Holding{
			Symbol: "BILL/USD", Status: types.PENDING,
		})
		_ = allocator.Allocate(thesis)
		if thesis.Decisions[0].ReservationID != "" {
			_ = balance.Release(thesis.Decisions[0].ReservationID)
		}
	}
}
