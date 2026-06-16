package trader

import (
	"context"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/symm/kraken/user"
)

type Balances struct {
	ctx      context.Context
	cancel   context.CancelFunc
	err      error
	balances structure.Ring[user.Balances]
}

func NewBalances(ctx context.Context) *Balances {
	ctx, cancel := context.WithCancel(ctx)

	balances := &Balances{
		ctx:    ctx,
		cancel: cancel,
	}

	return balances
}

func (balances *Balances) Update(update user.Balances) {
	if balances.balances == nil {
		balances.balances = structure.NewListRing[user.Balances](
			1,
			datura.Acquire("balances", datura.Artifact_Type_json),
		)
	}
}

func (balances *Balances) Error() error {
	return balances.err
}

func (balances *Balances) Close() error {
	balances.cancel()
	return nil
}
