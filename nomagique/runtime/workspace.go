package runtime

import (
	"bytes"
	capnp "capnproto.org/go/capnp/v3"
	"context"
	"errors"
	"iter"
	goruntime "runtime"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime/disruptor"
)

/*
WorkspaceServer is the Cap'n Proto owner of one native LMAX ring. Its capability
ports are the handler groups. LMAX alone orders their execution.
*/
type WorkspaceServer struct {
	*System
	ring       disruptor.Disruptor
	slots      []Slot
	mask       int64
	consumers  []Consumer
	progress   []*atomic.Int64
	failure    atomic.Pointer[error]
	groups     []Group
	writers    uint8
	epoch      int64
	published  int64
	started    bool
	finished   chan struct{}
	sources    int
	sourceMode bool
	data       []byte
}

/* NewWorkspace constructs an idle node. Write supplies its graph connections. */
func NewWorkspace(ctx context.Context) *WorkspaceServer {
	server := &WorkspaceServer{System: NewSystem(ctx, "runtime.workspace"), finished: make(chan struct{})}
	server.Transition(WAITING)
	return server
}

/* Write admits observations or advances the source group on the native ring. */
func (server *WorkspaceServer) Write(ctx context.Context, call Workspace_write) error {
	args := call.Args()

	if err := server.configure(ctx, args); err != nil {
		return err
	}

	if args.Admit() && !server.started {
		server.started = true
		server.Transition(READY)

		go func() {
			defer close(server.finished)
			server.ring.Listen()
		}()
	}

	payloads, err := args.Data()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "workspace: input", err))
	}

	if server.sourceMode {
		if payloads.Len() != 0 {
			return errnie.Error(errnie.Err(errnie.Validation, "workspace: source cycles cannot also receive external records", nil))
		}
		return server.advance(ctx)
	}

	for index := range payloads.Len() {
		payload, err := payloads.At(index)

		if err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "workspace: feed", err))
		}
		if len(payload) == 0 {
			continue
		}
		if _, err := server.publish(ctx, payload, server.epoch, server.published, Result_List{}); err != nil {
			return err
		}
	}

	return nil
}

/* configure binds stage capabilities directly to native LMAX handler groups. */
func (server *WorkspaceServer) configure(ctx context.Context, args Workspace_write_Params) error {
	if server.ring != nil {
		return server.validate(args)
	}
	capacity := args.Capacity()

	if capacity == 0 || capacity&(capacity-1) != 0 || args.Writers() == 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "workspace: capacity must be a power of two and writers must be positive", nil))
	}

	server.epoch = args.Epoch()

	if server.epoch == 0 {
		server.epoch = time.Now().UnixNano()
	}
	if server.epoch < 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "workspace: epoch must be positive", nil))
	}
	server.slots, server.mask = make([]Slot, capacity), int64(capacity)-1
	server.writers = args.Writers()
	server.sourceMode = args.Advance()
	options := disruptor.NewOptions(disruptor.Options.BufferCapacity(capacity), disruptor.Options.WriterCount(args.Writers()))
	groups, err := args.Groups()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "workspace: groups", err))
	}

	for index := range groups.Len() {
		group, err := groups.At(index)

		if err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "workspace: group capability", err))
		}

		if !group.IsValid() {
			return errnie.Error(errnie.Err(errnie.Validation, "workspace: missing group capability", nil))
		}

		handlers, err := server.bind(ctx, group)

		if err != nil {
			return err
		}
		if index == 0 {
			server.sources = len(handlers)
		}
		options = append(options, disruptor.Options.NewHandlerGroup(handlers...))
		server.groups = append(server.groups, group.AddRef())
	}

	ring, err := disruptor.New(options...)

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "workspace: configure ring", err))
	}
	server.ring = ring
	return nil
}

/* publish copies RPC-owned bytes into a slot reserved by the native sequencer. */
func (server *WorkspaceServer) publish(ctx context.Context, payload []byte, epoch, sequence int64, upstream Result_List) (int64, error) {
	if !server.started {
		return 0, errnie.Error(errnie.Err(errnie.Validation, "workspace: not admitted", nil))
	}

	var retained Completion
	if upstream.IsValid() {
		message, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
		if err != nil {
			return 0, errnie.Error(err)
		}
		retained, err = NewRootCompletion(segment)
		if err == nil {
			err = retained.SetOutputs(upstream)
		}
		if err != nil {
			message.Release()
			return 0, errnie.Error(err)
		}
	}

	for {
		if err := server.check(ctx); err != nil {
			if retained.IsValid() {
				retained.Message().Release()
			}
			return 0, err
		}
		reserved := server.ring.TryReserve(1)

		if reserved == disruptor.ErrCapacityUnavailable {
			goruntime.Gosched()
			continue
		}

		if previous := server.slots[reserved&server.mask].Upstream; previous.IsValid() {
			previous.Message().Release()
		}
		for _, result := range server.slots[reserved&server.mask].Results {
			if result.IsValid() {
				result.Message().Release()
			}
		}

		server.slots[reserved&server.mask] = Slot{Upstream: retained, Epoch: epoch, Sequence: sequence, Payload: bytes.Clone(payload), Results: make([]Completion, len(server.consumers))}
		server.published++
		server.ring.Commit(reserved, reserved)
		return reserved, nil
	}
}

