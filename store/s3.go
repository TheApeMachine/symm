package store

import (
	"context"
	"errors"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"gocloud.dev/blob"
	"gocloud.dev/gcerrors"
	"io"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura/s3"
	"github.com/theapemachine/errnie"
)

// NewS3 opens Datura's client using storage.s3 from the loaded configuration.
// Use ReadAll for complete downloads and client.Bucket() for writes and lists.
// Close the client when its owner shuts down.
func NewS3(ctx context.Context) (*s3.Client, error) {
	client, err := s3.NewClient(ctx, s3.Config{
		BucketURL: viper.GetString("storage.s3.bucket_url"),
		Bucket:    viper.GetString("storage.s3.bucket"),
		Region:    viper.GetString("storage.s3.region"),
		Prefix:    viper.GetString("storage.s3.prefix"),
	})

	if err != nil {
		return nil, errnie.Error(errnie.Err(errnie.IO, "store: open S3", err))
	}

	return client, nil
}

// ReadAll retries interrupted response bodies before exposing any bytes to a
// decoder. The SDK retries GetObject requests, but its returned body is streamed
// afterward. Its standard attempt budget and backoff also govern these retries.
func ReadAll(ctx context.Context, bucket *blob.Bucket, key string) ([]byte, error) {
	policy := retry.NewStandard()

	for attempt := 1; ; attempt++ {
		payload, err := bucket.ReadAll(ctx, key)

		if err == nil || gcerrors.Code(err) == gcerrors.NotFound {
			return payload, err
		}

		if !errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, errnie.Error(errnie.Err(errnie.IO, "store: download "+key, err))
		}

		if attempt >= policy.MaxAttempts() {
			return nil, errnie.Error(errnie.Err(errnie.IO, "store: interrupted download "+key,
				&retry.MaxAttemptsError{Attempt: attempt, Err: err}))
		}
		delay, err := policy.RetryDelay(attempt, err)

		if err != nil {
			return nil, errnie.Error(errnie.Err(errnie.IO, "store: download retry delay", err))
		}
		errnie.Warn("store: interrupted download; retrying", "key", key, "attempt", attempt, "delay", delay)
		timer := time.NewTimer(delay)

		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, errnie.Error(errnie.Err(errnie.IO, "store: download canceled "+key, ctx.Err()))
		case <-timer.C:
		}
	}
}
