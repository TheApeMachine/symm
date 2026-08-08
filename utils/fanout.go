package utils

import (
	"sync"

	"github.com/theapemachine/symm/types"
)

func Subscribe(
	subscribers *sync.Map,
	channel string,
	subscription *types.Subscription[any],
) *types.Subscription[any] {
	current, ok := subscribers.LoadOrStore(
		channel,
		[]*types.Subscription[any]{subscription},
	)

	if ok {
		subscribers.Store(
			channel,
			append(current.([]*types.Subscription[any]), subscription),
		)

		return subscription
	}

	return subscription
}
