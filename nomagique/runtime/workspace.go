package runtime

import (
	"context"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/viper"

	"github.com/theapemachine/errnie"
)

/*
Workspace is the system-wide streaming bus: one shared object for the whole
pipeline. Producers publish values to named typed channels; stages subscribe
to the channels they consume, and the workspace creates a ring per
subscription that connects each subscriber to whatever produces that channel.

Every channel fans each published value out to each subscription's bounded
ring for that value's key. Drains are scheduled on the shared worker pool with
one deduplicated task per (subscription, key), so per-key ordering holds,
independent keys run concurrently across shards, and no stage owns an
infinite goroutine or spins waiting for work. Rings are bounded and overwrite
the oldest retained value under overload, so the pipeline never queues behind
the market and producers never block.
*/
type Workspace struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *Pool[func()]
	channels    sync.Map // string → *Channel[T]
	taps        sync.Map // StreamKey → []func(string, any)
	shared      sync.Map // string → any (shared-object pool)
	listeners   sync.Map // string → []func() (trigger fan-out)
	listenersMu sync.Mutex
	firstErr    atomic.Pointer[error]
	failure     func(error)
}

/*
Channel returns the typed handle for a named channel on the bus, creating it
on first use. The key function routes every published value to its per-key
ring inside each subscription. Callers on both the producing and consuming
side look up the same name and get the same handle.
*/
func ChannelOf[T any](
	workspace *Workspace,
	name string,
	key func(T) string,
) *Channel[T] {
	if workspace == nil {
		panic("runtime: workspace required")
	}
	if key == nil {
		panic("runtime: channel key function required: " + name)
	}

	if existing, found := workspace.channels.Load(name); found {
		return existing.(*Channel[T])
	}

	channel := &Channel[T]{
		workspace: workspace,
		name:      name,
		key:       key,
		capacity:  streamLaneCapacity(),
	}
	actual, loaded := workspace.channels.LoadOrStore(name, channel)

	if loaded {
		return actual.(*Channel[T])
	}

	return channel
}

/*
Channel is one typed topic on the bus. A producer publishes a value once;
every subscription receives it and advances independently.
*/
type Channel[T any] struct {
	workspace *Workspace
	name      string
	key       func(T) string
	capacity  uint64

	subs     atomic.Pointer[[]*Subscription[T]]
	observer atomic.Pointer[workspaceObserver]
	failure  atomic.Pointer[workspaceError]

	laneCount atomic.Uint64
	active    atomic.Uint64
	pending   atomic.Uint64
	highWater atomic.Uint64
	submitted atomic.Uint64
	completed atomic.Uint64
	dropped   atomic.Uint64
}

/*
Subscription is one consumer of a channel. Its step runs once per published
value, in key order, on a pool worker.
*/
type Subscription[T any] struct {
	channel *Channel[T]
	id      string
	step    func(T) error
	keys    sync.Map // string → *subscriptionKey[T]
}

/*
subscriptionKey owns one key's bounded ring and the dedupe flag that
guarantees at most one in-flight drain task per (subscription, key).
*/
type subscriptionKey[T any] struct {
	subscription *Subscription[T]
	ring         *Ring[T]
	pending      atomic.Bool
}

/*
Subscribe registers one consumer on the channel. The step runs once per value
for this subscription, in key order, concurrently across keys.
*/
func (channel *Channel[T]) Subscribe(id string, step func(T) error) *Subscription[T] {
	if channel == nil {
		panic("runtime: channel required")
	}
	if step == nil {
		panic("runtime: subscription step required: " + id)
	}

	subscription := &Subscription[T]{
		channel: channel,
		id:      id,
		step:    step,
	}

	for {
		loaded := channel.subs.Load()
		var current []*Subscription[T]

		if loaded != nil {
			current = *loaded
		}

		next := make([]*Subscription[T], 0, len(current)+1)
		next = append(next, current...)
		next = append(next, subscription)

		if channel.subs.CompareAndSwap(loaded, &next) {
			return subscription
		}
	}
}

/*
Publish fans one value out to every subscription's key ring. It never blocks
and never allocates per value: a full ring overwrites the oldest retained
value and counts the drop.
*/
func (channel *Channel[T]) Publish(value T) {
	if channel == nil || channel.failure.Load() != nil {
		return
	}

	key := channel.key(value)

	if key == "" {
		key = "_"
	}

	if channel.workspace != nil {
		channel.workspace.notifyTaps(channel.name, key, value)
	}

	for _, subscription := range channel.subsSnapshot() {
		subscription.deliver(key, value)
	}
}

