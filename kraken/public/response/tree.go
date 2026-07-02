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
	capture   *replayCapture
}

func NewTreeHandler(tree *dmt.Tree) *TreeHandler {
	return &TreeHandler{
		tree:      tree,
		observers: &sync.Map{},
		capture:   newReplayCapture(),
	}
}

func (handler *TreeHandler) Send(artifact *datura.Artifact) *datura.Artifact {
	handler.tree.InsertArtifact(artifact.Prefix("role", "scope", "timestamp"), artifact)

	handler.observers.Range(func(_ any, value any) bool {
		value.(types.Socket).Send(artifact)
		return true
	})

	return artifact
}

func (handler *TreeHandler) Observe(sockets ...types.Socket) {
	for _, socket := range sockets {
		handler.observers.Store(uuid.NewString(), socket)
	}
}
