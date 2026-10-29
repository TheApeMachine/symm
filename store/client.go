package store

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/symm/system"
	"golang.design/x/lockfree/lf"
)

type Record struct {
	Key   string
	Value []byte
}

type Client struct {
	ctx      context.Context
	cancel   context.CancelFunc
	err      error
	S3Client *s3.Client
	bucket   string
	transfer *transfermanager.Client
	queue    *lf.Queue[Record]
	closing  atomic.Bool
	stopped  chan struct{}
}

var (
	liveClients []*Client
)

func NewClient(ctx context.Context, bucket string) *Client {
	ctx, cancel := context.WithCancel(ctx)

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

		cancel()
		return nil
	}

	client := s3.NewFromConfig(cfg, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(system.Cfg.Storage.S3.Endpoint)
		options.UsePathStyle = true
		options.Region = system.Cfg.Storage.S3.Region
		options.Credentials = credentials.NewStaticCredentialsProvider(
			"anonymous", "anonymous", "",
		)
		// SeaweedFS has no response checksum header; keep validation off and
		// silence the SDK's per-object debug skip line.
		options.ResponseChecksumValidation = aws.ResponseChecksumValidationWhenRequired
		options.DisableLogOutputChecksumValidationSkipped = true
	})

	_, err = client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucket),
	})

	if err != nil && !strings.Contains(err.Error(), "BucketAlready") {
		errnie.Error(errnie.Err(
			errnie.IO, "[store.client] create bucket error", err,
		))

		cancel()
		return nil
	}

	storeClient := &Client{
		ctx:      ctx,
		cancel:   cancel,
		S3Client: client,
		bucket:   bucket,
		transfer: transfermanager.New(client),
		queue:    lf.NewQueue[Record](),
		stopped:  make(chan struct{}),
	}

	registerClient(storeClient)
	go storeClient.Run()

	return storeClient
}

func registerClient(client *Client) {
	if client == nil {
		return
	}

	liveClients = append(liveClients, client)
}

/*
FlushAll writes every queued object before a restart. The upload loop shares
the process lifetime, so a return from Run would otherwise drop the tail.
*/
func FlushAll() {
	clients := append([]*Client(nil), liveClients...)

	for _, client := range clients {
		if client == nil {
			continue
		}

		client.closing.Store(true)
	}

	for _, client := range clients {
		if client == nil || client.stopped == nil {
			continue
		}

		select {
		case <-client.stopped:
		case <-time.After(45 * time.Second):
			errnie.Warn("[store] timed out waiting to flush uploads")
		}

		client.Flush()
	}
}

func (client *Client) Run() {
	defer close(client.stopped)
	defer client.Flush()

	for {
		if client.ctx.Err() != nil || client.closing.Load() {
			return
		}

		record, ok := client.queue.Dequeue()

		if !ok {
			time.Sleep(10 * time.Millisecond)
			continue
		}

		client.writeFromQueue(client.ctx, record.Key, record.Value)
	}
}

func (client *Client) Flush() {
	if client == nil || client.queue == nil {
		return
	}

	client.closing.Store(true)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for {
		record, ok := client.queue.Dequeue()
		if !ok {
			return
		}

		client.upload(ctx, record.Key, record.Value)
	}
}

func missingObject(err error) bool {
	if err == nil {
		return false
	}

	var api smithy.APIError
	if errors.As(err, &api) && api.ErrorCode() == "NoSuchKey" {
		return true
	}

	var respErr *smithyhttp.ResponseError
	return errors.As(err, &respErr) && respErr.HTTPStatusCode() == 404
}

/*
List yields every page of objects under prefix. It follows continuation
tokens, so a prefix longer than one page is complete. A listing failure
yields nothing: a partial page is not reported as the complete set.
*/
func (client *Client) List(
	prefix string,
) iter.Seq[*s3.ListObjectsV2Output] {
	return func(yield func(*s3.ListObjectsV2Output) bool) {
		if client == nil {
			return
		}

		pages, err := client.listPages(prefix)
		if err != nil {
			errnie.Error(errnie.Err(
				errnie.IO, "[store.client] list objects error", err,
			))

			return
		}

		for i := range pages {
			if !yield(&pages[i]) {
				return
			}
		}
	}
}

/*
listPages reads every page before the caller sees one. A later page that
fails discards the earlier pages instead of pretending the prefix ended.
*/
func (client *Client) listPages(prefix string) ([]s3.ListObjectsV2Output, error) {
	var token *string
	pages := make([]s3.ListObjectsV2Output, 0, 1)

	for {
		page, err := client.S3Client.ListObjectsV2(client.ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(client.bucket),
			Prefix:            aws.String(prefix),
			ContinuationToken: token,
		})
		if err != nil {
			return nil, err
		}

		if page == nil {
			return pages, nil
		}

		pages = append(pages, *page)

		if page.IsTruncated == nil || !*page.IsTruncated || page.NextContinuationToken == nil {
			return pages, nil
		}

		if token != nil && *token == *page.NextContinuationToken {
			return nil, fmt.Errorf("list continuation for %s did not advance", prefix)
		}

		token = page.NextContinuationToken
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

			_, err := client.transfer.DownloadObject(client.ctx, &transfermanager.DownloadObjectInput{
				Bucket:   aws.String(client.bucket),
				Key:      aws.String(*object.Key),
				WriterAt: writer,
			})

			if err != nil {
				return 0, errnie.Error(errnie.Err(
					errnie.IO, "[store.client] download error", err,
				))
			}

			n, err = value.Write(writer.Bytes())

			if err != nil {
				return n, errnie.Error(err)
			}
		}
	}

	return len(p), io.EOF
}

