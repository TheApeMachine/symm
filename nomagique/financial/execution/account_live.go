package execution

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/financial/kraken"
)

/* identity is a stable protocol UUID derived from the causal observation. */
func (server *AccountServer) identity() string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("%d/%d/%s", server.epoch, server.sequence, server.symbol)))
	return fmt.Sprintf("%x-%x-%x-%x-%x", digest[:4], digest[4:6], digest[6:8], digest[8:10], digest[10:16])
}

/* reconcile continues an existing order independently of new-risk permission. */
func (server *AccountServer) reconcile(ctx context.Context, position *positionState, args Account_write_Params) error {
	if !args.OrdersReady() || !args.Orders().IsValid() {
		return errnie.Error(errnie.Err(errnie.Validation, "execution account: live order capability unavailable", nil))
	}
	if position.orderId == "" {
		if position.attempted {
			return server.find(ctx, position, args.Orders())
		}
		if position.pending == "buy" && (!server.authorized || !args.Durable() || server.persistenceError != "") {
			server.cash = server.cash.SetScale(max(server.cash.GetScale(), position.reserved.GetScale())).Add(position.reserved)
			position.reserved = decimal.NewFromInt64(0)
			position.pending = ""
			server.revision++
			server.reason = "prepared_entry_permission_withdrawn"
			return nil
		}
		if position.pending == "buy" && server.durableRevision < position.intentRevision {
			server.reason = "entry_waiting_for_durable_intent"
			return nil
		}
		position.attempted = true
		future, release := args.Orders().Submit(ctx, func(params kraken.Orders_submit_Params) error {
			request, err := params.NewRequest()
			if err != nil {
				return err
			}
			for _, err := range []error{request.SetSymbol(server.symbol), request.SetQuantity(position.amount.String()), request.SetSide(position.pending), request.SetClientId(position.clientId)} {
				if err != nil {
					return err
				}
			}
			return nil
		})
		defer release()
		result, err := future.Struct()
		if err != nil {
			return errnie.Error(err)
		}
		position.orderId, err = result.Id()
		if err != nil {
			return errnie.Error(err)
		}
		server.revision++
		return nil
	}
	future, release := args.Orders().Inspect(ctx, func(params kraken.Orders_inspect_Params) error { return params.SetId(position.orderId) })
	defer release()
	result, err := future.Struct()
	if err != nil {
		return errnie.Error(err)
	}
	order, err := result.Order()
	if err != nil {
		return errnie.Error(err)
	}
	return server.cumulative(position, order)
}

/* find resolves a restored or uncertain request without submitting it twice. */
func (server *AccountServer) find(ctx context.Context, position *positionState, orders kraken.Orders) error {
	future, release := orders.Find(ctx, func(params kraken.Orders_find_Params) error { return params.SetClientId(position.clientId) })
	defer release()
	result, err := future.Struct()
	if err != nil {
		return errnie.Error(err)
	}
	if !result.Found() {
		server.reason = "order_submission_unresolved"
		return nil
	}
	order, err := result.Order()
	if err != nil {
		return errnie.Error(err)
	}
	position.orderId, err = order.Id()
	if err != nil {
		return errnie.Error(err)
	}
	server.revision++
	return server.cumulative(position, order)
}

/* cumulative applies only the venue's exact cumulative increments once. */
func (server *AccountServer) cumulative(position *positionState, order kraken.Execution) error {
	client, err := order.ClientId()
	if err != nil {
		return errnie.Error(err)
	}
	side, err := order.Side()
	if err != nil {
		return errnie.Error(err)
	}
	if client != position.clientId || side != position.pending {
		return errnie.Error(errnie.Err(errnie.Validation, "execution account: order identity or side does not match pending intent", nil))
	}
	status, err := order.Status()
	if err != nil {
		return errnie.Error(err)
	}
	terminal := status == "closed" || status == "canceled" || status == "expired"
	if !terminal && status != "open" && status != "pending" {
		return errnie.Error(errnie.Err(errnie.Validation, "execution account: unknown venue order status", nil))
	}
	previous := []*decimal.Decimal{position.filledQuantity, position.filledCost, position.filledFee}
	current := make([]*decimal.Decimal, 3)
	delta := make([]*decimal.Decimal, 3)
	for index, read := range []func() (string, error){order.Quantity, order.Cost, order.Fee} {
		current[index], err = amountArgument(read, "cumulative execution")
		if err != nil {
			return err
		}
		delta[index] = current[index].SetScale(max(current[index].GetScale(), previous[index].GetScale())).Sub(previous[index])
		if delta[index].Sign() < 0 {
			return errnie.Error(errnie.Err(errnie.Validation, "execution account: venue cumulative execution regressed", nil))
		}
	}
	if current[0].Cmp(position.amount) > 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "execution account: venue filled more than the submitted quantity", nil))
	}
	if delta[0].Sign() != 0 || delta[1].Sign() != 0 || delta[2].Sign() != 0 || terminal {
		if err := server.apply(position, delta[0], delta[1], delta[2], side == "buy", terminal, server.epoch, server.sequence); err != nil {
			return err
		}
	}
	position.filledQuantity, position.filledCost, position.filledFee = current[0], current[1], current[2]
	if terminal {
		position.pending = ""
	}
	return nil
}
