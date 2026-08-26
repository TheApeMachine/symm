package runtime

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/smarty/go-disruptor"
	"github.com/theapemachine/errnie"
	"golang.design/x/lockfree/lf"
)

const subscriberCapacity uint32 = 64 * 1024
const subscriberMask int64 = int64(subscriberCapacity - 1)

/*
SubscriberRuntime represents an independently executing processing stage backed by its own Disruptor input ring.
*/
type SubscriberRuntime struct {
	workspace  *Workspace
	inTopic    string
	outTopic   string
	step       func(any) any
	ring       []any
	disruptor  disruptor.Disruptor
	bufferMask int64
	inFlight   atomic.Int64
}

type subscriberDisruptorHandler struct {
	runtime *SubscriberRuntime
}

func (handler *subscriberDisruptorHandler) Handle(lowerSequence, upperSequence int64) {
	for sequence := lowerSequence; sequence <= upperSequence; sequence++ {
		value := handler.runtime.ring[sequence&handler.runtime.bufferMask]
		handler.runtime.process(value)
		handler.runtime.inFlight.Add(-1)
		handler.runtime.workspace.inFlight.Add(-1)
	}
}

func (runtime *SubscriberRuntime) process(value any) {
	defer func() {
		if recovered := recover(); recovered != nil {
			var err error

			if panicErr, ok := recovered.(error); ok {
				err = panicErr
			}

			if err == nil {
				err = fmt.Errorf("workspace: subscriber panic on %s: %v", runtime.inTopic, recovered)
			}

			if failureFn := runtime.workspace.failureHandler.Load(); failureFn != nil && *failureFn != nil {
				(*failureFn)(err)
			}

			errnie.Error(errnie.Err(
				errnie.Internal,
				"workspace: subscriber panic",
				err,
			))
		}
	}()

	output := runtime.step(value)

	if output != nil && runtime.outTopic != "" {
		runtime.workspace.Publish(runtime.outTopic, output)
	}
}

func (runtime *SubscriberRuntime) Enqueue(value any) {
	runtime.inFlight.Add(1)
	sequence := runtime.disruptor.Reserve(1)
	runtime.ring[sequence&runtime.bufferMask] = value
	runtime.disruptor.Commit(sequence, sequence)
}

func (runtime *SubscriberRuntime) Close() error {
	if runtime.disruptor != nil {
		return runtime.disruptor.Close()
	}

	return nil
}

/*
Workspace is the typed dataflow coordinator and routing substrate.
*/
type Workspace struct {
	ctx            context.Context
	cancel         context.CancelFunc
	pool           *Pool[func()]
	runtimesMu     sync.RWMutex
	runtimes       []*SubscriberRuntime
	subscribers    *lf.SkipList[string, *atomic.Pointer[[]*SubscriberRuntime]]
	shared         *lf.SkipList[string, any]
	signals        *lf.SkipList[string, *atomic.Pointer[[]func()]]
	failureHandler atomic.Pointer[func(error)]
	inFlight       atomic.Int64
}

func stringLess(first string, second string) bool {
	return first < second
}

func NewWorkspace(ctx context.Context) *Workspace {
	if ctx == nil {
		ctx = context.Background()
	}

	ctx, cancel := context.WithCancel(ctx)

	workerPool := NewPool[func()](func(task func()) {
		task()
	})
	workerPool.Start()

	workspace := &Workspace{
		ctx:         ctx,
		cancel:      cancel,
		pool:        workerPool,
		subscribers: lf.NewSkipList[string, *atomic.Pointer[[]*SubscriberRuntime]](stringLess),
		shared:      lf.NewSkipList[string, any](stringLess),
		signals:     lf.NewSkipList[string, *atomic.Pointer[[]func()]](stringLess),
	}

	return workspace
}

func (workspace *Workspace) Close() error {
	workspace.cancel()

	workspace.runtimesMu.Lock()
	for _, runtime := range workspace.runtimes {
		_ = runtime.Close()
	}
	workspace.runtimesMu.Unlock()

	if workspace.pool != nil {
		workspace.pool.Stop()
	}

	return nil
}

func (workspace *Workspace) SetFailureHandler(handler func(error)) {
	workspace.failureHandler.Store(&handler)
}

func (workspace *Workspace) WaitForQuiescence(args ...any) error {
	if workspace == nil {
		return nil
	}

	maxWait := 5 * time.Second

	for _, arg := range args {
		if duration, ok := arg.(time.Duration); ok && duration > 0 {
			maxWait = duration
		}
	}

	deadline := time.Now().Add(maxWait)

	for {
		if workspace.inFlight.Load() <= 0 {
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf(
				"workspace: quiescence timeout with %d events in flight",
				workspace.inFlight.Load(),
			)
		}

		time.Sleep(500 * time.Microsecond)
	}
}

func (workspace *Workspace) Publish(topic string, value any) {
	if subPtr, found := workspace.subscribers.Get(topic); found && subPtr != nil {
		subList := subPtr.Load()

		if subList != nil {
			for _, runtime := range *subList {
				workspace.inFlight.Add(1)
				runtime.Enqueue(value)
			}
		}
	}
}

