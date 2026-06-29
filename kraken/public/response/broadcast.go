package response

import (
	"slices"
	"sync"

	"github.com/google/uuid"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/types"
)

type BroadcastHandler struct {
	selectors   []string
	origin      string
	destination string
	group       *qpool.BroadcastGroup
	observers   *sync.Map
}

func NewBroadcastHandler(
	selectors []string,
	origin string,
	destination string,
	group *qpool.BroadcastGroup,
) *BroadcastHandler {
	return &BroadcastHandler{
		selectors:   selectors,
		origin:      origin,
		destination: destination,
		group:       group,
		observers:   &sync.Map{},
	}
}

func (handler *BroadcastHandler) Send(artifact *datura.Artifact) *datura.Artifact {
	if !slices.Contains(
		handler.selectors, datura.Peek[string](artifact, "role"),
	) {
		return nil
	}

	artifact.WithDestination(handler.destination)
	errnie.Error(handler.group.Send(artifact))

	handler.observers.Range(func(_ any, value any) bool {
		value.(types.Socket).Send(artifact)
		return true
	})

	return artifact
}

func (handler *BroadcastHandler) Observe(sockets ...types.Socket) {
	for _, socket := range sockets {
		handler.observers.Store(uuid.NewString(), socket)
	}
}
