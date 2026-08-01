package signal

import (
	"sync"

	"github.com/theapemachine/symm/types"
)

func Subscribe(
	mu *sync.Mutex,
	subscribers *sync.Map,
	channel string,
	subscription *types.Subscription[any],
) *types.Subscription[any] {
	mu.Lock()
	defer mu.Unlock()

	current, ok := subscribers.Load(channel)

	if !ok {
		subscribers.Store(channel, []*types.Subscription[any]{subscription})
		return subscription
	}

	found := current.([]*types.Subscription[any])
	next := append([]*types.Subscription[any]{}, found...)
	next = append(next, subscription)
	subscribers.Store(channel, next)

	return subscription
}