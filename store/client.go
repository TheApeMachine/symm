package store

import (
	"bytes"
	"context"
	"errors"
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
	s3Client   *s3.Client
	bucket     string
	downloader *manager.Downloader
	uploader   *manager.Uploader
	slots      chan struct{}
}

var _ io.ReadWriteCloser = (*Client)(nil)

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
		s3Client:   client,
		bucket:     bucket,
		downloader: manager.NewDownloader(client),
		uploader:   manager.NewUploader(client),
		slots:      make(chan struct{}, system.Cfg.Storage.S3.MaxIdleConns),
	}
}

func (client *Client) acquire(ctx context.Context) error {
	if client == nil || client.slots == nil {
		return nil
	}

	select {
	case client.slots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (client *Client) release() {
	if client == nil || client.slots == nil {
		return
	}

	<-client.slots
}

func listErr(err error) error {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}

	return errnie.Error(errnie.Err(
		errnie.IO, "[store.client] listing error", err,
	))
}

func (client *Client) listObjects(
	ctx context.Context,
	prefix,
	delimiter string,
) iter.Seq2[*s3.ListObjectsV2Output, error] {
	return func(yield func(*s3.ListObjectsV2Output, error) bool) {
		if client == nil || client.s3Client == nil {
			yield(nil, listErr(errnie.Err(
				errnie.Validation, "[store.client] client is nil", nil,
			)))

			return
		}

		input := &s3.ListObjectsV2Input{
			Bucket: aws.String(client.bucket),
			Prefix: aws.String(prefix),
		}

		if delimiter != "" {
			input.Delimiter = aws.String(delimiter)
		}

		paginator := s3.NewListObjectsV2Paginator(client.s3Client, input)

		for paginator.HasMorePages() {
			if err := ctx.Err(); err != nil {
				yield(nil, err)
				return
			}

			page, err := paginator.NextPage(ctx)

			if err != nil {
				yield(nil, listErr(err))
				return
			}

			if !yield(page, nil) {
				return
			}
		}
	}
}

func (client *Client) List(prefix string, delimiter ...string) iter.Seq[*s3.ListObjectsV2Output] {
	delim := ""

	if len(delimiter) > 0 {
		delim = delimiter[0]
	}

	return func(yield func(*s3.ListObjectsV2Output) bool) {
		if client == nil {
			return
		}

		for page, err := range client.listObjects(client.Context(), prefix, delim) {
			if err != nil || page == nil || !yield(page) {
				return
			}
		}
	}
}

func (client *Client) Download(key string) ([]byte, error) {
	if client == nil || client.downloader == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "[store.client] client or downloader is nil", nil,
		))
	}

	writer := manager.NewWriteAtBuffer(make([]byte, 0, 4096))

	_, err := client.downloader.Download(client.Context(), writer, &s3.GetObjectInput{
		Bucket: aws.String(client.bucket),
		Key:    aws.String(key),
	})

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO, "[store.client] download error", err,
		))
	}

	return writer.Bytes(), nil
}

/*
ListKeys collects object keys under prefixes, drops duplicates, and sorts them
by epoch, tick, then the full key. Canonical keys store those segments as
zero-padded decimals, so that string order is numeric order. A listing failure
returns no keys: a partial page is not reported as the complete set.
*/
func (client *Client) ListKeys(prefixes ...string) []string {
	return client.ListKeysProgress(nil, prefixes...)
}

/*
ListKeysProgress is ListKeys. onPage receives how many distinct keys are known
after each page, before the final sort.
*/
func (client *Client) ListKeysProgress(onPage func(int), prefixes ...string) []string {
	if client == nil {
		return nil
	}

	keys, err := client.collectKeys(client.Context(), onPage, prefixes...)

	if err != nil {
		return nil
	}

	return keys
}

