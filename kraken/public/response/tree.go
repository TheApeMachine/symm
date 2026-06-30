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
	return newTreeHandlerWithCapture(tree, newReplayCapture())
}

func NewTreeHandlerWithoutCapture(tree *dmt.Tree) *TreeHandler {
	return newTreeHandlerWithCapture(tree, nil)
}

func newTreeHandlerWithCapture(tree *dmt.Tree, capture *replayCapture) *TreeHandler {
	return &TreeHandler{
		tree:      tree,
		observers: &sync.Map{},
		capture:   capture,
	}
}

func (handler *TreeHandler) Send(artifact *datura.Artifact) *datura.Artifact {
	role := datura.Peek[string](artifact, "channel")

	if role == "" {
		role = datura.Peek[string](artifact, "role")
	}

	frameType := datura.Peek[string](artifact, "type")
	origin := datura.Peek[string](artifact, "origin")
	inserted := false

	if origin == "" {
		origin = "kraken:public"
	}

	// A Kraken frame may carry several symbols. Store one scoped artifact per
	// symbol row; data[0].symbol is only valid after the frame is split.
	for index := 0; ; index++ {
		row := datura.Peek[map[string]any](artifact, "data", index)

		if len(row) == 0 {
			break
		}

		symbol := datura.Peek[string](artifact, "data", index, "symbol")

		if symbol == "" {
			continue
		}

		payload := datura.Map[any]{
			"channel": role,
			"data":    []map[string]any{row},
		}

		if frameType != "" {
			payload["type"] = frameType
		}

		if sequence := datura.Peek[int64](artifact, "sequence"); sequence != 0 {
			payload["sequence"] = sequence
		}

		scoped := datura.Acquire(
			origin, datura.APPJSON,
		).WithRole(
			role,
		).WithScope(
			symbol,
		).WithPayload(
			payload.Marshal(),
		)

		if timestamp := artifact.Timestamp(); timestamp > 0 {
			scoped.SetTimestamp(timestamp)
		}

		handler.tree.InsertArtifact(scoped.Prefix(
			"role", "timestamp", "scope",
		), scoped)
		if scopedHistoryRole(role) {
			handler.tree.InsertArtifact(scoped.Prefix(
				"role", "scope", "timestamp",
			), scoped)
			handler.tree.InsertArtifact(latestScopedKey(role, symbol), scoped)
		}
		handler.capture.Write(scoped)

		handler.observers.Range(func(_ any, value any) bool {
			value.(types.Socket).Send(scoped)
			return true
		})

		inserted = true
	}

	if inserted {
		return artifact
	}

	scope := frameType

	if scope == "" {
		scope = datura.Peek[string](artifact, "scope")
	}

	artifact.WithRole(role).WithScope(scope)

	handler.tree.InsertArtifact(artifact.Prefix(
		"role", "timestamp", "scope",
	), artifact)
	if scopedHistoryRole(role) {
		handler.tree.InsertArtifact(artifact.Prefix(
			"role", "scope", "timestamp",
		), artifact)
		if scope != "" {
			handler.tree.InsertArtifact(latestScopedKey(role, scope), artifact)
		}
	}
	handler.capture.Write(artifact)

	handler.observers.Range(func(_ any, value any) bool {
		value.(types.Socket).Send(artifact)
		return true
	})

	return artifact
}

func scopedHistoryRole(role string) bool {
	switch role {
	case "book", "level3", "ticker", "trade":
		return true
	default:
		return false
	}
}

func latestScopedKey(role, symbol string) []byte {
	return []byte("latest/" + role + "/" + symbol)
}

func (handler *TreeHandler) Observe(sockets ...types.Socket) {
	for _, socket := range sockets {
		handler.observers.Store(uuid.NewString(), socket)
	}
}
