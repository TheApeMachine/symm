package broker

import (
	"context"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/nomagique/runtime"
)

type Desk struct {
	*runtime.System
	api *websocket.API
}

func NewDesk(ctx context.Context, api *websocket.API) *Desk {
	desk := &Desk{
		api: api,
	}

	desk.System = runtime.NewSystem(ctx, "desk", desk)
	return desk
}

func (desk *Desk) Enter(symbol string) *Position {
	entryRequest := &spot.AddOrderRequest{
		Pair: symbol,
		Type: "buy",
	}
	exitRequest := &spot.AddOrderRequest{
		Pair: symbol,
		Type: "sell",
	}

	position := NewPosition(entryRequest, exitRequest)

	res, err := desk.api.AddOrder(position.EntryOrder)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"[desk] failed to enter",
			err,
		))

		return nil
	}

	position.AddEntryResponse(&res)
	return position
}

func (desk *Desk) Exit(position *Position) {
	if position == nil || position.ExitOrder == nil {
		return
	}

	if position.EntryOrder != nil && position.ExitOrder.Volume == "" {
		position.ExitOrder.Volume = position.EntryOrder.Volume
	}

	res, err := desk.api.AddOrder(position.ExitOrder)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"[desk] failed to exit",
			err,
		))

		return
	}

	position.AddExitResponse(&res)
}
