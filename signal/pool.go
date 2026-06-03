package signal

import (
	"encoding/json"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/user"
)

var tradesPool = sync.Pool{
	New: func() any {
		return make([]market.TradeUpdate, 0)
	},
}

var tickersPool = sync.Pool{
	New: func() any {
		return make([]market.TickerUpdate, 0)
	},
}

var booksPool = sync.Pool{
	New: func() any {
		return make([]market.Book, 0)
	},
}

var ordersPool = sync.Pool{
	New: func() any {
		return make([]trading.OrderUpdate, 0)
	},
}

var executionsPool = sync.Pool{
	New: func() any {
		return make([]user.Execution, 0)
	},
}

var balancesPool = sync.Pool{
	New: func() any {
		return make([]user.Balance, 0)
	},
}

func GetTrades(data map[string]any) []market.TradeUpdate {
	trades := tradesPool.Get().([]market.TradeUpdate)
	defer tradesPool.Put(trades)

	sonic.Unmarshal(data["data"].(json.RawMessage), &trades)
	return trades
}

func GetTickers(data map[string]any) []market.TickerUpdate {
	tickers := tickersPool.Get().([]market.TickerUpdate)
	defer tickersPool.Put(tickers)

	sonic.Unmarshal(data["data"].(json.RawMessage), &tickers)
	return tickers
}

func GetBooks(data map[string]any) []market.Book {
	books := booksPool.Get().([]market.Book)
	defer booksPool.Put(books)

	sonic.Unmarshal(data["data"].(json.RawMessage), &books)
	return books
}

func GetOrders(data map[string]any) []trading.OrderUpdate {
	orders := ordersPool.Get().([]trading.OrderUpdate)
	defer ordersPool.Put(orders)

	sonic.Unmarshal(data["data"].(json.RawMessage), &orders)
	return orders
}

func GetExecutions(data map[string]any) []user.Execution {
	executions := executionsPool.Get().([]user.Execution)
	defer executionsPool.Put(executions)

	sonic.Unmarshal(data["data"].(json.RawMessage), &executions)
	return executions
}

func GetBalances(data map[string]any) []user.Balance {
	balances := balancesPool.Get().([]user.Balance)
	defer balancesPool.Put(balances)

	sonic.Unmarshal(data["data"].(json.RawMessage), &balances)
	return balances
}
