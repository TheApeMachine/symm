package market

import (
	"github.com/bytedance/sonic"
	"github.com/theapemachine/symm/kraken/public"
)

func DecodeTrades(message *public.SocketMessage) ([]TradeUpdate, error) {
	var trades []TradeUpdate

	if err := sonic.Unmarshal(message.Data, &trades); err != nil {
		return nil, err
	}

	return trades, nil
}

func DecodeTickers(message *public.SocketMessage) ([]TickerUpdate, error) {
	var tickers []TickerUpdate

	if err := sonic.Unmarshal(message.Data, &tickers); err != nil {
		return nil, err
	}

	return tickers, nil
}

func DecodeBooks(message *public.SocketMessage) ([]Book, error) {
	var books []Book

	if err := sonic.Unmarshal(message.Data, &books); err != nil {
		return nil, err
	}

	for index := range books {
		books[index].SetEnvelopeType(message.Type)
	}

	return books, nil
}
