package runtime

import (
	"strings"
	"context"
	"fmt"
	"hash/fnv"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	disruptor "github.com/smarty/go-disruptor"
	"github.com/theapemachine/errnie"
)

/*
subscriberCapacity is the fixed, power-of-two slot count of the one physical
LMAX Disruptor owned by a logical subscriber. It is never multiplied by the
handler count and never grows at runtime.
*/
const subscriberCapacity uint32 = 64 * 1024

// SubscriberCapacity is the fixed slot count of every subscriber's physical LMAX
// Disruptor. It is exported so diagnostics can report ring capacity as a stable
// infrastructure constant even before the first subscriber registers.
const SubscriberCapacity = uint64(subscriberCapacity)

const subscriberCapacityMask = int64(subscriberCapacity) - 1

const globalKey = "global"

/*
StepDurationBuckets splits Step latency into the histogram buckets diagnostics
uses. p50/p95/p99 and max are derived from these counts on demand.
*/
var StepDurationBuckets = []float64{
	1e3, 5e3, 1e4, 2.5e4, 5e4, 1e5, 2.5e5, 5e5,
	1e6, 2.5e6, 5e6, 1e7, 2.5e7, 5e7, 1e8,
}

/*
ServiceClass is the explicit scheduling class every subscription declares. Only
Analytics executes under the analytical semaphore; PriorityControl, Realtime,
and UI always skip it.
*/
type ServiceClass int

const (
	ServicePriorityControl ServiceClass = iota
	ServiceRealtime
	ServiceAnalytics
	ServiceUI
)

func (class ServiceClass) String() string {
	switch class {
	case ServicePriorityControl:
		return "priority_control"
	case ServiceRealtime:
		return "realtime"
	case ServiceAnalytics:
		return "analytics"
	case ServiceUI:
		return "ui"
	}

	return "unknown"
}

/*
DeliveryPolicy is the per-edge publication policy every subscription declares
explicitly. There is no implicit default; callers must choose.
*/
type DeliveryPolicy int

const (
	DeliveryReliableFIFO DeliveryPolicy = iota
	DeliveryObservationalFIFO
	DeliveryLatestByKey
	DeliveryPriorityFIFO
)

func (policy DeliveryPolicy) String() string {
	switch policy {
	case DeliveryReliableFIFO:
		return "reliable_fifo"
	case DeliveryObservationalFIFO:
		return "observational_fifo"
	case DeliveryLatestByKey:
		return "latest_by_key"
	case DeliveryPriorityFIFO:
		return "priority_fifo"
	}

	return "unknown"
}

/*
Event is the payload stored in a physical Disruptor slot together with its
publication sequence. Sequence is the LMAX sequence, not a synthetic index.
*/
type Event struct {
	Sequence int64
	Value    any
}

/*
ringBuffer is the contiguous slot array a Disruptor writes into and handlers
read from. It is allocated once, sized exactly to subscriberCapacity, and never
grows. The Disruptor is the queue; this array is only the storage it manages.
*/
type ringBuffer [subscriberCapacity]Event

/*
StepKind distinguishes a value-producing Step from a destructively-typed Step
that returns an error, used only for the typed generic helpers.
*/
type StepKind int

const (
	kindStep StepKind = iota
	kindStepFunc
)

/*
SubscriberWire captures registration-time knowledge: explicit key extractor and
step implementations, immutable class, delivery policy, and topic edges. The
extractor resolves `any` at registration, so there is no reflection on the hot
publication path.
*/
type SubscriberWire struct {
	InTopic  string
	OutTopic string
	Class    ServiceClass
	Delivery DeliveryPolicy
	Keyed    bool

	keyFunc  func(any) string
	step     func(any) any
	descStep func(any) error
	stepKind StepKind

	// keyLimit optionally bounds the LatestByKey cell cardinality. Zero means
	// the configured infrastructure default applies.
	keyLimit int
}

