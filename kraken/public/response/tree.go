package response

import (
	"sync"

	"github.com/google/uuid"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/kraken/types"
)

type TreeHandler struct {
	tree      *dmt.Tree
	observers *sync.Map
}

func NewTreeHandler(tree *dmt.Tree) *TreeHandler {
	return &TreeHandler{
		tree:      tree,
		observers: &sync.Map{},
	}
}

func (handler *TreeHandler) Send(message []byte) *types.SocketMessage {
	out := &types.SocketMessage{}
	out.Decode(message)

	artifact := datura.Acquire(
		"kraken:public", datura.APPJSON,
	).WithPayload(message)

	role := datura.Peek[string](artifact, "channel")

	artifact.WithRole(role).WithScope(
		datura.Peek[string](artifact, "type"),
	)

	handler.tree.InsertArtifact(artifact.Prefix(
		"role", "scope", "timestamp",
	), artifact)

	handler.observers.Range(func(_ any, value any) bool {
		value.(types.Socket).Send(out.Marshal())
		return true
	})

	return out
}

func (handler *TreeHandler) Observe(sockets ...types.Socket) {
	for _, socket := range sockets {
		handler.observers.Store(uuid.NewString(), socket)
	}
}
