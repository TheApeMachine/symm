package store

import (
	"bytes"
	"context"
	"errors"
	"io"
	"iter"
	"math"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/system"
)

/*
downloadErr reports a failed object read. A canceled caller stopped the
download on purpose, so that is not an error to log.
*/
func downloadErr(err error) error {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}

	return errnie.Error(errnie.Err(
		errnie.IO, "[store.values] download error", err,
	))
}

func ReadValue(client *Client, key string) (*data.Value, error) {
	if client == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "[store.values] client is nil", nil,
		))
	}

	return readValue(client.Context(), client, key)
}

func readObject(ctx context.Context, client *Client, key string) ([]byte, error) {
	if err := client.acquire(ctx); err != nil {
		return nil, downloadErr(err)
	}

	defer client.release()

	out, err := client.s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(client.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, downloadErr(err)
	}

	defer out.Body.Close()

	buf, err := readBody(out.Body, out.ContentLength)
	if err != nil {
		return nil, downloadErr(err)
	}

	return buf, nil
}

func readBody(body io.Reader, n *int64) ([]byte, error) {
	if n != nil && *n == 0 {
		return []byte{}, nil
	}

	if n != nil && *n > 0 && *n <= int64(math.MaxInt) {
		buf := make([]byte, int(*n))

		if _, err := io.ReadFull(body, buf); err != nil {
			return nil, err
		}

		return buf, nil
	}

	return io.ReadAll(body)
}

func readValue(ctx context.Context, client *Client, key string) (*data.Value, error) {
	buf, err := readObject(ctx, client, key)
	if err != nil {
		return nil, err
	}

	val := data.New()
	if _, err := val.Write(buf); err != nil {
		return nil, err
	}

	return val, nil
}

/*
ValuesPaged lists each prefix a page at a time and downloads that page while
later pages are still being listed. Zero-padded tick keys are already in tape
order, so the first page can be read before the listing finishes.
*/
func ValuesPaged(client *Client, prefixes ...string) iter.Seq2[*data.Value, error] {
	return func(yield func(*data.Value, error) bool) {
		if client == nil {
			yield(nil, errnie.Error(errnie.Err(
				errnie.Validation, "[store.values] client is nil", nil,
			)))

			return
		}

		ctx, cancel := context.WithCancel(client.Context())
		defer cancel()

		type pageKeys struct {
			keys []string
			err  error
		}

		pages := make(chan pageKeys, 2)

		go func() {
			defer close(pages)

			if len(prefixes) == 0 {
				prefixes = []string{""}
			}

			for _, prefix := range prefixes {
				for page, err := range client.listObjects(ctx, prefix, "") {
					if err != nil {
						select {
						case pages <- pageKeys{err: err}:
						case <-ctx.Done():
						}

						return
					}

					if page == nil {
						continue
					}

					keys := make([]string, 0, len(page.Contents))

					for _, object := range page.Contents {
						if object.Key != nil {
							keys = append(keys, *object.Key)
						}
					}

					select {
					case pages <- pageKeys{keys: keys}:
					case <-ctx.Done():
						return
					}
				}
			}
		}()

		for page := range pages {
			if page.err != nil {
				yield(nil, page.err)
				return
			}

			stopped := false

			parallelValues(client, page.keys, nil)(func(value *data.Value, err error) bool {
				stopped = !yield(value, err)
				return !stopped
			})

			if stopped {
				return
			}
		}
	}
}

func Values(client *Client, prefixes ...string) iter.Seq2[*data.Value, error] {
	return func(yield func(*data.Value, error) bool) {
		if client == nil {
			yield(nil, errnie.Error(errnie.Err(
				errnie.Validation, "[store.values] client is nil", nil,
			)))

			return
		}

		keys, err := client.collectKeys(client.Context(), nil, prefixes...)

		if err != nil {
			yield(nil, err)
			return
		}

		parallelValues(client, keys, nil)(yield)
	}
}

func ParallelValuesProgress(client *Client, keys []string, onObject func()) iter.Seq2[*data.Value, error] {
	return parallelValues(client, keys, onObject)
}

/*
ParallelValues downloads keys with a bounded worker window and yields values
in that key order. onObject, when set through ParallelValuesProgress, runs
once per finished object read.
*/
func ParallelValues(client *Client, keys []string) iter.Seq2[*data.Value, error] {
	return parallelValues(client, keys, nil)
}

func parallelValues(client *Client, keys []string, onObject func()) iter.Seq2[*data.Value, error] {
	return func(yield func(*data.Value, error) bool) {
		if len(keys) == 0 || client == nil {
			return
		}

		ctx, cancel := context.WithCancel(client.Context())
		defer cancel()

		parallelObjects(ctx, client, keys, onObject, func(buf []byte, err error) bool {
			if err != nil {
				return yield(nil, err)
			}

			val := data.New()
			if _, err := val.Write(buf); err != nil {
				return yield(nil, err)
			}

			return yield(val, nil)
		})
	}
}

const prefetchFrames = 8

func inflightLimit(client *Client) int {
	if client != nil && client.slots != nil {
		if n := cap(client.slots); n > 0 {
			return n
		}
	}

	return system.Cfg.Storage.S3.MaxIdleConns
}