/*
Subscriber is one logical subscriber: exactly one physical LMAX Disruptor and
one handler group of long-lived, key-affine handlers. It never owns multiple
rings and never admits a queue after the ring.
*/
type Subscriber struct {
	id        uint32
	name      string
	workspace *Workspace
	wire      SubscriberWire

	disruptor  disruptor.Disruptor
	buffer     *ringBuffer
	handlers   []*keyedHandler
	handlerCnt int

	// reliableRequested tracks that at least one reliable edge exists so
	// quiescence can distinguish "drained" from "never had work".
	started atomic.Bool

	// latest-state cells: fixed per-key current value plus an outstanding dirty
	// notification flag. The cells are bounded state, never a work queue.
	latestMu     sync.Mutex
	latestDirty  map[string]bool
	latestCells  map[string]*atomic.Pointer[any]

	// telemetry
	published      atomic.Uint64
	completed      atomic.Uint64
	tryReserveFmt  atomic.Uint64
	dropped        atomic.Uint64
	lastDrop       atomic.Int64
	typeMismatch   atomic.Uint64
	stepCount      atomic.Uint64
	stepTotalNanos atomic.Int64
	stepMaxNanos   atomic.Int64
	activeHandlers atomic.Int64
	lastComplete   atomic.Int64
}

/*
keyedHandler is one long-lived handler inside a subscriber's single handler
group. Every input has an explicit execution key; only the handler whose stable
lane identity matches hash(key) % handlerCount executes Step for that event. All
other handlers acknowledge/skip cheaply.
*/
type keyedHandler struct {
	subscriber *Subscriber
	id         int
}

func (handler *keyedHandler) Handle(lower, upper int64) {
	subscriber := handler.subscriber
	handlerCount := subscriber.handlerCnt

	subscriber.activeHandlers.Add(1)
	defer subscriber.activeHandlers.Add(-1)

	for sequence := lower; sequence <= upper; sequence++ {
		event := &subscriber.buffer[sequence&subscriberCapacityMask]

		if ownerOf(event.Value, subscriber.wire.keyFunc, handlerCount) != handler.id {
			continue
		}

		subscriber.execute(event)
	}
}

/*
ownerOf resolves hash(key) % handlerCount for an event's registered key. The
empty key collapses to the unkeyed/global lane.
*/
func ownerOf(value any, keyFunc func(any) string, handlerCount int) int {
	if handlerCount <= 1 {
		return 0
	}

	if keyFunc == nil {
		return 0
	}

	key := keyFunc(value)
	if key == "" {
		key = globalKey
	}

	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(key))

	return int(hasher.Sum32()) % handlerCount
}

/*
execute runs one Step for one owned event inside its owning handler. The
analytical token (Analytics class only) is acquired around Step and released
before any downstream publication, so a full downstream ring can never deadlock
against a downstream subscriber waiting for analytical CPU. The release is
guarded by the released flag so a Step panic — caught by the deferred recover —
can never leak the permit and silently deadlock the analytics chain.
*/
func (subscriber *Subscriber) execute(event *Event) {
	value := event.Value

	if subscriber.wire.Delivery == DeliveryLatestByKey {
		resolved := subscriber.resolveLatest(value)
		if resolved == nil {
			return
		}

		value = resolved
	}

	released := false
	acquired := false

	defer func() {
		if acquired && !released {
			subscriber.workspace.releaseAnalyticsToken()
		}

		if recovered := recover(); recovered != nil {
			subscriber.reportPanic(recovered)
		}
	}()

	started := time.Now()

	if subscriber.wire.Class == ServiceAnalytics {
		acquired = subscriber.workspace.acquireAnalyticsToken()
	}

	var output any

	switch subscriber.wire.stepKind {
	case kindStepFunc:
		if err := subscriber.wire.descStep(value); err != nil {
			subscriber.typeMismatch.Add(1)
			subscriber.workspace.reportFailure(err)
		}
	case kindStep:
		output = subscriber.wire.step(value)
	}

	if acquired {
		subscriber.workspace.releaseAnalyticsToken()
		released = true
	}

	subscriber.recordLatency(time.Since(started))
	subscriber.completed.Add(1)
	subscriber.lastComplete.Store(time.Now().UnixNano())

	if output != nil && subscriber.wire.OutTopic != "" {
		subscriber.workspace.publish(subscriber.wire.OutTopic, output)
	}
}

