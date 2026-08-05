package broker

import "context"

type Recovery struct {
	ctx    context.Context
	cancel context.CancelFunc
}

func NewRecovery(ctx context.Context) *Recovery {
	ctx, cancel := context.WithCancel(ctx)

	return &Recovery{
		ctx:    ctx,
		cancel: cancel,
	}
}
