package broker_test

import (
	"fmt"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
)

/*
moneyPair is one adversarial instrument surface used by money-math tests.
*/
type moneyPair struct {
	name   string
	ask    string
	feePct float64
	pair   kraken.InstrumentPair
}

func billPair() moneyPair {
	return moneyPair{
		name:   "BILL/USD",
		ask:    "0.02424",
		feePct: 0.40,
		pair: kraken.InstrumentPair{
			Symbol:         "BILL/USD",
			QtyMin:         100,
			QtyIncrement:   0.00001,
			QtyPrecision:   5,
			PricePrecision: 5,
			CostPrecision:  5,
			CostMin:        decimal.NewFromFloat64(0.5),
		},
	}
}

func btcPair() moneyPair {
	return moneyPair{
		name:   "BTC/USD",
		ask:    "68421.1",
		feePct: 0.26,
		pair: kraken.InstrumentPair{
			Symbol:         "BTC/USD",
			QtyMin:         0.0001,
			QtyIncrement:   0.00000001,
			QtyPrecision:   8,
			PricePrecision: 1,
			CostPrecision:  5,
			CostMin:        decimal.NewFromFloat64(0.5),
		},
	}
}

func coarseLotPair() moneyPair {
	return moneyPair{
		name:   "COARSE/USD",
		ask:    "1.37",
		feePct: 0.26,
		pair: kraken.InstrumentPair{
			Symbol:         "COARSE/USD",
			QtyMin:         1,
			QtyIncrement:   1,
			QtyPrecision:   0,
			PricePrecision: 2,
			CostPrecision:  2,
			CostMin:        decimal.NewFromFloat64(1),
		},
	}
}

func moneyPrice(fixture moneyPair) *broker.Price {
	price := broker.NewPrice(nil)
	_ = price.RememberFee(fixture.name, kraken.TradeVolumeFee{
		Fee: decimal.NewFromFloat64(fixture.feePct),
	})
	price.TickerAck(fmt.Appendf(nil,
		`{"channel":"ticker","type":"update","data":[{`+
			`"symbol":"%s","ask":"%s","bid":"%s","last":"%s"}]}`,
		fixture.name, fixture.ask, fixture.ask, fixture.ask,
	))

	return price
}

/*
assertQuantityFits is the money invariant: a successful Quantity never returns
a lot whose Taker cost exceeds the budget, and never returns below QtyMin.
*/
func assertQuantityFits(
	t *testing.T,
	price *broker.Price,
	pair *kraken.InstrumentPair,
	budget *decimal.Decimal,
) {
	t.Helper()

	quantity, err := price.Quantity(pair, budget)

	if err != nil {
		return
	}

	if quantity == nil || quantity.Sign() <= 0 {
		t.Fatalf("Quantity(%s) returned empty qty without error", budget)
	}

	minimum := decimal.NewFromFloat64(pair.QtyMin)

	if quantity.Cmp(minimum) < 0 {
		t.Fatalf(
			"Quantity(%s)=%s below QtyMin %s",
			budget, quantity, minimum,
		)
	}

	cost, costErr := price.Taker(pair, quantity)

	if costErr != nil || cost == nil {
		t.Fatalf("Taker(%s): %v", quantity, costErr)
	}

	if cost.Cmp(budget) > 0 {
		t.Fatalf(
			"Taker(%s)=%s exceeds budget %s for %s",
			quantity, cost, budget, pair.Symbol,
		)
	}
}

/*
TestQuantityNeverExceedsBudget scans dense budgets across penny, BTC, and coarse
lot instruments so fee/cost quantization cannot silently overspend.
*/
func TestQuantityNeverExceedsBudget(t *testing.T) {
	t.Parallel()

	for _, fixture := range []moneyPair{billPair(), btcPair(), coarseLotPair()} {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()

			price := moneyPrice(fixture)
			pair := fixture.pair
			minCost, err := price.Taker(
				&pair, decimal.NewFromFloat64(pair.QtyMin),
			)

			if err != nil {
				t.Fatal(err)
			}

			budgets := []*decimal.Decimal{
				minCost.Copy().Sub(decimal.NewFromFloat64(0.000001)),
				minCost.Copy(),
				minCost.Copy().Add(decimal.NewFromFloat64(0.000001)),
				minCost.Copy().Add(decimal.NewFromFloat64(0.01)),
				decimal.NewFromFloat64(1),
				decimal.NewFromFloat64(2.5),
				decimal.NewFromFloat64(5),
				decimal.NewFromFloat64(10),
				decimal.NewFromFloat64(23.746),
				decimal.NewFromFloat64(50),
				decimal.NewFromFloat64(100),
				decimal.NewFromFloat64(118.73),
				decimal.NewFromFloat64(1000),
			}

			step := decimal.NewFromFloat64(0.07)
			cursor := minCost.Copy()

			for range 400 {
				cursor = cursor.Copy().Add(step)
				budgets = append(budgets, cursor.Copy())
			}

			for _, budget := range budgets {
				assertQuantityFits(t, price, &pair, budget)
			}
		})
	}
}

/*
TestQuantityRejectsUnfundableMinimum proves Quantity never upsizes into a lot
the budget cannot pay — the failure that looks like “could not fit” with cash.
*/
func TestQuantityRejectsUnfundableMinimum(t *testing.T) {
	t.Parallel()

	fixture := billPair()
	price := moneyPrice(fixture)
	pair := fixture.pair
	minCost, err := price.Taker(&pair, decimal.NewFromFloat64(pair.QtyMin))

	if err != nil {
		t.Fatal(err)
	}

	under := minCost.Copy().Sub(decimal.NewFromFloat64(0.00001))
	quantity, qerr := price.Quantity(&pair, under)

	if quantity != nil || qerr == nil {
		t.Fatalf("want rejection under min cost, got qty=%v err=%v", quantity, qerr)
	}

	exact, err := price.Quantity(&pair, minCost.Copy())

	if err != nil || exact == nil {
		t.Fatalf("exact min cost should size: qty=%v err=%v", exact, err)
	}

	cost, err := price.Taker(&pair, exact)

	if err != nil || cost.Cmp(minCost) > 0 {
		t.Fatalf("exact min lot cost %v vs minCost %v err=%v", cost, minCost, err)
	}
}

