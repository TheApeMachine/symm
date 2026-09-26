package execution

import (
	"context"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/financial/kraken"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/* initialize selects one authored execution mode and its actual cash resource. */
func (server *AccountServer) initialize(ctx context.Context, args Account_write_Params) (bool, error) {
	initial, err := args.InitialCash()
	if err != nil {
		return false, errnie.Error(err)
	}
	server.authorized = args.Authorized()
	if args.Live() && !server.checkpoint.IsValid() && args.Checkpoint().IsValid() {
		server.checkpoint = args.Checkpoint().AddRef()
		server.checkpointKey, err = args.CheckpointKey()
		if err != nil {
			return false, errnie.Error(err)
		}
		if server.checkpointKey == "" {
			return false, errnie.Error(errnie.Err(errnie.Validation, "execution account: live checkpoint key is required", nil))
		}
		future, release := server.checkpoint.Load(ctx, func(params runtime.Checkpoint_load_Params) error { return params.SetKey(server.checkpointKey) })
		result, err := future.Struct()
		if err != nil {
			release()
			return false, errnie.Error(err)
		}
		encoded, err := result.Data()
		if err != nil {
			release()
			return false, errnie.Error(err)
		}
		if len(encoded) > 0 {
			err = server.restoreBytes(encoded)
		}
		release()
		if err != nil {
			return false, err
		}
	}
	if server.cash != nil {
		if server.live != args.Live() || !server.live && initial != server.configuration {
			return false, errnie.Error(errnie.Err(errnie.Validation, "execution account: mode or initial capital cannot change", nil))
		}
		return true, nil
	}
	server.live = args.Live()
	if server.live {
		if !server.authorized || !args.Durable() {
			server.reason = "live_entry_not_authorized_or_durable"
			return false, nil
		}
		if !args.OrdersReady() || !args.Orders().IsValid() || !args.Checkpoint().IsValid() {
			return false, errnie.Error(errnie.Err(errnie.Validation, "execution account: live orders and checkpoint capabilities are required", nil))
		}
		future, release := args.Terms().Balance(ctx, func(params kraken.Terms_balance_Params) error { return params.SetSymbol(server.symbol) })
		defer release()
		result, err := future.Struct()
		if err != nil {
			return false, errnie.Error(err)
		}
		initial, err = result.Available()
		if err != nil {
			return false, errnie.Error(err)
		}
	}
	server.initial, err = amountArgument(func() (string, error) { return initial, nil }, "initial cash")
	if err != nil {
		return false, err
	}
	if server.initial.Sign() <= 0 {
		return false, errnie.Error(errnie.Err(errnie.Validation, "execution account: positive available cash is required", nil))
	}
	server.cash, server.pnl, server.configuration = server.initial.Copy(), decimal.NewFromInt64(0), initial
	server.revision++

	return true, nil
}
