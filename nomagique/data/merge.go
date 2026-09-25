package data

import (
	"context"
	"encoding/json"

	"github.com/theapemachine/errnie"
)

// MergeServer overlays fields without converting values. Gathered overlays
// are optional publications in this evaluation; absence leaves the base alone.
type MergeServer struct{ out []byte }

func NewMerge() *MergeServer { return &MergeServer{} }

func (server *MergeServer) Write(ctx context.Context, call Merge_write) error {
	server.out = nil
	base, err := call.Args().Base()

	if err != nil {
		return mergeError("read base", err)
	}

	if len(base) == 0 {
		return nil
	}

	var document map[string]json.RawMessage

	if err := json.Unmarshal(base, &document); err != nil {
		return mergeError("base must be an object", err)
	}

	if document == nil {
		return mergeError("null is not an object", nil)
	}

	overlay, err := call.Args().Overlay()

	if err != nil {
		return mergeError("read overlay", err)
	}

	if err := mergeFields(document, overlay); err != nil {
		return err
	}

	overlays, err := call.Args().Overlays()

	if err != nil {
		return mergeError("read gathered overlays", err)
	}

	for index := range overlays.Len() {
		incoming, err := overlays.At(index)

		if err != nil {
			return mergeError("read overlay slot", err)
		}

		if err := mergeFields(document, incoming); err != nil {
			return err
		}
	}

	server.out, err = json.Marshal(document)

	if err != nil {
		return mergeError("encode object", err)
	}

	return nil
}

func mergeFields(document map[string]json.RawMessage, incoming []byte) error {
	if len(incoming) == 0 {
		return nil
	}

	var replacements map[string]json.RawMessage

	if err := json.Unmarshal(incoming, &replacements); err != nil {
		return mergeError("overlay must be an object", err)
	}

	if replacements == nil {
		return mergeError("null is not an object", nil)
	}

	for name, value := range replacements {
		document[name] = value
	}

	return nil
}

func mergeError(message string, cause error) error {
	return errnie.Error(errnie.Err(errnie.Validation, "data.merge: "+message, cause))
}

func (server *MergeServer) Done(ctx context.Context, call Merge_done) error {
	result, err := call.AllocResults()

	if err != nil {
		return mergeError("allocate result", err)
	}

	defer func() { server.out = nil }()

	if err := result.SetOut(server.out); err != nil {
		return mergeError("publish result", err)
	}

	return nil
}
