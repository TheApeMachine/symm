package store

import (
	"context"
	"errors"
	"io"
	"iter"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/system"
)

var (
	ErrBlobMissing             = errors.New("blob missing")
	ErrBlobStorageUnconfigured = errors.New("blob storage unconfigured")
)

type Client struct {
	*runtime.System
	s3Client       *s3.Client
	bucket         string
	key            string
	downloader     *manager.Downloader
	uploader       *manager.Uploader
	prefix         string
	keys           []string
	cursor         int
	currentPayload []byte
	payloadOffset  int
}

var _ io.ReadWriteCloser = (*Client)(nil)

func NewClient(ctx context.Context, bucket string, optFns ...func(*s3.Options)) *Client {
	cfg, err := config.LoadDefaultConfig(context.TODO())

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.IO, "[store.client] error", err,
		))

		return nil
	}

	options := append([]func(*s3.Options){
		func(options *s3.Options) {
			if options.BaseEndpoint == nil && system.Cfg.Storage.S3.Endpoint != "" {
				options.BaseEndpoint = aws.String(system.Cfg.Storage.S3.Endpoint)
				options.UsePathStyle = true
			}

			if options.Region == "" {
				options.Region = system.Cfg.Storage.S3.Region
			}

			if system.Cfg.Storage.S3.Anonymous || system.Cfg.Storage.S3.AccessKeyID == "" {
				options.Credentials = credentials.NewStaticCredentialsProvider(
					"anonymous", "anonymous", "",
				)
			}

			// Override Default Transport Values
			options.HTTPClient = awshttp.NewBuildableClient().WithTransportOptions(func(
				transport *http.Transport,
			) {
				transport.ResponseHeaderTimeout = 30 * time.Second
				transport.MaxIdleConns = 256
				transport.MaxIdleConnsPerHost = 64
				transport.WriteBufferSize = 1024 * 1024
				transport.ReadBufferSize = 1024 * 1024
			})
		},
	}, optFns...)

	client := s3.NewFromConfig(cfg, options...)

	_, err = client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucket),
	})

	if err != nil && !strings.Contains(err.Error(), "BucketAlready") {
		errnie.Error(errnie.Err(
			errnie.IO, "[store.client] create bucket error", err,
		))

		return nil
	}

	store := &Client{
		System:     runtime.NewSystem(ctx, "store.client"),
		s3Client:   client,
		bucket:     bucket,
		downloader: manager.NewDownloader(client),
		uploader:   manager.NewUploader(client),
	}

	// The bucket exists and the client is configured: it can take writes.
	store.Transition(runtime.READY)
	return store
}

