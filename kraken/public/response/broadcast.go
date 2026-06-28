package response

import (
	"slices"
	"sync"

	"github.com/google/uuid"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/types"
)

type BroadcastHandler struct {
	selectors   []string
	destination string
	group       *qpool.BroadcastGroup
	tree        *dmt.Tree
	origin      string
	observers   *sync.Map
}

func NewBroadcastHandler(
	selectors []string,
	destination string,
	group *qpool.BroadcastGroup,
) *BroadcastHandler {
	return NewBroadcastHandlerWithTree(
		"kraken:public", selectors, destination, group, nil,
	)
}

func NewBroadcastHandlerWithTree(
	origin string,
	selectors []string,
	destination string,
	group *qpool.BroadcastGroup,
	tree *dmt.Tree,
) *BroadcastHandler {
	return &BroadcastHandler{
		selectors:   selectors,
		destination: destination,
		group:       group,
		tree:        tree,
		origin:      origin,
		observers:   &sync.Map{},
	}
}

func (handler *BroadcastHandler) Send(message []byte) *types.SocketMessage {
	artifact := datura.Acquire(
		handler.origin, datura.APPJSON,
	).WithDestination(
		handler.destination,
	)

	artifact.Unpack(message)

	if slices.Contains(
		handler.selectors,
		datura.Peek[string](artifact, "role"),
	) {
		handler.group.Send(artifact)
	}

	handler.observers.Range(func(_ any, value any) bool {
		value.(types.Socket).Send(artifact.Pack())
		return true
	})

	return &types.SocketMessage{}
}

func (handler *BroadcastHandler) Observe(sockets ...types.Socket) {
	for _, socket := range sockets {
		handler.observers.Store(uuid.NewString(), socket)
	}
}
