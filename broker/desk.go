package broker

import (
	"context"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/google/uuid"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/nomagique/runtime"
)

type Desk struct {
	*runtime.System
	transport Transport
	price     *Price
	balance   *Balance
	rest      *network.RestClient
	positions *sync.Map
}

func NewDesk(
	ctx context.Context,
	transport Transport,
	price *Price,
	balance *Balance,
) *Desk {
	desk := &Desk{
		transport: transport,
		price:     price,
		balance:   balance,
		positions: &sync.Map{},
		rest:      network.NewRestClient(),
	}

	desk.System = runtime.NewSystem(ctx, "desk", desk)
	desk.Transition(runtime.READY)
	return desk
}

func (desk *Desk) Enter(symbol string, size string) error {
	_, ok := desk.positions.Load(symbol)

	if ok {
		return desk.Error(errnie.Err(
			errnie.NotAcceptable,
			"position already exists",
			nil,
		))
	}

	order := spot.AddOrderRequest{
		ClOrdId:   uuid.NewString(),
		OrderType: "limit",
		Type:      "buy",
		Volume:    size,
	}

	payload, err := sonic.Marshal(order)

	if err != nil {
		return desk.Error(errnie.Err(
			errnie.UnprocessableContent,
			"[desk] failed to marshal add order request",
			err,
		))
	}

	desk.positions.Store(symbol, size)
	desk.rest.Post("/rest/AddOrder", payload)

	return nil
}

func (desk *Desk) Exit(symbol string) error {
	sizeVal, ok := desk.positions.Load(symbol)

	if !ok {
		return desk.Error(errnie.Err(
			errnie.NotFound,
			"position not found",
			nil,
		))
	}

	size, _ := sizeVal.(string)

	order := spot.AddOrderRequest{
		ClOrdId:   uuid.NewString(),
		OrderType: "limit",
		Type:      "sell",
		Volume:    size,
	}

	payload, err := sonic.Marshal(order)

	if err != nil {
		return desk.Error(errnie.Err(
			errnie.UnprocessableContent,
			"[desk] failed to marshal exit order request",
			err,
		))
	}

	desk.positions.Delete(symbol)
	desk.rest.Post("/rest/AddOrder", payload)

	return nil
}
