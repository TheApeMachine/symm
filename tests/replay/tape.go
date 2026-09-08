package replay

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight"
)

// Tape reads cached original S3 capture objects in their persisted order.
// It refuses missing records, mixed runs and altered payloads before delivery.
type Tape struct {
	Directory string
	Through   time.Time
}

var errThrough = errors.New("replay: requested capture interval complete")

func (tape Tape) Read(ctx context.Context, visit func(hindsight.RawFrame) error) error {
	paths, err := filepath.Glob(filepath.Join(tape.Directory, "*.jsonl"))

	if err != nil {
		return errnie.Error(err)
	}

	if len(paths) == 0 {
		return errnie.Error(errnie.Err(errnie.NotFound, "replay: no capture objects", nil))
	}
	var previous hindsight.CaptureIdentity

	for _, path := range paths {
		if err := tape.read(ctx, path, &previous, visit); err != nil {
			if errors.Is(err, errThrough) {
				return nil
			}
			return err
		}
	}
	return nil
}

func (tape Tape) read(
	ctx context.Context, path string, previous *hindsight.CaptureIdentity,
	visit func(hindsight.RawFrame) error,
) (err error) {
	file, err := os.Open(path)

	if err != nil {
		return errnie.Error(err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			err = errors.Join(err, errnie.Error(closeErr))
		}
	}()
	decoder := json.NewDecoder(file)

	for {
		if err := ctx.Err(); err != nil {
			return errnie.Error(err)
		}
		var frame hindsight.RawFrame
		err := decoder.Decode(&frame)

		if err == io.EOF {
			return nil
		}

		if err != nil {
			return errnie.Error(err)
		}
		digest := sha256.Sum256(frame.Payload)

		if frame.Identity.Sequence != previous.Sequence+1 ||
			(previous.Run != "" && frame.Identity.Run != previous.Run) ||
			hex.EncodeToString(digest[:]) != frame.PayloadHash {
			return errnie.Error(errnie.Err(errnie.Validation, "replay: capture identity or payload integrity failed in "+path, nil))
		}
		*previous = frame.Identity

		if !tape.Through.IsZero() && frame.ReceivedAt.After(tape.Through) {
			return errThrough
		}

		if err := visit(frame); err != nil {
			return errnie.Error(err)
		}
	}
}
