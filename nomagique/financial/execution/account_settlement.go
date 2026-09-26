package execution

import (
	"context"
	"github.com/theapemachine/symm/nomagique/financial/paper"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
)

/* sweep calls the authored execution primitive on native, immutable book levels. */
func (server *AccountServer) sweep(ctx context.Context, target paper.Sweep, levels paper.Market_Level_List, amount, increment *decimal.Decimal, spend bool) (*decimal.Decimal, *decimal.Decimal, *decimal.Decimal, error) {
	if !target.IsValid() {
		return nil, nil, nil, errnie.Error(errnie.Err(errnie.Validation, "execution account: sweep capability is required", nil))
	}

	future, release := target.Execute(ctx, func(params paper.Sweep_execute_Params) error {
		params.SetSpend(spend)
		for _, err := range []error{params.SetLevels(levels), params.SetAmount(amount.String()), params.SetIncrement(increment.String())} {
			if err != nil {
				return err
			}
		}
		return nil
	})
	defer release()
	result, err := future.Struct()

	if err != nil {
		return nil, nil, nil, errnie.Error(err)
	}
	quantity, err := amountArgument(result.Quantity, "filled quantity")

	if err != nil {
		return nil, nil, nil, err
	}
	cost, err := amountArgument(result.Cost, "filled cost")

	if err != nil {
		return nil, nil, nil, err
	}
	unfilled, err := amountArgument(result.Unfilled, "unfilled order")
	return quantity, cost, unfilled, err
}

/* settle fills a pending decision only against a later reconciled observation. */
func (server *AccountServer) settle(ctx context.Context, position *positionState, args Account_write_Params) error {
	market, err := args.Market()

	if err != nil {
		return errnie.Error(err)
	}

	if !server.live && position.pending != "" && (args.Epoch() > position.epoch || args.Epoch() == position.epoch && args.Sequence() > position.sequence) {
		if err := server.fill(ctx, position, market, args); err != nil {
			return err
		}
	}

	if position.quantity.Sign() == 0 {
		position.mark = nil
		return nil
	}
	bids, err := market.Bids()

	if err != nil {
		return errnie.Error(err)
	}
	quantity, cost, _, err := server.sweep(ctx, args.Sweep(), bids, position.quantity, position.increment, false)

	if err != nil {
		return err
	}
	position.mark = nil

	if quantity.Cmp(position.quantity) == 0 {
		fee := cost.SetScale(cost.GetScale() + position.fee.GetScale()).Mul(position.fee)
		position.mark = cost.SetScale(max(cost.GetScale(), fee.GetScale())).Sub(fee)
	}
	return nil
}

/* fill reconciles exact quantities, fees and retained basis from one visible book. */
func (server *AccountServer) fill(ctx context.Context, position *positionState, market paper.Market, args Account_write_Params) error {
	levels, err := market.Bids()

	if err != nil {
		return errnie.Error(err)
	}
	amount := position.amount
	buy := position.pending == "buy"

	if buy {
		levels, err = market.Asks()

		if err != nil {
			return errnie.Error(err)
		}
	}
	quantity, cost, _, err := server.sweep(ctx, args.Sweep(), levels, amount, position.increment, false)

	if err != nil {
		return err
	}
	position.pending = ""

	// Venue minima constrain order admission; an admitted order may partially fill
	// below either minimum. Only an empty execution returns all reserved funds.
	if quantity.Sign() == 0 {
		server.cash = server.cash.SetScale(max(server.cash.GetScale(), position.reserved.GetScale())).Add(position.reserved)
		position.reserved = decimal.NewFromInt64(0)
		server.reason = "no_displayed_fill"
		return nil
	}
	fee := cost.SetScale(cost.GetScale() + position.fee.GetScale()).Mul(position.fee)

	return server.apply(position, quantity, cost, fee, buy, true, args.Epoch(), args.Sequence())
}

/* apply is the sole cumulative monetary reconciliation path for paper and live fills. */
func (server *AccountServer) apply(position *positionState, quantity, cost, fee *decimal.Decimal, buy, terminal bool, epoch, sequence int64) error {
	server.revision++
	if buy {
		outlay := cost.SetScale(max(cost.GetScale(), fee.GetScale())).Add(fee)
		available := server.cash.SetScale(max(server.cash.GetScale(), position.reserved.GetScale())).Add(position.reserved)
		if outlay.Cmp(available) > 0 {
			return errnie.Error(errnie.Err(errnie.Validation, "execution account: actual fill exceeds retained cash", nil))
		}
		used := outlay.Copy()
		if used.Cmp(position.reserved) > 0 {
			used = position.reserved.Copy()
		}
		position.reserved = position.reserved.SetScale(max(position.reserved.GetScale(), used.GetScale())).Sub(used)
		extra := outlay.SetScale(max(outlay.GetScale(), used.GetScale())).Sub(used)
		server.cash = server.cash.SetScale(max(server.cash.GetScale(), extra.GetScale())).Sub(extra)
		if terminal {
			server.cash = server.cash.SetScale(max(server.cash.GetScale(), position.reserved.GetScale())).Add(position.reserved)
			position.reserved = decimal.NewFromInt64(0)
		}
		position.quantity = position.quantity.SetScale(max(position.quantity.GetScale(), quantity.GetScale())).Add(quantity)
		position.basis = position.basis.SetScale(max(position.basis.GetScale(), outlay.GetScale())).Add(outlay)
		position.spent = position.spent.SetScale(max(position.spent.GetScale(), outlay.GetScale())).Add(outlay)
		if position.opened == "" && quantity.Sign() > 0 {
			position.opened = server.moment
		}
		return nil
	}
	net := cost.SetScale(max(cost.GetScale(), fee.GetScale())).Sub(fee)
	server.cash = server.cash.SetScale(max(server.cash.GetScale(), net.GetScale())).Add(net)
	position.proceeds = position.proceeds.SetScale(max(position.proceeds.GetScale(), net.GetScale())).Add(net)
	allocated := position.basis.Copy()

	if quantity.Cmp(position.quantity) < 0 {
		allocated = position.basis.SetScale(position.basis.GetScale() + quantity.GetScale()).Mul(quantity).SetScale(int64(position.costPlaces)).Div(position.quantity)
	}
	profit := net.SetScale(max(net.GetScale(), allocated.GetScale())).Sub(allocated)
	server.pnl = server.pnl.SetScale(max(server.pnl.GetScale(), profit.GetScale())).Add(profit)
	position.basis = position.basis.SetScale(max(position.basis.GetScale(), allocated.GetScale())).Sub(allocated)
	position.quantity = position.quantity.SetScale(max(position.quantity.GetScale(), quantity.GetScale())).Sub(quantity)

	if position.quantity.Sign() > 0 || !terminal {
		return nil
	}
	profit = position.proceeds.SetScale(max(position.proceeds.GetScale(), position.spent.GetScale())).Sub(position.spent)
	// This ratio is a non-monetary metric; exact monetary facts remain Decimal.
	edge := profit.Float64() / position.spent.Float64()
	server.outcomes++
	deviation := edge - server.mean
	server.mean += deviation / float64(server.outcomes)
	server.m2 += deviation * (edge - server.mean)

	if profit.Sign() > 0 {
		server.positives++
	}
	server.closed = &closedPosition{server.symbol, position.spent.Copy(), position.proceeds.Copy(), profit, edge, epoch, sequence, position.opened, server.moment}
	return nil
}
