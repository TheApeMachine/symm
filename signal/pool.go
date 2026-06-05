package signal

import (
	"encoding/json"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
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
	trades := tradesPool.Get().([]market.TradeUpdate)[:0]
	defer tradesPool.Put(trades[:0])

	if err := sonic.Unmarshal(data.Data, &trades); err != nil {
		errnie.Error(err)
		return nil
	}

	return append([]market.TradeUpdate(nil), trades...)
}

func GetTickers(data *public.SocketMessage) []market.TickerUpdate {
	tickers := tickersPool.Get().([]market.TickerUpdate)[:0]
	defer tickersPool.Put(tickers[:0])

	if err := sonic.Unmarshal(data.Data, &tickers); err != nil {
		errnie.Error(err)
		return nil
	}

	return append([]market.TickerUpdate(nil), tickers...)
}

func GetBooks(data *public.SocketMessage) []market.Book {
	books := booksPool.Get().([]market.Book)[:0]
	defer booksPool.Put(books[:0])

	if err := sonic.Unmarshal(data.Data, &books); err != nil {
		errnie.Error(err)
		return nil
	}

	for index := range books {
		books[index].SetEnvelopeType(data.Type)
	}

	return append([]market.Book(nil), books...)
}

func GetOrders(data *public.SocketMessage) []trading.OrderUpdate {
	orders := ordersPool.Get().([]trading.OrderUpdate)[:0]
	defer ordersPool.Put(orders[:0])

	if err := sonic.Unmarshal(data.Data, &orders); err != nil {
		errnie.Error(err)
		return nil
	}

	return append([]trading.OrderUpdate(nil), orders...)
}

func GetExecutions(data *public.SocketMessage) []user.Execution {
	executions := executionsPool.Get().([]user.Execution)[:0]
	defer executionsPool.Put(executions[:0])

	if err := sonic.Unmarshal(data.Data, &executions); err != nil {
		errnie.Error(err)
		return nil
	}

	return append([]user.Execution(nil), executions...)
}

func GetBalances(data *public.SocketMessage) []user.Balance {
	balances := balancesPool.Get().([]user.Balance)[:0]
	defer balancesPool.Put(balances[:0])

	if err := sonic.Unmarshal(data.Data, &balances); err != nil {
		errnie.Error(err)
		return nil
	}

	return append([]user.Balance(nil), balances...)
}

func SocketMessageFromValue(value any) (*public.SocketMessage, bool) {
	switch typed := value.(type) {
	case *public.SocketMessage:
		if typed == nil {
			return nil, false
		}

		return typed, true
	case map[string]any:
		channel, _ := typed["channel"].(string)
		frameType, _ := typed["type"].(string)

		if channel == "" {
			return nil, false
		}

		data, ok := rawMessage(typed["data"])

		if !ok {
			return nil, false
		}

		return &public.SocketMessage{Channel: channel, Type: frameType, Data: data}, true
	default:
		return nil, false
	}
}

func rawMessage(value any) (json.RawMessage, bool) {
	switch typed := value.(type) {
	case json.RawMessage:
		return append(json.RawMessage(nil), typed...), true
	case []byte:
		return append(json.RawMessage(nil), typed...), true
	case string:
		return json.RawMessage(typed), true
	default:
		return nil, false
	}
}
