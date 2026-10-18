package store

import (
	"bytes"
	"context"
	"io"
	"iter"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/system"
)

type Client struct {
	*runtime.System
	S3Client   *s3.Client
	bucket     string
	downloader *manager.Downloader
	uploader   *manager.Uploader
}

func NewClient(ctx context.Context, bucket string) *Client {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithHTTPClient(&http.Client{
		Transport: &http.Transport{
			MaxIdleConns:          system.Cfg.Storage.S3.MaxIdleConns,
			MaxIdleConnsPerHost:   system.Cfg.Storage.S3.MaxIdleConnsPerHost,
			MaxConnsPerHost:       system.Cfg.Storage.S3.MaxConnsPerHost,
			IdleConnTimeout:       system.Cfg.Storage.S3.IdleConnTimeout,
			ResponseHeaderTimeout: system.Cfg.Storage.S3.ResponseHeaderTimeout,
		},
		Timeout: system.Cfg.Storage.S3.RequestTimeout,
	}))

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.IO, "[store.client] error", err,
		))

		return nil
	}

	client := s3.NewFromConfig(cfg, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(system.Cfg.Storage.S3.Endpoint)
		options.UsePathStyle = true
		options.Region = system.Cfg.Storage.S3.Region
		options.Credentials = credentials.NewStaticCredentialsProvider(
			"anonymous", "anonymous", "",
		)
		// SeaweedFS does not send a checksum header. The SDK's default
		// (when supported) logs that skip once per object.
		options.ResponseChecksumValidation = aws.ResponseChecksumValidationWhenRequired
	})

	_, err = client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucket),
	})

	if err != nil && !strings.Contains(err.Error(), "BucketAlready") {
		errnie.Error(errnie.Err(
			errnie.IO, "[store.client] create bucket error", err,
		))

		return nil
	}

	return &Client{
		System:     runtime.NewSystem(ctx, "store.client"),
		S3Client:   client,
		bucket:     bucket,
		downloader: manager.NewDownloader(client),
		uploader:   manager.NewUploader(client),
	}
}

func (client *Client) List(
	prefix string,
) iter.Seq[*s3.ListObjectsV2Output] {
	return func(yield func(*s3.ListObjectsV2Output) bool) {
		if client == nil {
			return
		}

		page, err := client.S3Client.ListObjectsV2(client.Context(), &s3.ListObjectsV2Input{
			Bucket: aws.String(client.bucket),
			Prefix: aws.String(prefix),
		})

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.IO, "[store.client] list objects error", err,
			))

			return
		}

		if !yield(page) {
			return
		}
	}
}

/*
ListKeys collects object keys under prefixes, drops duplicates, and sorts them
by epoch, tick, then the full key. Canonical keys store those segments as
zero-padded decimals, so that string order is numeric order. A listing failure
returns no keys: a partial page is not reported as the complete set.
*/
func (client *Client) ListKeys(prefix string) iter.Seq[string] {
	return func(yield func(string) bool) {
		for page := range client.List(prefix) {
			for _, object := range page.Contents {
				if object.Key == nil {
					continue
				}

				if !yield(*object.Key) {
					return
				}
			}
		}
	}
}

/*
Read on the shared client does not replay a prefix; use Values(prefix) or Open(prefix).
*/
func (client *Client) Read(p []byte) (n int, err error) {
	value := data.From(p)

	for page := range client.List(value.Prefix) {
		for _, object := range page.Contents {
			if object.Key == nil {
				continue
			}

			writer := manager.NewWriteAtBuffer(make([]byte, 0, 4096))

			_, err := client.downloader.Download(client.Context(), writer, &s3.GetObjectInput{
				Bucket: aws.String(client.bucket),
				Key:    aws.String(*object.Key),
			})

			if err != nil {
				return 0, errnie.Error(errnie.Err(
					errnie.IO, "[store.client] download error", err,
				))
			}

			n, err = value.Write(writer.Bytes())

			if err != nil {
				return n, client.Error(err)
			}
		}
	}

	return len(p), io.EOF
}

/*
Stream writes each object under prefix to dst, one Write per object.
A canceled ctx stops the stream before the next object.
*/
func (client *Client) Stream(ctx context.Context, prefix string, dst io.Writer) error {
	if client == nil {
		return errnie.Error(errnie.Err(
			errnie.ServiceUnavailable, "[store.client] no client", nil,
		))
	}

	for key := range client.ListKeys(prefix) {
		if err := ctx.Err(); err != nil {
			return err
		}

		body := manager.NewWriteAtBuffer(make([]byte, 0, 4096))

		_, err := client.downloader.Download(ctx, body, &s3.GetObjectInput{
			Bucket: aws.String(client.bucket),
			Key:    aws.String(key),
		})

		if err != nil {
			return errnie.Error(errnie.Err(
				errnie.IO, "[store.client] download error", err,
			))
		}

		if _, err = dst.Write(body.Bytes()); err != nil {
			return err
		}
	}

	return nil
}

/*
Write uploads a serialized Value. Prefix is not in the body, so a Value
copied onto the client uses WriteAtPrefix and the key is that prefix.
*/
func (client *Client) Write(p []byte) (int, error) {
	return client.WriteAtPrefix(data.From(p).Prefix, p)
}

/*
WriteAtPrefix uploads p at prefix. An empty prefix is refused.
*/
func (client *Client) WriteAtPrefix(prefix string, p []byte) (int, error) {
	if prefix == "" {
		return 0, client.Error(errnie.Err(
			errnie.Validation, "[store.client] upload key is empty", nil,
		))
	}

	_, err := client.uploader.Upload(client.Context(), &s3.PutObjectInput{
		Bucket: aws.String(client.bucket),
		Key:    aws.String(prefix),
		Body:   bytes.NewReader(p),
	})

	if err != nil {
		return 0, client.Error(errnie.Err(
			errnie.IO, "[store.client] upload error", err,
		))
	}

	return len(p), nil
}

/*
DeletePrefix deletes all objects under the specified prefix.
*/
func (client *Client) DeletePrefix(prefix string) error {
	for page := range client.List(prefix) {
		for _, object := range page.Contents {
			if object.Key == nil {
				continue
			}

			_, err := client.S3Client.DeleteObject(client.Context(), &s3.DeleteObjectInput{
				Bucket: aws.String(client.bucket),
				Key:    aws.String(*object.Key),
			})

			if err != nil {
				return errnie.Error(errnie.Err(
					errnie.IO, "[store.client] delete object error", err,
				))
			}
		}
	}

	return nil
}

/*
MovePrefix copies all objects matching sourcePrefix to destPrefix, then deletes the originals.
*/
func (client *Client) MovePrefix(sourcePrefix, destPrefix string) error {
	for page := range client.List(sourcePrefix) {
		for _, object := range page.Contents {
			if object.Key == nil {
				continue
			}

			_, err := client.S3Client.CopyObject(client.Context(), &s3.CopyObjectInput{
				Bucket:     aws.String(client.bucket),
				Key:        aws.String(*object.Key),
				CopySource: aws.String(client.bucket + "/" + *object.Key),
			})

			if err != nil {
				return errnie.Error(errnie.Err(
					errnie.IO, "[store.client] copy object error", err,
				))
			}

			_, err = client.S3Client.DeleteObject(client.Context(), &s3.DeleteObjectInput{
				Bucket: aws.String(client.bucket),
				Key:    aws.String(*object.Key),
			})
		}
	}

	return nil
}
