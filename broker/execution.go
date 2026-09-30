package broker

import (
	"context"

	"github.com/bytedance/sonic"
	"github.com/google/uuid"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker/position"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/nomagique/runtime"
)

type Execution struct {
	*runtime.System
	private *network.WebsocketClient
	price   *Price
	balance *Balance
}

func NewExecution(
	ctx context.Context,
	private *network.WebsocketClient,
	price *Price,
	balance *Balance,
) *Execution {
	execution := &Execution{
		private: private,
		price:   price,
		balance: balance,
	}
	execution.System = runtime.NewSystem(ctx, "execution", execution)
	return execution
}

func (exec *Execution) Enter(symbol string, onPending ...func(*position.Regulator)) (*position.Regulator, error) {
	if exec == nil || exec.private == nil || exec.price == nil || exec.balance == nil {
		return nil, exec.Error(errnie.Err(
			errnie.Validation, "[execution] uninitialized dependencies", nil,
		))
	}

	if exec.Status() == runtime.ERROR || exec.price.Status() == runtime.ERROR {
		return nil, exec.Error(errnie.Err(
			errnie.Validation,
			"[execution] halted: broker in error status",
			nil,
		))
	}

	if exec.price.Anomalies() != nil && exec.price.Anomalies().HasSevereFault(symbol) {
		return nil, exec.Error(errnie.Err(
			errnie.Validation,
			"[execution] halted: severe structural venue fault for "+symbol,
			nil,
		))
	}

	if exec.price.MarketHealth(symbol) <= 0.0 {
		return nil, exec.Error(errnie.Err(
			errnie.Validation,
			"[execution] halted: market health critical failure for "+symbol,
			nil,
		))
	}

	maxFraction := viper.GetFloat64("trading.allocation.max_fraction")

	if maxFraction <= 0 || maxFraction > 1 {
		return nil, exec.Error(errnie.Err(
			errnie.Validation,
			"[execution] invalid trading.allocation.max_fraction configuration",
			nil,
		))
	}

	cash := exec.balance.Cash()

	if cash == nil || cash.Sign() <= 0 {
		exec.balance.Update()
		cash = exec.balance.Cash()
	}

	if cash == nil || cash.Sign() <= 0 {
		return nil, exec.Error(errnie.Err(
			errnie.Validation,
			"[execution] insufficient cash to enter "+symbol,
			nil,
		))
	}

	spend := cash.SetScale(decimal.DefaultScale).Mul(decimal.NewFromFloat64(maxFraction))
	reg := position.NewRegulator(symbol)

	if len(onPending) > 0 && onPending[0] != nil {
		onPending[0](reg)
	}

	if err := exec.EnterWithRegulator(reg, spend); err != nil {
		return nil, err
	}

	return reg, nil
}

func (exec *Execution) EnterWithRegulator(reg *position.Regulator, spend *decimal.Decimal) error {
	if exec == nil || exec.private == nil || exec.price == nil || reg == nil || spend == nil || spend.Sign() <= 0 {
		return exec.Error(errnie.Err(
			errnie.Validation, "[execution] invalid enter request", nil,
		))
	}

	symbol := reg.Symbol

	if exec.Status() == runtime.ERROR || exec.price.Status() == runtime.ERROR {
		return exec.Error(errnie.Err(
			errnie.Validation,
			"[execution] halted: broker in error status",
			nil,
		))
	}

	volume, err := exec.price.Quantity(symbol, spend)

	if err != nil || volume == nil || volume.Sign() <= 0 {
		return exec.Error(errnie.Err(
			errnie.Validation,
			"[execution] cannot determine valid entry volume for "+symbol,
			err,
		))
	}

	entryRequest := &kraken.AddOrderRequest{
		Pair:    symbol,
		Type:    "buy",
		Volume:  volume.String(),
		ClOrdId: reg.PositionID,
		OrdType: "limit",
	}

	if mark := exec.price.CurrentMark(symbol); mark != nil {
		entryRequest.Price = mark.String()
	}

	if err := reg.Begin(entryRequest); err != nil {
		return err
	}

	msg, _ := sonic.Marshal(kraken.NewAddOrderMessage("", entryRequest))
	err = exec.private.Write(msg)

	if err != nil {
		reg.CancelPending()
		return exec.Error(errnie.Err(
			errnie.UnprocessableContent,
			"[execution] failed to enter "+symbol,
			err,
		))
	}

	if exec.balance != nil {
		exec.balance.Update()
	}

	return nil
}

func (exec *Execution) Exit(reg *position.Regulator) error {
	if exec == nil || exec.private == nil || reg == nil {
		return exec.Error(errnie.Err(
			errnie.Validation,
			"[execution] invalid exit request",
			nil,
		))
	}

	vol := reg.Volume()

	if vol == nil || vol.Sign() <= 0 {
		return exec.Error(errnie.Err(
			errnie.Validation,
			"[execution] cannot exit: no held volume",
			nil,
		))
	}

	exitRequest := &kraken.AddOrderRequest{
		Pair:    reg.Symbol,
		Type:    "sell",
		Volume:  vol.String(),
		ClOrdId: uuid.New().String(),
		OrdType: "limit",
	}

	if mark := exec.price.CurrentMark(reg.Symbol); mark != nil {
		exitRequest.Price = mark.String()
	}

	if err := reg.Begin(exitRequest); err != nil {
		return err
	}

	msg, _ := sonic.Marshal(kraken.NewAddOrderMessage("", exitRequest))
	err := exec.private.Write(msg)

	if err != nil {
		reg.CancelPending()
		return exec.Error(errnie.Err(
			errnie.UnprocessableContent,
			"[execution] failed to exit",
			err,
		))
	}

	if exec.balance != nil {
		exec.balance.Update()
	}

	return nil
}
