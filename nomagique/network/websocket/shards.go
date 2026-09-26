package websocket

import (
	capnp "capnproto.org/go/capnp/v3"
	"context"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/* ShardsServer owns stable membership and independently instantiated shard graph capabilities. */
type ShardsServer struct {
	members  map[string]bool
	shards   []shard
	capacity uint32
	next     int
}

/* shard retains only admission work; market observations stay in their source node. */
type shard struct {
	stage   runtime.StageNode
	count   uint32
	pending []string
}

/* NewShards creates an idle node with no connections. */
func NewShards() *ShardsServer { return &ShardsServer{members: make(map[string]bool)} }

/* Write assigns each new symbol once, growing the capability collection without a fixed ceiling. */
func (server *ShardsServer) Write(ctx context.Context, call Shards_write) error {
	symbols, err := call.Args().Symbols()

	if err != nil {
		return errnie.Error(err)
	}
	capacity := call.Args().Capacity()

	if capacity == 0 || server.capacity != 0 && server.capacity != capacity {
		return errnie.Error(errnie.Err(errnie.Validation, "websocket shards: positive, immutable per-connection capacity required", nil))
	}
	server.capacity = capacity
	factory := call.Args().Factory()

	if !factory.IsValid() {
		return errnie.Error(errnie.Err(errnie.Validation, "websocket shards: graph factory required", nil))
	}
	for index := range symbols.Len() {
		symbol, err := symbols.At(index)

		if err != nil {
			return errnie.Error(err)
		}

		if symbol == "" {
			return errnie.Error(errnie.Err(errnie.Validation, "websocket shards: empty symbol", nil))
		}

		if server.members[symbol] {
			continue
		}

		if len(server.shards) == 0 || server.shards[len(server.shards)-1].count == capacity {
			future, release := factory.Create(ctx, nil)
			result, err := future.Struct()

			if err != nil {
				release()
				return errnie.Error(err)
			}
			stage := result.Stage().AddRef()
			release()

			if !stage.IsValid() {
				return errnie.Error(errnie.Err(errnie.Validation, "websocket shards: factory returned no stage", nil))
			}
			server.shards = append(server.shards, shard{stage: stage})
		}
		target := &server.shards[len(server.shards)-1]
		target.pending = append(target.pending, symbol)
		target.count++
		server.members[symbol] = true
	}
	return nil
}

/* Done scans fairly until a frame arrives; idle means every shard was idle. */
func (server *ShardsServer) Done(ctx context.Context, call Shards_done) error {
	result, err := call.AllocResults()

	if err != nil {
		return errnie.Error(err)
	}
	result.SetIdle()

	for range server.shards {
		target := &server.shards[server.next]
		server.next = (server.next + 1) % len(server.shards)
		frame, release, err := target.Step(ctx)

		if err != nil {
			release()
			return err
		}

		if frame.IsValid() && frame.Which() == Received_Which_frame {
			err := capnp.Struct(result).CopyFrom(capnp.Struct(frame))
			release()
			return errnie.Error(err)
		}
		release()
	}
	return nil
}

/* Step advances the authored admission graph and returns its source-owned observation. */
func (target *shard) Step(ctx context.Context) (Received, func(), error) {
	future, release := target.stage.Step(ctx, func(args runtime.StageNode_step_Params) error {
		if len(target.pending) > 0 {
			if err := args.SetEntry("admission.texts"); err != nil {
				return err
			}
			texts, err := args.NewTexts(int32(len(target.pending)))

			if err != nil {
				return err
			}
			for index, symbol := range target.pending {
				if err := texts.Set(index, symbol); err != nil {
					return err
				}
			}
		}
		outputs, err := args.NewOutputs(1)

		if err != nil {
			return err
		}
		return outputs.Set(0, "socket")
	})
	completed, err := future.Struct()

	if err != nil {
		return Received{}, release, errnie.Error(err)
	}
	target.pending = nil
	outputs, err := completed.Outputs()

	if err != nil {
		return Received{}, release, errnie.Error(err)
	}

	if outputs.Len() == 0 {
		return Received{}, release, nil
	}
	output := outputs.At(0)

	if output.InterfaceId() != WebSocketClient_TypeID {
		return Received{}, release, errnie.Error(errnie.Err(errnie.Validation, "websocket shards: socket must be a WebSocketClient node", nil))
	}
	value, err := output.Value()

	if err != nil {
		return Received{}, release, errnie.Error(err)
	}
	return Received(value.Struct()), release, nil
}

/* Shutdown releases every child graph and its source connections. */
func (server *ShardsServer) Shutdown() {
	for _, child := range server.shards {
		child.stage.Release()
	}
	server.shards = nil
}
