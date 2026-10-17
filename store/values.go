package store

import (
	"bytes"
	"io"
	"iter"

	"github.com/theapemachine/symm/data"
)

func ReadValue(client *Client, key string) (*data.Value, error) {
	buf, err := client.Download(key)
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
	return func(yield func(*data.Value, error) bool) {
		for _, key := range client.ListKeys(prefixes...) {
			val, err := ReadValue(client, key)
			if !yield(val, err) {
				return
			}
		}
	}
}

func Open(client *Client, prefixes ...string) io.ReadWriteCloser {
	return &prefixStream{
		client: client,
		keys:   client.ListKeys(prefixes...),
	}
}

type prefixStream struct {
	client *Client
	keys   []string
	at     int
	frame  *bytes.Reader
}

func (stream *prefixStream) Read(p []byte) (int, error) {
	if stream.frame == nil {
		if stream.at >= len(stream.keys) {
			return 0, io.EOF
		}

		buf, err := stream.client.Download(stream.keys[stream.at])
		stream.at++

		if err != nil {
			return 0, err
		}

		stream.frame = bytes.NewReader(buf)
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
	stream.frame = nil
	stream.at = len(stream.keys)
	return nil
}

func SortKeys(keys []string) []string {
	sortKeys(keys)
	return keys
}

func Download(client *Client, keys []string) iter.Seq2[*data.Value, error] {
	return func(yield func(*data.Value, error) bool) {
		for _, key := range keys {
			val, err := ReadValue(client, key)
			if !yield(val, err) {
				return
			}

			if err != nil {
				return
			}
		}
	}
}
