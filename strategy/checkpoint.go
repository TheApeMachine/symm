package strategy

import (
 "context"

 "github.com/theapemachine/errnie"
 "gocloud.dev/blob"
 "gocloud.dev/gcerrors"
)

/* Checkpoint stores the associative model's original encoded state in S3. */
type Checkpoint struct { Bucket *blob.Bucket }

func (checkpoint Checkpoint) Load(ctx context.Context) ([]byte, bool, error) {
 data, err := checkpoint.Bucket.ReadAll(ctx, "models/agent.gob")

 if gcerrors.Code(err) == gcerrors.NotFound { return nil, false, nil }

 if err != nil { return nil, false, errnie.Error(errnie.Err(errnie.IO, "learning: load model", err)) }
 return data, true, nil
}

func (checkpoint Checkpoint) Save(ctx context.Context, data []byte) error {
 if err := checkpoint.Bucket.WriteAll(ctx, "models/agent.gob", data, nil); err != nil {
  return errnie.Error(errnie.Err(errnie.IO, "learning: save model", err))
 }
 return nil
}