func (workspace *Workspace) Wire(inTopic string, outTopic string, step func(any) any) {
	runtime := &SubscriberRuntime{
		workspace:  workspace,
		inTopic:    inTopic,
		outTopic:   outTopic,
		step:       step,
		ring:       make([]any, subscriberCapacity),
		bufferMask: subscriberMask,
	}

	handler := &subscriberDisruptorHandler{runtime: runtime}
	disruptorInstance, err := disruptor.New(
		disruptor.Options.BufferCapacity(subscriberCapacity),
		disruptor.Options.WriterCount(255),
		disruptor.Options.NewHandlerGroup(handler),
	)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Internal,
			"workspace: failed to initialize subscriber disruptor",
			err,
		))
	}

	runtime.disruptor = disruptorInstance
	go disruptorInstance.Listen()

	workspace.runtimesMu.Lock()
	workspace.runtimes = append(workspace.runtimes, runtime)
	workspace.runtimesMu.Unlock()

	ptr, found := workspace.subscribers.Get(inTopic)

	if !found || ptr == nil {
		newPtr := &atomic.Pointer[[]*SubscriberRuntime]{}
		workspace.subscribers.Set(inTopic, newPtr)
		ptr, _ = workspace.subscribers.Get(inTopic)
	}

	for {
		current := ptr.Load()
		var updated []*SubscriberRuntime

		if current == nil {
			updated = []*SubscriberRuntime{runtime}
		}

		if current != nil {
			updated = make([]*SubscriberRuntime, len(*current)+1)
			copy(updated, *current)
			updated[len(*current)] = runtime
		}

		if ptr.CompareAndSwap(current, &updated) {
			break
		}
	}
}

func WireNode[T any, U any](workspace *Workspace, inTopic string, outTopic string, node Node[T, U]) {
	workspace.Wire(inTopic, outTopic, func(input any) any {
		typedInput, ok := input.(T)

		if !ok {
			return nil
		}

		return node.Step(typedInput)
	})
}

func WireFunc[T any, U any](workspace *Workspace, inTopic string, outTopic string, step func(T) U) {
	workspace.Wire(inTopic, outTopic, func(input any) any {
		typedInput, ok := input.(T)

		if !ok {
			return nil
		}

		return step(typedInput)
	})
}

func sharedKey(name string, ids []string) string {
	key := fmt.Sprintf("%d:%s/", len(name), name)

	for _, id := range ids {
		if id == "" {
			continue
		}

		key += fmt.Sprintf("%d:%s/", len(id), id)
	}

	return key
}

func (workspace *Workspace) Share(name string, value any, id ...string) {
	key := sharedKey(name, id)
	workspace.shared.Set(key, value)
}

func (workspace *Workspace) Shared(name string, id ...string) (any, bool) {
	key := sharedKey(name, id)
	return workspace.shared.Get(key)
}

func (workspace *Workspace) On(signal string, handler func()) {
	ptr, found := workspace.signals.Get(signal)

	if !found || ptr == nil {
		newPtr := &atomic.Pointer[[]func()]{}
		workspace.signals.Set(signal, newPtr)
		ptr, _ = workspace.signals.Get(signal)
	}

	for {
		current := ptr.Load()
		var updated []func()

		if current == nil {
			updated = []func(){handler}
		}

		if current != nil {
			updated = make([]func(), len(*current)+1)
			copy(updated, *current)
			updated[len(*current)] = handler
		}

		if ptr.CompareAndSwap(current, &updated) {
			break
		}
	}
}

func (workspace *Workspace) Notify(signal string) {
	if ptr, found := workspace.signals.Get(signal); found && ptr != nil {
		handlers := ptr.Load()

		if handlers != nil {
			for _, handler := range *handlers {
				targetHandler := handler
				err := workspace.pool.AddTask(func() {
					targetHandler()
				})

				if err != nil {
					errnie.Error(errnie.Err(
						errnie.TooManyRequests,
						"workspace: failed to dispatch signal to pool",
						err,
					))
				}
			}
		}
	}
}

/*
WorkspaceSnapshot captures high-level queue metrics for telemetry and diagnostics.
*/
type WorkspaceSnapshot struct {
	Pending  uint64
	Capacity uint64
	Lanes    uint64
	Dropped  uint64
}

func (workspace *Workspace) Snapshot() WorkspaceSnapshot {
	if workspace == nil {
		return WorkspaceSnapshot{}
	}

	inFlight := workspace.inFlight.Load()

	if inFlight < 0 {
		inFlight = 0
	}

	workspace.runtimesMu.RLock()
	laneCount := uint64(len(workspace.runtimes))
	workspace.runtimesMu.RUnlock()

	return WorkspaceSnapshot{
		Pending:  uint64(inFlight),
		Capacity: uint64(subscriberCapacity),
		Lanes:    laneCount,
		Dropped:  0,
	}
}

func (workspace *Workspace) TopicSnapshot(topic string) WorkspaceSnapshot {
	if workspace == nil {
		return WorkspaceSnapshot{}
	}

	if subPtr, found := workspace.subscribers.Get(topic); found && subPtr != nil {
		subList := subPtr.Load()

		if subList != nil {
			pending := uint64(0)

			for _, runtime := range *subList {
				count := runtime.inFlight.Load()

				if count > 0 {
					pending += uint64(count)
				}
			}

			return WorkspaceSnapshot{
				Pending:  pending,
				Capacity: uint64(subscriberCapacity),
				Lanes:    uint64(len(*subList)),
				Dropped:  0,
			}
		}
	}

	return workspace.Snapshot()
}