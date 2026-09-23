package paper

import (
	"context"
	"encoding/json"
	"math/big"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/* SweepServer takes one market order through the levels it is given. */
type SweepServer struct {
	*runtime.System
	quantity, cost, unfilled []byte
}

func NewSweep(ctx context.Context) *SweepServer {
	return &SweepServer{
		System: runtime.NewSystem(ctx, "paper.sweep"),
	}
}

func (server *SweepServer) Write(ctx context.Context, call Sweep_write) error {
	raw, err := call.Args().Levels()

	if err != nil {
		return server.Error(errnie.Err(errnie.Validation, "paper.sweep: levels", err))
	}
	var levels [][2]string

	if err := json.Unmarshal(raw, &levels); err != nil {
		return server.Error(errnie.Err(errnie.Validation, "paper.sweep: decode levels", err))
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
	quantity, cost, unfilled := new(big.Rat), new(big.Rat), new(big.Rat).Set(amount)

	for _, level := range levels {
		if unfilled.Sign() == 0 {
			break
		}
		price, ok := new(big.Rat).SetString(level[0])
		size, sized := new(big.Rat).SetString(level[1])

		if !ok || !sized {
			return server.Error(errnie.Err(errnie.Validation, "paper.sweep: level is not two decimals", nil))
		}
		take := takeFrom(price, size, unfilled, increment, call.Args().Spend())

		if take.Sign() == 0 {
			break
		}
		spent := new(big.Rat).Mul(price, take)
		quantity.Add(quantity, take)
		cost.Add(cost, spent)

		if call.Args().Spend() {
			unfilled.Sub(unfilled, spent)
			continue
		}
		unfilled.Sub(unfilled, take)
	}

	for _, rendered := range []struct {
		target *[]byte
		value  *big.Rat
	}{{&server.quantity, quantity}, {&server.cost, cost}, {&server.unfilled, unfilled}} {
		*rendered.target, err = core.WriteDecimal(rendered.value)

		if err != nil {
			return server.Error(err)
		}
	}
	return nil
}

/* takeFrom is how much of one level the order takes: all of it, or what remains. */
func takeFrom(price, size, unfilled, increment *big.Rat, spend bool) *big.Rat {
	if !spend {
		if size.Cmp(unfilled) > 0 {
			return new(big.Rat).Set(unfilled)
		}
		return new(big.Rat).Set(size)
	}

	if new(big.Rat).Mul(price, size).Cmp(unfilled) <= 0 {
		return new(big.Rat).Set(size)
	}
	units := new(big.Rat).Quo(new(big.Rat).Quo(unfilled, price), increment)
	whole := new(big.Int).Div(units.Num(), units.Denom())
	return new(big.Rat).Mul(new(big.Rat).SetInt(whole), increment)
}

func decimalArgument(read func() ([]byte, error), name string) (*big.Rat, error) {
	raw, err := read()

	if err != nil {
		return nil, errnie.Err(errnie.Validation, "paper.sweep: "+name, err)
	}
	return core.ReadDecimal(raw, name)
}

func (server *SweepServer) Done(ctx context.Context, call Sweep_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return server.Error(errnie.Err(errnie.Internal, "paper.sweep: allocate result", err))
	}

	for _, err := range []error{
		results.SetQuantity(server.quantity),
		results.SetCost(server.cost),
		results.SetUnfilled(server.unfilled),
	} {
		if err != nil {
			return server.Error(errnie.Err(errnie.Internal, "paper.sweep: emit", err))
		}
	}
	server.quantity, server.cost, server.unfilled = nil, nil, nil
	return nil
}
