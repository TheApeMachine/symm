package store

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
RevisionServer owns an append-only keyed collection of documents and hands out
frozen immutable revisions on Done.
*/
type RevisionServer struct {
	*runtime.System
	entries  map[string]json.RawMessage
	revision int64
	out      []byte
	changed  bool
}

func NewRevision(ctx context.Context) *RevisionServer {
	server := &RevisionServer{
		System:  runtime.NewSystem(ctx, "store.revision"),
		entries: make(map[string]json.RawMessage),
	}

	server.Transition(runtime.READY)
	return server
}

func (server *RevisionServer) Write(ctx context.Context, call Revision_write) error {
	args := call.Args()

	if args.Reset() {
		server.entries = make(map[string]json.RawMessage)
		server.revision = 0
		server.out = nil
		server.changed = false
	}

	key, err := args.Key()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[store.revision.Write] failed to read key",
			err,
		))
	}

	data, err := args.Data()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[store.revision.Write] failed to read data",
			err,
		))
	}

	if len(data) > 0 {
		effectiveKey := key

		if effectiveKey == "" {
			effectiveKey = extractKey(data)
		}

		if effectiveKey == "" {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"[store.revision.Write] document missing key and self-identifying pair coordinates",
				nil,
			))
		}

		server.entries[effectiveKey] = bytes.Clone(data)
		server.changed = true
	}

	if (args.Flush() || len(data) > 0) && server.changed && len(server.entries) > 0 {
		encoded, err := json.Marshal(server.entries)

		if err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"[store.revision.Write] failed to encode revision map",
				err,
			))
		}

		server.revision++
		server.out = encoded
		server.changed = false
	}

	return nil
}

func (server *RevisionServer) Done(ctx context.Context, call Revision_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[store.revision.Done] failed to allocate results",
			err,
		))
	}

	results.SetStatus(runtime.Status(server.Status()))
	results.SetRevision(server.revision)
	results.SetCount(int64(len(server.entries)))

	if len(server.out) == 0 {
		return nil
	}

	if err := results.SetOut(server.out); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[store.revision.Done] failed to set out",
			err,
		))
	}

	return nil
}

/*
extractKey attempts to resolve a canonical key from an evidence payload.
*/
func extractKey(payload []byte) string {
	var document struct {
		Key  string `json:"key"`
		Pair struct {
			Left  any `json:"left"`
			Right any `json:"right"`
		} `json:"pair"`
	}

	if err := json.Unmarshal(payload, &document); err != nil {
		return ""
	}

	if document.Key != "" {
		return document.Key
	}

	if document.Pair.Left != nil && document.Pair.Right != nil {
		return fmt.Sprintf("%v:%v", document.Pair.Left, document.Pair.Right)
	}

	return ""
}
