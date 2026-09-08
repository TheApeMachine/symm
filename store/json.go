package store

import (
	"context"
	"io"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"gocloud.dev/blob"
)

// Write stores the supplied value as JSON at its logical object key.
func Write(ctx context.Context, bucket *blob.Bucket, key string, value any) error {
	data, err := sonic.Marshal(value)

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "store: encode "+key, err))
	}

	if err := bucket.WriteAll(ctx, key, data, nil); err != nil {
		return errnie.Error(errnie.Err(errnie.IO, "store: write "+key, err))
	}

	return nil
}

// Read decodes the original stored type. Missing objects remain errors; callers
// can distinguish them using gcerrors.Code(err) == gcerrors.NotFound.
func Read[Value any](ctx context.Context, bucket *blob.Bucket, key string) (Value, error) {
	var value Value
	data, err := bucket.ReadAll(ctx, key)

	if err != nil {
		return value, errnie.Error(errnie.Err(errnie.IO, "store: read "+key, err))
	}

	if err := sonic.Unmarshal(data, &value); err != nil {
		return value, errnie.Error(errnie.Err(errnie.Validation, "store: decode "+key, err))
	}

	return value, nil
}

// Scan reads one JSON object at a time under a prefix. Returning false stops
// the traversal. Listing, reading, decoding, and visitor failures are returned.
func Scan[Value any](
	ctx context.Context, bucket *blob.Bucket, prefix string,
	visit func(Value) (bool, error),
) error {
	objects := bucket.List(&blob.ListOptions{Prefix: prefix})

	for {
		object, err := objects.Next(ctx)

		if err == io.EOF {
			return nil
		}

		if err != nil {
			return errnie.Error(errnie.Err(errnie.IO, "store: list "+prefix, err))
		}

		value, err := Read[Value](ctx, bucket, object.Key)

		if err != nil {
			return err
		}

		more, err := visit(value)

		if err != nil {
			return errnie.Error(errnie.Err(errnie.Internal, "store: visit "+object.Key, err))
		}

		if !more {
			return nil
		}
	}
}

// List decodes a prefix into the original record type for collection readers.
func List[Value any](ctx context.Context, bucket *blob.Bucket, prefix string) ([]Value, error) {
	values := []Value{}
	err := Scan(ctx, bucket, prefix, func(value Value) (bool, error) {
		values = append(values, value)
		return true, nil
	})

	return values, err
}