/*
TestQuantityRejectsCostMinAndNonPositive budgets that the exchange will refuse
before qty math runs.
*/
func TestQuantityRejectsCostMinAndNonPositive(t *testing.T) {
	t.Parallel()

	fixture := billPair()
	price := moneyPrice(fixture)
	pair := fixture.pair

	if _, err := price.Quantity(&pair, decimal.NewFromFloat64(0.49)); err == nil {
		t.Fatal("want cost_min rejection for 0.49")
	}

	if _, err := price.Quantity(&pair, decimal.NewFromFloat64(0)); err == nil {
		t.Fatal("want non-positive rejection")
	}

	if _, err := price.Quantity(&pair, nil); err == nil {
		t.Fatal("want nil budget rejection")
	}
}

/*
TestQuantizeNeverRoundsUp is the floor contract: exchange lots must not inflate
past the funded guess.
*/
func TestQuantizeNeverRoundsUp(t *testing.T) {
	t.Parallel()

	price := broker.NewPrice(nil)

	for _, fixture := range []moneyPair{billPair(), btcPair(), coarseLotPair()} {
		pair := fixture.pair
		samples := []float64{
			0.000000001, 0.5, 0.9999999, 1, 1.0000001,
			99.99999, 100, 100.00001, 980.123456789, 1e6 + 0.9,
		}

		for _, raw := range samples {
			input := decimal.NewFromFloat64(raw)
			got := price.Quantize(&pair, input)

			if got == nil {
				continue
			}

			if got.Cmp(input) > 0 {
				t.Fatalf(
					"%s Quantize(%s)=%s rounded up",
					fixture.name, input, got,
				)
			}
		}
	}
}

/*
TestTakerIsNotionalPlusFee locks the executable buy boundary used by Quantity
and Allocator so fee truncation cannot drift from the sizing path.
*/
func TestTakerIsNotionalPlusFee(t *testing.T) {
	t.Parallel()

	for _, fixture := range []moneyPair{billPair(), btcPair(), coarseLotPair()} {
		price := moneyPrice(fixture)
		pair := fixture.pair
		quantities := []float64{
			pair.QtyMin,
			pair.QtyMin * 2,
			pair.QtyMin + pair.QtyIncrement,
			123.456789,
			1000,
		}

		for _, raw := range quantities {
			qty := price.Quantize(&pair, decimal.NewFromFloat64(raw))

			if qty == nil || qty.Cmp(decimal.NewFromFloat64(pair.QtyMin)) < 0 {
				continue
			}

			ask, err := price.ReferencePrice(&pair)

			if err != nil {
				t.Fatal(err)
			}

			notional := price.Notional(&pair, ask, qty)
			fee := price.Fee(&pair, notional)
			taker, err := price.Taker(&pair, qty)

			if err != nil || fee == nil || notional == nil {
				t.Fatalf("components unavailable: %v", err)
			}

			sum := notional.Add(fee)

			if taker.Cmp(sum) != 0 {
				t.Fatalf(
					"%s Taker(%s)=%s want notional+fee %s",
					fixture.name, qty, taker, sum,
				)
			}
		}
	}
}

/*
TestQuantityCoarseLotStepsDownWhenRatioStallWouldOverspend covers integer-lot
instruments where ratio shrink can fail to reduce qty.
*/
func TestQuantityCoarseLotStepsDownWhenRatioStallWouldOverspend(t *testing.T) {
	t.Parallel()

	fixture := coarseLotPair()
	price := moneyPrice(fixture)
	pair := fixture.pair

	// Probe budgets that land between two integer lots after fees.
	for cents := 100; cents < 5000; cents++ {
		budget := decimal.NewFromFloat64(float64(cents) / 100)
		assertQuantityFits(t, price, &pair, budget)
	}
}

/*
TestMulDivFractionalAskDoesNotCollapseToZero guards scale lift on penny asks
times large base quantities (BILL-class books).
*/
func TestMulDivFractionalAskDoesNotCollapseToZero(t *testing.T) {
	t.Parallel()

	price := broker.NewPrice(nil)
	ask := decimal.NewFromFloat64(0.02424)
	qty := decimal.NewFromFloat64(980)
	product := price.Mul(ask, qty)

	if product == nil || product.Sign() <= 0 {
		t.Fatalf("Mul collapsed: %v", product)
	}

	if product.Float64() < 20 || product.Float64() > 25 {
		t.Fatalf("Mul(ask,qty)=%s want ~23.75", product)
	}

	unit := price.Mul(ask, decimal.NewFromInt64(1).Add(
		decimal.NewFromFloat64(0.004),
	))
	recovered := price.Div(product, unit)

	if recovered == nil || recovered.Float64() < 900 {
		t.Fatalf("Div collapsed recovered qty=%v", recovered)
	}
}

func BenchmarkQuantityMoneyScan(b *testing.B) {
	fixture := billPair()
	price := moneyPrice(fixture)
	pair := fixture.pair
	budget := decimal.NewFromFloat64(23.746)

	b.ReportAllocs()

	for b.Loop() {
		_, _ = price.Quantity(&pair, budget)
	}
}
