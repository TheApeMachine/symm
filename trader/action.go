package trader

import (
	"context"

	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/symm/logic"
)

type Action struct {
	ctx     context.Context
	cancel  context.CancelFunc
	err     error
	actions structure.Ring[logic.Action]
}

func NewAction(ctx context.Context) *Action {
	ctx, cancel := context.WithCancel(ctx)

	action := &Action{
		ctx:    ctx,
		cancel: cancel,
	}

	return action
}

func (action *Action) Update(update *logic.Action) {
	if action.actions == nil {
		action.actions = structure.NewListRing[logic.Action](
			1,
		)
	}
}

func (action *Action) Error() error {
	return action.err
}

func (action *Action) Close() error {
	action.cancel()
	return nil
}
