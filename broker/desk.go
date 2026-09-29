package broker

import (
	"context"

	"github.com/google/uuid"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker/position"
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

	if price != nil && price.Anomalies() != nil {
		price.Anomalies().SetOnFault(func(symbol string) {
			desk.Transition(runtime.ERROR)
		})

		price.Anomalies().SetOnRecover(func(symbol string) {
			if !price.Anomalies().HasAnySevereFault() {
				desk.Transition(runtime.READY)
			}
		})
	}

	return desk
}

func (desk *Desk) Enter(symbol string, onPending ...func(*position.Regulator)) (*position.Regulator, error) {
	if desk == nil || desk.api == nil || desk.price == nil || desk.balance == nil {
		return nil, desk.Error(errnie.Err(
			errnie.Validation, "[desk] uninitialized desk dependencies", nil,
		))
	}

	if desk.Status() == runtime.ERROR || desk.price.Status() == runtime.ERROR {
		return nil, desk.Error(errnie.Err(
			errnie.Validation,
			"[desk] execution halted: broker in error status",
			nil,
		))
	}

	if desk.price.Anomalies() != nil && desk.price.Anomalies().HasSevereFault(symbol) {
		return nil, desk.Error(errnie.Err(
			errnie.Validation,
			"[desk] execution halted: severe structural venue fault for "+symbol,
			nil,
		))
	}

	if desk.price.MarketHealth(symbol) <= 0.0 {
		return nil, desk.Error(errnie.Err(
			errnie.Validation,
			"[desk] execution halted: market health critical failure for "+symbol,
			nil,
		))
	}

	maxFraction := viper.GetFloat64("trading.allocation.max_fraction")

	if maxFraction <= 0 || maxFraction > 1 {
		return nil, desk.Error(errnie.Err(
			errnie.Validation,
			"[desk] invalid trading.allocation.max_fraction configuration",
			nil,
		))
	}

	cash := desk.balance.Cash()

	if cash == nil || cash.Sign() <= 0 {
		desk.balance.Update()
		cash = desk.balance.Cash()
	}

	if cash == nil || cash.Sign() <= 0 {
		return nil, desk.Error(errnie.Err(
			errnie.Validation,
			"[desk] insufficient cash to enter "+symbol,
			nil,
		))
	}

	spend := cash.SetScale(decimal.DefaultScale).Mul(decimal.NewFromFloat64(maxFraction))
	reg := position.NewRegulator(symbol)

	if len(onPending) > 0 && onPending[0] != nil {
		onPending[0](reg)
	}

	if err := desk.EnterWithRegulator(reg, spend); err != nil {
		return nil, err
	}

	return reg, nil
}

func (desk *Desk) EnterWithRegulator(reg *position.Regulator, spend *decimal.Decimal) error {
	if desk == nil || desk.api == nil || desk.price == nil || reg == nil || spend == nil || spend.Sign() <= 0 {
		return desk.Error(errnie.Err(
			errnie.Validation, "[desk] invalid enter request", nil,
		))
	}

	symbol := reg.Symbol

	if desk.Status() == runtime.ERROR || desk.price.Status() == runtime.ERROR {
		return desk.Error(errnie.Err(
			errnie.Validation,
			"[desk] execution halted: broker in error status",
			nil,
		))
	}

	if desk.price.Anomalies() != nil && desk.price.Anomalies().HasSevereFault(symbol) {
		return desk.Error(errnie.Err(
			errnie.Validation,
			"[desk] execution halted: severe structural venue fault for "+symbol,
			nil,
		))
	}

	if desk.price.MarketHealth(symbol) <= 0.0 {
		return desk.Error(errnie.Err(
			errnie.Validation,
			"[desk] execution halted: market health critical failure for "+symbol,
			nil,
		))
	}

	volume, err := desk.price.Quantity(symbol, spend)

	if err != nil || volume == nil || volume.Sign() <= 0 {
		return desk.Error(errnie.Err(
			errnie.Validation,
			"[desk] cannot determine valid entry volume for "+symbol,
			err,
		))
	}

	entryRequest := &spot.AddOrderRequest{
		Pair:    symbol,
		Type:    "buy",
		Volume:  volume.String(),
		ClOrdId: reg.PositionID,
	}

	if mark := desk.price.CurrentMark(symbol); mark != nil {
		entryRequest.Price = mark.String()
	}

	if err := reg.Begin(entryRequest); err != nil {
		return err
	}

	response, err := desk.api.AddOrder(reg.Pending)

	if err != nil {
		reg.CancelPending()
		return desk.Error(errnie.Err(
			errnie.UnprocessableContent,
			"[desk] failed to enter "+symbol,
			err,
		))
	}

	if len(response.ID) > 0 {
		reg.SetOrderID(response.ID[0])
	}

	if desk.balance != nil {
		desk.balance.Update()
	}

	return nil
}

func (desk *Desk) Exit(reg *position.Regulator) error {
	if desk == nil || desk.api == nil || reg == nil {
		return desk.Error(errnie.Err(
			errnie.Validation,
			"[desk] invalid exit request",
			nil,
		))
	}

	vol := reg.Volume()

	if vol == nil || vol.Sign() <= 0 {
		return desk.Error(errnie.Err(
			errnie.Validation,
			"[desk] cannot exit: no held volume",
			nil,
		))
	}

	exitRequest := &spot.AddOrderRequest{
		Pair:    reg.Symbol,
		Type:    "sell",
		Volume:  vol.String(),
		ClOrdId: uuid.New().String(),
	}

	if mark := desk.price.CurrentMark(reg.Symbol); mark != nil {
		exitRequest.Price = mark.String()
	}

	if err := reg.Begin(exitRequest); err != nil {
		return err
	}

	response, err := desk.api.AddOrder(reg.Pending)

	if err != nil {
		reg.CancelPending()

		return desk.Error(errnie.Err(
			errnie.UnprocessableContent,
			"[desk] failed to exit",
			err,
		))
	}

	if len(response.ID) > 0 {
		reg.SetOrderID(response.ID[0])
	}

	if desk.balance != nil {
		desk.balance.Update()
	}

	return nil
}
