package bus

import (
	"sync"
	"time"

	"github.com/theapemachine/qpool"
)

var registryMu sync.Mutex

var groupsByPool = map[*qpool.Q[any]]map[string]*qpool.BroadcastGroup{}

/*
Group returns the single broadcast group for id on this pool.
Repeated calls share one *BroadcastGroup so producers and subscribers wire together.
*/
func Group(pool *qpool.Q[any], id string, ttl time.Duration) *qpool.BroadcastGroup {
	if pool == nil {
		return nil
	}

	registryMu.Lock()
	defer registryMu.Unlock()

	channels, ok := groupsByPool[pool]

	if !ok {
		channels = make(map[string]*qpool.BroadcastGroup)
		groupsByPool[pool] = channels
	}

	existing, ok := channels[id]

	if ok {
		return existing
	}

	created := pool.CreateBroadcastGroup(id, ttl)
	channels[id] = created

	return created
}