/* Done reports actual completion and errors; it does not dispatch graph work. */
func (server *WorkspaceServer) Done(ctx context.Context, call Workspace_done) error {
	if err := server.check(ctx); err != nil {
		return err
	}
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "workspace: allocate progress", err))
	}
	completed := server.completed()
	results.SetEpoch(server.epoch)
	results.SetPublished(server.published)
	results.SetCompleted(completed)
	results.SetPending(uint64(server.published - completed))
	results.SetStatus(Status(server.Status()))
	return errnie.Error(results.SetData(server.data))
}

/* completed reads the slowest handler's finished observation count. */
func (server *WorkspaceServer) completed() int64 {
	completed := server.published

	for _, progress := range server.progress {
		completed = min(completed, progress.Load())
	}
	return completed
}

/* check surfaces stage failures and cancellation to the publishing caller. */
func (server *WorkspaceServer) check(ctx context.Context) error {
	if failure := server.failure.Load(); failure != nil {
		return *failure
	}

	if err := ctx.Err(); err != nil {
		return errnie.Error(errnie.Err(errnie.IO, "workspace: interrupted", err))
	}
	if err := server.Context().Err(); err != nil {
		return errnie.Error(errnie.Err(errnie.IO, "workspace: closed", err))
	}
	return nil
}

/* Flush drains admitted work before the graph flushes its durable nodes. */
func (server *WorkspaceServer) Flush(ctx context.Context, call Durable_flush) error {
	return server.fence(ctx)
}

/* Fence propagates a parent stage fence through this workspace. */
func (server *WorkspaceServer) Fence(ctx context.Context, call StageNode_fence) error {
	return server.fence(ctx)
}

/* fence joins native progress before flushing every child owner. */
func (server *WorkspaceServer) fence(ctx context.Context) error {
	if err := server.drain(ctx, server.published); err != nil {
		return err
	}
	var failures []error
	for _, consumer := range server.consumers {
		future, release := consumer.Fence(ctx, nil)
		_, err := future.Struct()
		release()
		if err != nil {
			failures = append(failures, errnie.Error(errnie.Err(errnie.IO, "workspace: fence consumer", err)))
		}
	}
	return errors.Join(failures...)
}

/* Step lets a workspace be a stage node; completion includes its inner stages. */
func (server *WorkspaceServer) Step(ctx context.Context, call StageNode_step) error {
	payload, err := call.Args().Data()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "workspace: stage input", err))
	}
	upstream, err := call.Args().Upstream()
	if err != nil {
		return errnie.Error(err)
	}
	sequence, err := server.publish(ctx, payload, call.Args().Epoch(), call.Args().Sequence(), upstream)

	if err != nil {
		return err
	}
	if err := server.drain(ctx, sequence+1); err != nil {
		return err
	}
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(err)
	}
	slot := server.slots[sequence&server.mask]
	count := 0
	for _, completed := range slot.Results {
		outputs, err := completed.Outputs()
		if err != nil {
			return errnie.Error(err)
		}
		count += outputs.Len()
	}
	outputs, err := results.NewOutputs(int32(count))
	if err != nil {
		return errnie.Error(err)
	}
	position := 0
	for _, completed := range slot.Results {
		values, err := completed.Outputs()
		if err != nil {
			return errnie.Error(err)
		}
		for index := range values.Len() {
			if err := outputs.Set(position, values.At(index)); err != nil {
				return errnie.Error(err)
			}
			position++
		}
	}
	return nil
}

/* drain observes native handler progress; it does not schedule stages. */
func (server *WorkspaceServer) drain(ctx context.Context, target int64) error {
	for {
		if err := server.check(ctx); err != nil {
			return err
		}
		if server.completed() >= target {
			return nil
		}
		goruntime.Gosched()
	}
}

/* Shutdown joins all native handlers before releasing their capabilities. */
func (server *WorkspaceServer) Shutdown() {
	if err := server.Close(); err != nil {
		errnie.Error(err)
	}
	if server.ring != nil {
		if err := server.ring.Close(); err != nil {
			errnie.Error(err)
		}
	}

	if server.started {
		<-server.finished
	}
	for _, slot := range server.slots {
		if slot.Upstream.IsValid() {
			slot.Upstream.Message().Release()
		}
		for _, result := range slot.Results {
			if result.IsValid() {
				result.Message().Release()
			}
		}
	}
	for _, consumer := range server.consumers {
		consumer.Release()
	}
	for _, group := range server.groups {
		group.Release()
	}
}