func (client *Client) List(prefix string, delimiter ...string) iter.Seq[*s3.ListObjectsV2Output] {
	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(client.bucket),
		Prefix: aws.String(prefix),
	}

	if len(delimiter) > 0 && delimiter[0] != "" {
		input.Delimiter = aws.String(delimiter[0])
	}

	paginator := s3.NewListObjectsV2Paginator(client.s3Client, input)

	return func(yield func(*s3.ListObjectsV2Output) bool) {
		for paginator.HasMorePages() {
			page, err := paginator.NextPage(client.Context())

			if err != nil {
				errnie.Error(errnie.Err(
					errnie.IO, "[store.client] listing error", err,
				))

				return
			}

			if !yield(page) {
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

func parseKeyOrder(key string) (epoch, tick, seqIdx, rest string) {
	parts := strings.Split(key, "/")

	if len(parts) >= 4 {
		return parts[len(parts)-4], parts[len(parts)-3], parts[len(parts)-2], parts[len(parts)-1]
	}

	return "", "", "", key
}

func sortKeys(keys []string) {
	slices.SortFunc(keys, func(firstKey, secondKey string) int {
		epochA, tickA, seqA, restA := parseKeyOrder(firstKey)
		epochB, tickB, seqB, restB := parseKeyOrder(secondKey)

		if epochA != epochB {
			return strings.Compare(epochA, epochB)
		}

		if tickA != tickB {
			return strings.Compare(tickA, tickB)
		}

		if seqA != seqB {
			return strings.Compare(seqA, seqB)
		}

		return strings.Compare(restA, restB)
	})
}

/*
ListKeys collects all object keys matching the given prefix(es) and sorts them
chronologically by (Epoch, Tick, SeqIdx).
*/
func (client *Client) ListKeys(prefixes ...string) []string {
	if len(prefixes) == 0 {
		prefixes = []string{""}
	}

	keys := make([]string, 0)
	seen := make(map[string]bool)

	for _, prefix := range prefixes {
		for page := range client.List(prefix) {
			for _, object := range page.Contents {
				if object.Key != nil && !seen[*object.Key] {
					seen[*object.Key] = true
					keys = append(keys, *object.Key)
				}
			}
		}
	}

	sortKeys(keys)
	return keys
}

/*
Values returns an iterator yielding each Value found under prefixes.
*/
func Values(client *Client, prefixes ...string) iter.Seq[*data.Value] {
	return func(yield func(*data.Value) bool) {
		if client == nil {
			return
		}

		for _, key := range client.ListKeys(prefixes...) {
			payload, err := client.Download(key)
			if err != nil {
				continue
			}

			val := data.From(payload)
			if val == nil {
				continue
			}

			if !yield(val) {
				return
			}
		}
	}
}

/*
Read downloads the next chronological Value from S3 into p,
returning io.EOF when completed.
*/
func (client *Client) Read(p []byte) (n int, err error) {
	if client.currentPayload == nil || client.payloadOffset >= len(client.currentPayload) {
		if client.keys == nil {
			client.keys = client.ListKeys(client.prefix)
			client.cursor = 0
		}

		if client.cursor >= len(client.keys) {
			return 0, io.EOF
		}

		key := client.keys[client.cursor]
		client.cursor++

		payload, err := client.Download(key)
		if err != nil {
			return 0, err
		}

		client.currentPayload = payload
		client.payloadOffset = 0
	}

	n = copy(p, client.currentPayload[client.payloadOffset:])
	client.payloadOffset += n

	if client.payloadOffset >= len(client.currentPayload) && client.cursor >= len(client.keys) {
		return n, io.EOF
	}

	return n, nil
}

/*
ReadFrom reconstructs and uploads a Value directly from an incoming stream r.
Implements io.ReaderFrom for zero-intermediate streaming via io.Copy.
*/
func (client *Client) ReadFrom(r io.Reader) (int64, error) {
	val := data.New()
	n, err := val.ReadFrom(r)
	if err != nil && err != io.EOF {
		return n, err
	}

	_, uploadErr := client.uploader.Upload(client.Context(), &s3.PutObjectInput{
		Bucket: aws.String(client.bucket),
		Key:    aws.String(val.Prefix),
		Body:   val,
	})
	if uploadErr != nil {
		client.uploadFailed(val.Prefix, uploadErr)
		return n, uploadErr
	}

	return n, nil
}

/*
uploadFailed raises a failed upload of key. An upload cut off by an orderly
shutdown is reported at info with its key, so the lost tick stays traceable
without posing as a store failure.
*/
func (client *Client) uploadFailed(key string, err error) {
	if runtime.ShuttingDown(client.Context(), err) {
		errnie.Info("[store.client] upload of " + key + " canceled by shutdown")
		return
	}

	errnie.Error(errnie.Err(errnie.IO, "[store.client] upload of "+key+" failed", err))
}

/*
Write uploads the serialized Value in p to S3 at Key = val.Prefix.
p must be a freshly serialized Value (io.Copy(store, value) hands over
value.Bytes()); its layers alias p, which the async upload keeps using.
*/
func (client *Client) Write(p []byte) (n int, err error) {
	val := data.From(p)

	go func() {
		if _, err := client.uploader.Upload(client.Context(), &s3.PutObjectInput{
			Bucket: aws.String(client.bucket),
			Key:    aws.String(val.Prefix),
			Body:   val,
		}); err != nil {
			client.uploadFailed(val.Prefix, err)
		}
	}()

	return len(p), nil
}

/*
DeletePrefix deletes all objects under the specified prefix.
*/
func (client *Client) DeletePrefix(prefix string) error {
	if client == nil || client.s3Client == nil {
		return errnie.Error(errnie.Err(errnie.Validation, "[store.client] no S3 client", nil))
	}

	keys := client.ListKeys(prefix)
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
		return errnie.Error(errnie.Err(errnie.Validation, "[store.client] no S3 client", nil))
	}

	keys := client.ListKeys(sourcePrefix)
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
