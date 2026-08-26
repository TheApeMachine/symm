package runtime

import (
	"context"
	"fmt"
	"reflect"
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
	id                uint32
	workspace         *Workspace
	inTopic           string
	outTopic          string
	keyFunc           func(any) string
	step              func(any) any
	ring              []any
	disruptor         disruptor.Disruptor
	bufferMask        int64
	inFlight          atomic.Int64
	executorPending   atomic.Int64
	completedEvents   atomic.Uint64
	stepCount         atomic.Uint64
	stepDurationNanos atomic.Int64
	queueWaitNanos    atomic.Int64
}

type subscriberDisruptorHandler struct {
	runtime *SubscriberRuntime
}

func (handler *subscriberDisruptorHandler) Handle(lowerSequence, upperSequence int64) {
	for sequence := lowerSequence; sequence <= upperSequence; sequence++ {
		value := handler.runtime.ring[sequence&handler.runtime.bufferMask]
		key := handler.runtime.keyFunc(value)
		handler.runtime.workspace.executor.Submit(handler.runtime, key, value)
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
DefaultKeyExtractor attempts to extract a partition/ordering key from value.
If none is identifiable, it returns "global".
*/
func DefaultKeyExtractor(value any) string {
	if value == nil {
		return "global"
	}

	switch item := value.(type) {
	case string:
		return item
	case interface{ ExecutionKey() string }:
		return item.ExecutionKey()
	case interface{ Symbol() string }:
		return item.Symbol()
	case interface{ GetSymbol() string }:
		return item.GetSymbol()
	}

	val := reflect.ValueOf(value)

	if val.Kind() == reflect.Pointer {
		val = val.Elem()
	}

	if val.IsValid() && val.Kind() == reflect.Slice && val.Len() > 0 {
		first := val.Index(0)

		if first.Kind() == reflect.Pointer {
			first = first.Elem()
		}

		if first.IsValid() && first.Kind() == reflect.Struct {
			if field := first.FieldByName("Symbol"); field.IsValid() && field.Kind() == reflect.String {
				if symbol := field.String(); symbol != "" {
					return symbol
				}
			}

			if field := first.FieldByName("Label"); field.IsValid() && field.Kind() == reflect.String {
				if label := field.String(); label != "" {
					return label
				}
			}
		}
	}

	if val.IsValid() && val.Kind() == reflect.Struct {
		if field := val.FieldByName("Symbol"); field.IsValid() && field.Kind() == reflect.String {
			if symbol := field.String(); symbol != "" {
				return symbol
			}
		}

		if field := val.FieldByName("Label"); field.IsValid() && field.Kind() == reflect.String {
			if label := field.String(); label != "" {
				return label
			}
		}
	}

	return "global"
}

/*
Workspace is the typed dataflow coordinator and routing substrate.
*/
type Workspace struct {
	ctx            context.Context
	cancel         context.CancelFunc
	executor       *KeyedExecutor
	pool           *Pool[func()]
	nextRuntimeID  atomic.Uint32
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
	return NewWorkspaceWithWorkers(ctx, 0)
}

func NewWorkspaceWithWorkers(ctx context.Context, maxWorkers int) *Workspace {
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
		executor:    NewKeyedExecutor(ctx, maxWorkers),
		pool:        workerPool,
		subscribers: lf.NewSkipList[string, *atomic.Pointer[[]*SubscriberRuntime]](stringLess),
		shared:      lf.NewSkipList[string, any](stringLess),
		signals:     lf.NewSkipList[string, *atomic.Pointer[[]func()]](stringLess),
	}

	return workspace
}

func (workspace *Workspace) Close() error {
	workspace.cancel()

	if workspace.executor != nil {
		_ = workspace.executor.Close()
	}

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

func (workspace *Workspace) Executor() *KeyedExecutor {
	return workspace.executor
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
		if workspace.inFlight.Load() <= 0 && (workspace.executor == nil || workspace.executor.Pending() <= 0) {
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf(
				"workspace: quiescence timeout with %d events in flight, %d executor pending",
				workspace.inFlight.Load(),
				workspace.executor.Pending(),
			)
		}

		time.Sleep(200 * time.Microsecond)
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
	workspace.WireWithKey(inTopic, outTopic, DefaultKeyExtractor, step)
}

func (workspace *Workspace) WireWithKey(
	inTopic string,
	outTopic string,
	keyFunc func(any) string,
	step func(any) any,
) {
	if keyFunc == nil {
		keyFunc = DefaultKeyExtractor
	}

	runtimeID := workspace.nextRuntimeID.Add(1)

	runtime := &SubscriberRuntime{
		id:         runtimeID,
		workspace:  workspace,
		inTopic:    inTopic,
		outTopic:   outTopic,
		keyFunc:    keyFunc,
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
	workspace.WireWithKey(inTopic, outTopic, DefaultKeyExtractor, func(input any) any {
		typedInput, ok := input.(T)

		if !ok {
			return nil
		}

		return node.Step(typedInput)
	})
}

func WireFunc[T any, U any](workspace *Workspace, inTopic string, outTopic string, step func(T) U) {
	workspace.WireWithKey(inTopic, outTopic, DefaultKeyExtractor, func(input any) any {
		typedInput, ok := input.(T)

		if !ok {
			return nil
		}

		return step(typedInput)
	})
}

func WireKeyed[T any, U any](
	workspace *Workspace,
	inTopic string,
	outTopic string,
	keyFunc func(T) string,
	step func(T) U,
) {
	WireKeyedFunc(workspace, inTopic, outTopic, keyFunc, step)
}

func WireKeyedFunc[T any, U any](
	workspace *Workspace,
	inTopic string,
	outTopic string,
	keyFunc func(T) string,
	step func(T) U,
) {
	wrappedKeyFunc := func(input any) string {
		if typedInput, ok := input.(T); ok && keyFunc != nil {
			return keyFunc(typedInput)
		}

		return DefaultKeyExtractor(input)
	}

	workspace.WireWithKey(inTopic, outTopic, wrappedKeyFunc, func(input any) any {
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
	Pending       uint64
	Capacity      uint64
	Lanes         uint64
	Dropped       uint64
	ActiveWorkers uint64
	ActiveKeys    uint64
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

	activeWorkers := uint64(0)
	activeKeys := uint64(0)

	if workspace.executor != nil {
		activeWorkers = uint64(workspace.executor.Workers())
		activeKeys = uint64(workspace.executor.ActiveKeys())
	}

	return WorkspaceSnapshot{
		Pending:       uint64(inFlight),
		Capacity:      uint64(subscriberCapacity),
		Lanes:         laneCount,
		Dropped:       0,
		ActiveWorkers: activeWorkers,
		ActiveKeys:    activeKeys,
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

	return WorkspaceSnapshot{
		Pending:  0,
		Capacity: uint64(subscriberCapacity),
		Lanes:    0,
		Dropped:  0,
	}
}