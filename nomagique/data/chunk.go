package data

import (
	"bytes"
	"context"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
)

/*
ChunkServer partitions a JSON array into deterministic sequential slices of up to
a declared maximum chunk size (default 200).
*/
type ChunkServer struct {
	data     []byte
	size     int64
	endpoint string
}

func NewChunk() *ChunkServer {
	return &ChunkServer{
		size:     200,
		endpoint: "wss://ws-l3.kraken.com/v2",
	}
}

func (server *ChunkServer) Write(ctx context.Context, call Chunk_write) error {
	args := call.Args()
	dataBytes, err := args.Data()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "data.chunk: read data", err))
	}

	server.data = bytes.Clone(dataBytes)

	if args.Size() > 0 {
		server.size = args.Size()
	}

	endpoint, _ := args.Endpoint()

	if endpoint != "" {
		server.endpoint = endpoint
	}

	return nil
}

func (server *ChunkServer) Done(ctx context.Context, call Chunk_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "data.chunk: alloc results", err))
	}

	if len(server.data) == 0 {
		return nil
	}

	var items []any
	err = sonic.Unmarshal(server.data, &items)

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "data.chunk: unmarshal data array", err))
	}

	chunkSize := int(server.size)

	if chunkSize <= 0 {
		chunkSize = 200
	}

	totalItems := len(items)

	setChunk := func(startOffset, endOffset int, dataSetter func([]byte) error, epSetter func(string) error) error {
		if startOffset >= totalItems {
			return nil
		}

		if endOffset > totalItems {
			endOffset = totalItems
		}

		sliced := items[startOffset:endOffset]

		if len(sliced) == 0 {
			return nil
		}

		encoded, err := sonic.Marshal(sliced)

		if err != nil {
			return errnie.Error(errnie.Err(errnie.Internal, "data.chunk: marshal chunk", err))
		}

		if err := dataSetter(encoded); err != nil {
			return err
		}

		if server.endpoint != "" {
			return epSetter(server.endpoint)
		}

		return nil
	}

	if err := setChunk(0, chunkSize, results.SetOut0, results.SetEndpoint0); err != nil {
		return err
	}

	if err := setChunk(chunkSize, 2*chunkSize, results.SetOut1, results.SetEndpoint1); err != nil {
		return err
	}

	if err := setChunk(2*chunkSize, 3*chunkSize, results.SetOut2, results.SetEndpoint2); err != nil {
		return err
	}

	if err := setChunk(3*chunkSize, 4*chunkSize, results.SetOut3, results.SetEndpoint3); err != nil {
		return err
	}

	server.data = nil
	return nil
}
