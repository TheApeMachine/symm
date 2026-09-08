package strategy

import (
	"context"
)

// modelKey names the associative model's encoded state in the object store.
const modelKey = "models/agent.gob"

/*
Checkpoint stores the associative model's encoded state.

The model is a gob blob rather than a set of rows, so it stays in object
storage while every Hindsight record family lives in an Iceberg table.
*/
type Checkpoint struct{ Store Blobs }

/*
Blobs is the whole-object storage the checkpoint needs.

It is an interface rather than the concrete store so a test can supply memory
instead of a live bucket: the model is the only thing left in object storage,
and standing up S3 to exercise save-and-restore would test the venue rather
than the learner.
*/
type Blobs interface {
	Read(ctx context.Context, key string) ([]byte, bool, error)
	Write(ctx context.Context, key string, data []byte) error
}

// Load returns the saved model, reporting found=false on a first run.
func (checkpoint Checkpoint) Load(ctx context.Context) ([]byte, bool, error) {
	return checkpoint.Store.Read(ctx, modelKey)
}

// Save writes the model, waiting for the write to be acknowledged.
func (checkpoint Checkpoint) Save(ctx context.Context, data []byte) error {
	return checkpoint.Store.Write(ctx, modelKey, data)
}
