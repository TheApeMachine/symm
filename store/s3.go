/*
Package store is SYMM's object storage for whole-blob artifacts.

Hindsight's record families live in Iceberg tables, not here. What remains is
the small set of artifacts that are genuinely opaque blobs rather than rows —
the associative model's encoded state above all — so this package deliberately
offers only whole-object reads and writes.
*/
package store

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/pool"
)

/*
S3 is a connection to the object store.

Writes go through an elastic worker pool so a caller on a hot path is never
blocked on the network; reads are synchronous, because every caller of Read
needs the bytes before it can continue.
*/
type S3 struct {
	client *s3.Client
	bucket string
	pool   *pool.Pool[func()]
}

// NewS3 opens the object store described by the storage.s3 configuration.
func NewS3(ctx context.Context) (*S3, error) {
	httpClient := awshttp.NewBuildableClient().
		WithTransportOptions(func(t *http.Transport) {
			t.MaxIdleConns = viper.GetInt("storage.s3.max_idle_conns")
			t.MaxIdleConnsPerHost = viper.GetInt("storage.s3.max_idle_conns_per_host")
			t.MaxConnsPerHost = viper.GetInt("storage.s3.max_conns_per_host")
		})

	options := []func(*config.LoadOptions) error{
		config.WithRegion(viper.GetString("storage.s3.region")),
		config.WithHTTPClient(httpClient),
	}

	// An unauthenticated cluster has no credentials to resolve. Left to the
	// default chain the SDK signs with whatever it finds, or fails looking, so
	// anonymity is stated rather than discovered.
	if viper.GetBool("storage.s3.anonymous") {
		options = append(options, config.WithCredentialsProvider(aws.AnonymousCredentials{}))
	}

	cfg, err := config.LoadDefaultConfig(ctx, options...)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.BadGateway,
			"[s3] failed to load AWS config",
			err,
		))
	}

	bucket := viper.GetString("storage.s3.bucket")

	if bucket == "" {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"[s3] storage.s3.bucket is required",
			nil,
		))
	}

	store := &S3{
		client: s3.NewFromConfig(cfg, func(o *s3.Options) {
			if endpoint := viper.GetString("storage.s3.endpoint"); endpoint != "" {
				o.BaseEndpoint = aws.String(endpoint)
			}

			o.UsePathStyle = true
		}),
		bucket: bucket,
		pool:   pool.New(func(task func()) { task() }),
	}

	store.pool.Start()

	return store, nil
}

/*
Read returns one object whole.

A missing object is not an error: it reports found=false, because every caller
here is asking whether a previously saved artifact exists at all.
*/
func (store *S3) Read(ctx context.Context, key string) (data []byte, found bool, err error) {
	out, err := store.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &store.bucket,
		Key:    &key,
	})

	if err != nil {
		var missing *types.NoSuchKey

		if errors.As(err, &missing) {
			return nil, false, nil
		}

		return nil, false, errnie.Error(errnie.Err(
			errnie.BadGateway,
			"[s3] failed to read "+key,
			err,
		))
	}

	defer out.Body.Close()

	// ContentLength is authoritative, so the read lands in one allocation
	// rather than io.ReadAll's doubling.
	body := bytes.NewBuffer(make([]byte, 0, aws.ToInt64(out.ContentLength)))

	if _, err := body.ReadFrom(out.Body); err != nil {
		return nil, false, errnie.Error(errnie.Err(
			errnie.BadGateway,
			"[s3] failed to read body of "+key,
			err,
		))
	}

	return body.Bytes(), true, nil
}

// Write stores one object whole, waiting for the venue to acknowledge it.
func (store *S3) Write(ctx context.Context, key string, data []byte) error {
	if _, err := store.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: &store.bucket,
		Key:    &key,
		Body:   bytes.NewReader(data),
	}); err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadGateway,
			"[s3] failed to write "+key,
			err,
		))
	}

	return nil
}

/*
Enqueue stores one object without waiting for it, for callers that must not
block. Failures are reported through errnie rather than returned, so this is
only appropriate where losing the write would not corrupt anything.
*/
func (store *S3) Enqueue(ctx context.Context, key string, body io.Reader) {
	data, err := io.ReadAll(body)

	if err != nil {
		errnie.Error(errnie.Err(errnie.IO, "[s3] failed to buffer "+key, err))

		return
	}

	if err := store.pool.AddTask(func() {
		errnie.Error(store.Write(ctx, key, data))
	}); err != nil {
		errnie.Error(err)
	}
}

// Close retires the write pool's workers.
func (store *S3) Close() error {
	store.pool.StopAndWait()

	return nil
}
