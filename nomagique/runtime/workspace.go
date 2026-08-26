package runtime

import (
	"context"
	"fmt"
	"hash/fnv"
	"reflect"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/errnie"
	"golang.design/x/lockfree/lf"
)

const subscriberCapacity int64 = 64 * 1024

/*
Lane is one fixed, key-affine physical ring within a subscriber.
*/
type Lane struct {
	id      int
	ring    *Ring
	runtime *SubscriberRuntime
}

func (lane *Lane) start(ctx context.Context) {
	for {
		event, ok := lane.ring.WaitNext(ctx)
		if !ok {
			return
		}

		lane.runtime.workspace.acquireAnalyticsToken(ctx)
		lane.process(event)
		lane.runtime.workspace.releaseAnalyticsToken()
	}
}

func (lane *Lane) process(event RingEvent) {
	defer func() {
		if recovered := recover(); recovered != nil {
			var err error

			if panicErr, ok := recovered.(error); ok {
				err = fmt.Errorf("workspace: subscriber panic on %s (out: %s): %w\n%s", lane.runtime.inTopic, lane.runtime.outTopic, panicErr, string(debug.Stack()))
			}

			if err == nil {
				err = fmt.Errorf("workspace: subscriber panic on %s (out: %s): %v\n%s", lane.runtime.inTopic, lane.runtime.outTopic, recovered, string(debug.Stack()))
			}

			if failureFn := lane.runtime.workspace.failureHandler.Load(); failureFn != nil && *failureFn != nil {
				(*failureFn)(err)
			}

			errnie.Error(errnie.Err(
				errnie.Internal,
				"workspace: subscriber panic",
				err,
			))
		}
	}()

	started := time.Now()
	output := lane.runtime.step(event.Value)
	duration := time.Since(started)

	lane.runtime.stepDurationNanos.Add(duration.Nanoseconds())
	lane.runtime.stepCount.Add(1)
	lane.runtime.completedEvents.Add(1)

	if output != nil && lane.runtime.outTopic != "" {
		lane.runtime.workspace.Publish(lane.runtime.outTopic, output)
	}
}

/*
SubscriberRuntime represents an independently executing processing stage backed by its own bounded lanes.
*/
type SubscriberRuntime struct {
	id                uint32
	workspace         *Workspace
	inTopic           string
	outTopic          string
	keyFunc           func(any) string
	step              func(any) any
	lanes             []*Lane
	completedEvents   atomic.Uint64
	stepCount         atomic.Uint64
	stepDurationNanos atomic.Int64
}

func (runtime *SubscriberRuntime) Enqueue(value any) {
	key := runtime.keyFunc(value)
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(key))
	laneIndex := int(hasher.Sum32()) % len(runtime.lanes)

	lane := runtime.lanes[laneIndex]
	_ = lane.ring.Enqueue(value) // Enqueue never blocks, it overwrites oldest if full for observational.
}

func (runtime *SubscriberRuntime) Close() error {
	for _, lane := range runtime.lanes {
		lane.ring.Close()
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
	pool           *Pool[func()]
	nextRuntimeID  atomic.Uint32
	runtimesMu     sync.RWMutex
	runtimes       []*SubscriberRuntime
	subscribers    *lf.SkipList[string, *atomic.Pointer[[]*SubscriberRuntime]]
	shared         *lf.SkipList[string, any]
	signals        *lf.SkipList[string, *atomic.Pointer[[]func()]]
	failureHandler atomic.Pointer[func(error)]
	
	analyticsSem   chan struct{}
	laneCount      int
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

	if maxWorkers <= 0 {
		maxWorkers = 32 // reasonable default for analytical concurrency budget
	}
	
	laneCount := 16 // reasonable hash lanes per subscriber

	workspace := &Workspace{
		ctx:          ctx,
		cancel:       cancel,
		pool:         workerPool,
		subscribers:  lf.NewSkipList[string, *atomic.Pointer[[]*SubscriberRuntime]](stringLess),
		shared:       lf.NewSkipList[string, any](stringLess),
		signals:      lf.NewSkipList[string, *atomic.Pointer[[]func()]](stringLess),
		analyticsSem: make(chan struct{}, maxWorkers),
		laneCount:    laneCount,
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

func (workspace *Workspace) acquireAnalyticsToken(ctx context.Context) {
	select {
	case workspace.analyticsSem <- struct{}{}:
	case <-ctx.Done():
	}
}

func (workspace *Workspace) releaseAnalyticsToken() {
	select {
	case <-workspace.analyticsSem:
	default:
	}
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
		pending := int64(0)
		workspace.runtimesMu.RLock()
		for _, runtime := range workspace.runtimes {
			for _, lane := range runtime.lanes {
				pending += lane.ring.Occupancy()
			}
		}
		workspace.runtimesMu.RUnlock()
		
		if pending <= 0 {
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf(
				"workspace: quiescence timeout with %d events pending in rings",
				pending,
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
		lanes:      make([]*Lane, workspace.laneCount),
	}

	for i := 0; i < workspace.laneCount; i++ {
		lane := &Lane{
			id:      i,
			ring:    NewRing(subscriberCapacity, StreamObservational),
			runtime: runtime,
		}
		runtime.lanes[i] = lane
		go lane.start(workspace.ctx)
	}

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

	pending := uint64(0)
	dropped := uint64(0)
	lanes := uint64(0)
	
	workspace.runtimesMu.RLock()
	for _, runtime := range workspace.runtimes {
		lanes += uint64(len(runtime.lanes))
		for _, lane := range runtime.lanes {
			pending += uint64(lane.ring.Occupancy())
			dropped += uint64(lane.ring.Dropped())
		}
	}
	workspace.runtimesMu.RUnlock()

	return WorkspaceSnapshot{
		Pending:       pending,
		Capacity:      uint64(subscriberCapacity * int64(lanes)), // Total aggregate capacity across all lanes
		Lanes:         lanes,
		Dropped:       dropped,
		ActiveWorkers: uint64(cap(workspace.analyticsSem) - len(workspace.analyticsSem)), // rough estimate
		ActiveKeys:    0,
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
			dropped := uint64(0)
			lanes := uint64(0)

			for _, runtime := range *subList {
				lanes += uint64(len(runtime.lanes))
				for _, lane := range runtime.lanes {
					pending += uint64(lane.ring.Occupancy())
					dropped += uint64(lane.ring.Dropped())
				}
			}

			return WorkspaceSnapshot{
				Pending:  pending,
				Capacity: uint64(subscriberCapacity * int64(lanes)),
				Lanes:    lanes,
				Dropped:  dropped,
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