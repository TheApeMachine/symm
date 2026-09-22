package data

import (
	"context"
	"encoding/json"
	"github.com/theapemachine/errnie"
)

/* MergeServer overlays fields without converting the values or synthesizing operands. */
type MergeServer struct{ out []byte }

/* NewMerge constructs an idle object-overlay primitive. */
func NewMerge() *MergeServer { return &MergeServer{} }

/* Write preserves JSON values byte-for-byte while overlay fields replace matching base fields. */
func (server *MergeServer) Write(ctx context.Context, call Merge_write) error {
	server.out = nil
	base, err := call.Args().Base()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "merge: base", err))
	}
	overlay, err := call.Args().Overlay()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "merge: overlay", err))
	}
	var document, replacements map[string]json.RawMessage
	if err := json.Unmarshal(base, &document); err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "merge: base must be an object", err))
	}
	if err := json.Unmarshal(overlay, &replacements); err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "merge: overlay must be an object", err))
	}
	if document == nil || replacements == nil {
		return errnie.Error(errnie.Err(errnie.Validation, "merge: null is not an object", nil))
	}
	for name, value := range replacements {
		document[name] = value
	}
	server.out, err = json.Marshal(document)
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "merge: encode object", err))
	}
	return nil
}

/* Done emits the object and resets evaluation state. */
func (server *MergeServer) Done(ctx context.Context, call Merge_done) error {
	result, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "merge: results", err))
	}
	if err := result.SetOut(server.out); err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "merge: output", err))
	}
	server.out = nil
	return nil
}
