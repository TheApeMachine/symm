package utils

import (
	"sync"

	"github.com/theapemachine/symm/types"
)

func Fanout(subscribers *sync.Map, name string, thesis *types.Thesis) {
	found, ok := subscribers.Load(name)

	if ok && found != nil {
		for _, subscriber := range found.([]*types.Subscription[any]) {
			subscriber.Send(thesis)
		}
	}
}
