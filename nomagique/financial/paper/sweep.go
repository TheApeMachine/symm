package paper

import (
	"context"

	"github.com/krakenfx/api-go/v2/pkg/decimal"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/* SweepServer takes one market order through the levels it is given. */
type SweepServer struct {
	*runtime.System
}

func NewSweep(ctx context.Context) *SweepServer {
	return &SweepServer{
		System: runtime.NewSystem(ctx, "paper.sweep"),
	}
}

/* Write activates the calculation capability without retaining order state. */
func (server *SweepServer) Write(context.Context, Sweep_write) error { return nil }

/* Execute owns one complete immutable sweep request and its response. */
func (server *SweepServer) Execute(ctx context.Context, call Sweep_execute) error {
	levels, err := call.Args().Levels()

	if err != nil {
		return server.Error(errnie.Err(errnie.Validation, "paper.sweep: levels", err))
	}
	amount, err := decimalArgument(call.Args().Amount, "amount")

	if err != nil {
		return server.Error(err)
	}
	increment, err := decimalArgument(call.Args().Increment, "increment")

	if err != nil {
		return server.Error(err)
	}

	if amount.Sign() < 0 || increment.Sign() <= 0 {
		return server.Error(errnie.Err(errnie.Validation, "paper.sweep: amount must not be negative and increment must be positive", nil))
	}
	quantity, cost, unfilled := decimal.NewFromInt64(0).SetRounding(core.FloorDecimal), decimal.NewFromInt64(0).SetRounding(core.FloorDecimal), amount.Copy()

	for index := range levels.Len() {
		level := levels.At(index)
		if unfilled.Sign() == 0 {
			break
		}
		price, err := decimalArgument(level.Price, "level price")
		if err != nil {
			return server.Error(err)
		}
		size, err := decimalArgument(level.Quantity, "level size")
		if err != nil {
			return server.Error(err)
		}
		if price.Sign() <= 0 || size.Sign() < 0 {
			return server.Error(errnie.Err(errnie.Validation, "paper.sweep: level price must be positive and size nonnegative", nil))
		}
		take := takeFrom(price, size, unfilled, increment, call.Args().Spend())

		if take.Sign() == 0 {
			break
		}
		spent := price.SetScale(price.GetScale() + take.GetScale()).Mul(take)
		quantity = quantity.SetScale(max(quantity.GetScale(), take.GetScale())).Add(take)
		cost = cost.SetScale(max(cost.GetScale(), spent.GetScale())).Add(spent)

		if call.Args().Spend() {
			unfilled = unfilled.SetScale(max(unfilled.GetScale(), spent.GetScale())).Sub(spent)
			continue
		}
		unfilled = unfilled.SetScale(max(unfilled.GetScale(), take.GetScale())).Sub(take)
	}

	result, err := call.AllocResults()
	if err != nil {
		return server.Error(err)
	}
	for _, err := range []error{result.SetQuantity(quantity.String()), result.SetCost(cost.String()), result.SetUnfilled(unfilled.String())} {
		if err != nil {
			return server.Error(err)
		}
	}
	return nil
}

/* takeFrom is how much of one level the order takes: all of it, or what remains. */
func takeFrom(price, size, unfilled, increment *decimal.Decimal, spend bool) *decimal.Decimal {
	if !spend {
		if size.Cmp(unfilled) > 0 {
			return unfilled.Copy()
		}
		return size.Copy()
	}

	if price.SetScale(price.GetScale()+size.GetScale()).Mul(size).Cmp(unfilled) <= 0 {
		return size.Copy()
	}

	unitCost := price.SetScale(price.GetScale() + increment.GetScale()).Mul(increment)
	scale := max(unfilled.GetScale(), unitCost.GetScale())
	units := unfilled.SetScale(scale).Div(unitCost).SetScale(0)
	return units.SetScale(increment.GetScale()).Mul(increment)
}

func decimalArgument(read func() (string, error), name string) (*decimal.Decimal, error) {
	raw, err := read()

	if err != nil {
		return nil, errnie.Err(errnie.Validation, "paper.sweep: "+name, err)
	}
	return core.ReadDecimal([]byte(raw), name)
}

func (server *SweepServer) Done(ctx context.Context, call Sweep_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return server.Error(errnie.Err(errnie.Internal, "paper.sweep: allocate result", err))
	}
	results.SetReady(true)

	return nil
}