/* Slot is immutable from publication until every LMAX group passes it. */
type Slot struct {
	Upstream Completion
	Epoch    int64
	Sequence int64
	Payload  []byte
	Results  []Completion
}

/* bind connects node capabilities to native handler calls without another scheduler. */
func (server *WorkspaceServer) bind(ctx context.Context, group Group) ([]disruptor.Handler, error) {
	future, release := group.Members(ctx, nil)
	defer release()
	results, err := future.Struct()

	if err != nil {
		return nil, errnie.Error(errnie.Err(errnie.IO, "workspace: group membership", err))
	}
	members, err := results.Consumers()

	if err != nil {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "workspace: group consumers", err))
	}
	handlers := make([]disruptor.Handler, 0, members.Len())
	previous := len(server.consumers)

	for index := range members.Len() {
		consumer, err := members.At(index)

		if err != nil {
			return nil, errnie.Error(errnie.Err(errnie.Validation, "workspace: consumer capability", err))
		}
		consumer = consumer.AddRef()
		progress := &atomic.Int64{}
		position := len(server.consumers)
		server.consumers = append(server.consumers, consumer)
		server.progress = append(server.progress, progress)
		handlers = append(handlers, disruptor.HandlerFunc(func(batch iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
			return server.handle(consumer, progress, previous, position, batch)
		}))
	}
	return handlers, nil
}

/* handle acknowledges native work only after the Consumer node returns. */
func (server *WorkspaceServer) handle(consumer Consumer, progress *atomic.Int64, previous, position int, batch iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	for reserved := range batch {
		slot := server.slots[*(*int64)(reserved)&server.mask]
		result, err := server.consume(consumer, slot, previous, position)
		if err == nil {
			slot.Results[position] = result
		}

		if err != nil {
			err = errnie.Error(errnie.Err(errnie.IO, "workspace: consumer failed", err))
			server.failure.CompareAndSwap(nil, &err)
			return nil
		}
		progress.Add(1)
	}
	return batch
}

/* validate refuses changing the ring's configuration after it has been bound. */
func (server *WorkspaceServer) validate(args Workspace_write_Params) error {
	if args.Capacity() != 0 && args.Capacity() != uint32(len(server.slots)) ||
		args.Writers() != 0 && args.Writers() != server.writers ||
		args.Epoch() != 0 && args.Epoch() != server.epoch {
		return errnie.Error(errnie.Err(errnie.Validation, "workspace: ring configuration cannot change", nil))
	}
	groups, err := args.Groups()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "workspace: groups", err))
	}

	if groups.Len() == 0 {
		return nil
	}

	if groups.Len() != len(server.groups) {
		return errnie.Error(errnie.Err(errnie.Validation, "workspace: groups cannot change", nil))
	}

	for index, existing := range server.groups {
		group, err := groups.At(index)

		if err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "workspace: group capability", err))
		}

		if !existing.IsSame(group) {
			return errnie.Error(errnie.Err(errnie.Validation, "workspace: group capability cannot change", nil))
		}
	}
	return nil
}

/* upstream reads only results behind this consumer's native group barrier. */
func (server *WorkspaceServer) upstream(args StageNode_step_Params, slot Slot, previous int) error {
	incoming, err := slot.Upstream.Outputs()
	if err != nil {
		return errnie.Error(err)
	}
	if server.sourceMode && previous > 0 {
		incoming = Result_List{}
	}
	count := incoming.Len()
	lists := make([]Result_List, previous+1)
	lists[previous] = incoming

	for index := range previous {
		outputs, err := slot.Results[index].Outputs()

		if err != nil {
			return errnie.Error(errnie.Err(errnie.Internal, "workspace: upstream results", err))
		}
		lists[index] = outputs
		count += outputs.Len()
	}
	if server.sourceMode && previous > 0 {
		count = 0
		for _, outputs := range lists {
			for index := range outputs.Len() {
				if outputs.At(index).Sequence() == args.Sequence() {
					count++
				}
			}
		}
	}
	upstream, err := args.NewUpstream(int32(count))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "workspace: allocate upstream", err))
	}
	position := 0

	for _, outputs := range lists {
		for index := range outputs.Len() {
			if server.sourceMode && previous > 0 && outputs.At(index).Sequence() != args.Sequence() {
				continue
			}
			if err := upstream.Set(position, outputs.At(index)); err != nil {
				return errnie.Error(errnie.Err(errnie.Internal, "workspace: copy upstream", err))
			}
			position++
		}
	}
	return nil
}