func (subscriber *Subscriber) reportPanic(recovered any) {
	var failure error

	switch panicErr := recovered.(type) {
	case error:
		failure = fmt.Errorf(
			"workspace: subscriber panic on %s (out: %s): %w\n%s",
			subscriber.wire.InTopic,
			subscriber.wire.OutTopic,
			panicErr,
			string(debug.Stack()),
		)
	default:
		failure = fmt.Errorf(
			"workspace: subscriber panic on %s (out: %s): %v\n%s",
			subscriber.wire.InTopic,
			subscriber.wire.OutTopic,
			recovered,
			string(debug.Stack()),
		)
	}

	subscriber.workspace.reportFailure(failure)
	errnie.Error(errnie.Err(errnie.Internal, "workspace: subscriber panic", failure))
}

func (subscriber *Subscriber) recordLatency(duration time.Duration) {
	nanos := duration.Nanoseconds()
	subscriber.stepCount.Add(1)
	subscriber.stepTotalNanos.Add(nanos)

	for {
		maxSeen := subscriber.stepMaxNanos.Load()
		if nanos <= maxSeen || subscriber.stepMaxNanos.CompareAndSwap(maxSeen, nanos) {
			break
		}
	}
}

/*
pendingList is copy-on-write downstream edge registration. The slice is replaced
atomically on mutation; the hot publication path only Loads it.
*/
type pendingList struct {
	pointer atomic.Pointer[[]*Subscriber]
}

func (list *pendingList) load() []*Subscriber {
	if list == nil {
		return nil
	}

	loaded := list.pointer.Load()
	if loaded == nil {
		return nil
	}

	return *loaded
}

func (list *pendingList) append(target *Subscriber) {
	if list == nil {
		return
	}

	for {
		current := list.pointer.Load()

		var next []*Subscriber

		if current == nil {
			next = []*Subscriber{target}
		} else {
			next = make([]*Subscriber, len(*current)+1)
			copy(next, *current)
			next[len(*current)] = target
		}

		if list.pointer.CompareAndSwap(current, &next) {
			return
		}
	}
}

/*
Workspace is SYMM's real-time streaming execution fabric. SYMM is a stream
end-to-end: the LMAX Disruptor ring is the queue, and there is no queue after
the ring.
*/
type Workspace struct {
	ctx    context.Context
	cancel context.CancelFunc

	analyticsSem chan struct{}
	handlerCount int

	nextSubscriberID atomic.Uint32
	subscribersMu    sync.RWMutex
	subscribers      []*Subscriber

	routerMu sync.RWMutex
	router   map[string]*pendingList

	sharedMu sync.Mutex
	shared   map[string]any

	signalsMu sync.RWMutex
	signals   map[string][]func()

	failureHandler atomic.Pointer[func(error)]

	// latestKeyLimit bounds LatestByKey cell cardinality; zero means unbounded
	// growth is allowed only for UI/diagnostics streams whose universe is fixed.
}

func NewWorkspace(ctx context.Context) *Workspace {
	if ctx == nil {
		ctx = context.Background()
	}

	workspaceCtx, cancel := context.WithCancel(ctx)
	handlerCount := runtime.GOMAXPROCS(0)
	if handlerCount < 1 {
		handlerCount = 1
	}

	return &Workspace{
		ctx:          workspaceCtx,
		cancel:       cancel,
		analyticsSem: make(chan struct{}, handlerCount),
		handlerCount: handlerCount,
		router:       make(map[string]*pendingList),
		shared:       make(map[string]any),
		signals:      make(map[string][]func()),
	}
}

func (workspace *Workspace) acquireAnalyticsToken() bool {
	if workspace == nil || workspace.analyticsSem == nil {
		return false
	}

	select {
	case workspace.analyticsSem <- struct{}{}:
		return true
	case <-workspace.ctx.Done():
		return false
	}
}

func (workspace *Workspace) releaseAnalyticsToken() {
	if workspace == nil || workspace.analyticsSem == nil {
		return
	}

	select {
	case <-workspace.analyticsSem:
	default:
	}
}

func (workspace *Workspace) SetFailureHandler(handler func(error)) {
	if workspace == nil {
		return
	}

	workspace.failureHandler.Store(&handler)
}

