package store

import (
	"bytes"
	"context"
	"errors"
	"iter"
	"net/http"
	"net/url"
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
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/system"
)

var (
	ErrBlobMissing             = errors.New("blob missing")
	ErrBlobStorageUnconfigured = errors.New("blob storage unconfigured")
)

type Client struct {
	*runtime.System
	s3Client   *s3.Client
	bucket     string
	downloader *manager.Downloader
	uploader   *manager.Uploader
}

func NewClient(ctx context.Context, bucket string, optFns ...func(*s3.Options)) *Client {
	cfg, err := config.LoadDefaultConfig(context.TODO())

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.IO, "[store.client] error", err,
		))

		return nil
	}

	if system.Cfg == nil || system.Cfg.Storage == nil || system.Cfg.Storage.S3 == nil {
		system.Cfg = system.NewConfig()
	}

	s3Cfg := system.Cfg.Storage.S3

	if bucket == "" {
		bucket = s3Cfg.Bucket
	}

	options := append([]func(*s3.Options){
		func(options *s3.Options) {
			if options.BaseEndpoint == nil && s3Cfg.Endpoint != "" {
				options.BaseEndpoint = aws.String(s3Cfg.Endpoint)
				options.UsePathStyle = true
			}

			if options.Region == "" {
				options.Region = s3Cfg.Region
			}

			if s3Cfg.Anonymous || s3Cfg.AccessKeyID == "" {
				options.Credentials = credentials.NewStaticCredentialsProvider("anonymous", "anonymous", "")
			}

			// Override Default Transport Values
			options.HTTPClient = awshttp.NewBuildableClient().WithTransportOptions(func(transport *http.Transport) {
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

	return &Client{
		System:     runtime.NewSystem(ctx, "store.client"),
		s3Client:   client,
		bucket:     bucket,
		downloader: manager.NewDownloader(client),
		uploader:   manager.NewUploader(client),
	}
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
Get retrieves measurements matching the given prefix(es) in strict chronological
order by (Epoch, Tick, SeqIdx), downloading and unmarshaling one measurement at a time.
*/
func (client *Client) Get(prefixes ...string) iter.Seq[*data.Measurement] {
	return func(yield func(*data.Measurement) bool) {
		for _, key := range client.ListKeys(prefixes...) {
			payload, err := client.Download(key)

			if err != nil || payload == nil {
				continue
			}

			measurement := &data.Measurement{}
			err = measurement.UnmarshalBinary(payload)

			if err != nil {
				continue
			}

			if !yield(measurement) {
				return
			}
		}
	}
}

/*
GetBytes retrieves raw payloads matching the given prefix(es) in strict chronological
order by (Epoch, Tick, SeqIdx).
*/
func (client *Client) GetBytes(prefixes ...string) iter.Seq[[]byte] {
	return func(yield func([]byte) bool) {
		for _, key := range client.ListKeys(prefixes...) {
			payload, err := client.Download(key)

			if err != nil || payload == nil {
				continue
			}

			if !yield(payload) {
				return
			}
		}
	}
}

func (client *Client) Put(key string, payload []byte) error {
	if client == nil || client.uploader == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation, "[store.client] client or uploader is nil", nil,
		))
	}

	if key == "" {
		return errnie.Error(errnie.Err(
			errnie.Validation, "[store.client] key is empty", nil,
		))
	}

	_, err := client.uploader.Upload(client.Context(), &s3.PutObjectInput{
		Bucket: aws.String(client.bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader(payload),
	})

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO, "[store.client] upload error", err,
		))
	}

	return nil
}

/*
Delete removes the specified object keys from the S3 bucket.
*/
func (client *Client) Delete(keys ...string) error {
	if client == nil || client.s3Client == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation, "[store.client] client is nil", nil,
		))
	}

	for _, key := range keys {
		if key == "" {
			continue
		}

		_, err := client.s3Client.DeleteObject(client.Context(), &s3.DeleteObjectInput{
			Bucket: aws.String(client.bucket),
			Key:    aws.String(key),
		})

		if err != nil {
			return errnie.Error(errnie.Err(
				errnie.IO, "[store.client] delete error", err,
			))
		}
	}

	return nil
}

/*
DeletePrefix removes all object keys matching the given prefix from the S3 bucket.
*/
func (client *Client) DeletePrefix(prefix string) error {
	keys := client.ListKeys(prefix)

	if len(keys) == 0 {
		return nil
	}

	return client.Delete(keys...)
}

/*
Copy duplicates an object within the store from srcKey to dstKey using server-side S3 CopyObject.
*/
func (client *Client) Copy(srcKey, dstKey string) error {
	if client == nil || client.s3Client == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation, "[store.client] client is nil", nil,
		))
	}

	_, err := client.s3Client.CopyObject(client.Context(), &s3.CopyObjectInput{
		Bucket:     aws.String(client.bucket),
		CopySource: aws.String(url.PathEscape(client.bucket + "/" + srcKey)),
		Key:        aws.String(dstKey),
	})

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO, "[store.client] copy error", err,
		))
	}

	return nil
}

/*
MovePrefix server-side moves all object keys matching srcPrefix to dstPrefix.
*/
func (client *Client) MovePrefix(srcPrefix, dstPrefix string) error {
	keys := client.ListKeys(srcPrefix)

	if len(keys) == 0 {
		return nil
	}

	for _, key := range keys {
		rel := strings.TrimPrefix(key, srcPrefix)
		dstKey := dstPrefix + rel

		if err := client.Copy(key, dstKey); err != nil {
			return err
		}

		_ = client.Delete(key)
	}

	return nil
}