func (client *Client) collectKeys(ctx context.Context, onPage func(int), prefixes ...string) ([]string, error) {
	if client == nil || client.s3Client == nil {
		return nil, listErr(errnie.Err(
			errnie.Validation, "[store.client] client is nil", nil,
		))
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if len(prefixes) == 0 {
		prefixes = []string{""}
	}

	seen := make(map[string]struct{})
	keys := make([]string, 0)

	for _, prefix := range prefixes {
		for page, err := range client.listObjects(ctx, prefix, "") {
			if err != nil {
				return nil, err
			}

			if page == nil {
				continue
			}

			for _, object := range page.Contents {
				if object.Key == nil {
					continue
				}

				key := *object.Key

				if _, ok := seen[key]; ok {
					continue
				}

				seen[key] = struct{}{}
				keys = append(keys, key)
			}

			if onPage != nil {
				onPage(len(keys))
			}
		}
	}

	sortKeys(keys)
	return keys, nil
}

/*
Read on the shared client does not replay a prefix; use Values(prefix) or Open(prefix).
*/
func (client *Client) Read(p []byte) (int, error) {
	_ = p
	return 0, io.EOF
}

/*
Write uploads the serialized Value in p to S3 at Key = val.Prefix.
Buffers chunks if p is streamed partially.
*/
func (client *Client) Write(p []byte) (n int, err error) {
	val := data.From(p)

	_, err = client.uploader.Upload(client.Context(), &s3.PutObjectInput{
		Bucket: aws.String(client.bucket),
		Key:    aws.String(val.Prefix),
		Body:   val,
	})

	if err != nil {
		return 0, errnie.Error(errnie.Err(
			errnie.IO, "[store.client] upload error", err,
		))
	}

	return len(p), nil
}

/*
Put uploads body at key exactly. Write derives its key from the Value, which
recomputes Prefix and cannot name a grammar path.
*/
func (client *Client) Put(key string, body []byte) error {
	if client == nil || client.uploader == nil || key == "" {
		return errnie.Error(errnie.Err(
			errnie.Validation, "[store.client] object key required", nil,
		))
	}

	_, err := client.uploader.Upload(client.Context(), &s3.PutObjectInput{
		Bucket: aws.String(client.bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader(body),
	})

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO, "[store.client] upload error", err,
		))
	}

	return nil
}

/*
DeletePrefix deletes all objects under the specified prefix.
*/
func (client *Client) DeletePrefix(prefix string) error {
	if client == nil || client.s3Client == nil {
		return nil
	}

	keys, err := client.collectKeys(client.Context(), nil, prefix)

	if err != nil {
		return err
	}

	for _, key := range keys {
		_, err := client.s3Client.DeleteObject(client.Context(), &s3.DeleteObjectInput{
			Bucket: aws.String(client.bucket),
			Key:    aws.String(key),
		})
		if err != nil {
			return errnie.Error(errnie.Err(
				errnie.IO, "[store.client] delete object error", err,
			))
		}
	}
	return nil
}

/*
MovePrefix copies all objects matching sourcePrefix to destPrefix, then deletes the originals.
*/
func (client *Client) MovePrefix(sourcePrefix, destPrefix string) error {
	if client == nil || client.s3Client == nil {
		return nil
	}

	keys, err := client.collectKeys(client.Context(), nil, sourcePrefix)

	if err != nil {
		return err
	}

	for _, key := range keys {
		relKey := strings.TrimPrefix(key, sourcePrefix)
		destKey := strings.TrimSuffix(destPrefix, "/") + "/" + strings.TrimPrefix(relKey, "/")

		copySource := client.bucket + "/" + key
		_, err := client.s3Client.CopyObject(client.Context(), &s3.CopyObjectInput{
			Bucket:     aws.String(client.bucket),
			Key:        aws.String(destKey),
			CopySource: aws.String(copySource),
		})
		if err != nil {
			return errnie.Error(errnie.Err(
				errnie.IO, "[store.client] copy object error", err,
			))
		}

		_, err = client.s3Client.DeleteObject(client.Context(), &s3.DeleteObjectInput{
			Bucket: aws.String(client.bucket),
			Key:    aws.String(key),
		})
		if err != nil {
			return errnie.Error(errnie.Err(
				errnie.IO, "[store.client] delete source object error", err,
			))
		}
	}
	return nil
}
