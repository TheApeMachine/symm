package main

import (
	"context"
	"encoding/json"
	"io"

	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/store"
	"gocloud.dev/blob"
)

// exportFrames writes the original captured records as JSONL.
func exportFrames(bucket *blob.Bucket, run string, through uint64, output io.Writer) error {
	encoder := json.NewEncoder(output)
	return store.Scan(context.Background(), bucket, hindsight.RunID(run).Prefix("captures"), func(frame hindsight.RawFrame) (bool, error) {
		if through > 0 && uint64(frame.Identity.Sequence) > through {
			return false, nil
		}
		switch frame.Kind {
		case "ticker", "trade", "level3", "instrument":
			return true, encoder.Encode(frame)
		default:
			return true, nil
		}
	})
}
