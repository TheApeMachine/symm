package strategy

import (
	"context"
	indicator "github.com/cinar/indicator/v2/strategy"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
CountTransactionsServer counts total executed buy and sell transactions.
*/
type CountTransactionsServer struct {
	*runtime.System
	actions chan indicator.Action
	out     <-chan int
	result  int64
}

func NewCountTransactions(ctx context.Context) *CountTransactionsServer {
	actions := make(chan indicator.Action, 1)

	server := &CountTransactionsServer{
		System:  runtime.NewSystem(ctx, "financial.strategy.count_transactions"),
		actions: actions,
		out:     indicator.CountTransactions(actions),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *CountTransactionsServer) Write(ctx context.Context, call CountTransactions_write) error {
	actionVal := call.Args().Action()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case server.actions <- indicator.Action(actionVal):
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case res, ok := <-server.out:
		if !ok {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"[financial.strategy.count_transactions.Write] calculator channel closed",
				nil,
			))
		}

		server.result = int64(res)
	}
	return nil
}

/*
Done returns calculated strategy results.
*/
func (server *CountTransactionsServer) Done(ctx context.Context, call CountTransactions_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.strategy.count_transactions.Done] failed to allocate done results",
			err,
		))
	}

	results.SetCount(server.result)
	return nil
}
