package store

import (
	"context"
	"errors"
	"io"
	"iter"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
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
	uploads        sync.WaitGroup
	failedMu       sync.Mutex
	failed         []error
}

var _ io.ReadWriteCloser = (*Client)(nil)

func NewClient(ctx context.Context, bucket string, optFns ...func(*s3.Options)) *Client {
	client, err := newS3(optFns...)

	if err != nil {
		errnie.Error(err)
		return nil
	}

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

/*
List pages the objects under prefix. A page that fails to list is yielded as
an error naming the prefix and ends the sequence.
*/
func (client *Client) List(prefix string, delimiter ...string) iter.Seq2[*s3.ListObjectsV2Output, error] {
	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(client.bucket),
		Prefix: aws.String(prefix),
	}

	if len(delimiter) > 0 && delimiter[0] != "" {
		input.Delimiter = aws.String(delimiter[0])
	}

	paginator := s3.NewListObjectsV2Paginator(client.s3Client, input)

	return func(yield func(*s3.ListObjectsV2Output, error) bool) {
		for paginator.HasMorePages() {
			page, err := paginator.NextPage(client.Context())

			if err != nil {
				yield(nil, errnie.Err(errnie.IO, "[store.client] listing "+prefix, err))
				return
			}

			if !yield(page, nil) {
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
IsNotFound reports whether err (from Download) is S3 answering that the key
does not exist, as opposed to a transport, auth or decode failure.
*/
func IsNotFound(err error) bool {
	var noSuchKey *types.NoSuchKey

	if errors.As(err, &noSuchKey) {
		return true
	}

	var api smithy.APIError

	return errors.As(err, &api) && (api.ErrorCode() == "NoSuchKey" || api.ErrorCode() == "NotFound")
}

/*
sortKeys orders keys chronologically by the epoch and tick segments every
key ends with (data.Value.FormatPrefix: .../epoch%019d/tick%019d.bin), then
by key. Zero-padded segments compare correctly as strings.
*/
func sortKeys(keys []string) {
	order := func(key string) (string, string) {
		rest, tick, _ := cutLast(key)
		_, epoch, _ := cutLast(rest)
		return epoch, tick
	}

	slices.SortFunc(keys, func(firstKey, secondKey string) int {
		epochA, tickA := order(firstKey)
		epochB, tickB := order(secondKey)

		if epochA != epochB {
			return strings.Compare(epochA, epochB)
		}

		if tickA != tickB {
			return strings.Compare(tickA, tickB)
		}

		return strings.Compare(firstKey, secondKey)
	})
}

func cutLast(key string) (string, string, bool) {
	idx := strings.LastIndex(key, "/")

	if idx < 0 {
		return "", key, false
	}

	return key[:idx], key[idx+1:], true
}

/*
Epochs lists the epochs stored under prefix (a data.Prefix without epoch),
oldest first. A listed entry without a %019d epoch segment is an error: it
was not written through data.Value.FormatPrefix.
*/
func Epochs(client *Client, prefix string) ([]uint64, error) {
	if client == nil {
		return nil, errnie.Err(errnie.Validation, "[store.client] no client to list epochs under "+prefix, nil)
	}

	epochs := make([]uint64, 0)

	for page, err := range client.List(prefix, "/") {
		if err != nil {
			return nil, err
		}

		for _, common := range page.CommonPrefixes {
			if common.Prefix == nil {
				continue
			}

			epoch, err := data.KeyEpoch(prefix, *common.Prefix)

			if err != nil {
				return nil, err
			}

			epochs = append(epochs, epoch)
		}
	}

	return sortedEpochs(epochs), nil
}

/*
ListKeys collects all object keys matching the given prefix(es) and sorts them
chronologically by (Epoch, Tick, SeqIdx). A listing failure is returned.
*/
func (client *Client) ListKeys(prefixes ...string) ([]string, error) {
	if len(prefixes) == 0 {
		prefixes = []string{""}
	}

	keys := make([]string, 0)
	seen := make(map[string]bool)

	for _, prefix := range prefixes {
		for page, err := range client.List(prefix) {
			if err != nil {
				return nil, err
			}

			for _, object := range page.Contents {
				if object.Key != nil && !seen[*object.Key] {
					seen[*object.Key] = true
					keys = append(keys, *object.Key)
				}
			}
		}
	}

	sortKeys(keys)
	return keys, nil
}

/*
Values yields each Value found under prefixes, in key order. A listing,
download or decode failure is yielded as an error naming the key and ends
the sequence: callers return it, nothing is skipped.
*/
func Values(client *Client, prefixes ...string) iter.Seq2[*data.Value, error] {
	return func(yield func(*data.Value, error) bool) {
		if client == nil {
			yield(nil, errnie.Err(errnie.Validation, "[store.client] no client to read values from", nil))
			return
		}

		keys, err := client.ListKeys(prefixes...)

		if err != nil {
			yield(nil, err)
			return
		}

		for _, key := range keys {
			payload, err := client.Download(key)

			if err != nil {
				yield(nil, errnie.Err(errnie.IO, "[store.client] download "+key, err))
				return
			}

			val := data.New()

			if _, err := val.Write(payload); err != nil {
				yield(nil, errnie.Err(errnie.UnprocessableContent, "[store.client] "+key+" does not decode as a Value", err))
				return
			}

			if !yield(val, nil) {
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
			keys, err := client.ListKeys(client.prefix)

			if err != nil {
				return 0, err
			}

			client.keys = keys
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
	val := data.New()

	if _, err := val.Write(p); err != nil {
		return 0, errnie.Error(errnie.Err(errnie.UnprocessableContent, "[store.client] write does not decode as a Value", err))
	}

	client.uploads.Add(1)

	go func() {
		defer client.uploads.Done()

		if _, err := client.uploader.Upload(client.Context(), &s3.PutObjectInput{
			Bucket: aws.String(client.bucket),
			Key:    aws.String(val.Prefix),
			Body:   val,
		}); err != nil {
			client.uploadFailed(val.Prefix, err)

			client.failedMu.Lock()
			client.failed = append(client.failed, errnie.Err(errnie.IO, "[store.client] upload of "+val.Prefix+" failed", err))
			client.failedMu.Unlock()
		}
	}()

	return len(p), nil
}

/*
Drain waits for every upload Write started and returns the uploads that
failed since the last Drain. A batch command drains before it exits, so
nothing it wrote is lost to process exit and no failure goes unreported.
*/
func (client *Client) Drain() error {
	client.uploads.Wait()

	client.failedMu.Lock()
	defer client.failedMu.Unlock()

	err := errors.Join(client.failed...)
	client.failed = nil

	return err
}

/*
DeletePrefix deletes all objects under the specified prefix.
*/
func (client *Client) DeletePrefix(prefix string) error {
	if client == nil || client.s3Client == nil {
		return errnie.Error(errnie.Err(errnie.Validation, "[store.client] no S3 client", nil))
	}

	keys, err := client.ListKeys(prefix)

	if err != nil {
		return errnie.Error(err)
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
		return errnie.Error(errnie.Err(errnie.Validation, "[store.client] no S3 client", nil))
	}

	keys, err := client.ListKeys(sourcePrefix)

	if err != nil {
		return errnie.Error(err)
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

/*
newS3 is the configured S3 API client every store view shares: endpoint,
region and credentials from system.Cfg.Storage.S3, then optFns.
*/
func newS3(optFns ...func(*s3.Options)) (*s3.Client, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO())

	if err != nil {
		return nil, errnie.Err(errnie.IO, "[store.client] aws config", err)
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

	return s3.NewFromConfig(cfg, options...), nil
}

func sortedEpochs(epochs []uint64) []uint64 {
	slices.Sort(epochs)
	return slices.Compact(epochs)
}
