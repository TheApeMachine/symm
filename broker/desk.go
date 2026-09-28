package broker

import (
	"context"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/nomagique/runtime"
)

type Desk struct {
	*runtime.System
	api     *websocket.API
	price   *Price
	balance *Balance
}

func NewDesk(
	ctx context.Context,
	api *websocket.API,
	price *Price,
	balance *Balance,
) *Desk {
	desk := &Desk{
		api:     api,
		price:   price,
		balance: balance,
	}

	desk.System = runtime.NewSystem(ctx, "desk", desk)
	return desk
}

func (desk *Desk) Enter(symbol string) *Position {
	if desk == nil || desk.api == nil || desk.price == nil || desk.balance == nil {
		return nil
	}

	if desk.price.Anomalies() != nil && desk.price.Anomalies().HasSevereFault(symbol) {
		desk.Transition(runtime.ERROR)
		errnie.Error(errnie.Err(
			errnie.Validation,
			"[desk] execution halted: severe structural venue fault for "+symbol,
			nil,
		))

		return nil
	}

	if desk.price.MarketHealth(symbol) <= 0.0 {
		desk.Transition(runtime.ERROR)
		errnie.Error(errnie.Err(
			errnie.Validation,
			"[desk] execution halted: market health critical failure for "+symbol,
			nil,
		))

		return nil
	}

	if desk.Status() == runtime.ERROR || desk.price.Status() == runtime.ERROR {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"[desk] execution halted: broker in error status",
			nil,
		))

		return nil
	}

	maxFraction := viper.GetFloat64("trading.allocation.max_fraction")

	if maxFraction <= 0 || maxFraction > 1 {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"[desk] invalid trading.allocation.max_fraction configuration",
			nil,
		))

		return nil
	}

	cash := desk.balance.Cash()

	if cash == nil || cash.Sign() <= 0 {
		desk.balance.Update()
		cash = desk.balance.Cash()
	}

	if cash == nil || cash.Sign() <= 0 {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"[desk] insufficient cash to enter "+symbol,
			nil,
		))

		return nil
	}

	spend := cash.SetScale(decimal.DefaultScale).Mul(decimal.NewFromFloat64(maxFraction))
	volume, err := desk.price.Quantity(symbol, spend)

	if err != nil || volume == nil || volume.Sign() <= 0 {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"[desk] cannot determine valid entry volume for "+symbol,
			err,
		))

		return nil
	}

	entryRequest := &spot.AddOrderRequest{
		Pair:   symbol,
		Type:   "buy",
		Volume: volume.String(),
	}

	if mark := desk.price.CurrentMark(symbol); mark != nil {
		entryRequest.Price = mark.String()
	}

	exitRequest := &spot.AddOrderRequest{
		Pair:   symbol,
		Type:   "sell",
		Volume: volume.String(),
	}

	position := NewPosition(entryRequest, exitRequest)

	response, err := desk.api.AddOrder(position.EntryOrder)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"[desk] failed to enter "+symbol,
			err,
		))

		return nil
	}

	position.AddEntryResponse(&response)
	desk.balance.Update()
	return position
}

func (desk *Desk) Exit(position *Position) error {
	if desk == nil || desk.api == nil || position == nil || position.ExitOrder == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[desk] invalid exit request",
			nil,
		))
	}

	if position.EntryOrder != nil && position.ExitOrder.Volume == "" {
		position.ExitOrder.Volume = position.EntryOrder.Volume
	}

	if mark := desk.price.CurrentMark(position.ExitOrder.Pair); mark != nil {
		position.ExitOrder.Price = mark.String()
	}

	response, err := desk.api.AddOrder(position.ExitOrder)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"[desk] failed to exit",
			err,
		))
	}

	position.AddExitResponse(&response)

	if desk.balance != nil {
		desk.balance.Update()
	}

	return nil
}
