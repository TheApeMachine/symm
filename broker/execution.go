package broker

import (
	"context"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/google/uuid"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker/position"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/system"
)

type Execution struct {
	*runtime.System
	transport Transport
	price     *Price
	balance   *Balance
	auth      *kraken.Auth
}

func NewExecution(
	ctx context.Context,
	transport Transport,
	price *Price,
	balance *Balance,
) *Execution {
	execution := &Execution{
		transport: transport,
		price:     price,
		balance:   balance,
		auth:      kraken.NewAuth(),
	}

	execution.System = runtime.NewSystem(ctx, "execution", execution)
	return execution
}

func (exec *Execution) Enter(symbol string, onPending ...func(*position.Regulator)) (*position.Regulator, error) {
	if exec == nil || exec.transport == nil || exec.price == nil || exec.balance == nil {
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
		if err := exec.balance.Update(); err != nil {
			errnie.Error(err)
		}

		cash = exec.balance.Cash()
	}

	if cash == nil || cash.Sign() <= 0 {
		// Soft validation: depleted paper cash must not halt Execution/Training.
		return nil, softEnterErr("[execution] insufficient cash to enter "+symbol, nil)
	}

	spend := cash.SetScale(decimal.DefaultScale).Mul(decimal.NewFromFloat64(maxFraction))
	reg := position.NewRegulator(symbol)

	if len(onPending) > 0 && onPending[0] != nil {
		onPending[0](reg)
	}

	if err := exec.EnterWithRegulator(reg, spend); err != nil {
		// Return reg so callers that registered onPending can clean up or
		// await a fill that raced ahead of the error path.
		return reg, err
	}

	return reg, nil
}

func (exec *Execution) EnterWithRegulator(reg *position.Regulator, spend *decimal.Decimal) error {
	if exec == nil || exec.transport == nil || exec.price == nil || reg == nil || spend == nil || spend.Sign() <= 0 {
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
		return softEnterErr(
			"[execution] cannot determine valid entry volume for "+symbol,
			err,
		)
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

	var token string

	if exec.auth != nil && system.Cfg.Market.Model != "paper" {
		var err error
		token, err = exec.auth.Token()

		if err != nil {
			reg.CancelPending()

			return exec.Error(errnie.Err(
				errnie.NotAcceptable,
				"[execution] missing websockets auth token",
				err,
			))
		}
	}

	msg, err := sonic.Marshal(kraken.NewAddOrderMessage(token, entryRequest))

	if err != nil {
		reg.CancelPending()

		return exec.Error(errnie.Err(
			errnie.UnprocessableContent,
			"[execution] failed to marshal order message",
			err,
		))
	}

	err = exec.transport.Write(msg)

	if err != nil {
		reg.CancelPending()

		if IsEnterSoftFail(err) {
			return softEnterErr("[execution] enter skipped for "+symbol, err)
		}

		return exec.Error(errnie.Err(
			errnie.UnprocessableContent,
			"[execution] failed to enter "+symbol,
			err,
		))
	}

	if exec.balance != nil {
		if err := exec.balance.Update(); err != nil {
			errnie.Error(err)
		}
	}

	return nil
}

func (exec *Execution) Exit(reg *position.Regulator) error {
	if exec == nil || exec.transport == nil || reg == nil {
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

	var token string
	if exec.auth != nil && system.Cfg.Market.Model != "paper" {
		var err error
		token, err = exec.auth.Token()

		if err != nil {
			reg.CancelPending()

			return exec.Error(errnie.Err(
				errnie.NotAcceptable,
				"[execution] missing websockets auth token",
				err,
			))
		}
	}

	msg, err := sonic.Marshal(kraken.NewAddOrderMessage(token, exitRequest))

	if err != nil {
		reg.CancelPending()

		return exec.Error(errnie.Err(
			errnie.UnprocessableContent,
			"[execution] failed to marshal order message",
			err,
		))
	}

	err = exec.transport.Write(msg)

	if err != nil {
		reg.CancelPending()

		return exec.Error(errnie.Err(
			errnie.UnprocessableContent,
			"[execution] failed to exit",
			err,
		))
	}

	if exec.balance != nil {
		if err := exec.balance.Update(); err != nil {
			errnie.Error(err)
		}
	}

	return nil
}


/*
softEnterErr returns a validation-class error without transitioning Execution
to ERROR. Depleted paper cash / below-min / venue insufficient-available are
expected during training and must leave the broker READY for a later retry.
*/
func softEnterErr(message string, cause error) error {
	return errnie.Error(errnie.Err(errnie.Validation, message, cause))
}

/*
IsEnterSoftFail reports enter failures that must not cascade Training → ERROR:
insufficient available cash, below-min sizing, and paper place validation.
Not every Validation (e.g. uninitialized deps) — only cash/size abstentions.
*/
func IsEnterSoftFail(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())

	return strings.Contains(msg, "insufficient") ||
		strings.Contains(msg, "below minimum") ||
		strings.Contains(msg, "available cash") ||
		strings.Contains(msg, "cannot determine valid entry volume") ||
		strings.Contains(msg, "positive cash required") ||
		strings.Contains(msg, "order rejected") ||
		strings.Contains(msg, "enter skipped") ||
		strings.Contains(msg, "book unavailable")
}
