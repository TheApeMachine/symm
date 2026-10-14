package store

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"iter"
	"net/http"
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
				transport.ResponseHeaderTimeout = 1 * time.Second
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

func (client *Client) List(prefix string) iter.Seq[*s3.ListObjectsV2Output] {
	paginator := s3.NewListObjectsV2Paginator(client.s3Client, &s3.ListObjectsV2Input{
		Bucket: aws.String(client.bucket),
		Prefix: aws.String(prefix),
	})

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

func (client *Client) dowload(key string) []byte {
	writer := manager.NewWriteAtBuffer(make([]byte, 0, 4096))

	_, err := client.downloader.Download(client.Context(), writer, &s3.GetObjectInput{
		Bucket: aws.String(client.bucket),
		Key:    aws.String(key),
	})

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.IO, "[store.client] download error", err,
		))

		return nil
	}

	return writer.Bytes()
}

func (client *Client) Get(prefix string) iter.Seq[*data.Measurement] {
	return func(yield func(*data.Measurement) bool) {
		for page := range client.List(prefix) {
			for _, object := range page.Contents {
				payload := client.dowload(*object.Key)

				if payload == nil {
					continue
				}

				measurement := &data.Measurement{}

				if err := measurement.UnmarshalBinary(payload); err != nil {
					errnie.Error(errnie.Err(
						errnie.IO, "[store.client] unmarshal error", err,
					))

					continue
				}

				if !yield(measurement) {
					return
				}
			}
		}
	}
}

func (client *Client) GetBytes(prefix string) iter.Seq[[]byte] {
	return func(yield func([]byte) bool) {
		for page := range client.List(prefix) {
			for _, object := range page.Contents {
				payload := client.dowload(*object.Key)

				if payload == nil {
					continue
				}

				if !yield(payload) {
					return
				}
			}
		}
	}
}

func (client *Client) Put(measurement *data.Measurement) error {
	if client == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation, "[store.client] client is nil", nil,
		))
	}

	if measurement == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation, "[store.client] measurement is nil", nil,
		))
	}

	payload, err := measurement.MarshalBinary()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation, "[store.client] failed to marshal binary", err,
		))
	}

	key := fmt.Sprintf("%d/%s/%s/%d.bin", measurement.Epoch, measurement.Label, measurement.Source, measurement.SeqIdx)

	_, err = client.uploader.Upload(client.Context(), &s3.PutObjectInput{
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