func (workspace *Workspace) reportFailure(failure error) {
	if workspace == nil || failure == nil {
		return
	}

	if handler := workspace.failureHandler.Load(); handler != nil && *handler != nil {
		(*handler)(failure)
	}
}

/*
Close stops publication by cancelling the workspace context and closes every
subscriber's Disruptor so handler goroutines drain and exit.
*/
func (workspace *Workspace) Close() error {
	if workspace == nil {
		return nil
	}

	workspace.cancel()

	workspace.subscribersMu.Lock()
	subscribers := workspace.subscribers
	workspace.subscribers = nil
	workspace.subscribersMu.Unlock()

	for _, subscriber := range subscribers {
		if subscriber != nil && subscriber.disruptor != nil {
			_ = subscriber.disruptor.Close()
		}
	}

	return nil
}

/*
Wire registers an unkeyed/global subscriber with the default Analytics class and
ObservationalFIFO delivery. Global subscribers explicitly declare key = global.
*/
func (workspace *Workspace) Wire(inTopic string, outTopic string, step func(any) any) {
	workspace.register(SubscriberWire{
		InTopic:  inTopic,
		OutTopic: outTopic,
		Class:    ServiceAnalytics,
		Delivery: DeliveryObservationalFIFO,
		keyFunc:  func(any) string { return globalKey },
		step:     step,
		stepKind: kindStep,
	})
}

/*
WireFunc registers a destructively-typed subscriber with the default Analytics
class and ObservationalFIFO delivery.
*/
func WireFunc[T any, U any](workspace *Workspace, inTopic string, outTopic string, step func(T) U) {
	if workspace == nil {
		return
	}

	workspace.register(SubscriberWire{
		InTopic:  inTopic,
		OutTopic: outTopic,
		Class:    ServiceAnalytics,
		Delivery: DeliveryObservationalFIFO,
		keyFunc:  func(any) string { return globalKey },
		step:     func(input any) any { return step(input.(T)) },
		stepKind: kindStep,
	})
}

func WireNode[T any, U any](workspace *Workspace, inTopic string, outTopic string, node Node[T, U]) {
	if workspace == nil {
		return
	}

	WireFunc[T, U](workspace, inTopic, outTopic, node.Step)
}

/*
WireKeyed registers a keyed subscriber with an explicit, registration-time key
extractor. No reflection happens on the publication path.
*/
func WireKeyed[T any, U any](
	workspace *Workspace,
	inTopic string,
	outTopic string,
	keyFunc func(T) string,
	step func(T) U,
) {
	if workspace == nil {
		return
	}

	if keyFunc == nil {
		keyFunc = func(T) string { return globalKey }
	}

	workspace.register(SubscriberWire{
		InTopic:  inTopic,
		OutTopic: outTopic,
		Class:    ServiceAnalytics,
		Delivery: DeliveryObservationalFIFO,
		Keyed:    true,
		keyFunc:  func(input any) string { return keyFunc(input.(T)) },
		step:     func(input any) any { return step(input.(T)) },
		stepKind: kindStep,
	})
}

/*
WireKeyedFunc is retained for source compatibility with existing callers.
*/
func WireKeyedFunc[T any, U any](
	workspace *Workspace,
	inTopic string,
	outTopic string,
	keyFunc func(T) string,
	step func(T) U,
) {
	WireKeyed(workspace, inTopic, outTopic, keyFunc, step)
}

/*
WireWithKey registers a subscriber with an explicit `any` key extractor and an
already-boxed step, preserving the pre-v3 dynamic API.
*/
func (workspace *Workspace) WireWithKey(
	inTopic string,
	outTopic string,
	keyFunc func(any) string,
	step func(any) any,
) {
	isKeyed := keyFunc != nil

	if keyFunc == nil {
		keyFunc = func(any) string { return globalKey }
	}

	workspace.register(SubscriberWire{
		InTopic:  inTopic,
		OutTopic: outTopic,
		Class:    ServiceAnalytics,
		Delivery: DeliveryObservationalFIFO,
		Keyed:    isKeyed,
		keyFunc:  keyFunc,
		step:     step,
		stepKind: kindStep,
	})
}

