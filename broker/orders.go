package broker

import (
	"slices"
	"strings"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

type Orders struct {
	desk *Desk
	ui   chan []byte
}

func NewOrders(desk *Desk, ui chan []byte) *Orders {
	return &Orders{
		desk: desk,
		ui:   ui,
	}
}

func (orders *Orders) On(data []byte) {
	for _, order := range *kraken.NewOrderDataSlice(data) {
		symbol := strings.TrimSpace(order.Pair)

		if symbol == "" {
			symbol = strings.TrimSpace(order.Description.Pair)
		}

		position, ok := orders.desk.positions.Load(symbol)

		if ok {
			position.(*Position).Order(&order)
		}
	}

	if orders.ui == nil {
		return
	}

	rows := make([]*kraken.OrderData, 0)

	orders.desk.positions.Range(func(_ any, value any) bool {
		position := value.(*Position)

		if slices.Contains(
			[]types.Status{types.CLOSED, types.FATAL}, position.status,
		) {
			return true
		}

		if position.order != nil {
			rows = append(rows, position.order)
		}

		return true
	})

	if len(rows) == 0 {
		return
	}

	orders.ui <- datura.Map[any]{
		"orders": rows,
	}.Marshal()
}

func (orders *Orders) Ack(data []byte) {
	order := kraken.NewOrderResponse(data)

	orders.desk.positions.Range(func(_ any, value any) bool {
		position := value.(*Position)

		if position.orderID == order.Result.ClOrdID {
			position.OrderAck(order)
		}

		return true
	})

	if orders.ui == nil {
		return
	}

	rows := make([]*kraken.OrderData, 0)

	orders.desk.positions.Range(func(_ any, value any) bool {
		position := value.(*Position)

		if slices.Contains(
			[]types.Status{types.CLOSED, types.FATAL}, position.status,
		) {
			return true
		}

		if position.order != nil {
			rows = append(rows, position.order)
		}

		return true
	})

	if len(rows) == 0 {
		return
	}

	orders.ui <- datura.Map[any]{
		"orders": rows,
	}.Marshal()
}
