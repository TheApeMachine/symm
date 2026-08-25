package runtime

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/smarty/go-disruptor"
	"github.com/theapemachine/errnie"
	"golang.design/x/lockfree/lf"
)

const bufferCapacity uint32 = 64 * 1024
const bufferMask int64 = int64(bufferCapacity - 1)

type eventSlot struct {
	topic string
	value any
}

/*
ObserverFunc is the normalized callback for workspace observers.
*/
type ObserverFunc func(topic string, key string, value any)

func normalizeObserver(observer any) ObserverFunc {
	if observer == nil {
		return nil
	}

	if obs3, ok := observer.(func(string, string, any)); ok {
		return obs3
	}

	if obs2, ok := observer.(func(string, any)); ok {
		return func(topic string, _ string, value any) {
			obs2(topic, value)
		}
	}

	if obs1, ok := observer.(func(any)); ok {
		return func(_ string, _ string, value any) {
			obs1(value)
		}
	}

	if obs0, ok := observer.(ObserverFunc); ok {
		return obs0
	}

	return nil
}

/*
Subscriber represents a connected processing step wired to the Workspace.
*/
type Subscriber struct {
	InTopic  string
	OutTopic string
	Step     func(any) any
}

type workspaceDisruptorHandler struct {
	workspace *Workspace
}

func (handler *workspaceDisruptorHandler) Handle(lowerSequence, upperSequence int64) {
	for sequence := lowerSequence; sequence <= upperSequence; sequence++ {
		slot := handler.workspace.ring[sequence&bufferMask]
		handler.workspace.dispatch(slot.topic, slot.value)
	}
}

/*
Workspace is the system-wide lock-free streaming bus and data-routing substrate.
*/
type Workspace struct {
	ctx             context.Context
	cancel          context.CancelFunc
	pool            *Pool[func()]
	disruptor       disruptor.Disruptor
	ring            []eventSlot
	subscribers     *lf.SkipList[string, *atomic.Pointer[[]*Subscriber]]
	observers       *lf.SkipList[string, *atomic.Pointer[[]ObserverFunc]]
	globalObservers atomic.Pointer[[]ObserverFunc]
	shared          *lf.SkipList[string, any]
	signals         *lf.SkipList[string, *atomic.Pointer[[]func()]]
	failureHandler  atomic.Pointer[func(error)]
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
		ring:        make([]eventSlot, bufferCapacity),
		subscribers: lf.NewSkipList[string, *atomic.Pointer[[]*Subscriber]](stringLess),
		observers:   lf.NewSkipList[string, *atomic.Pointer[[]ObserverFunc]](stringLess),
		shared:      lf.NewSkipList[string, any](stringLess),
		signals:     lf.NewSkipList[string, *atomic.Pointer[[]func()]](stringLess),
	}

	handler := &workspaceDisruptorHandler{workspace: workspace}
	disruptorInstance, err := disruptor.New(
		disruptor.Options.BufferCapacity(bufferCapacity),
		disruptor.Options.WriterCount(255),
		disruptor.Options.NewHandlerGroup(handler),
	)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Internal,
			"workspace: failed to initialize disruptor",
			err,
		))
	}

	workspace.disruptor = disruptorInstance
	go disruptorInstance.Listen()

	return workspace
}

func (workspace *Workspace) Close() error {
	workspace.cancel()

	if workspace.disruptor != nil {
		_ = workspace.disruptor.Close()
	}

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

	time.Sleep(20 * time.Millisecond)
	return nil
}

func (workspace *Workspace) Publish(topic string, value any) {
	sequence := workspace.disruptor.Reserve(1)
	workspace.ring[sequence&bufferMask] = eventSlot{
		topic: topic,
		value: value,
	}
	workspace.disruptor.Commit(sequence, sequence)
}

func (workspace *Workspace) dispatch(topic string, value any) {
	globalList := workspace.globalObservers.Load()

	if globalList != nil {
		for _, observer := range *globalList {
			observer(topic, "", value)
		}
	}

	if obsPtr, found := workspace.observers.Get(topic); found && obsPtr != nil {
		topicList := obsPtr.Load()

		if topicList != nil {
			for _, observer := range *topicList {
				observer(topic, "", value)
			}
		}
	}

	if subPtr, found := workspace.subscribers.Get(topic); found && subPtr != nil {
		subList := subPtr.Load()

		if subList != nil {
			for _, subscriber := range *subList {
				output := subscriber.Step(value)

				if output != nil && subscriber.OutTopic != "" {
					workspace.Publish(subscriber.OutTopic, output)
				}
			}
		}
	}
}

func (workspace *Workspace) Wire(inTopic string, outTopic string, step func(any) any) {
	subscriber := &Subscriber{
		InTopic:  inTopic,
		OutTopic: outTopic,
		Step:     step,
	}

	ptr, found := workspace.subscribers.Get(inTopic)

	if !found || ptr == nil {
		newPtr := &atomic.Pointer[[]*Subscriber]{}
		workspace.subscribers.Set(inTopic, newPtr)
		ptr, _ = workspace.subscribers.Get(inTopic)
	}

	for {
		current := ptr.Load()
		var updated []*Subscriber

		if current == nil {
			updated = []*Subscriber{subscriber}
		}

		if current != nil {
			updated = make([]*Subscriber, len(*current)+1)
			copy(updated, *current)
			updated[len(*current)] = subscriber
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

func (workspace *Workspace) Observe(topic string, observer any) {
	normalized := normalizeObserver(observer)

	if normalized == nil {
		return
	}

	ptr, found := workspace.observers.Get(topic)

	if !found || ptr == nil {
		newPtr := &atomic.Pointer[[]ObserverFunc]{}
		workspace.observers.Set(topic, newPtr)
		ptr, _ = workspace.observers.Get(topic)
	}

	for {
		current := ptr.Load()
		var updated []ObserverFunc

		if current == nil {
			updated = []ObserverFunc{normalized}
		}

		if current != nil {
			updated = make([]ObserverFunc, len(*current)+1)
			copy(updated, *current)
			updated[len(*current)] = normalized
		}

		if ptr.CompareAndSwap(current, &updated) {
			break
		}
	}
}

func (workspace *Workspace) ObserveAll(observer any) {
	normalized := normalizeObserver(observer)

	if normalized == nil {
		return
	}

	for {
		current := workspace.globalObservers.Load()
		var updated []ObserverFunc

		if current == nil {
			updated = []ObserverFunc{normalized}
		}

		if current != nil {
			updated = make([]ObserverFunc, len(*current)+1)
			copy(updated, *current)
			updated[len(*current)] = normalized
		}

		if workspace.globalObservers.CompareAndSwap(current, &updated) {
			break
		}
	}
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

	return WorkspaceSnapshot{
		Pending:  0,
		Capacity: uint64(bufferCapacity),
		Lanes:    1,
		Dropped:  0,
	}
}

func (workspace *Workspace) TopicSnapshot(topic string) WorkspaceSnapshot {
	return workspace.Snapshot()
}