package runtime

import (
	"context"
	"slices"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/bytedance/sonic"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/ui"
)

/* ConsumerServer owns a stage capability and records successful completion. */
type ConsumerServer struct {
	*System
	target        StageNode
	receiver      ui.Receiver
	snapshot      Snapshot
	checkpoint    Checkpoint
	entry         string
	record        string
	name          string
	configuration string
	bindings      []struct {
		Producer string `json:"producer"`
		Node     string `json:"node"`
		Field    string `json:"field"`
		Target   string `json:"target"`
		Stamp    string `json:"stamp"`
		Gate     bool   `json:"gate"`
	}
	outputs   []string
	result    Completion
	epoch     int64
	sequence  int64
	completed uint64
	failure   error
}

/* NewConsumer constructs an idle Cap'n Proto consumer. */
func NewConsumer(ctx context.Context) *ConsumerServer {
	server := &ConsumerServer{System: NewSystem(ctx, "runtime.consumer")}
	server.Transition(WAITING)
	return server
}

/* Write binds the work capability before the consumer processes observations. */
func (server *ConsumerServer) Write(ctx context.Context, call Consumer_write) error {
	target := call.Args().Target()
	entry, err := call.Args().Entry()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "consumer: entry port", err))
	}

	if !target.IsValid() {
		return errnie.Error(errnie.Err(errnie.Validation, "consumer: missing stage capability", nil))
	}

	record, err := call.Args().Record()

	if err != nil {
		return errnie.Error(err)
	}

	name, err := call.Args().Name()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "consumer: name", err))
	}
	configuration, err := call.Args().Bindings()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "consumer: bindings", err))
	}

	outputs, err := call.Args().Outputs()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "consumer: output selection", err))
	}

	selected := make([]string, outputs.Len())

	for index := range outputs.Len() {
		output, err := outputs.At(index)

		if err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "consumer: output name", err))
		}
		selected[index] = output
	}
	if server.target.IsValid() {
		if !server.target.IsSame(target) || entry != server.entry || record != server.record || name != server.name ||
			configuration != server.configuration || !slices.Equal(selected, server.outputs) || !server.receiver.IsSame(call.Args().Receiver()) || !server.snapshot.IsSame(call.Args().Snapshot()) || !server.checkpoint.IsSame(call.Args().Checkpoint()) {
			return errnie.Error(errnie.Err(errnie.Validation, "consumer: configuration cannot change", nil))
		}
		return nil
	}

	if configuration != "" {
		if err := sonic.Unmarshal([]byte(configuration), &server.bindings); err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "consumer: bindings", err))
		}
	}
	server.outputs = selected
	server.name, server.configuration = name, configuration
	server.target = target.AddRef()
	server.receiver = call.Args().Receiver().AddRef()
	server.entry, server.record = entry, record
	server.snapshot = call.Args().Snapshot().AddRef()
	server.checkpoint = call.Args().Checkpoint().AddRef()
	if server.snapshot.IsValid() != server.checkpoint.IsValid() {
		return errnie.Error(errnie.Err(errnie.Validation, "consumer: snapshot and checkpoint must be wired together", nil))
	}
	if server.checkpoint.IsValid() {
		if !call.Args().CheckpointReady() {
			return errnie.Error(errnie.Err(errnie.Validation, "consumer: checkpoint is not configured", nil))
		}
		future, release := server.checkpoint.Load(ctx, func(params Checkpoint_load_Params) error { return params.SetKey(server.name) })
		defer release()
		result, err := future.Struct()
		if err != nil {
			return errnie.Error(err)
		}
		encoded, err := result.Data()
		if err != nil {
			return errnie.Error(err)
		}
		if len(encoded) > 0 {
			restored, release := server.snapshot.Restore(ctx, func(params Snapshot_restore_Params) error { return params.SetData(encoded) })
			_, err = restored.Struct()
			release()
			if err != nil {
				return errnie.Error(err)
			}
		}
	}
	server.Transition(READY)
	return nil
}

