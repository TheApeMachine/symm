package store

import (
	"bytes"
	"context"
	"iter"
	"net/http"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

var writerPool = sync.Pool{
	New: func() any {
		return manager.NewWriteAtBuffer([]byte{})
	},
}

type Client[T any] struct {
	*runtime.System
	s3Client   *s3.Client
	bucket     string
	downloader *manager.Downloader
	uploader   *manager.Uploader
}

func NewClient[T any](ctx context.Context, bucket string) *Client[T] {
	cfg, err := config.LoadDefaultConfig(context.TODO())

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.IO, "[store.client] error", err,
		))

		return nil
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		// Override Default Transport Values
		o.HTTPClient = awshttp.NewBuildableClient().WithTransportOptions(func(tr *http.Transport) {
			tr.ResponseHeaderTimeout = 1 * time.Second
			tr.WriteBufferSize = 1024 * 1024
			tr.ReadBufferSize = 1024 * 1024
		})
	})

	return &Client[T]{
		System:     runtime.NewSystem(ctx, "store.client"),
		s3Client:   client,
		bucket:     bucket,
		downloader: manager.NewDownloader(client),
		uploader:   manager.NewUploader(client),
	}
}

func (client *Client[T]) Get(prefix string) iter.Seq[T] {
	var zero T

	return func(yield func(T) bool) {
		paginator := s3.NewListObjectsV2Paginator(client.s3Client, &s3.ListObjectsV2Input{
			Bucket: aws.String(client.bucket),
			Prefix: aws.String(prefix),
		})

		for paginator.HasMorePages() {
			page, err := paginator.NextPage(client.Context())

			if err != nil {
				errnie.Error(errnie.Err(
					errnie.IO, "[store.client] error", err,
				))
			}

			for _, object := range page.Contents {
				writer := writerPool.Get().(*manager.WriteAtBuffer)

				client.downloader.Download(client.Context(), writer, &s3.GetObjectInput{
					Bucket: aws.String(client.bucket),
					Key:    object.Key,
				})

				if err := sonic.Unmarshal(writer.Bytes(), &zero); err != nil {
					errnie.Error(errnie.Err(
						errnie.IO, "[store.client] error", err,
					))

					writerPool.Put(writer)
					continue
				}

				if !yield(zero) {
					writerPool.Put(writer)
					return
				}

				writerPool.Put(writer)
			}
		}
	}
}

func (client *Client[T]) Put(key string, data []byte) error {
	_, err := client.uploader.Upload(client.Context(), &s3.PutObjectInput{
		Bucket: aws.String(client.bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader(data),
	})

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO, "[store.client] error", err,
		))
	}

	return nil
}
