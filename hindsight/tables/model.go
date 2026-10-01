package tables

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/system"
)

// ErrBlobMissing is the cause when the model object has not been written.
var ErrBlobMissing = errors.New("model blob is missing")

func (catalog *Catalog) PutBlob(ctx context.Context, key string, body []byte) error {
	client, bucket, err := catalog.blobs()

	if err != nil {
		return err
	}

	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader(body),
	})

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"catalog: failed to write "+key,
			err,
		))
	}

	return nil
}

func (catalog *Catalog) GetBlob(ctx context.Context, key string) ([]byte, error) {
	client, bucket, err := catalog.blobs()

	if err != nil {
		return nil, err
	}

	output, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})

	if err != nil && blobMissing(err) {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"catalog: "+key+" is missing",
			ErrBlobMissing,
		))
	}

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			"catalog: failed to read "+key,
			err,
		))
	}

	defer output.Body.Close()
	body, err := io.ReadAll(output.Body)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			"catalog: failed to read "+key,
			err,
		))
	}

	return body, nil
}

func (catalog *Catalog) blobs() (*s3.Client, string, error) {
	if catalog == nil || catalog.awsConfig == nil {
		return nil, "", errnie.Error(errnie.Err(
			errnie.Validation,
			"catalog: object storage is not configured",
			nil,
		))
	}

	if system.Cfg == nil || system.Cfg.Storage == nil || system.Cfg.Storage.S3 == nil || system.Cfg.Storage.S3.Bucket == "" {
		return nil, "", errnie.Error(errnie.Err(
			errnie.Validation,
			"catalog: storage.s3 bucket is required",
			nil,
		))
	}

	endpoint := system.Cfg.Storage.S3.Endpoint
	client := s3.NewFromConfig(*catalog.awsConfig, func(options *s3.Options) {
		options.UsePathStyle = true

		if endpoint != "" {
			options.BaseEndpoint = aws.String(endpoint)
		}
	})

	return client, system.Cfg.Storage.S3.Bucket, nil
}

func blobMissing(err error) bool {
	var api smithy.APIError

	if errors.As(err, &api) && (api.ErrorCode() == "NoSuchKey" || api.ErrorCode() == "NotFound") {
		return true
	}

	var response *awshttp.ResponseError

	return errors.As(err, &response) && response.Response != nil && response.Response.StatusCode == http.StatusNotFound
}