/*
Observe attaches a typed observer callback directly to this channel.
*/
func (channel *Channel[T]) Observe(observer func(key string, value T)) {
	if channel == nil || channel.workspace == nil || observer == nil {
		return
	}

	channel.workspace.Observe(channel.name, func(_ string, key string, value any) {
		if typedValue, ok := value.(T); ok {
			observer(key, typedValue)
		}
	})
}

func (channel *Channel[T]) subsSnapshot() []*Subscription[T] {
	if channel == nil {
		return nil
	}

	current := channel.subs.Load()

	if current == nil {
		return nil
	}

	return *current
}

func (subscription *Subscription[T]) deliver(key string, value T) {
	entry := subscription.keyEntry(key)
	channel := subscription.channel

	beforeDropped := entry.ring.Dropped()
	entry.ring.Push(value)
	afterDropped := entry.ring.Dropped()

	channel.submitted.Add(1)

	if dropped := afterDropped - beforeDropped; dropped > 0 {
		channel.dropped.Add(dropped)
	} else {
		pending := channel.pending.Add(1)
		updateMaximum(&channel.highWater, pending)
	}

	if entry.pending.CompareAndSwap(false, true) {
		channel.schedule(key, entry)
	}
}

func (channel *Channel[T]) schedule(key string, entry *subscriptionKey[T]) {
	if err := channel.workspace.pool.AddKeyedTask(key, func() { entry.drain() }); err != nil {
		// The pool is saturated; never lose the wake, so enqueue with
		// blocking. Producers of the topic itself are never blocked here.
		_ = channel.workspace.pool.AddTaskWithBlocking(func() { entry.drain() })
	}
}

func (subscription *Subscription[T]) keyEntry(key string) *subscriptionKey[T] {
	if existing, found := subscription.keys.Load(key); found {
		return existing.(*subscriptionKey[T])
	}

	entry := &subscriptionKey[T]{
		subscription: subscription,
		ring:         NewRing[T](subscription.channel.capacity),
	}
	actual, loaded := subscription.keys.LoadOrStore(key, entry)

	if loaded {
		return actual.(*subscriptionKey[T])
	}

	subscription.channel.laneCount.Add(1)

	return entry
}

/*
drain consumes one key's ring in order until empty, then re-arms the dedupe
flag and re-checks the ring so a value that arrived mid-drain is never
stranded. It runs on a pool worker; at most one drain per (subscription, key)
is ever in flight.
*/
func (entry *subscriptionKey[T]) drain() {
	subscription := entry.subscription
	channel := subscription.channel

	for {
		for {
			if channel.workspace.ctx.Err() != nil {
				return
			}

			value, ok := entry.ring.Pop()

			if !ok {
				break
			}

			channel.pending.Add(^uint64(0))

			channel.active.Add(1)
			observer := channel.observer.Load()

			if observer != nil && observer.begin != nil {
				observer.begin(subscription.id)
			}

			started := time.Time{}

			if observer != nil && observer.end != nil {
				started = time.Now()
			}

			err := subscription.step(value)

			if observer != nil && observer.end != nil {
				observer.end(subscription.id, time.Since(started))
			}

			channel.active.Add(^uint64(0))
			channel.completed.Add(1)

			if err != nil {
				channel.failure.CompareAndSwap(nil, &workspaceError{err: err})
				channel.workspace.fail(channel.name, err)

				return
			}
		}

		entry.pending.Store(false)

		if entry.ring.Length() == 0 {
			return
		}

		if !entry.pending.CompareAndSwap(false, true) {
			return
		}
	}
}

/*
SetObserver installs the optional per-step diagnostics clock on a channel.
The observer receives each subscription's id and the exact step duration.
*/
func (channel *Channel[T]) SetObserver(
	begin func(string), end func(string, time.Duration),
) {
	if channel == nil {
		return
	}

	if begin == nil && end == nil {
		channel.observer.Store(nil)

		return
	}

	channel.observer.Store(&workspaceObserver{begin: begin, end: end})
}