/*
WireClass registers a fully-specified subscriber: explicit service class,
delivery policy, key extractor, and step.
*/
func (workspace *Workspace) WireClass(
	inTopic string,
	outTopic string,
	class ServiceClass,
	delivery DeliveryPolicy,
	keyFunc func(any) string,
	step func(any) any,
) {
	isKeyed := keyFunc != nil

	if keyFunc == nil {
		keyFunc = func(any) string { return globalKey }
	}

	workspace.register(SubscriberWire{
		InTopic:  inTopic,
		OutTopic: outTopic,
		Class:    class,
		Delivery: delivery,
		Keyed:    isKeyed,
		keyFunc:  keyFunc,
		step:     step,
		stepKind: kindStep,
	})
}

/*
WireClassStepFunc registers a destructive subscriber with an explicit class and
delivery whose step reports a descriptive error on type mismatch.
*/
func (workspace *Workspace) WireClassStepFunc(
	inTopic string,
	outTopic string,
	class ServiceClass,
	delivery DeliveryPolicy,
	keyFunc func(any) string,
	step func(any) error,
) {
	isKeyed := keyFunc != nil

	if keyFunc == nil {
		keyFunc = func(any) string { return globalKey }
	}

	workspace.register(SubscriberWire{
		InTopic:  inTopic,
		OutTopic: outTopic,
		Class:    class,
		Delivery: delivery,
		Keyed:    isKeyed,
		keyFunc:  keyFunc,
		descStep: step,
		stepKind: kindStepFunc,
	})
}

/*
AdaptiveWaitStrategy provides low-latency spin during active traffic and backoff
sleep during idle periods, preventing CPU saturation across subscriber rings.
*/
type AdaptiveWaitStrategy struct{}

func (AdaptiveWaitStrategy) Gate(count int64) {
	if count < 100 {
		runtime.Gosched()
		return
	}

	time.Sleep(10 * time.Microsecond)
}

func (AdaptiveWaitStrategy) Idle(count int64) {
	if count < 100 {
		runtime.Gosched()
		return
	}

	if count < 1000 {
		time.Sleep(20 * time.Microsecond)
		return
	}

	time.Sleep(100 * time.Microsecond)
}

func (AdaptiveWaitStrategy) Reserve(count int64) {
	if count < 50 {
		runtime.Gosched()
		return
	}

	time.Sleep(10 * time.Microsecond)
}

/*
register constructs exactly one Subscriber with exactly one physical Disruptor
and one handler group of handlerCount long-lived keyed handlers. It is the only
place a Disruptor is created; a subscriber never gets more than one.
*/
func (workspace *Workspace) register(wire SubscriberWire) *Subscriber {
	handlerCount := workspace.handlerCount

	if wire.Delivery == DeliveryLatestByKey || !wire.Keyed {
		// Latest-state consumers render per key and coalesce, so a single
		// consumer sequence is sufficient and avoids fanning one render across
		// keyed handlers that would still need to be serialized per key.
		// Unkeyed / single-lane subscribers also only use a single handler
		// because all events route to lane 0.
		handlerCount = 1
	}

	if handlerCount < 1 {
		handlerCount = 1
	}

	handlers := make([]*keyedHandler, handlerCount)
	group := make([]disruptor.Handler, handlerCount)

	for index := 0; index < handlerCount; index++ {
		handlers[index] = &keyedHandler{id: index}
		group[index] = handlers[index]
	}

	buffer := new(ringBuffer)
	disruptorInstance, err := disruptor.New(
		disruptor.Options.BufferCapacity(subscriberCapacity),
		disruptor.Options.WriterCount(64),
		disruptor.Options.WaitStrategy(AdaptiveWaitStrategy{}),
		disruptor.Options.NewHandlerGroup(group...),
	)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Internal,
			"workspace: failed to construct disruptor for "+wire.InTopic,
			err,
		))

		return nil
	}

	subscriberID := workspace.nextSubscriberID.Add(1)
	subscriber := &Subscriber{
		id:     subscriberID,
		name:   wire.InTopic,
		workspace: workspace,
		wire:        wire,
		disruptor:   disruptorInstance,
		buffer:      buffer,
		handlers:    handlers,
		handlerCnt:  handlerCount,
		latestDirty: make(map[string]bool),
		latestCells: make(map[string]*atomic.Pointer[any]),
	}

	for _, handler := range handlers {
		handler.subscriber = subscriber
	}

	go disruptorInstance.Listen()
	subscriber.started.Store(true)

	workspace.subscribersMu.Lock()
	workspace.subscribers = append(workspace.subscribers, subscriber)
	workspace.subscribersMu.Unlock()

	workspace.routerMu.Lock()
	list := workspace.router[wire.InTopic]
	if list == nil {
		list = &pendingList{}
		workspace.router[wire.InTopic] = list
	}
	workspace.routerMu.Unlock()

	list.append(subscriber)

	return subscriber
}

