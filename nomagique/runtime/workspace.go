package runtime

import (
	"context"
	"fmt"
	"reflect"
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
LMAX Disruptor owned by one producer-consumer edge. It is never multiplied by
the handler count and never grows at runtime.
*/
const subscriberCapacity uint32 = 64 * 1024

// SubscriberCapacity is the fixed slot count of every edge's physical LMAX
// Disruptor. It is exported so diagnostics can report ring capacity as a stable
// infrastructure constant even before the first node registers.
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
ServiceClass is the explicit scheduling class every node declares. Only
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
DeliveryPolicy is the per-edge publication policy every node declares
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
Lane is the owning handler index, resolved once by the publisher instead of
recomputed by every handler in the group.
*/
type Event struct {
	Sequence int64
	Value    any
	Lane     int32
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
NodeSpec captures registration-time knowledge for one node: its wanted input
type, its produced output type, its explicit class and delivery policy, and
its step implementation. There is no topic string anywhere in this system —
every edge is discovered by matching a producer's declared output type against
a consumer's declared input type.
*/
type NodeSpec struct {
	WantType   reflect.Type
	ReturnType reflect.Type
	Class      ServiceClass
	Delivery   DeliveryPolicy
	Keyed      bool

	keyFunc  func(any) string
	step     func(any) any
	descStep func(any) error
	stepKind StepKind

	// keyLimit optionally bounds the LatestByKey cell cardinality. Zero means
	// the configured infrastructure default applies.
	keyLimit int
}

/*
Subscriber is one node's registration: its NodeSpec plus one dedicated
physical LMAX Disruptor ring per distinct producer feeding it. A ring is
never shared between two different producers even though they feed the same
consumer node — each ring has exactly one writer (one producer identity) and
exactly one reader (this node's handler group), which is the single-writer
contract go-disruptor's Sequencer requires, upheld structurally rather than by
locking.
*/
type Subscriber struct {
	id        uint32
	name      string
	workspace *Workspace
	node      *NodeSpec

	// rings is one physical ring per feeding producerID, created lazily on
	// that producer's first dispatch to this node. ringsMu guards only the
	// rare creation path; an established ring is read through the atomic
	// snapshot so the hot publish path never takes a lock.
	ringsMu sync.Mutex
	rings   atomic.Pointer[map[uint32]*ring]

	handlerCnt int

	started atomic.Bool

	// latest-state cells: fixed per-key current value plus an outstanding dirty
	// notification flag, held in a lock-free map so the hot publish/resolve path
	// never blocks on a mutex.
	latestCells   sync.Map // string -> *latestCell
	latestCellCnt atomic.Int64

	// telemetry: publisher-written counters (touched only by the producer
	// goroutines feeding this node's rings) live on their own cache line,
	// separated from the handler-written counters below by _publishPad.
	published     atomic.Uint64
	dropped       atomic.Uint64
	tryReserveFmt atomic.Uint64
	lastDrop      atomic.Int64
	_publishPad   [64]byte

	// telemetry: handler-written counters (touched only by execute/Handle,
	// i.e. this node's own consumer goroutines).
	completed      atomic.Uint64
	typeMismatch   atomic.Uint64
	stepCount      atomic.Uint64
	stepTotalNanos atomic.Int64
	stepMaxNanos   atomic.Int64
	activeHandlers atomic.Int64
	lastComplete   atomic.Int64
}

/*
ring is one physical single-writer/single-reader Disruptor dedicated to
exactly one (producer, consumer) edge. Its handler group belongs to the
consumer Subscriber (so keyed lane affinity still works across every producer
feeding that consumer), but the Disruptor instance and buffer are this edge's
alone — no other producer ever calls Reserve/Commit on it.
*/
type ring struct {
	disruptor disruptor.Disruptor
	buffer    *ringBuffer
	handlers  []*keyedHandler
}

/*
producerRings returns this node's ring for producerID, creating it (and its
own dedicated Disruptor + handler group) on first contact from that producer.
*/
func (subscriber *Subscriber) ringFor(producerID uint32) *ring {
	if loaded := subscriber.rings.Load(); loaded != nil {
		if existing, ok := (*loaded)[producerID]; ok {
			return existing
		}
	}

	subscriber.ringsMu.Lock()
	defer subscriber.ringsMu.Unlock()

	current := subscriber.rings.Load()
	if current != nil {
		if existing, ok := (*current)[producerID]; ok {
			return existing
		}
	}

	created := subscriber.workspace.newRing(subscriber)
	if created == nil {
		return nil
	}

	next := make(map[uint32]*ring, len(*deref(current))+1)
	for id, r := range *deref(current) {
		next[id] = r
	}
	next[producerID] = created
	subscriber.rings.Store(&next)

	return created
}

func deref(pointer *map[uint32]*ring) *map[uint32]*ring {
	if pointer == nil {
		empty := map[uint32]*ring{}
		return &empty
	}
	return pointer
}

/*
keyedHandler is one long-lived handler inside one ring's handler group. Every
input has an explicit execution key; only the handler whose stable lane
identity matches hash(key) % handlerCount executes Step for that event. All
other handlers acknowledge/skip cheaply. A consumer node with several feeding
producers has one independent handler group per producer ring, all sharing
the same lane-count and key semantics so a keyed value always lands on the
same lane regardless of which producer's ring carried it.
*/
type keyedHandler struct {
	subscriber *Subscriber
	ring       *ring
	id         int
}

/*
Handle drains one Disruptor-delivered sequence range for this handler's lane
on this handler's own ring.
*/
func (handler *keyedHandler) Handle(lower, upper int64) {
	subscriber := handler.subscriber

	subscriber.activeHandlers.Add(1)
	defer subscriber.activeHandlers.Add(-1)

	var batchCompleted, batchStepCount uint64
	var batchStepNanos int64

	for sequence := lower; sequence <= upper; sequence++ {
		event := &handler.ring.buffer[sequence&subscriberCapacityMask]

		if int(event.Lane) != handler.id {
			continue
		}

		if ok, nanos := subscriber.execute(event); ok {
			batchCompleted++
			batchStepCount++
			batchStepNanos += nanos
		}
	}

	if batchCompleted > 0 {
		subscriber.completed.Add(batchCompleted)
		subscriber.stepCount.Add(batchStepCount)
		subscriber.stepTotalNanos.Add(batchStepNanos)
		subscriber.lastComplete.Store(time.Now().UnixNano())
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

	return int(fnv32a(key)) % handlerCount
}

/*
fnv32a computes the 32-bit FNV-1a hash of a string with no allocation and no
interface dispatch, unlike hash/fnv's Hash32 (a heap-allocated struct behind
an io.Writer interface). Every keyed event pays this on the hot publication
path, so it is inlined here as a tight byte loop instead.
*/
func fnv32a(s string) uint32 {
	const offset32 = 2166136261
	const prime32 = 16777619

	hash := uint32(offset32)
	for i := 0; i < len(s); i++ {
		hash ^= uint32(s[i])
		hash *= prime32
	}

	return hash
}

/*
execute runs one Step for one owned event inside its owning handler. The
analytical token (Analytics class only) is acquired around Step and released
before any downstream dispatch, so a full downstream ring can never deadlock
against a downstream subscriber waiting for analytical CPU.

Returns (true, elapsedNanos) only when a Step actually ran and completed
without panicking.
*/
func (subscriber *Subscriber) execute(event *Event) (ok bool, elapsedNanos int64) {
	value := event.Value

	if subscriber.node.Delivery == DeliveryLatestByKey {
		resolved := subscriber.resolveLatest(value)
		if resolved == nil {
			return false, 0
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

	if subscriber.node.Class == ServiceAnalytics {
		acquired = subscriber.workspace.acquireAnalyticsToken()
	}

	var output any

	switch subscriber.node.stepKind {
	case kindStepFunc:
		if err := subscriber.node.descStep(value); err != nil {
			subscriber.typeMismatch.Add(1)
			subscriber.workspace.reportFailure(err)
		}
	case kindStep:
		output = subscriber.node.step(value)
	}

	if acquired {
		subscriber.workspace.releaseAnalyticsToken()
		released = true
	}

	elapsed := time.Since(started)
	subscriber.recordMax(elapsed)

	if output != nil && subscriber.node.ReturnType != nil {
		subscriber.workspace.dispatch(subscriber.id, output)
	}

	return true, elapsed.Nanoseconds()
}

func (subscriber *Subscriber) reportPanic(recovered any) {
	var failure error

	inType := "none"
	if subscriber.node.WantType != nil {
		inType = subscriber.node.WantType.String()
	}

	outType := "none"
	if subscriber.node.ReturnType != nil {
		outType = subscriber.node.ReturnType.String()
	}

	switch panicErr := recovered.(type) {
	case error:
		failure = fmt.Errorf(
			"workspace: subscriber panic on %s (out: %s): %w\n%s",
			inType,
			outType,
			panicErr,
			string(debug.Stack()),
		)
	default:
		failure = fmt.Errorf(
			"workspace: subscriber panic on %s (out: %s): %v\n%s",
			inType,
			outType,
			recovered,
			string(debug.Stack()),
		)
	}

	subscriber.workspace.reportFailure(failure)
	errnie.Error(errnie.Err(errnie.Internal, "workspace: subscriber panic", failure))
}

/*
recordMax updates only the running max-latency watermark. stepCount and
stepTotalNanos are additive and batched by Handle instead.
*/
func (subscriber *Subscriber) recordMax(duration time.Duration) {
	nanos := duration.Nanoseconds()

	for {
		maxSeen := subscriber.stepMaxNanos.Load()
		if nanos <= maxSeen || subscriber.stepMaxNanos.CompareAndSwap(maxSeen, nanos) {
			break
		}
	}
}

/*
Workspace is SYMM's real-time streaming execution fabric. Every node declares
exactly two things at registration: the type it wants and the type it returns.
The workspace is the sole router — when it has a value of some type, it calls
Step on every node that wants that type, on that node's own dedicated ring, and
recursively dispatches whatever each Step returns the same way. There is no
topic string, and nothing ever calls a "Publish" method to hand a value to the
bus: a node's Step return value IS its emission, and Feed is the only entry
point for a value with no upstream producer (e.g. a value parsed off a
websocket).
*/
type Workspace struct {
	ctx    context.Context
	cancel context.CancelFunc

	analyticsSem chan struct{}
	handlerCount int

	nextSubscriberID atomic.Uint32
	subscribersMu    sync.RWMutex
	subscribers      []*Subscriber

	// registry maps a wanted reflect.Type to every subscriber (ring) that wants
	// it. Copy-on-write: dispatch only ever loads the pointer, so the hot
	// dispatch path never takes a lock. registerMu serializes the rare
	// registration-time replacement (registration happens at boot, not on the
	// event path).
	registry   atomic.Pointer[map[reflect.Type][]*Subscriber]
	registerMu sync.Mutex

	sharedMu sync.RWMutex
	shared   map[string]any

	signalsMu sync.RWMutex
	signals   map[string][]func()

	failureHandler atomic.Pointer[func(error)]
}

func NewWorkspace(ctx context.Context) *Workspace {
	if ctx == nil {
		ctx = context.Background()
	}

	ctx, cancel := context.WithCancel(ctx)
	handlerCount := max(runtime.GOMAXPROCS(0), 1)

	workspace := &Workspace{
		ctx:          ctx,
		cancel:       cancel,
		analyticsSem: make(chan struct{}, handlerCount),
		handlerCount: handlerCount,
		shared:       make(map[string]any),
		signals:      make(map[string][]func()),
	}

	emptyRegistry := make(map[reflect.Type][]*Subscriber)
	workspace.registry.Store(&emptyRegistry)

	return workspace
}

/*
acquireAnalyticsToken is non-blocking: a handler goroutine runs inside a
Disruptor ring's own sequence-consuming loop, so blocking here would stall
that ring's progression whenever the process-wide analytics semaphore is
saturated by an unrelated subscriber.
*/
func (workspace *Workspace) acquireAnalyticsToken() bool {
	if workspace == nil || workspace.analyticsSem == nil {
		return false
	}

	select {
	case workspace.analyticsSem <- struct{}{}:
		return true
	default:
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
		if subscriber == nil {
			continue
		}

		loaded := subscriber.rings.Load()
		if loaded == nil {
			continue
		}

		for _, target := range *loaded {
			if target != nil && target.disruptor != nil {
				_ = target.disruptor.Close()
			}
		}
	}

	return nil
}

/*
Register declares one node: it wants values of type T and, on receiving one,
calls step and dispatches whatever step returns (type U) to every node that
wants U. keyFunc is optional; nil means unkeyed (global lane). Every call to
Register creates exactly one dedicated single-writer/single-reader ring for
this (producer-type, consumer) edge — the ring belongs to this registration
alone, never shared with any other consumer of the same type.
*/
func Register[T any, U any](
	workspace *Workspace,
	keyFunc func(T) string,
	step func(T) U,
) {
	if workspace == nil {
		return
	}

	var boxedKeyFunc func(any) string
	keyed := keyFunc != nil

	if keyed {
		boxedKeyFunc = func(input any) string { return keyFunc(input.(T)) }
	} else {
		boxedKeyFunc = func(any) string { return globalKey }
	}

	var wantType reflect.Type = reflect.TypeFor[T]()
	var returnType reflect.Type = reflect.TypeFor[U]()

	workspace.register(&NodeSpec{
		WantType:   wantType,
		ReturnType: returnType,
		Class:      ServiceAnalytics,
		Delivery:   DeliveryObservationalFIFO,
		Keyed:      keyed,
		keyFunc:    boxedKeyFunc,
		step:       func(input any) any { return step(input.(T)) },
		stepKind:   kindStep,
	})
}

/*
RegisterClass declares one node with an explicit service class and delivery
policy, otherwise identical to Register.
*/
func RegisterClass[T any, U any](
	workspace *Workspace,
	class ServiceClass,
	delivery DeliveryPolicy,
	keyFunc func(T) string,
	step func(T) U,
) {
	if workspace == nil {
		return
	}

	var boxedKeyFunc func(any) string
	keyed := keyFunc != nil

	if keyed {
		boxedKeyFunc = func(input any) string { return keyFunc(input.(T)) }
	} else {
		boxedKeyFunc = func(any) string { return globalKey }
	}

	workspace.register(&NodeSpec{
		WantType:   reflect.TypeFor[T](),
		ReturnType: reflect.TypeFor[U](),
		Class:      class,
		Delivery:   delivery,
		Keyed:      keyed,
		keyFunc:    boxedKeyFunc,
		step:       func(input any) any { return step(input.(T)) },
		stepKind:   kindStep,
	})
}

/*
RegisterSink declares one node that wants values of type T and produces
nothing (a terminal consumer — a broker desk applying a stop, a UI hub
broadcasting a frame). Otherwise identical to Register.
*/
func RegisterSink[T any](
	workspace *Workspace,
	keyFunc func(T) string,
	step func(T),
) {
	if workspace == nil {
		return
	}

	var boxedKeyFunc func(any) string
	keyed := keyFunc != nil

	if keyed {
		boxedKeyFunc = func(input any) string { return keyFunc(input.(T)) }
	} else {
		boxedKeyFunc = func(any) string { return globalKey }
	}

	workspace.register(&NodeSpec{
		WantType: reflect.TypeFor[T](),
		Class:    ServiceAnalytics,
		Delivery: DeliveryObservationalFIFO,
		Keyed:    keyed,
		keyFunc:  boxedKeyFunc,
		step:     func(input any) any { step(input.(T)); return nil },
		stepKind: kindStep,
	})
}

/*
RegisterSinkClass declares a terminal, no-output node with an explicit service
class and delivery policy.
*/
func RegisterSinkClass[T any](
	workspace *Workspace,
	class ServiceClass,
	delivery DeliveryPolicy,
	keyFunc func(T) string,
	step func(T),
) {
	if workspace == nil {
		return
	}

	var boxedKeyFunc func(any) string
	keyed := keyFunc != nil

	if keyed {
		boxedKeyFunc = func(input any) string { return keyFunc(input.(T)) }
	} else {
		boxedKeyFunc = func(any) string { return globalKey }
	}

	workspace.register(&NodeSpec{
		WantType: reflect.TypeFor[T](),
		Class:    class,
		Delivery: delivery,
		Keyed:    keyed,
		keyFunc:  boxedKeyFunc,
		step:     func(input any) any { step(input.(T)); return nil },
		stepKind: kindStep,
	})
}

/*
RegisterSinkStepFunc declares a terminal node whose step reports a descriptive
error on type mismatch or failure, instead of returning a value.
*/
func RegisterSinkStepFunc[T any](
	workspace *Workspace,
	class ServiceClass,
	delivery DeliveryPolicy,
	keyFunc func(T) string,
	step func(T) error,
) {
	if workspace == nil {
		return
	}

	var boxedKeyFunc func(any) string
	keyed := keyFunc != nil

	if keyed {
		boxedKeyFunc = func(input any) string { return keyFunc(input.(T)) }
	} else {
		boxedKeyFunc = func(any) string { return globalKey }
	}

	workspace.register(&NodeSpec{
		WantType: reflect.TypeFor[T](),
		Class:    class,
		Delivery: delivery,
		Keyed:    keyed,
		keyFunc:  boxedKeyFunc,
		descStep: func(input any) error { return step(input.(T)) },
		stepKind: kindStepFunc,
	})
}

/*
register constructs exactly one Subscriber with exactly one physical Disruptor
and one handler group of handlerCount long-lived keyed handlers, and adds it to
the registry under its WantType. It is the only place a Disruptor is created;
a subscriber never gets more than one, and a ring is never shared between two
different registrations even when they want the same type.
*/
func (workspace *Workspace) register(node *NodeSpec) *Subscriber {
	handlerCount := workspace.handlerCount

	if node.Delivery == DeliveryLatestByKey || !node.Keyed {
		handlerCount = 1
	}

	if handlerCount < 1 {
		handlerCount = 1
	}

	subscriberID := workspace.nextSubscriberID.Add(1)
	subscriber := &Subscriber{
		id:         subscriberID,
		name:       node.WantType.String(),
		workspace:  workspace,
		node:       node,
		handlerCnt: handlerCount,
	}

	emptyRings := make(map[uint32]*ring)
	subscriber.rings.Store(&emptyRings)
	subscriber.started.Store(true)

	workspace.subscribersMu.Lock()
	workspace.subscribers = append(workspace.subscribers, subscriber)
	workspace.subscribersMu.Unlock()

	workspace.registerMu.Lock()
	current := *workspace.registry.Load()
	next := make(map[reflect.Type][]*Subscriber, len(current)+1)
	for wantType, list := range current {
		next[wantType] = list
	}
	next[node.WantType] = append(append([]*Subscriber{}, next[node.WantType]...), subscriber)
	workspace.registry.Store(&next)
	workspace.registerMu.Unlock()

	return subscriber
}

/*
newRing constructs exactly one physical Disruptor + buffer + handler group
dedicated to one (producer, consumer) edge, using the consumer subscriber's
own handlerCnt so keyed lane affinity is identical across every producer
feeding it. It never fails silently: a construction error is reported and nil
is returned, which ringFor treats as "this edge drops its input" rather than
panicking the caller.
*/
func (workspace *Workspace) newRing(subscriber *Subscriber) *ring {
	handlerCount := subscriber.handlerCnt

	handlers := make([]*keyedHandler, handlerCount)
	group := make([]disruptor.Handler, handlerCount)

	target := &ring{buffer: new(ringBuffer)}

	for index := 0; index < handlerCount; index++ {
		handlers[index] = &keyedHandler{subscriber: subscriber, ring: target, id: index}
		group[index] = handlers[index]
	}

	target.handlers = handlers

	disruptorInstance, err := disruptor.New(
		disruptor.Options.BufferCapacity(subscriberCapacity),
		disruptor.Options.NewHandlerGroup(group...),
	)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Internal,
			"workspace: failed to construct disruptor for "+subscriber.node.WantType.String(),
			err,
		))

		return nil
	}

	target.disruptor = disruptorInstance

	go disruptorInstance.Listen()

	return target
}

/*
Feed is a stable producer identity for values that originate outside the node
graph — bytes parsed off a websocket, a broker's own position update, a UI
bridge's frame. Every distinct external origin obtains its own Feed via
NewFeed and reuses it for every value it hands in, so the rings it feeds are
created once and reused, exactly like a registered node's own producer
identity. There is no free-standing Publish/Feed function: a value always
enters through a stable identity, whether that identity is a registered
node's Subscriber or an external Feed handle.
*/
type Feed struct {
	id        uint32
	workspace *Workspace
}

/*
NewFeed allocates one stable producer identity on this workspace. Call it once
per external origin (one per websocket stream, one per broker component, ...)
and reuse the returned Feed for every value that origin hands in.
*/
func (workspace *Workspace) NewFeed() *Feed {
	if workspace == nil {
		return nil
	}

	return &Feed{
		id:        workspace.nextSubscriberID.Add(1),
		workspace: workspace,
	}
}

/*
Emit hands the workspace one value from this Feed's stable producer identity.
The workspace fans it out to every node that wants its concrete type, on the
dedicated ring belonging to (this Feed, that node) alone.
*/
func (feed *Feed) Emit(value any) {
	if feed == nil || feed.workspace == nil {
		return
	}

	feed.workspace.dispatch(feed.id, value)
}

/*
dispatch fans a value out to every subscriber whose declared WantType matches
the value's concrete dynamic type, writing into the dedicated ring belonging
to (producerID, that subscriber) alone. This is the entire hot path:
copy-on-write registry load, no global mutex, no string routing, no ring ever
shared between two different producer identities.
*/
func (workspace *Workspace) dispatch(producerID uint32, value any) {
	if value == nil {
		return
	}

	valueType := reflect.TypeOf(value)

	list := (*workspace.registry.Load())[valueType]

	for _, subscriber := range list {
		subscriber.publish(producerID, value)
	}
}

func (subscriber *Subscriber) publish(producerID uint32, value any) {
	target := subscriber.ringFor(producerID)
	if target == nil {
		return
	}

	switch subscriber.node.Delivery {
	case DeliveryLatestByKey:
		subscriber.publishLatest(target, value)
	case DeliveryReliableFIFO, DeliveryPriorityFIFO:
		subscriber.publishReliable(target, value)
	case DeliveryObservationalFIFO:
		subscriber.publishObservational(target, value)
	}
}

/*
publishObservational uses the library's native TryReserve: a non-blocking
reservation that reports ErrCapacityUnavailable when the consumer has not
advanced far enough. On failure it records the drop explicitly; it never
blocks and never creates another queue. Because this ring belongs to exactly
one producer-consumer edge, TryReserve+write+Commit here can never race
against a different goroutine reserving on the same Sequencer — go-disruptor's
Sequencer is documented single-writer, and single-writer is structurally
guaranteed by construction (one producer identity + one consumer node = one
ring = one writer).
*/
func (subscriber *Subscriber) publishObservational(target *ring, value any) {
	sequence := target.disruptor.TryReserve(1)
	if sequence < 0 {
		subscriber.dropped.Add(1)
		subscriber.tryReserveFmt.Add(1)
		subscriber.lastDrop.Store(time.Now().UnixNano())
		return
	}

	lane := ownerOf(value, subscriber.node.keyFunc, subscriber.handlerCnt)
	target.buffer[sequence&subscriberCapacityMask] = Event{Sequence: sequence, Value: value, Lane: int32(lane)}
	target.disruptor.Commit(sequence, sequence)
	subscriber.published.Add(1)
}

/*
publishReliable uses the library's blocking Reserve: backpressure is real,
visible, and bounded. No event is dropped and no second queue evades it.
*/
func (subscriber *Subscriber) publishReliable(target *ring, value any) {
	lane := ownerOf(value, subscriber.node.keyFunc, subscriber.handlerCnt)
	subscriber.publishReliableLane(target, value, lane)
}

/*
publishReliableLane is publishReliable with an explicit, caller-supplied lane
instead of one resolved from the node's keyFunc. publishLatest is the one
caller that needs this: its payload is a *latestCell token, never the T the
node's keyFunc was registered to extract a key from. LatestByKey subscribers
are always forced to a single handler at registration, so lane 0 is the only
valid lane regardless.
*/
func (subscriber *Subscriber) publishReliableLane(target *ring, value any, lane int) {
	sequence := target.disruptor.Reserve(1)
	if sequence < 0 {
		subscriber.workspace.reportFailure(errnie.Err(
			errnie.NotAcceptable,
			"workspace: reliable reservation failed",
			nil,
		))

		return
	}

	target.buffer[sequence&subscriberCapacityMask] = Event{Sequence: sequence, Value: value, Lane: int32(lane)}
	target.disruptor.Commit(sequence, sequence)
	subscriber.published.Add(1)
}

/*
latestCell is one key's fixed slot for LatestByKey delivery: the current value
and its outstanding-notification flag co-located in one allocation, both
accessed only through atomics. No mutex guards a latestCell; publishLatest and
resolveLatest never block on each other or on a sibling key.
*/
type latestCell struct {
	value atomic.Pointer[any]
	dirty atomic.Bool
}

/*
publishLatest implements LatestByKey using fixed per-key state plus LMAX dirty
notifications, entirely lock-free. The latest cell is replaced on every update;
if the key is already dirty no further notification is emitted, so there is at
most one outstanding notification per key. The Disruptor carries only the
dirty key token.
*/
func (subscriber *Subscriber) publishLatest(target *ring, value any) {
	key := subscriber.node.keyFunc(value)
	if key == "" {
		key = globalKey
	}

	loaded, exists := subscriber.latestCells.Load(key)
	if !exists {
		limit := subscriber.node.keyLimit
		if limit <= 0 {
			limit = 640
		}

		if subscriber.latestCellCnt.Load() >= int64(limit) {
			subscriber.workspace.reportFailure(errnie.Err(
				errnie.TooManyRequests,
				fmt.Sprintf("workspace: latest-state key cardinality exceeds limit %d", limit),
				nil,
			))

			return
		}

		created := &latestCell{}
		actual, loadedExisting := subscriber.latestCells.LoadOrStore(key, created)
		if !loadedExisting {
			subscriber.latestCellCnt.Add(1)
		}
		loaded = actual
	}

	cell := loaded.(*latestCell)
	cell.value.Store(&value)
	alreadyDirty := cell.dirty.Swap(true)

	if alreadyDirty {
		return
	}

	// Publish the resolved *latestCell itself through LMAX, not just its key:
	// resolveLatest then reads the cell directly with no second sync.Map
	// lookup. The payload still never queues — only this one dirty notification
	// travels through the ring. Lane is explicit 0 (see publishReliableLane's
	// doc): a *latestCell is never a value the node's own keyFunc can extract
	// a key from, so it must never be routed through ownerOf(keyFunc, ...).
	subscriber.publishReliableLane(target, cell, 0)
}

/*
resolveLatest drains the fixed latest cell for a dirty key and clears the dirty
flag. It returns nil (skip) if the cell was never populated.
*/
func (subscriber *Subscriber) resolveLatest(value any) any {
	cell, ok := value.(*latestCell)
	if !ok {
		return nil
	}

	cell.dirty.Store(false)

	pointer := cell.value.Load()
	if pointer == nil {
		return nil
	}

	return *pointer
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

		if subscriber.node.Delivery == DeliveryLatestByKey {
			hasDirty := false

			subscriber.latestCells.Range(func(_, loaded any) bool {
				if loaded.(*latestCell).dirty.Load() {
					hasDirty = true
					return false
				}
				return true
			})

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
	Name             string
	WantType         string
	ReturnType       string
	ServiceClass     string
	Delivery         string
	Capacity         uint64
	Published        uint64
	Completed        uint64
	Dropped          uint64
	TryReserveFail   uint64
	TypeMismatch     uint64
	StepCalls        uint64
	StepTotalNanos   int64
	StepMaxNanos     int64
	HandlerCount     int
	ActiveHandlers   uint64
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

	returnType := "none"
	if subscriber.node.ReturnType != nil {
		returnType = subscriber.node.ReturnType.String()
	}

	return Snapshot{
		Name:             subscriber.name,
		WantType:         subscriber.node.WantType.String(),
		ReturnType:       returnType,
		ServiceClass:     subscriber.node.Class.String(),
		Delivery:         subscriber.node.Delivery.String(),
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
TypeSnapshot aggregates per-subscriber telemetry for every node that wants
type T, identified by a zero-value type parameter so diagnostics can ask "how
is everyone consuming this type doing" without a string topic name.
*/
func TypeSnapshot[T any](workspace *Workspace) Snapshot {
	if workspace == nil {
		return Snapshot{}
	}

	wantType := reflect.TypeFor[T]()

	list := (*workspace.registry.Load())[wantType]

	aggregate := Snapshot{Name: wantType.String(), WantType: wantType.String()}

	for _, subscriber := range list {
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

	workspace.sharedMu.RLock()
	value, found := workspace.shared[key]
	workspace.sharedMu.RUnlock()

	return value, found
}

func sharedKey(name string, ids []string) string {
	key := name + "/"

	for _, id := range ids {
		if id == "" {
			continue
		}

		key += id + "/"
	}

	return key
}

/*
On registers a signal listener; Notify dispatches to every listener. This is a
separate, deliberately string-keyed mechanism for lifecycle/control signals
(e.g. "disconnect") that carry no payload and are not part of the typed value
graph.
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
