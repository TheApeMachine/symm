package controlflow

import (
	"bytes"
	"context"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
BatchServer collects streaming items into chunks of declared size.
Once the batch size is reached or a flush signal arrives, it emits
the aggregated array payload and signals ready=true.
*/
type BatchServer struct {
	*runtime.System
	size  int64
	items [][]byte
	ready bool
	out   []byte
}

func NewBatch(ctx context.Context) *BatchServer {
	server := &BatchServer{
		System: runtime.NewSystem(ctx, "controlflow.batch"),
		size:   10,
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write accepts an incoming item, desired batch size, or flush directive.
*/
func (server *BatchServer) Write(ctx context.Context, call Batch_write) error {
	size := call.Args().Size()

	if size > 0 {
		server.size = size
	}

	item, err := call.Args().Item()

	if err == nil && len(item) > 0 {
		server.items = append(server.items, bytes.Clone(item))
	}

	flush := call.Args().Flush()
	server.ready = false

	isFull := server.size > 0 && int64(len(server.items)) >= server.size
	shouldEmit := (isFull || flush) && len(server.items) > 0

	if shouldEmit {
		rawItems := make([]any, 0, len(server.items))

		for _, it := range server.items {
			var unmarshaled any
			unmarshalErr := sonic.Unmarshal(it, &unmarshaled)

			if unmarshalErr == nil {
				rawItems = append(rawItems, unmarshaled)
			}

			if unmarshalErr != nil {
				rawItems = append(rawItems, string(it))
			}
		}

		encoded, encodeErr := sonic.Marshal(rawItems)

		if encodeErr == nil {
			server.out = encoded
			server.ready = true
			server.items = nil
		}
	}

	return nil
}

/*
Done emits the batched JSON array when ready, along with the current item count.
*/
func (server *BatchServer) Done(ctx context.Context, call Batch_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"controlflow.batch: failed to allocate done results",
			err,
		))
	}

	results.SetCount(int64(len(server.items)))
	results.SetReady(server.ready)

	if server.ready && len(server.out) > 0 {
		server.ready = false

		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"controlflow.batch: failed to set out",
				err,
			))
		}
	}

	return nil
}
