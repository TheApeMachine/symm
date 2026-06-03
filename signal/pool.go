package signal

import (
	"sync"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/public"
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

func GetTrades(data *public.SocketMessage) []market.TradeUpdate {
	trades := tradesPool.Get().([]market.TradeUpdate)
	defer tradesPool.Put(trades)

	sonic.Unmarshal(data.Data, &trades)
	return trades
}

func GetTickers(data *public.SocketMessage) []market.TickerUpdate {
	tickers := tickersPool.Get().([]market.TickerUpdate)
	defer tickersPool.Put(tickers)

	sonic.Unmarshal(data.Data, &tickers)
	return tickers
}

func GetBooks(data *public.SocketMessage) []market.Book {
	books := booksPool.Get().([]market.Book)
	defer booksPool.Put(books)

	sonic.Unmarshal(data.Data, &books)
	return books
}

func GetOrders(data *public.SocketMessage) []trading.OrderUpdate {
	orders := ordersPool.Get().([]trading.OrderUpdate)
	defer ordersPool.Put(orders)

	sonic.Unmarshal(data.Data, &orders)
	return orders
}

func GetExecutions(data *public.SocketMessage) []user.Execution {
	executions := executionsPool.Get().([]user.Execution)
	defer executionsPool.Put(executions)

	sonic.Unmarshal(data.Data, &executions)
	return executions
}

func GetBalances(data *public.SocketMessage) []user.Balance {
	balances := balancesPool.Get().([]user.Balance)
	defer balancesPool.Put(balances)

	sonic.Unmarshal(data.Data, &balances)
	return balances
}
