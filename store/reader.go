package store

import (
	"context"
	"errors"
	"fmt"
	"iter"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
)

/*
Reader is the read-only view of a bucket: it lists and downloads, nothing
else. It never creates the bucket and holds no uploader, and it exposes no
write, delete or move method, so code handed a Reader cannot write storage:
an attempt does not compile. Every failure is returned, never skipped.
*/
type Reader struct {
	ctx        context.Context
	api        *s3.Client
	downloader *manager.Downloader
	bucket     string
}

/*
NewReader checks that bucket is reachable (HeadBucket, a read) and returns
its read-only view. An unreachable or missing bucket is an error.
*/
func NewReader(ctx context.Context, bucket string, optFns ...func(*s3.Options)) (*Reader, error) {
	api, err := newS3(optFns...)

	if err != nil {
		return nil, err
	}

	if _, err := api.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(bucket)}); err != nil {
		return nil, errnie.Err(errnie.IO, "[store.reader] bucket "+bucket+" is unreachable", err)
	}

	return &Reader{ctx: ctx, api: api, downloader: manager.NewDownloader(api), bucket: bucket}, nil
}

/*
list pages every listing under prefix; a page that fails is an error.
*/
func (reader *Reader) list(prefix, delimiter string, page func(*s3.ListObjectsV2Output)) error {
	input := &s3.ListObjectsV2Input{Bucket: aws.String(reader.bucket), Prefix: aws.String(prefix)}

	if delimiter != "" {
		input.Delimiter = aws.String(delimiter)
	}

	paginator := s3.NewListObjectsV2Paginator(reader.api, input)

	for paginator.HasMorePages() {
		out, err := paginator.NextPage(reader.ctx)

		if err != nil {
			return errnie.Err(errnie.IO, "[store.reader] listing "+prefix, err)
		}

		page(out)
	}

	return nil
}

/*
Epochs lists the epochs stored under prefix (a data.Prefix without epoch),
oldest first.
*/
func (reader *Reader) Epochs(prefix string) ([]uint64, error) {
	epochs := make([]uint64, 0)
	var malformed error

	err := reader.list(prefix, "/", func(out *s3.ListObjectsV2Output) {
		for _, common := range out.CommonPrefixes {
			if common.Prefix == nil {
				continue
			}

			epoch, err := data.KeyEpoch(prefix, *common.Prefix)

			if err != nil {
				malformed = errors.Join(malformed, err)
				continue
			}

			epochs = append(epochs, epoch)
		}
	})

	if err := errors.Join(err, malformed); err != nil {
		return nil, err
	}

	return sortedEpochs(epochs), nil
}

/*
Values yields every Value under prefixes, in key order, downloading one at
a time so an epoch of ticks is never held whole. A key that fails to list,
download or decode is yielded as an error naming it and ends the sequence;
so does a canceled context.
*/
func (reader *Reader) Values(prefixes ...string) iter.Seq2[*data.Value, error] {
	return func(yield func(*data.Value, error) bool) {
		keys := make([]string, 0)
		seen := make(map[string]bool)

		for _, prefix := range prefixes {
			err := reader.list(prefix, "", func(out *s3.ListObjectsV2Output) {
				for _, object := range out.Contents {
					if object.Key != nil && !seen[*object.Key] {
						seen[*object.Key] = true
						keys = append(keys, *object.Key)
					}
				}
			})

			if err != nil {
				yield(nil, err)
				return
			}
		}

		sortKeys(keys)

		for _, key := range keys {
			if err := reader.ctx.Err(); err != nil {
				yield(nil, err)
				return
			}

			buffer := manager.NewWriteAtBuffer(make([]byte, 0, 4096))

			if _, err := reader.downloader.Download(reader.ctx, buffer, &s3.GetObjectInput{
				Bucket: aws.String(reader.bucket),
				Key:    aws.String(key),
			}); err != nil {
				yield(nil, errnie.Err(errnie.IO, "[store.reader] download "+key, err))
				return
			}

			value := data.New()

			if _, err := value.Write(buffer.Bytes()); err != nil {
				yield(nil, errnie.Err(errnie.UnprocessableContent, fmt.Sprintf("[store.reader] %s does not decode as a Value", key), err))
				return
			}

			if !yield(value, nil) {
				return
			}
		}
	}
}