/*
Snapshot is an instantaneous aggregate for the diagnostics page.
*/
func (channel *Channel[T]) Snapshot() WorkspaceSnapshot {
	if channel == nil {
		return WorkspaceSnapshot{}
	}

	return WorkspaceSnapshot{
		Lanes: channel.laneCount.Load(), Active: channel.active.Load(),
		Pending: channel.pending.Load(), Capacity: channel.laneCount.Load() * channel.capacity,
		HighWater: channel.highWater.Load(), Submitted: channel.submitted.Load(),
		Completed: channel.completed.Load(), Dropped: channel.dropped.Load(),
	}
}

/*
Idle reports whether every subscription has drained its rings and no step is
running.
*/
func (channel *Channel[T]) Idle() bool {
	if channel == nil {
		return true
	}

	return channel.pending.Load() == 0 && channel.active.Load() == 0
}

/*
Error returns the first step failure, if any.
*/
func (channel *Channel[T]) Error() error {
	if channel == nil {
		return nil
	}

	if failure := channel.failure.Load(); failure != nil {
		return failure.err
	}

	return nil
}

/*
NewWorkspace constructs the system bus. A nil pool uses the shared bus pool.
*/
func NewWorkspace(pool *Pool[func()]) *Workspace {
	if pool == nil {
		pool = BusPool()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Workspace{
		ctx:    ctx,
		cancel: cancel,
		pool:   pool,
	}
}

/*
Close cancels the bus; in-flight drains stop at the next value boundary.
*/
func (workspace *Workspace) Close() error {
	if workspace == nil {
		return nil
	}

	workspace.cancel()

	return nil
}

/*
SetFailureHandler installs the system failure sink. A stage step error is
logged loudly and forwarded here before the bus is cancelled, so a failed
stage can never kill the pipeline silently.
*/
func (workspace *Workspace) SetFailureHandler(handler func(error)) {
	if workspace != nil {
		workspace.failure = handler
	}
}

/*
fail surfaces a stage step error: it is logged with the owning channel name,
forwarded to the system failure handler when one is installed, and then the
bus is cancelled so no stage keeps running on a broken assumption.
*/
func (workspace *Workspace) fail(channel string, err error) {
	errnie.Error(errnie.Err(
		errnie.Internal,
		"workspace: stage failed on channel "+channel,
		err,
	))

	workspace.firstErr.CompareAndSwap(nil, &err)

	if workspace.failure != nil {
		workspace.failure(err)
	}

	workspace.cancel()
}

/*
Error returns the first stage failure the bus retained, if any.
*/
func (workspace *Workspace) Error() error {
	if workspace == nil {
		return nil
	}

	if first := workspace.firstErr.Load(); first != nil {
		return *first
	}

	return nil
}

/*
Share places one shared object into the workspace pool under the key built from
name and ids. It is how a producer (e.g. the book manager or cross-section
owner) hands a resource to every signal without any signal importing another.
Signals read the object via Shared and never see the producing component.
*/
func (workspace *Workspace) Share(name string, value any, ids ...string) {
	if workspace == nil || name == "" || value == nil {
		return
	}

	workspace.shared.Store(sharedKey(name, ids), value)
}

func sharedKey(name string, ids []string) string {
	var key strings.Builder
	writeKeyComponent(&key, name)

	for _, id := range ids {
		if id != "" {
			writeKeyComponent(&key, id)
		}
	}

	return key.String()
}

// writeKeyComponent length-prefixes one shared-key component so embedded
// separators (such as colons) cannot collide with the component boundary.
func writeKeyComponent(key *strings.Builder, value string) {
	key.WriteString(strconv.Itoa(len(value)))
	key.WriteByte(':')
	key.WriteString(value)
}

/*
Shared returns the shared object registered under the key built from name and
the given ids. An empty id list reads the object stored directly under name;
otherwise the ids are joined to form the storage key (e.g. Shared("book",
"BTC/USD") reads the book registered for that symbol). Signals only ever talk
to the workspace through this call, never to the producing component.
*/
func (workspace *Workspace) Shared(name string, ids ...string) (any, bool) {
	if workspace == nil || name == "" {
		return nil, false
	}

	key := sharedKey(name, ids)

	return workspace.shared.Load(key)
}

/*
On subscribes one listener to a trigger topic. Triggers fan out a notification
(e.g. a book update) to every registered listener without the listener knowing
the producer. It is the observer hook signals use to learn that a shared object
they read has changed.
*/
func (workspace *Workspace) On(topic string, fn func()) {
	if workspace == nil || fn == nil {
		return
	}

	workspace.listenersMu.Lock()
	defer workspace.listenersMu.Unlock()

	current, _ := workspace.listeners.LoadOrStore(topic, []func(){})
	updated := append(append([]func(){}, current.([]func())...), fn)
	workspace.listeners.Store(topic, updated)
}

/*
Notify fires every listener registered on topic.
*/
func (workspace *Workspace) Notify(topic string) {
	if workspace == nil {
		return
	}

	if listeners, found := workspace.listeners.Load(topic); found {
		for _, fn := range listeners.([]func()) {
			fn()
		}
	}
}

/*
Observer receives one published value from an observed channel on the bus,
with the channel name, key, and typed value.
*/
type Observer func(channel string, key string, value any)

/*
Observe attaches an observer callback to a named channel on the bus.
*/
func (workspace *Workspace) Observe(channel string, observer Observer) {
	if workspace == nil || channel == "" || observer == nil {
		return
	}

	current, _ := workspace.taps.LoadOrStore(channel, []Observer{})
	updated := append(current.([]Observer), observer)
	workspace.taps.Store(channel, updated)
}

/*
ObserveAll attaches an observer callback to all channels on the bus.
*/
func (workspace *Workspace) ObserveAll(observer Observer) {
	if workspace == nil || observer == nil {
		return
	}

	workspace.Observe("*", observer)
}

func (workspace *Workspace) notifyTaps(channel string, key string, value any) {
	if workspace == nil {
		return
	}

	if taps, found := workspace.taps.Load(channel); found {
		for _, tap := range taps.([]Observer) {
			tap(channel, key, value)
		}
	}

	if allTaps, found := workspace.taps.Load("*"); found {
		for _, tap := range allTaps.([]Observer) {
			tap(channel, key, value)
		}
	}
}

/*
BusPool is the shared worker pool every channel schedules its ring drains on.
Workers are bounded and elastic: no stage owns a per-key goroutine.
*/
func BusPool() *Pool[func()] {
	return busPool()
}

var busPool = sync.OnceValue(func() *Pool[func()] {
	pool := NewPool(func(task func()) { task() })
	pool.SetNumShards(defaultNumShards())
	pool.SetShardMinWorkers(4)
	pool.SetQueueSize(8192)
	pool.Start()

	return pool
})

/*
WorkspaceSnapshot is an instantaneous aggregate suitable for the diagnostics
page. Pending and HighWater describe actual retained values; Active is the
number of steps currently running; Dropped counts values a full ring
overwrote so the pipeline never blocks a producer.
*/
type WorkspaceSnapshot struct {
	Lanes     uint64
	Active    uint64
	Pending   uint64
	Capacity  uint64
	HighWater uint64
	Submitted uint64
	Completed uint64
	Dropped   uint64
}

type workspaceObserver struct {
	begin func(string)
	end   func(string, time.Duration)
}

type workspaceError struct{ err error }

/*
streamLaneCapacity is the shared per-key ring bound for every subscription.
It is explicit in the system config; a full ring overwrites the oldest
retained value instead of blocking the producer.
*/
var streamLaneCapacity = sync.OnceValue(func() uint64 {
	capacity := viper.GetUint64("system.streaming.lane_capacity")

	if capacity < 64 {
		capacity = 4096
	}

	return nextPowerOfTwo(capacity)
})

func updateMaximum(target *atomic.Uint64, value uint64) {
	for {
		current := target.Load()

		if value <= current || target.CompareAndSwap(current, value) {
			return
		}
	}
}

func nextPowerOfTwo(value uint64) uint64 {
	if value < 2 {
		return 2
	}
	value--
	value |= value >> 1
	value |= value >> 2
	value |= value >> 4
	value |= value >> 8
	value |= value >> 16
	value |= value >> 32
	return value + 1
}

/*
Idle reports whether every channel on the bus has drained and no step is
running. It is the streaming replacement for the old work-scheduler
quiescence check.
*/
func (workspace *Workspace) Idle() bool {
	if workspace == nil {
		return true
	}

	idle := true
	workspace.channels.Range(func(_ any, value any) bool {
		channel, ok := value.(interface{ Idle() bool })

		if ok && !channel.Idle() {
			idle = false

			return false
		}

		return true
	})

	return idle
}

/*
WaitForQuiescence blocks until every channel on the bus is idle, or the
context is cancelled. Replay and tests use it as the stable fixed point
between injected observations.
*/
func (workspace *Workspace) WaitForQuiescence(ctx context.Context) error {
	for {
		if workspace.Idle() {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			runtime.Gosched()
		}
	}
}
