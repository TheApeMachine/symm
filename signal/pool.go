package signal

import (
	"encoding/json"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/user"
	"github.com/theapemachine/symm/market/perspectives/types"
)

// The previous sync.Pool pattern here (Get pooled slice, unmarshal, copy to a
// fresh slice, Put) allocated the full copy on every call anyway — the pool
// saved nothing and added contention. Plain unmarshal into a local slice is the
// same allocation profile with none of the machinery.

func GetTrades(data *public.SocketMessage) []market.TradeUpdate {
	var trades []market.TradeUpdate

	if err := sonic.Unmarshal(data.Data, &trades); err != nil {
		errnie.Error(err)
		return nil
	}

	return trades
}

func GetTickers(data *public.SocketMessage) []market.TickerUpdate {
	var tickers []market.TickerUpdate

	if err := sonic.Unmarshal(data.Data, &tickers); err != nil {
		errnie.Error(err)
		return nil
	}

	return tickers
}

func GetBooks(data *public.SocketMessage) []market.Book {
	var books []market.Book

	if err := sonic.Unmarshal(data.Data, &books); err != nil {
		errnie.Error(err)
		return nil
	}

	for index := range books {
		books[index].SetEnvelopeType(data.Type)
	}

	return books
}

func GetExecutions(data *public.SocketMessage) []user.Execution {
	var executions []user.Execution

	if err := sonic.Unmarshal(data.Data, &executions); err != nil {
		errnie.Error(err)
		return nil
	}

	return executions
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

func GetMeasurement(data *qpool.QValue[any]) types.Measurement {
	var (
		ok          bool
		measurement types.Measurement
	)

	if measurement, ok = data.Value.(types.Measurement); !ok {
		return types.Measurement{}
	}

	return measurement
}
