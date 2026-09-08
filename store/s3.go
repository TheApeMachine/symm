package store

import (
	"context"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura/s3"
	"github.com/theapemachine/errnie"
)

// NewS3 opens Datura's client using storage.s3 from the loaded configuration.
// Use client.Bucket() directly to read, write, and list objects; close the client
// when its owner shuts down.
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
