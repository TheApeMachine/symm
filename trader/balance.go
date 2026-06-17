package trader

import (
	"context"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/types"
	"github.com/theapemachine/symm/kraken/user"
)

type Balances struct {
	ctx        context.Context
	cancel     context.CancelFunc
	err        error
	pool       *qpool.Q[any]
	broadcasts *sync.Map
	balances   *user.Balances
}

func NewBalances(ctx context.Context, pool *qpool.Q[any]) *Balances {
	ctx, cancel := context.WithCancel(ctx)

	balances := &Balances{
		ctx:        ctx,
		cancel:     cancel,
		pool:       pool,
		broadcasts: &sync.Map{},
		balances:   &user.Balances{},
	}

	for _, channel := range []string{"kraken:private"} {
		balances.broadcasts.Store(channel, pool.CreateBroadcastGroup(channel))
	}

	return balances
}

func (balances *Balances) Update(update user.Balances) {
	balances.balances = &update
}

func (balances *Balances) Snapshot() *user.Balances {
	return balances.balances
}

func (balances *Balances) Subscribe() error {
	message, err := types.NewKrakenMessage("subscribe", user.BalanceParams{
		Channel:  "balances",
		Snapshot: true,
	}, 0)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"balances: failed to build subscribe message",
			err,
		))
	}

	payload, err := sonic.Marshal(message)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"balances: failed to marshal subscribe",
			err,
		))
	}

	artifact := datura.Acquire(
		"balances", datura.Artifact_Type_json,
	).WithDestination(
		"kraken:private",
	).WithRole(
		"balances",
	).WithPayload(
		payload,
	)

	bg, _ := balances.broadcasts.Load("kraken:private")
	return errnie.Error(bg.(*qpool.BroadcastGroup).Send(artifact))
}

func (balances *Balances) Error() error {
	return balances.err
}

func (balances *Balances) Close() error {
	balances.cancel()
	return nil
}
