package market

import (
	"github.com/bytedance/sonic"
	"github.com/theapemachine/symm/kraken/public"
)

func DecodeTrade(message *public.SocketMessage) (TradeUpdate, error) {
	var trade TradeUpdate

	if err := sonic.Unmarshal(message.Data, &trade); err != nil {
		return TradeUpdate{}, err
	}

	return trade, nil
}

func DecodeTicker(message *public.SocketMessage) (TickerUpdate, error) {
	var ticker TickerUpdate

	if err := sonic.Unmarshal(message.Data, &ticker); err != nil {
		return TickerUpdate{}, err
	}

	return ticker, nil
}

func DecodeBook(message *public.SocketMessage) (Book, error) {
	var book Book

	if err := sonic.Unmarshal(message.Data, &book); err != nil {
		return Book{}, err
	}

	book.SetEnvelopeType(message.Type)

	return book, nil
}