/*
Publish fans a value out to every eligible subscriber on a topic, following each
subscriber's declared delivery policy. This is the entire hot path: copy-on-write
list load, no global mutex, no reflection, no routing rebuild.
*/
func (workspace *Workspace) Publish(topic string, value any) {
	workspace.publish(topic, value)
}

func (workspace *Workspace) publish(topic string, value any) {
	workspace.routerMu.RLock()
	list := workspace.router[topic]
	workspace.routerMu.RUnlock()

	if list == nil {
		return
	}

	for _, subscriber := range list.load() {
		subscriber.publish(value)
	}
}

func (subscriber *Subscriber) publish(value any) {
	switch subscriber.wire.Delivery {
	case DeliveryLatestByKey:
		subscriber.publishLatest(value)
	case DeliveryReliableFIFO, DeliveryPriorityFIFO:
		subscriber.publishReliable(value)
	case DeliveryObservationalFIFO:
		subscriber.publishObservational(value)
	}
}

/*
publishObservational uses the library's native TryReserve: a non-blocking
reservation that reports ErrCapacityUnavailable when consumers have not advanced
far enough. On failure it records the drop explicitly; it never blocks and never
creates another queue.
*/
func (subscriber *Subscriber) publishObservational(value any) {
	sequence := subscriber.disruptor.TryReserve(1)
	if sequence < 0 {
		subscriber.dropped.Add(1)
		subscriber.tryReserveFmt.Add(1)
		subscriber.lastDrop.Store(time.Now().UnixNano())
		return
	}

	subscriber.buffer[sequence&subscriberCapacityMask] = Event{Sequence: sequence, Value: value}
	subscriber.disruptor.Commit(sequence, sequence)
	subscriber.published.Add(1)
}

/*
publishReliable uses the library's blocking Reserve: backpressure is real,
visible, and bounded. No event is dropped and no second queue evades it.
*/
func (subscriber *Subscriber) publishReliable(value any) {
	sequence := subscriber.disruptor.Reserve(1)
	if sequence < 0 {
		subscriber.workspace.reportFailure(errnie.Err(
			errnie.NotAcceptable,
			"workspace: reliable reservation failed",
			nil,
		))

		return
	}

	subscriber.buffer[sequence&subscriberCapacityMask] = Event{Sequence: sequence, Value: value}
	subscriber.disruptor.Commit(sequence, sequence)
	subscriber.published.Add(1)
}

/*
publishLatest implements LatestByKey using fixed per-key state plus LMAX dirty
notifications. The latest cell is replaced on every update; if the key is already
dirty no further notification is emitted, so there is at most one outstanding
notification per key. The Disruptor carries only the dirty key token.
*/
func (subscriber *Subscriber) publishLatest(value any) {
	key := subscriber.wire.keyFunc(value)
	if key == "" {
		key = globalKey
	}

	subscriber.latestMu.Lock()

	cell, exists := subscriber.latestCells[key]
	if !exists {
		limit := subscriber.wire.keyLimit
		if limit <= 0 {
			limit = 640
		}

		if len(subscriber.latestCells) >= limit {
			subscriber.workspace.reportFailure(errnie.Err(
				errnie.TooManyRequests,
				fmt.Sprintf("workspace: latest-state key cardinality exceeds limit %d", limit),
				nil,
			))

			subscriber.latestMu.Unlock()
			return
		}

		subscriber.latestCells[key] = &atomic.Pointer[any]{}
		subscriber.latestDirty[key] = false
		cell = subscriber.latestCells[key]
	}

	cell.Store(&value)
	alreadyDirty := subscriber.latestDirty[key]
	subscriber.latestDirty[key] = true

	subscriber.latestMu.Unlock()

	if alreadyDirty {
		return
	}

	// Publish only the dirty key token through LMAX; the payload stays in the
	// fixed latest cell and never queues.
	token := key
	subscriber.publishReliable(token)
}