func (client *Client) Download(ctx context.Context, key string) *data.Value {
	body := manager.NewWriteAtBuffer(make([]byte, 0, 4096))

	_, err := client.transfer.DownloadObject(ctx, &transfermanager.DownloadObjectInput{
		Bucket:   aws.String(client.bucket),
		Key:      aws.String(key),
		WriterAt: body,
	})

	if err != nil {
		if !missingObject(err) {
			errnie.Error(errnie.Err(
				errnie.IO, "[store.client] download error", err,
			))
		}

		return nil
	}

	return data.From(body.Bytes())
}

/*
Stream writes each object under prefix to dst, one Write per object.
A canceled ctx stops the stream before the next object.
*/
func (client *Client) Stream(ctx context.Context, prefix string) iter.Seq[*data.Value] {
	return func(yield func(*data.Value) bool) {
		for key := range client.ListKeys(prefix) {
			if err := ctx.Err(); err != nil {
				errnie.Error(errnie.Err(
					errnie.IO, "[store.client] context canceled", err,
				))

				return
			}

			body := manager.NewWriteAtBuffer(make([]byte, 0, 4096))

			_, err := client.transfer.DownloadObject(ctx, &transfermanager.DownloadObjectInput{
				Bucket:   aws.String(client.bucket),
				Key:      aws.String(key),
				WriterAt: body,
			})

			if err != nil {
				if missingObject(err) {
					continue
				}

				errnie.Error(errnie.Err(
					errnie.IO, "[store.client] download error", err,
				))

				return
			}

			if !yield(data.From(body.Bytes())) {
				return
			}
		}
	}
}

/*
Write is unsupported because serialized Values do not carry an upload key prefix.
Use WriteAtPrefix or io.Copy with a *data.Value (which routes through WriteAtPrefix).
*/
func (client *Client) Write(p []byte) (int, error) {
	return client.WriteAtPrefix(data.From(p).Prefix, p)
}

func (client *Client) WriteAtPrefix(key string, value []byte) (int, error) {
	if key == "" {
		return client.writeFromQueue(client.ctx, "", value)
	}

	if client.closing.Load() {
		ctx := client.ctx
		if ctx == nil || ctx.Err() != nil {
			ctx = context.Background()
		}

		return client.writeFromQueue(ctx, key, value)
	}

	if client.queue == nil {
		return 0, errnie.Error(errnie.Err(
			errnie.IO, "[store.client] upload queue is not ready", nil,
		))
	}

	client.queue.Enqueue(Record{Key: key, Value: value})
	return len(value), nil
}

/*
WriteAtPrefix uploads p at prefix. An empty prefix is refused.
*/
func (client *Client) writeFromQueue(ctx context.Context, prefix string, p []byte) (int, error) {
	if ctx != nil && ctx.Err() != nil {
		client.queue.Enqueue(Record{Key: prefix, Value: p})
		return len(p), nil
	}

	return client.upload(ctx, prefix, p)
}

func (client *Client) upload(ctx context.Context, prefix string, p []byte) (int, error) {
	if prefix == "" {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation, "[store.client] upload key is empty", nil,
		))
	}

	if ctx == nil {
		ctx = context.Background()
	}

	_, err := client.transfer.UploadObject(ctx, &transfermanager.UploadObjectInput{
		Bucket: aws.String(client.bucket),
		Key:    aws.String(prefix),
		Body:   bytes.NewReader(p),
	})

	if err != nil {
		return 0, errnie.Error(errnie.Err(
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

			_, err := client.S3Client.DeleteObject(client.ctx, &s3.DeleteObjectInput{
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
DeleteOtherKeys deletes every object under prefix except keep.
keep may be empty to delete the whole prefix.
*/
func (client *Client) DeleteOtherKeys(prefix, keep string) error {
	if client == nil || client.S3Client == nil || prefix == "" {
		return nil
	}

	pages, err := client.listPages(prefix)
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO, "[store.client] list objects error", err,
		))
	}

	for _, page := range pages {
		for _, object := range page.Contents {
			if object.Key == nil || *object.Key == keep {
				continue
			}

			_, err := client.S3Client.DeleteObject(client.ctx, &s3.DeleteObjectInput{
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
An object keeps the path it holds under sourcePrefix, so "position/open/BTC" moved to
"position/closed/BTC" lands on "position/closed/BTC/<epoch>/<tick>".
*/
func (client *Client) MovePrefix(sourcePrefix, destPrefix string) error {
	for page := range client.List(sourcePrefix) {
		for _, object := range page.Contents {
			if object.Key == nil {
				continue
			}

			dest := destKey(destPrefix, *object.Key, sourcePrefix)

			if dest == *object.Key {
				continue
			}

			_, err := client.S3Client.CopyObject(client.ctx, &s3.CopyObjectInput{
				Bucket:     aws.String(client.bucket),
				Key:        aws.String(dest),
				CopySource: aws.String(client.bucket + "/" + *object.Key),
			})

			if err != nil {
				return errnie.Error(errnie.Err(
					errnie.IO, "[store.client] copy object error", err,
				))
			}

			_, err = client.S3Client.DeleteObject(client.ctx, &s3.DeleteObjectInput{
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
destKey is where an object lands when its prefix moves: the path the object holds under the
source prefix is kept under the destination prefix.
*/
func destKey(destPrefix, key, sourcePrefix string) string {
	rel := strings.TrimPrefix(key, sourcePrefix)

	if rel == "" {
		return destPrefix
	}

	return strings.TrimSuffix(destPrefix, "/") + "/" + strings.TrimPrefix(rel, "/")
}

func (client *Client) Error() error {
	return client.err
}

func (client *Client) Close() error {
	client.cancel()
	return nil
}
