package broker

import (
	"context"
	"sync"

	"github.com/theapemachine/symm/logic"
)

type Trader struct {
	ctx     context.Context
	cancel  context.CancelFunc
	actions *sync.Map
}

func NewTrader(ctx context.Context) *Trader {
	ctx, cancel := context.WithCancel(ctx)

	trader := &Trader{
		ctx:     ctx,
		cancel:  cancel,
		actions: &sync.Map{},
	}

	return trader
}

func (trader *Trader) Update(actions []*logic.Action) {
	for _, action := range actions {
		trader.actions.Store(action.ActionID, action)
	}
}
