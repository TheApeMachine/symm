package broker

import (
	"math"
	"testing"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
)

func TestBalanceBookDistinguishesZeroFromMissing(t *testing.T) {
	book := NewBalanceBook()
	artifact := datura.Acquire("test", datura.APPJSON).WithRole("balances").WithPayload(datura.Map[any]{
		"data": []datura.Map[any]{{
			"asset":   "USD",
			"balance": 0.0,
		}},
	}.Marshal())

	if err := book.Update(artifact); err != nil {
		t.Fatalf("update balance book: %v", err)
	}

	funds, ok := book.Funds("USD")
	if !ok {
		t.Fatalf("expected USD row to exist")
	}

	if funds != 0 {
		t.Fatalf("expected zero funds, got %f", funds)
	}

	if _, err := book.RequireFunds("EUR"); err == nil {
		t.Fatalf("expected missing EUR to return an error")
	}
}

func TestOrderFactoryBuildsMarketBuyFromFraction(t *testing.T) {
	viper.Set("market.quote_currency", "USD")
	factory := NewOrderFactory()
	balances := testBalances(t, "USD", 200)
	ticker := testTicker(t, "BTC/USD", 99, 100, 100)
	action := testAction(t, "market", "buy", "BTC/USD")
	action.Poke(0.5, "fraction")

	order, pending, err := factory.Build(action, balances, ticker)
	if err != nil {
		t.Fatalf("build order: %v", err)
	}

	if datura.Peek[string](order, "method") != "add_order" {
		t.Fatalf("expected add_order method")
	}

	if datura.Peek[string](order, "params", "order_type") != "market" {
		t.Fatalf("expected market order")
	}

	if datura.Peek[string](order, "params", "side") != "buy" {
		t.Fatalf("expected buy side")
	}

	if !near(datura.Peek[float64](order, "params", "order_qty"), 1.0) {
		t.Fatalf("expected 1 BTC order qty, got %f", datura.Peek[float64](order, "params", "order_qty"))
	}

	if pending.ClOrdID == "" || pending.Symbol != "BTC/USD" || pending.Side != "buy" {
		t.Fatalf("pending order not populated: %#v", pending)
	}
}

func TestOrderFactoryBuildsPassiveLimitPriceFromQuote(t *testing.T) {
	viper.Set("market.quote_currency", "USD")
	factory := NewOrderFactory()
	balances := testBalances(t, "USD", 200)
	ticker := testTicker(t, "BTC/USD", 99, 100, 100)
	action := testAction(t, "limit", "buy", "BTC/USD")
	action.Poke(0.05, "fraction")

	order, _, err := factory.Build(action, balances, ticker)
	if err != nil {
		t.Fatalf("build order: %v", err)
	}

	if !near(datura.Peek[float64](order, "params", "limit_price"), 99) {
		t.Fatalf("expected passive bid limit, got %f", datura.Peek[float64](order, "params", "limit_price"))
	}
}

func TestOrderFactoryRejectsBuyWithoutQuote(t *testing.T) {
	viper.Set("market.quote_currency", "USD")
	factory := NewOrderFactory()
	balances := testBalances(t, "USD", 200)
	action := testAction(t, "market", "buy", "BTC/USD")
	action.Poke(0.05, "fraction")

	if _, _, err := factory.Build(action, balances, NewTicker()); err == nil {
		t.Fatalf("expected missing quote error")
	}
}

func TestPendingBookRemovesTerminalUpdates(t *testing.T) {
	book := NewPendingBook()
	if !book.Add(PendingOrder{ClOrdID: "abc", Symbol: "BTC/USD"}) {
		t.Fatalf("expected pending add")
	}

	book.Update(datura.Acquire("test", datura.APPJSON).WithRole("executions").WithPayload(datura.Map[any]{
		"data": []datura.Map[any]{{
			"cl_ord_id":    "abc",
			"order_status": "filled",
		}},
	}.Marshal()))

	if count := book.Count(); count != 0 {
		t.Fatalf("expected empty pending book, got %d", count)
	}
}

func testBalances(t *testing.T, asset string, balance float64) *BalanceBook {
	t.Helper()

	book := NewBalanceBook()
	artifact := datura.Acquire("test", datura.APPJSON).WithRole("balances").WithPayload(datura.Map[any]{
		"data": []datura.Map[any]{{
			"asset":   asset,
			"balance": balance,
		}},
	}.Marshal())

	if err := book.Update(artifact); err != nil {
		t.Fatalf("balance update: %v", err)
	}

	return book
}

func testTicker(t *testing.T, symbol string, bid float64, ask float64, last float64) *Ticker {
	t.Helper()

	ticker := NewTicker()
	artifact := datura.Acquire("test", datura.APPJSON).WithRole("ticker").WithPayload(datura.Map[any]{
		"data": []datura.Map[any]{{
			"symbol": symbol,
			"bid":    bid,
			"ask":    ask,
			"last":   last,
		}},
	}.Marshal())

	if err := ticker.Update(artifact); err != nil {
		t.Fatalf("ticker update: %v", err)
	}

	return ticker
}

func testAction(t *testing.T, actionType string, side string, symbol string) *datura.Artifact {
	t.Helper()

	return datura.Acquire("story", datura.APPJSON).
		WithRole(side).
		WithScope(symbol).
		WithPayload(datura.Map[any]{
			"type":   actionType,
			"side":   side,
			"symbol": symbol,
		}.Marshal())
}

func near(left float64, right float64) bool {
	return math.Abs(left-right) < 1e-9
}
