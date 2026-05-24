package engine

import "context"

type tickDrainKey struct{}

/*
TickDrain drains orchestrator tick consumers between measure steps.
*/
type TickDrain func()

/*
WithTickDrain attaches a drain callback used while signals measure symbols.
*/
func WithTickDrain(ctx context.Context, drain TickDrain) context.Context {
	if drain == nil {
		return ctx
	}

	return context.WithValue(ctx, tickDrainKey{}, drain)
}

/*
DrainTicks runs the orchestrator drain hook when present.
*/
func DrainTicks(ctx context.Context) {
	drain, ok := ctx.Value(tickDrainKey{}).(TickDrain)

	if !ok || drain == nil {
		return
	}

	drain()
}