/*
resolveLatest drains the fixed latest cell for a dirty key and clears the dirty
flag. It returns nil (skip) if the cell was never populated.
*/
func (subscriber *Subscriber) resolveLatest(value any) any {
	key, ok := value.(string)
	if !ok {
		key = globalKey
	}

	subscriber.latestMu.Lock()
	cell, exists := subscriber.latestCells[key]
	if exists {
		subscriber.latestDirty[key] = false
	}
	subscriber.latestMu.Unlock()

	if !exists {
		return nil
	}

	loaded := cell.Load()
	if loaded == nil {
		return nil
	}

	return *loaded
}

/*
WaitForQuiescence blocks until every reliable stream has drained (committed
sequences consumed and no handler executing) and every latest-state stream has
no dirty keys pending. It uses Disruptor progress, never sleep-based guessing.
*/
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
		if workspace.quiescent() {
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("workspace: quiescence timeout")
		}

		time.Sleep(200 * time.Microsecond)
	}
}

func (workspace *Workspace) quiescent() bool {
	workspace.subscribersMu.RLock()
	subscribers := workspace.subscribers
	workspace.subscribersMu.RUnlock()

	for _, subscriber := range subscribers {
		if subscriber == nil {
			continue
		}

		if subscriber.activeHandlers.Load() > 0 {
			return false
		}

		if subscriber.wire.Delivery == DeliveryLatestByKey {
			subscriber.latestMu.Lock()
			hasDirty := false

			for _, dirty := range subscriber.latestDirty {
				if dirty {
					hasDirty = true
					break
				}
			}

			subscriber.latestMu.Unlock()

			if hasDirty {
				return false
			}
		}

		// A reliable/observational subscriber is drained when published equals
		// completed; the Disruptor sequence tracking makes this the actual
		// committed-vs-completed measure, never a software backlog guess.
		if subscriber.published.Load() != subscriber.completed.Load() {
			return false
		}
	}

	return true
}

/*
Snapshot is the per-subscriber telemetry record. Capacity is the physical LMAX
ring capacity; it never exceeds that and never reflects any hidden pending work.
*/
type Snapshot struct {
	Name           string
	InTopic        string
	OutTopic       string
	ServiceClass   string
	Delivery       string
	Capacity       uint64
	Published      uint64
	Completed      uint64
	Dropped        uint64
	TryReserveFail uint64
	TypeMismatch   uint64
	StepCalls      uint64
	StepTotalNanos int64
	StepMaxNanos   int64
	HandlerCount   int
	ActiveHandlers uint64
	LastDropUnixNano int64

	// Legacy diagnostics fields. Pending is derived from actual Disruptor
	// progress (published minus completed), never a software backlog; Lanes is
	// the count of key-affine handlers, not the number of physical rings.
	Pending uint64
	Lanes   uint64
}

/*
WorkspaceSnapshot is the compatibility name the diagnostics layer uses. It is
an alias of Snapshot; its fields are ring-meaningful, never exceeding physical
ring capacity for a single logical subscriber.
*/
type WorkspaceSnapshot = Snapshot

func (subscriber *Subscriber) snapshot() Snapshot {
	published := subscriber.published.Load()
	completed := subscriber.completed.Load()

	var pending uint64
	if published > completed {
		pending = published - completed
	}

	return Snapshot{
		Name:             subscriber.name,
		InTopic:          subscriber.wire.InTopic,
		OutTopic:         subscriber.wire.OutTopic,
		ServiceClass:     subscriber.wire.Class.String(),
		Delivery:         subscriber.wire.Delivery.String(),
		Capacity:         uint64(subscriberCapacity),
		Published:        published,
		Completed:        completed,
		Dropped:          subscriber.dropped.Load(),
		TryReserveFail:   subscriber.tryReserveFmt.Load(),
		TypeMismatch:     subscriber.typeMismatch.Load(),
		StepCalls:        subscriber.stepCount.Load(),
		StepTotalNanos:   subscriber.stepTotalNanos.Load(),
		StepMaxNanos:     subscriber.stepMaxNanos.Load(),
		HandlerCount:     subscriber.handlerCnt,
		ActiveHandlers:   uint64(subscriber.activeHandlers.Load()),
		LastDropUnixNano: subscriber.lastDrop.Load(),
		Pending:          pending,
		Lanes:            uint64(subscriber.handlerCnt),
	}
}