type objectResult struct {
	buf []byte
	err error
}

type objectDone struct {
	index int
	objectResult
}

func parallelObjects(
	ctx context.Context,
	client *Client,
	keys []string,
	onObject func(),
	yield func([]byte, error) bool,
) {
	if len(keys) == 0 {
		return
	}

	parent := ctx
	ctx, cancel := context.WithCancel(parent)
	limit := inflightLimit(client)

	if limit > len(keys) {
		limit = len(keys)
	}

	if limit < 1 {
		limit = 1
	}

	jobs := make(chan int)
	results := make(chan objectDone)
	var wg sync.WaitGroup
	wg.Add(limit)

	for range limit {
		go func() {
			defer wg.Done()

			for index := range jobs {
				buf, err := readObject(ctx, client, keys[index])

				if onObject != nil {
					onObject()
				}

				select {
				case results <- objectDone{index: index, objectResult: objectResult{buf: buf, err: err}}:
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	defer func() {
		cancel()
		close(jobs)
		wg.Wait()
	}()

	slots := make([]objectResult, limit)
	ready := make([]bool, limit)
	next := 0
	started := 0

	for next < len(keys) {
		sendTo := jobs
		job := started

		if started >= len(keys) || started >= next+limit {
			sendTo = nil
		}

		select {
		case <-ctx.Done():
			if err := parent.Err(); err != nil {
				yield(nil, downloadErr(err))
			}

			return
		case sendTo <- job:
			started++
		case done := <-results:
			slot := done.index % limit
			slots[slot] = done.objectResult
			ready[slot] = true

			for next < len(keys) && ready[next%limit] {
				item := slots[next%limit]
				slots[next%limit] = objectResult{}
				ready[next%limit] = false
				next++

				if !yield(item.buf, item.err) {
					return
				}
			}
		}
	}
}

func Open(client *Client, prefixes ...string) io.ReadWriteCloser {
	if client == nil {
		return &prefixStream{err: errnie.Error(errnie.Err(
			errnie.Validation, "[store.values] client is nil", nil,
		))}
	}

	ctx, cancel := context.WithCancel(client.Context())
	stream := &prefixStream{
		client: client,
		ctx:    ctx,
		cancel: cancel,
		frames: make(chan prefixFrame, prefetchFrames),
	}

	go stream.run(prefixes)

	return stream
}

type prefixFrame struct {
	buf []byte
	err error
}

type prefixStream struct {
	client *Client
	ctx    context.Context
	cancel context.CancelFunc
	frames chan prefixFrame
	frame  *bytes.Reader
	err    error
}

func (stream *prefixStream) run(prefixes []string) {
	defer close(stream.frames)

	keys, err := stream.client.collectKeys(stream.ctx, nil, prefixes...)

	if err != nil {
		select {
		case <-stream.ctx.Done():
		case stream.frames <- prefixFrame{err: err}:
		}

		return
	}

	parallelObjects(stream.ctx, stream.client, keys, nil, func(buf []byte, err error) bool {
		select {
		case <-stream.ctx.Done():
			return false
		case stream.frames <- prefixFrame{buf: buf, err: err}:
			return err == nil
		}
	})
}

func (stream *prefixStream) Read(p []byte) (int, error) {
	if stream.err != nil {
		return 0, stream.err
	}

	if len(p) == 0 {
		return 0, nil
	}

	if stream.frames == nil {
		return 0, io.EOF
	}

	total := 0

	for total < len(p) {
		if stream.frame == nil || stream.frame.Len() == 0 {
			next, ok := <-stream.frames

			if !ok {
				err := io.EOF

				if stream.ctx != nil && stream.ctx.Err() != nil {
					err = stream.ctx.Err()
				}

				stream.err = err

				if total > 0 {
					return total, nil
				}

				return 0, err
			}

			if next.err != nil {
				stream.err = next.err

				if total > 0 {
					return total, nil
				}

				return 0, next.err
			}

			if len(next.buf) == 0 {
				continue
			}

			stream.frame = bytes.NewReader(next.buf)
		}

		n, err := stream.frame.Read(p[total:])
		total += n

		if stream.frame.Len() == 0 {
			stream.frame = nil
		}

		if err != nil && err != io.EOF {
			stream.err = err

			if total > 0 {
				return total, nil
			}

			return 0, err
		}
	}

	return total, nil
}

func (stream *prefixStream) Write(p []byte) (int, error) {
	if stream.client == nil {
		if stream.err != nil {
			return 0, stream.err
		}

		return 0, errnie.Error(errnie.Err(
			errnie.Validation, "[store.values] client is nil", nil,
		))
	}

	return stream.client.Write(p)
}

func (stream *prefixStream) Close() error {
	if stream.cancel != nil {
		stream.cancel()
	}

	stream.frame = nil
	return nil
}

func SortKeys(keys []string) []string {
	sortKeys(keys)
	return keys
}

func Download(client *Client, keys []string) iter.Seq2[*data.Value, error] {
	return ParallelValues(client, keys)
}
