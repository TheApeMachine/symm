package bus

import (
	"context"
	"time"

	"github.com/theapemachine/qpool"
)

/*
PollFor returns the next broadcast value or ctx.Err() when the deadline passes.
*/
func PollFor(ctx context.Context, consumer *qpool.BroadcastConsumer) (*qpool.QValue[any], error) {
	if consumer == nil {
		return nil, context.Canceled
	}

	for {
		if value := consumer.Poll(); value != nil {
			return value, nil
		}

		if err := ctx.Err(); err != nil {
			return nil, err
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(1 * time.Millisecond):
		}
	}
}