/* Step acknowledges the observation only after the bound node completes it. */
func (server *ConsumerServer) Step(ctx context.Context, call StageNode_step) error {
	if server.failure != nil {
		return server.failure
	}

	if !server.target.IsValid() {
		return errnie.Error(errnie.Err(errnie.Validation, "consumer: not configured", nil))
	}

	args := call.Args()

	if args.Epoch() <= 0 || args.Sequence() < 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "consumer: invalid observation stamp", nil))
	}
	payload, err := args.Data()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "consumer: observation", err))
	}

	future, release := server.target.Step(ctx, func(params StageNode_step_Params) error {
		params.SetEpoch(args.Epoch())
		params.SetSequence(args.Sequence())
		upstream, err := args.Upstream()

		if err != nil {
			return err
		}

		if err := params.SetUpstream(upstream); err != nil {
			return err
		}
		bindings, err := params.NewBindings(int32(len(server.bindings)))

		if err != nil {
			return err
		}

		for index, binding := range server.bindings {
			target := bindings.At(index)
			target.SetGate(binding.Gate)
			switch binding.Stamp {
			case "", "value":
				target.SetStamp(Stamp_value)
			case "epoch":
				target.SetStamp(Stamp_epoch)
			case "sequence":
				target.SetStamp(Stamp_sequence)
			default:
				return errnie.Error(errnie.Err(errnie.Validation, "consumer: unknown binding stamp "+binding.Stamp, nil))
			}

			for _, err := range []error{target.SetProducer(binding.Producer), target.SetNode(binding.Node), target.SetField(binding.Field), target.SetTarget(binding.Target)} {
				if err != nil {
					return err
				}
			}
		}
		outputs, err := params.NewOutputs(int32(len(server.outputs)))

		if err != nil {
			return err
		}

		for index, name := range server.outputs {
			if err := outputs.Set(index, name); err != nil {
				return err
			}
		}
		if err := params.SetEntry(server.entry); err != nil {
			return err
		}
		if err := params.SetRecord(server.record); err != nil {
			return err
		}
		return params.SetData(payload)
	})
	defer release()

	result, err := future.Struct()

	if err != nil {
		server.failure = errnie.Error(errnie.Err(errnie.IO, "consumer: stage failed", err))
		server.Transition(ERROR)
		return server.failure
	}

	allocated, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "consumer: results", err))
	}

	if err := capnp.Struct(allocated).CopyFrom(capnp.Struct(result)); err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "consumer: copy results", err))
	}
	outputs, err := allocated.Outputs()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "consumer: read outputs", err))
	}

	for index := range outputs.Len() {
		output := outputs.At(index)
		output.SetEpoch(args.Epoch())
		output.SetSequence(args.Sequence())

		if err := output.SetProducer(server.name); err != nil {
			return errnie.Error(errnie.Err(errnie.Internal, "consumer: output producer", err))
		}
	}
	retained, err := allocated.Clone()

	if err != nil {
		return err
	}

	if server.result.IsValid() {
		server.result.Message().Release()
	}
	server.result = retained

	frame, err := allocated.Bindings()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "consumer: UI bindings", err))
	}
	if len(frame) > 0 && !server.receiver.IsValid() {
		server.failure = errnie.Error(errnie.Err(errnie.Validation, "consumer: UI bindings require a receiver capability", nil))
		server.Transition(ERROR)
		return server.failure
	}
	if len(frame) > 0 {
		future, release := server.receiver.Publish(ctx, func(params ui.Receiver_publish_Params) error {
			return params.SetData(frame)
		})
		_, err := future.Struct()
		release()

		if err != nil {
			server.failure = errnie.Error(errnie.Err(errnie.IO, "consumer: publish bindings", err))
			server.Transition(ERROR)
			return server.failure
		}
	}
	server.epoch, server.sequence = args.Epoch(), args.Sequence()
	server.completed++
	return nil
}

/* Done reports only work that completed successfully. Failures remain visible. */
func (server *ConsumerServer) Done(ctx context.Context, call Consumer_done) error {
	if server.failure != nil {
		return server.failure
	}

	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "consumer: allocate progress", err))
	}

	results.SetEpoch(server.epoch)
	results.SetSequence(server.sequence)
	results.SetCompleted(server.completed)
	results.SetStatus(Status(server.Status()))

	if server.result.IsValid() {
		payload, err := server.result.Data()
		if err != nil {
			return errnie.Error(err)
		}
		if err := results.SetData(payload); err != nil {
			return errnie.Error(err)
		}
		outputs, err := server.result.Outputs()

		if err != nil {
			return errnie.Error(errnie.Err(errnie.Internal, "consumer: output results", err))
		}

		if err := results.SetOutputs(outputs); err != nil {
			return errnie.Error(errnie.Err(errnie.Internal, "consumer: publish outputs", err))
		}
	}
	return nil
}

/* Shutdown releases the capability owned by this node. */
func (server *ConsumerServer) Shutdown() {
	server.snapshot.Release()
	server.checkpoint.Release()
	server.receiver.Release()
	server.target.Release()

	if server.result.IsValid() {
		server.result.Message().Release()
	}

	if err := server.Close(); err != nil {
		errnie.Error(err)
	}
}

/* Fence reaches the durable owners behind the configured stage capability. */
func (server *ConsumerServer) Fence(ctx context.Context, call StageNode_fence) error {
	if !server.target.IsValid() {
		return nil
	}
	future, release := server.target.Fence(ctx, nil)
	defer release()
	_, err := future.Struct()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.IO, "consumer: fence stage", err))
	}
	if server.checkpoint.IsValid() {
		future, release := server.snapshot.Snapshot(ctx, nil)
		defer release()
		result, err := future.Struct()
		if err != nil {
			return errnie.Error(err)
		}
		encoded, err := result.Data()
		if err != nil {
			return errnie.Error(err)
		}
		saved, release := server.checkpoint.Save(ctx, func(params Checkpoint_save_Params) error {
			if err := params.SetKey(server.name); err != nil {
				return err
			}
			return params.SetData(encoded)
		})
		_, err = saved.Struct()
		release()
		if err != nil {
			return errnie.Error(err)
		}
	}

	return nil
}
