package websocket

import (
	"fmt"
	"strconv"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/callback"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	sdkkraken "github.com/krakenfx/api-go/v2/pkg/kraken"
	"github.com/theapemachine/symm/kraken"
)

/*
spotBookManager is the BookManager surface level3Apply needs for subscribe
book creation and per-symbol mutation.
*/
type spotBookManager interface {
	Update(event *callback.Event[*sdkkraken.WebSocketMessage]) error
	GetBook(symbol string) *book.Book
}

/*
applyFrame mutates one SDK book from a decoded level3 payload and validates the
server checksum using retained wire text.
*/
func (apply *level3Apply) applyFrame(
	manager spotBookManager,
	raw []byte,
) error {
	if apply == nil || manager == nil || len(raw) == 0 {
		return nil
	}

	var head struct {
		Method  string `json:"method"`
		Channel string `json:"channel"`
	}

	if err := sonic.Unmarshal(raw, &head); err != nil {
		return err
	}

	if head.Method == "subscribe" {
		return manager.Update(&callback.Event[*sdkkraken.WebSocketMessage]{
			Data: sdkkraken.NewWebSocketMessage(raw),
		})
	}

	if head.Channel != "level3" {
		return nil
	}

	frame := kraken.NewLevel3(raw)

	for _, data := range frame.Data {
		if err := apply.applyData(manager, data); err != nil {
			return err
		}
	}

	return nil
}

/*
applyData applies one symbol's L3 data block and validates its checksum.
Depth enforcement runs through the book's OnChecksummed callback.
*/
func (apply *level3Apply) applyData(
	manager spotBookManager,
	data kraken.Level3Data,
) error {
	symbolBook := manager.GetBook(data.Symbol)

	if symbolBook == nil {
		return fmt.Errorf("%s not found in library", data.Symbol)
	}

	if err := apply.applySide(symbolBook, data.Symbol, book.Bid, data.Bids); err != nil {
		return err
	}

	if err := apply.applySide(symbolBook, data.Symbol, book.Ask, data.Asks); err != nil {
		return err
	}

	server := strconv.FormatUint(uint64(data.Checksum), 10)
	local := apply.checksum(symbolBook, data.Symbol)
	result := &book.ChecksumResult{
		Level:          3,
		ServerChecksum: server,
		LocalChecksum:  local,
		Match:          local == server,
	}
	symbolBook.OnChecksummed.Call(result)

	if !result.Match {
		return fmt.Errorf(
			"checksum failed, server \"%s\" versus local \"%s\"",
			result.ServerChecksum,
			result.LocalChecksum,
		)
	}

	return nil
}

/*
applySide folds one bid or ask order list into the SDK book and wire ledger.
*/
func (apply *level3Apply) applySide(
	symbolBook *book.Book,
	symbol string,
	direction book.BookDirection,
	orders []kraken.Level3Order,
) error {
	for _, order := range orders {
		if err := apply.applyOrder(symbolBook, symbol, direction, order); err != nil {
			return err
		}
	}

	return nil
}

/*
applyOrder updates one resting order and keeps checksum wire text in sync.
*/
func (apply *level3Apply) applyOrder(
	symbolBook *book.Book,
	symbol string,
	direction book.BookDirection,
	order kraken.Level3Order,
) error {
	priceText := order.ChecksumLimitPrice()

	if priceText == "" {
		priceText = strconv.FormatFloat(order.LimitPrice, 'f', -1, 64)
	}

	price, err := decimal.NewFromString(priceText)

	if err != nil {
		return fmt.Errorf("price: %w", err)
	}

	quantity := decimal.NewFromInt64(0)

	if order.Event != "delete" {
		qtyText := order.ChecksumOrderQty()

		if qtyText == "" {
			qtyText = strconv.FormatFloat(order.OrderQty, 'f', -1, 64)
		}

		quantity, err = decimal.NewFromString(qtyText)

		if err != nil {
			return fmt.Errorf("quantity: %w", err)
		}

		apply.remember(symbol, order.OrderID, level3WireFromText(priceText, qtyText))
	}

	if order.Event == "delete" {
		apply.forget(symbol, order.OrderID)
	}

	symbolBook.Update(&book.UpdateOptions{
		Direction: direction,
		ID:        order.OrderID,
		Price:     price,
		Quantity:  quantity,
		Timestamp: order.Timestamp,
	})

	return nil
}