/*
Snapshots returns per-subscriber telemetry for every registered subscriber.
*/
func (workspace *Workspace) Snapshots() []Snapshot {
	if workspace == nil {
		return nil
	}

	workspace.subscribersMu.RLock()
	defer workspace.subscribersMu.RUnlock()

	snapshots := make([]Snapshot, 0, len(workspace.subscribers))
	for _, subscriber := range workspace.subscribers {
		snapshots = append(snapshots, subscriber.snapshot())
	}

	return snapshots
}

/*
TopicSnapshot aggregates per-subscriber telemetry for one topic so the existing
diagnostics layer keeps its shape.
*/
func (workspace *Workspace) TopicSnapshot(topic string) Snapshot {
	if workspace == nil {
		return Snapshot{}
	}

	workspace.routerMu.RLock()
	list := workspace.router[topic]
	workspace.routerMu.RUnlock()

	if list == nil {
		return Snapshot{Name: topic, InTopic: topic}
	}

	aggregate := Snapshot{Name: topic, InTopic: topic}

	for _, subscriber := range list.load() {
		snap := subscriber.snapshot()
		aggregate.Published += snap.Published
		aggregate.Completed += snap.Completed
		aggregate.Dropped += snap.Dropped
		aggregate.TryReserveFail += snap.TryReserveFail
		aggregate.TypeMismatch += snap.TypeMismatch
		aggregate.StepCalls += snap.StepCalls
		aggregate.ActiveHandlers += snap.ActiveHandlers
		aggregate.Lanes += snap.Lanes
		if snap.StepMaxNanos > aggregate.StepMaxNanos {
			aggregate.StepMaxNanos = snap.StepMaxNanos
		}
		if aggregate.Capacity == 0 {
			aggregate.Capacity = snap.Capacity
		}
	}

	aggregate.Pending = aggregate.Published - aggregate.Completed

	if aggregate.Capacity == 0 {
		aggregate.Capacity = uint64(subscriberCapacity)
	}

	return aggregate
}

/*
Share stores a shared runtime object under a composite key built from name and
optional identity components.
*/
func (workspace *Workspace) Share(name string, value any, id ...string) {
	if workspace == nil {
		return
	}

	key := sharedKey(name, id)

	workspace.sharedMu.Lock()
	workspace.shared[key] = value
	workspace.sharedMu.Unlock()
}

/*
Shared retrieves a shared runtime object and reports whether it exists.
*/
func (workspace *Workspace) Shared(name string, id ...string) (any, bool) {
	if workspace == nil {
		return nil, false
	}

	key := sharedKey(name, id)

	workspace.sharedMu.Lock()
	value, found := workspace.shared[key]
	workspace.sharedMu.Unlock()

	return value, found
}

func sharedKey(name string, ids []string) string {
	var key strings.Builder; fmt.Fprintf(&key, "%d:%s/", len(name), name)
	for _, id := range ids {
		if id == "" {
			continue
		}

		fmt.Fprintf(&key, "%d:%s/", len(id), id)
	}

	return key.String()
}

/*
On registers a signal listener; Notify dispatches to every listener.
*/
func (workspace *Workspace) On(signal string, handler func()) {
	if workspace == nil {
		return
	}

	workspace.signalsMu.Lock()
	workspace.signals[signal] = append(workspace.signals[signal], handler)
	workspace.signalsMu.Unlock()
}

func (workspace *Workspace) Notify(signal string) {
	if workspace == nil {
		return
	}

	workspace.signalsMu.RLock()
	handlers := append([]func(){}, workspace.signals[signal]...)
	workspace.signalsMu.RUnlock()

	for _, handler := range handlers {
		go handler()
	}
}
