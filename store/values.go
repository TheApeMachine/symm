package store

import (
	"bytes"
	"context"
	"errors"
	"io"
	"iter"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
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

	buf, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, downloadErr(err)
	}

	return buf, nil
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

func Values(client *Client, prefixes ...string) iter.Seq2[*data.Value, error] {
	return ParallelValues(client, client.ListKeys(prefixes...))
}

/*
ParallelValues splits an already-sorted key list into contiguous groups,
downloads each group together, and yields the groups in that same order.
Several groups stay in flight so the next slice is downloading while the
detector walks the one it already has.
*/
func ParallelValues(client *Client, keys []string) iter.Seq2[*data.Value, error] {
	return func(yield func(*data.Value, error) bool) {
		if len(keys) == 0 || client == nil {
			return
		}

		ctx, cancel := context.WithCancel(client.Context())
		defer cancel()

		parallelObjects(ctx, client, keys, func(buf []byte, err error) bool {
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

func parallelObjects(
	ctx context.Context,
	client *Client,
	keys []string,
	yield func([]byte, error) bool,
) {
	const groupSize = 64

	groupsInFlight := inflightGets / groupSize

	type item struct {
		buf []byte
		err error
	}

	type batch struct {
		items []item
	}

	groupCount := (len(keys) + groupSize - 1) / groupSize
	batches := make([]chan batch, groupCount)

	for i := range batches {
		batches[i] = make(chan batch, 1)
	}

	started := 0
	start := func(group int) {
		lo := group * groupSize
		hi := min(lo+groupSize, len(keys))

		slice := keys[lo:hi]

		go func() {
			items := make([]item, len(slice))
			var wg sync.WaitGroup

			for i := range slice {
				wg.Add(1)

				go func(i int) {
					defer wg.Done()

					if ctx.Err() != nil {
						items[i] = item{err: ctx.Err()}
						return
					}

					buf, err := readObject(ctx, client, slice[i])
					items[i] = item{buf, err}
				}(i)
			}

			wg.Wait()

			select {
			case batches[group] <- batch{items}:
			case <-ctx.Done():
			}
		}()
	}

	for group := range groupCount {
		if ctx.Err() != nil {
			return
		}

		for started < groupCount && started < group+groupsInFlight {
			start(started)
			started++
		}

		select {
		case <-ctx.Done():
			return
		case got := <-batches[group]:
			for _, it := range got.items {
				if !yield(it.buf, it.err) {
					return
				}
			}
		}
	}
}

func Open(client *Client, prefixes ...string) io.ReadWriteCloser {
	ctx, cancel := context.WithCancel(client.Context())
	stream := &prefixStream{
		client: client,
		cancel: cancel,
		frames: make(chan prefixFrame, inflightGets/64),
	}

	go stream.prefetch(ctx, client.ListKeys(prefixes...))

	return stream
}

type prefixFrame struct {
	buf []byte
	err error
}

type prefixStream struct {
	client *Client
	cancel context.CancelFunc
	frames chan prefixFrame
	frame  *bytes.Reader
	err    error
}

func (stream *prefixStream) prefetch(ctx context.Context, keys []string) {
	defer close(stream.frames)

	parallelObjects(ctx, stream.client, keys, func(buf []byte, err error) bool {
		select {
		case <-ctx.Done():
			return false
		case stream.frames <- prefixFrame{buf, err}:
			return err == nil
		}
	})
}

func (stream *prefixStream) Read(p []byte) (int, error) {
	if stream.err != nil {
		return 0, stream.err
	}

	if stream.frame == nil {
		next, ok := <-stream.frames

		if !ok {
			return 0, io.EOF
		}

		if next.err != nil {
			stream.err = next.err
			return 0, next.err
		}

		stream.frame = bytes.NewReader(next.buf)
	}

	n, err := stream.frame.Read(p)
	if stream.frame.Len() == 0 {
		stream.frame = nil
		if err == nil {
			err = io.EOF
		}
	}

	return n, err
}

func (stream *prefixStream) Write(p []byte) (int, error) {
	return stream.client.Write(p)
}

func (stream *prefixStream) Close() error {
	stream.cancel()
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
