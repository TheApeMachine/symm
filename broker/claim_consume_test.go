package broker

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
TestClaimConsumedOnBuyFill isolates the entry-claim lifecycle: a bound buy claim
reserves cash, and a confirmed buy fill must consume it so effective available
returns to the wallet total. It is the coverage the existing ExecutionAck test
skips by leaving the claim unbound.
*/
func TestClaimConsumedOnBuyFill(t *testing.T) {
	fees := &sync.Map{}
	fees.Store("BTC/USD", kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(0.26)})
	tickers := &sync.Map{}
	tickers.Store("BTC/USD", &kraken.TickerData{
		Symbol: "BTC/USD",
		Ask:    decimal.NewFromInt64(100),
		Bid:    decimal.NewFromInt64(100),
		Last:   decimal.NewFromInt64(100),
	})
	price := &Price{fees: fees, tickers: tickers}
	price.status.Store(types.READY)

	holdings := &sync.Map{}
	holding := types.NewHolding(context.Background(), "BTC/USD", decimal.NewFromInt64(1))
	holding.Asset = "BTC"
	holdings.Store("BTC/USD", holding)

	balance := NewBalance(nil, nil, nil)
	balance.status = types.READY
	balance.quote = "USD"
	balance.holdings = holdings
	balance.model = &kraken.Balance{Data: []kraken.BalanceData{{
		Asset:     "USD",
		Available: decimal.NewFromFloat64(1000),
		Reserved:  decimal.NewFromFloat64(0),
	}}}

	claim, err := balance.Book(decimal.NewFromFloat64(100), nil, "BTC/USD")

	if err != nil || claim == nil {
		t.Fatalf("Book failed: %v %#v", err, claim)
	}

	if cash, _ := balance.AvailableCash(); cash.Float64() != 900 {
		t.Fatalf("after Book: want available 900, got %v", cash)
	}

	request := kraken.NewMarketOrder("buy", decimal.NewFromInt64(1), "BTC/USD")
	position := &Position{
		status:  types.PENDING,
		price:   price,
		balance: balance,
		pair: &kraken.InstrumentPair{
			Symbol: "BTC/USD", Base: "BTC",
			QtyIncrement:  decimal.NewFromFloat64(0.00000001),
			QtyMin:        decimal.NewFromFloat64(0.00000001),
			CostPrecision: 8,
		},
		request: request,
	}
	position.claim.Bind(balance, claim.ID)

	position.OrderAck([]byte(`{"method":"add_order","result":{"order_id":"order-1"},"success":true,"req_id":` +
		strconv.FormatInt(request.ReqID, 10) + `}`))

	execution := &kraken.Execution{
		Channel: "executions", Type: "update",
		Data: []kraken.ExecutionData{{
			OrderID: "order-1", ExecID: "buy-1", ExecType: "trade",
			Symbol: "BTC/USD", Side: "buy", OrderType: "market",
			LastQty: decimal.NewFromInt64(1), CumQty: decimal.NewFromInt64(1),
			OrderStatus: "filled",
			LastPrice:   decimal.NewFromInt64(100),
			AvgPrice:    decimal.NewFromInt64(100),
			Cost:        decimal.NewFromInt64(100),
			FeeUsdEquiv: decimal.NewFromFloat64(0.26), Timestamp: time.Unix(1, 0),
		}},
	}
	buffer, marshalErr := execution.MarshalJSON()

	if marshalErr != nil {
		t.Fatalf("marshal execution: %v", marshalErr)
	}

	position.ExecutionAck(buffer)

	if balance.Funded(claim.ID, decimal.NewFromFloat64(1)) {
		t.Fatalf("claim still funded after buy fill: it was not consumed")
	}

	cash, cashErr := balance.AvailableCash()

	if cashErr != nil {
		t.Fatalf("available cash after fill: %v", cashErr)
	}

	if cash.Float64() != 1000 {
		t.Fatalf("after buy fill the consumed claim must free the wallet: want 1000, got %v", cash)
	}
}
