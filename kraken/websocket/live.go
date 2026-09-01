package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/system"

	"github.com/bytedance/sonic"
	gorillawebsocket "github.com/gorilla/websocket"
	"github.com/krakenfx/api-go/v2/pkg/callback"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	sdkkraken "github.com/krakenfx/api-go/v2/pkg/kraken"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

var entityMap = map[string]func([]byte) any{
	"ticker":     func(buf []byte) any { return kraken.NewTicker(buf) },
	"book":       func(buf []byte) any { return kraken.NewBook(buf) },
	"trade":      func(buf []byte) any { return kraken.NewTrade(buf) },
	"ohlc":       func(buf []byte) any { return kraken.NewOHLC(buf) },
	"level3":     func(buf []byte) any { return kraken.NewLevel3(buf) },
	"instrument": func(buf []byte) any { return kraken.NewInstrument(buf) },
	"balances":   func(buf []byte) any { return kraken.NewBalance(buf) },
	"executions": func(buf []byte) any { return kraken.NewExecution(buf) },
	"status":     func(buf []byte) any { return true },
	"heartbeat":  func(buf []byte) any { return true },
	"subscribe":  func(buf []byte) any { return true },
	"pong": func(buf []byte) any {
		pong := map[string]any{}
		errnie.Error(sonic.Unmarshal(buf, &pong))
		return pong
	},
}

/*
Live is one spot websocket session: SDK client, channel fan-out, auth/nonce,
and Sub* resubscribe after the SDK reconnects.
*/
type Live struct {
	ctx            context.Context
	cancel         context.CancelFunc
	status         *runtime.Status
	err            error
	client         *spot.WebSocket
	quote          string
	ingress        map[string]*runtime.Workload[*types.Envelope]
	simulator      *Simulator
	normalizer     *spot.Normalizer
	level3         *sync.Map
	symbols        []string
	publicMu       sync.RWMutex
	public         map[string][][]string
	auth           bool
	nonce          *AuthNonce
	nonceErr       error
	subscribers    *sync.Map
	callbacks      *sync.Map
	paper          *Paper
	model          string
	capture        CaptureSink
	manifestSink   ManifestSink
	captureName    string
	reconnect      func()
	failureMu      sync.RWMutex
	failure        func(error)
	observer       atomic.Pointer[func(string, time.Duration)]
	connectedCount atomic.Int32

	// opStreams owns the operational per-stream epoch/sequence bookkeeping this
	// transport session maintains independent of Hindsight. The transport mints
	// the StreamRef for every frame and bumps epochs on reconnect; Hindsight
	// records the same fact but is never the source of it.
	opMu      sync.Mutex
	opStreams map[hindsight.Stream]streamSpan

	// level3Client overrides the venue client SubL3 dials when set. Fixtures
	// inject the level3 listener's client here so a replay's level3 frames
	// feed the session's book manager instead of dialing the real venue.
	level3Client func() *spot.WebSocket

	// l3forward funnels level3 envelopes from this session to the shared
	// ingestion sequencer. It is set on every Level3 child session so all
	// children push through exactly one writer goroutine instead of calling
	// Push on the shared ring concurrently. The parent/private/ticker/trade
	// sessions leave it nil and push directly.
	l3forward *level3Sequencer
}

/*
level3Sequencer owns the single writer goroutine for the shared Level3 ingress
ring. SubL3 chunks the universe into many child websocket sessions, each of
which would otherwise call Push concurrently on the same ring; go-disruptor
requires WriterCount(2+) for concurrent Reserve/Commit, so a single producer is
forwarded through here. Every child hands its envelope to the one writer, which
commits it to the ring in arrival order.
*/
type level3Sequencer struct {
	ingress chan *types.Envelope
	done    chan struct{}
}

func newLevel3Sequencer(workload *runtime.Workload[*types.Envelope], capacity int) *level3Sequencer {
	sequencer := &level3Sequencer{
		ingress: make(chan *types.Envelope, capacity),
		done:    make(chan struct{}),
	}

	go func() {
		for {
			select {
			case envelope := <-sequencer.ingress:
				workload.Push(envelope)
			case <-sequencer.done:
				return
			}
		}
	}()

	return sequencer
}

/*
Push forwards one level3 envelope to the shared ring's single writer. It drops
the envelope when the sequencer is closed rather than pushing onto a dead ring.
*/
func (sequencer *level3Sequencer) Push(envelope *types.Envelope) {
	if sequencer == nil || envelope == nil {
		return
	}

	select {
	case sequencer.ingress <- envelope:
	case <-sequencer.done:
	}
}

func (sequencer *level3Sequencer) Close() {
	if sequencer == nil {
		return
	}

	close(sequencer.done)
}

/*
streamSpan is one operational connection span within a transport stream: its
epoch and the frame sequence within that epoch. It is owned by the transport
session (not Hindsight) so reconnect invalidation is an operational transport
fact available even when capture is disabled.
*/
type streamSpan struct {
	epoch    hindsight.StreamEpoch
	sequence uint64
}

/*
nextStreamRef mints the operational StreamRef for one inbound frame on the given
channel. The stream name mirrors Hindsight's endpoint:kind naming so one
transport fact has one stable identity; the epoch starts at 1 and the sequence
within the span is monotonic.
*/
func (live *Live) nextStreamRef(channel string) hindsight.StreamRef {
	if live == nil {
		return hindsight.StreamRef{}
	}

	stream := hindsight.Stream(live.client.URL + ":" + channel)

	live.opMu.Lock()
	defer live.opMu.Unlock()

	if live.opStreams == nil {
		live.opStreams = make(map[hindsight.Stream]streamSpan)
	}

	span := live.opStreams[stream]

	if span.epoch == 0 {
		span.epoch = 1
	}

	span.sequence++
	live.opStreams[stream] = span

	return hindsight.StreamRef{
		Stream:   stream,
		Epoch:    span.epoch,
		Sequence: span.sequence,
	}
}

/*
reconnectStreams bumps the operational epoch for every stream this transport
session has seen and resets each stream's per-epoch sequence. Called on a second
(or later) connection in this process lifetime, completely independent of any
capture sink.
*/
func (live *Live) reconnectStreams() {
	if live == nil {
		return
	}

	live.opMu.Lock()
	defer live.opMu.Unlock()

	for stream, span := range live.opStreams {
		span.epoch++
		span.sequence = 0
		live.opStreams[stream] = span
	}
}

/*
Capture returns the underlying capture sink attached to the live connection.
*/
func (live *Live) Capture() CaptureSink {
	if live == nil {
		return nil
	}

	return live.capture
}

/*
SetReconnect installs the callback invoked when this session's transport
reconnects (a second or later connect within one process lifetime). It is the
single seam through which reconnect soft-reboots the subscription universe and
advances the Hindsight stream epochs — nothing else in the session owns that.
*/
func (live *Live) SetReconnect(handler func()) {
	if live == nil {
		return
	}

	live.reconnect = handler
}

/*
New opens a spot websocket session and wires SDK callbacks in the constructor.
*/
func New(
	ctx context.Context,
	workloads map[string]*runtime.Workload[*types.Envelope],
	simulator *Simulator,
	auth bool,
	endpoint string,
	recorders ...CaptureSink,
) *Live {
	return NewWithClient(
		ctx, workloads, simulator, auth, endpoint, nil, recorders...,
	)
}

/*
NewWithClient opens a spot websocket session using an injected spot.WebSocket client instance.
A nil Thesis creates an explicit parsing-only session; SetThesis attaches event routing before
the connection becomes part of a running system.
*/
func NewWithClient(
	ctx context.Context,
	workloads map[string]*runtime.Workload[*types.Envelope],
	simulator *Simulator,
	auth bool,
	endpoint string,
	client *spot.WebSocket,
	recorders ...CaptureSink,
) *Live {
	if client == nil {
		client = spot.NewWebSocket()
		client.URL = endpoint
	}

	ctx, cancel := context.WithCancel(ctx)

	viper.SetDefault("market.quote_currency", "USD")

	captureName := "public"

	if auth {
		captureName = "private"
	}

	if endpoint == system.Cfg.WebSocket.Endpoints.Level3 {
		captureName = "level3"
	}

	live := &Live{
		ctx:         ctx,
		cancel:      cancel,
		status:      runtime.NewStatus(),
		simulator:   simulator,
		client:      client,
		normalizer:  spot.NewNormalizer(),
		auth:        auth,
		subscribers: &sync.Map{},
		callbacks:   &sync.Map{},
		public:      make(map[string][][]string),
		paper:       NewPaper(ctx, simulator, workloads),
		ingress:     workloads,
		model:       viper.GetViper().GetString("trading.model"),
		quote:       viper.GetViper().GetString("market.quote_currency"),
		captureName: captureName,
	}

	if len(recorders) == 1 {
		live.capture = recorders[0]

		if manifestSink, ok := recorders[0].(ManifestSink); ok {
			live.manifestSink = manifestSink
		}
	}

	if err := live.normalizer.Use(live.client.REST); err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"websocket: failed to initialize normalizer",
			err,
		))

		cancel()
		return nil
	}

	if auth {
		nonce, err := processAuthNonce()
		live.nonce = nonce
		live.nonceErr = err
		live.client.REST.PublicKey = os.Getenv("KRAKEN_API_KEY")
		live.client.REST.PrivateKey = os.Getenv("KRAKEN_API_SECRET")

		if live.nonceErr != nil || live.nonce == nil {
			return nil
		}

		// Private and every Level3 batch authenticate with the same key; they
		// must share one monotonic nonce sequence or concurrent token fetches
		// collide (EAPI:Invalid nonce).
		live.client.REST.Nonce = live.nonce.Next
	}

	if endpoint == system.Cfg.WebSocket.Endpoints.Level3 {
		live.level3 = &sync.Map{}
	}

	live.client.OnReceived.Recurring(func(event *callback.Event[*sdkkraken.WebSocketMessage]) {
		raw := event.Data.Bytes()
		channel := utils.GetString(raw, "channel")

		if channel == "" {
			if method := utils.GetString(raw, "method"); method != "" {
				channel = method
			}
		}

		// Every spot frame — public, private, or level3 — reaches the same
		// capture sink as the futures stream, so the events store sees the
		// whole system rather than futures alone. Capture mints the Hindsight
		// capture identity, persists the raw frame with it, and returns it so
		// every envelope parsed from this frame carries the exact same origin.
		// A failed capture fails loudly and skips this frame's dispatch: no
		// envelope may carry a zero/ambiguous identity while Hindsight is on.
		// The transport mints the operational StreamRef first — independent of
		// capture — so the same epoch/sequence fact exists with Hindsight both
		// on and off.
		streamRef := live.nextStreamRef(channel)

		var captureID hindsight.CaptureIdentity

		if live.capture != nil {
			var captureErr error

			captureID, captureErr = live.capture.Capture(
				channel,
				live.client.URL,
				raw,
				time.Now().UTC(),
				streamRef,
			)

			if captureErr != nil {
				errnie.Error(errnie.Err(
					errnie.IO,
					fmt.Sprintf("websocket: capture failed for %s frame: %s", channel, captureErr.Error()),
					captureErr,
				))

				live.status.Transition(runtime.ERROR)
				return
			}
		}

		// An unsubscribe acknowledgement answers the instrument's paced
		// recovery of a checksum-diverged symbol; there is nothing to
		// dispatch for it.
		if channel == "unsubscribe" {
			return
		}

		handler, ok := entityMap[channel]

		if !ok {
			live.err = errnie.Error(errnie.Err(
				errnie.NotFound,
				"websocket: unhandled channel "+channel,
				nil,
			))

			live.status.Transition(runtime.ERROR)
			return
		}

		out := handler(raw)

		if channel == "subscribe" {
			errMessage := utils.GetString(raw, "error")

			if errMessage != "" {
				// A rejected subscription is a per-request acknowledgement, not
				// a transport failure. Kraken answers duplicate subscriptions
				// (e.g. the soft-reboot resubscribe path re-issuing the same
				// universe) with "Already subscribed", which is benign and
				// recoverable. It must not poison the session lifecycle status
				// — the transport is still connected and READY.
				live.err = errnie.Error(errnie.Err(
					errnie.IO,
					fmt.Sprintf("[websocket] subscription rejected: %s", errMessage),
					nil,
				))

				return
			}
		}

		// Dispatch one-shot callbacks (e.g. "instrument" snapshot)
		if cb, ok := live.callbacks.LoadAndDelete(channel); ok {
			if msgChan, ok := cb.(chan any); ok {
				msgChan <- out
			}
		}

		switch channel {
		case "pong":
			// Check the error field in the pong response. If it is not empty, log the error.
			if errMsg := utils.GetString(raw, "error"); errMsg != "" {
				errnie.Error(errnie.Err(
					errnie.IO,
					fmt.Sprintf("websocket: pong error: %s", errMsg),
					nil,
				))

				return
			}

			return
		case "ticker", "trade", "level3", "executions":
			// This session does not feed the pipeline until the subscription
			// authority has marked it READY, which happens once the WHOLE
			// universe is subscribed. The frame is still captured above, so
			// Hindsight sees the complete stream; it simply does not reach a
			// strategy that would otherwise judge a partly-subscribed market.
			if live.Status() != runtime.READY {
				return
			}

			envelopes, manifests := IngestEnvelopes(channel, out, captureID)

			for index, envelope := range envelopes {
				// Live trading reads the operational StreamRef; Hindsight's
				// CaptureID records the same fact but is never the source.
				envelope.Stream = streamRef

				if channel == "level3" && live.l3forward != nil {
					live.l3forward.Push(envelope)
				} else {
					live.ingress[channel].Push(envelope)
				}

				if live.manifestSink != nil {
					_ = live.manifestSink.WriteManifest(manifests[index])
				}
			}
		}
	})

	live.client.OnConnected.Recurring(func(event *callback.Event[any]) {
		errnie.Info(fmt.Sprintf("websocket: connected to %s", live.client.URL))

		count := live.connectedCount.Add(1)

		if auth {
			errnie.Error(live.authenticate())
			return
		}

		// A second (or later) connect in this process lifetime is a reconnect:
		// the same subscription universe must be soft-rebooted through the one
		// subscription authority and the operational stream epochs advanced,
		// rather than a second subscription path being invented here. The epoch
		// is a transport fact; Hindsight observes it, never supplies it.
		if count > 1 && live.reconnect != nil {
			live.reconnectStreams()
			live.reconnect()
		}

		if live.captureName == "level3" {
			if count > 1 && len(live.symbols) > 0 {
				live.subscribeLevel3Group(live)
			}
		}

		// Connected is not ready. This session pushes into the trading
		// pipeline, and the universe is subscribed in paced batches after the
		// socket comes up, so the first frames are whichever handful of
		// symbols happen to be live already. BUSY says exactly that: the
		// transport is up and working, awaiting the subscription authority.
		live.status.Transition(runtime.BUSY)
	})

	live.client.OnDisconnected.Recurring(func(event *callback.Event[error]) {
		if gorillawebsocket.IsCloseError(
			event.Data,
			gorillawebsocket.CloseNormalClosure,
		) {
			return
		}

		errnie.Error(errnie.Err(
			errnie.Unauthorized,
			fmt.Sprintf("websocket %s disconnected: %s - %s", endpoint, event.Data.Error(), event.Data),
			event.Data,
		))

		live.status.Transition(runtime.WAITING)
	})

	if auth {
		live.client.OnAuthenticated.Recurring(func(event *callback.Event[string]) {
			errnie.Info(fmt.Sprintf("websocket: authenticated to %s", live.client.URL))

			if endpoint == system.Cfg.WebSocket.Endpoints.Private {
				if err := live.subscribeAccount(event.Data); err != nil {
					errnie.Error(errnie.Err(
						errnie.IO,
						"websocket: failed to subscribe to private account channels",
						err,
					))

					live.status.Transition(runtime.ERROR)
					return
				}
			}

			// Authenticated, not ready: same reason as OnConnected above.
			live.status.Transition(runtime.BUSY)
		})
	}

	errnie.Info(fmt.Sprintf("websocket: connecting to %s", live.client.URL))
	live.status.Transition(runtime.WAITING)

	if err := live.client.Connect(); err != nil {
		errnie.Error(errnie.Err(
			errnie.IO,
			"websocket: failed to connect",
			err,
		))
	}

	return live
}

/*
captureFrame records one untouched websocket payload with its origin kind,
receive order, and canonical endpoint so the live feed is directly consumable by
market replay.
*/
func (live *Live) captureFrame(kind, endpoint string, payload []byte) error {
	if live.capture == nil {
		return nil
	}

	if endpoint == "" || len(payload) == 0 {
		return fmt.Errorf("websocket: capture endpoint and payload required")
	}

	// Capture persists synchronously before this callback returns, so the SDK's
	// frame view remains valid for the complete write and needs no clone.
	_, err := live.capture.Capture(
		kind,
		endpoint,
		payload,
		time.Now().UTC(),
		live.nextStreamRef(kind),
	)

	return err
}

/*
CaptureSink receives one untouched transport payload with its origin kind,
endpoint, arrival time, and the operational StreamRef the transport minted for
that frame, and returns the CaptureIdentity it minted for that frame. kind
identifies the frame's channel/method/feed (e.g. "ticker", "trade", "book",
"level3", "pong"); endpoint names the stream it arrived on; ref is the
transport-owned epoch/sequence fact the returned identity must record/copy. The
returned identity is what the caller stamps onto every envelope parsed from the
frame. Implementations own persistence; the transport only reports.
*/
type CaptureSink interface {
	Capture(kind, endpoint string, payload []byte, receivedAt time.Time, ref hindsight.StreamRef) (hindsight.CaptureIdentity, error)
}

/*
ManifestSink receives one EnvelopeManifest — how a raw frame entered Workspace —
keyed by its EnvelopeRef. A capture recorder that also implements this interface
gets the manifests for the envelopes it produced a capture identity for, so raw
capture and semantic ingress are persisted together and joinable by identity.
*/
type ManifestSink interface {
	WriteManifest(manifest hindsight.EnvelopeManifest) error
}

func (live *Live) Status() runtime.Stage {
	return live.status.Current()
}

func (live *Live) authenticate() (err error) {
	errnie.Info(fmt.Sprintf("websocket[%s]: authenticating", live.client.URL))

	if live.nonceErr != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"websocket: auth nonce unavailable",
			live.nonceErr,
		))
	}

	if err = live.client.Authenticate(); err != nil && !strings.Contains(
		err.Error(), "Invalid nonce",
	) {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"websocket: authentication failed",
			err,
		))
	}

	if err == nil {
		return nil
	}

	if live.nonce != nil {
		live.nonce.Bump()
	}

	return live.client.Authenticate()
}

/*
subscribeAccount activates Kraken's private wallet and execution streams after
each token refresh. Kraken closes an authenticated socket that does not submit
a private subscription within its token deadline, so reconnect authentication
must always repeat these requests with the new token.

The execution subscription fans out into the executions ingress workload, which
is wired after the desk exists — later in boot than the private session's first
authenticate. Subscribing before that workload is ready would deliver execution
frames into a nil workload. The subscription therefore waits on the executions
workload's readiness before it writes, within the token deadline.
*/
func (live *Live) subscribeAccount(token string) error {
	// The balance subscription is submitted immediately: it satisfies Kraken's
	// authenticated-socket deadline (which closes a socket that submits no
	// private subscription) and its frames flow through callbacks, never the
	// executions ingress.
	if err := live.Write(kraken.NewBalanceSubscription(token)); err != nil {
		return err
	}

	// The execution subscription fans out into the executions ingress workload,
	// which is wired later in boot than the private session's first authenticate.
	// Submitting it before that workload exists would deliver execution frames
	// into a nil workload, so it is submitted asynchronously once the workload
	// is ready — without blocking the auth goroutine and stalling readiness.
	go live.subscribeExecutionsWhenReady(token)

	return nil
}

/*
subscribeExecutionsWhenReady submits the private execution subscription once the
executions ingress workload is ready. It owns the gate so the initial
authenticate returns immediately (and the balance subscription already satisfied
the token deadline), while execution frames cannot arrive before their consumer.
*/
func (live *Live) subscribeExecutionsWhenReady(token string) {
	live.waitReady()

	if err := live.Write(kraken.NewExecutionSubscription(token)); err != nil {
		errnie.Error(errnie.Err(
			errnie.IO,
			"websocket: failed to subscribe to executions",
			err,
		))
	}
}

/*
MarkReady is called by the subscription authority once the WHOLE market
universe is subscribed. Only then does this session feed the trading pipeline:
before it, the socket is connected and parsing but the universe is still being
subscribed in paced batches, so what arrives is a partial market.

Level3 child sessions are marked with their parent, since they push into the
same ingress through the shared sequencer.
*/
func (live *Live) MarkReady() {
	if live == nil {
		return
	}

	live.status.Transition(runtime.READY)

	if live.level3 == nil {
		return
	}

	live.level3.Range(func(_, value any) bool {
		if child, valid := value.(*Live); valid && child != nil {
			child.status.Transition(runtime.READY)
		}

		return true
	})
}

/*
waitReady blocks until this session is READY, or the session context ends. It
waits on the session's own status rather than polling the shared ingress map,
so it never reads a map another goroutine is writing.
*/
func (live *Live) waitReady() {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		if live.Status() == runtime.READY {
			return
		}

		select {
		case <-live.ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (live *Live) Client() *spot.WebSocket {
	if live == nil {
		return nil
	}

	return live.client
}

func (live *Live) SubInstrument(callback chan any) {
	errnie.Info("websocket: subscribing to instrument")

	live.Write(kraken.NewInstrumentSubscription(), Callback[any]{
		Channel: "instrument",
		Message: callback,
	})
}

func (live *Live) SubTicker(symbols []string) {
	errnie.Error(live.client.SubTicker(symbols))
}

func (live *Live) SubTrades(symbols []string) {
	errnie.Error(live.client.SubTrades(symbols))
}

func (live *Live) SubL3(symbols []string) {
	if live.level3 == nil {
		live.level3 = &sync.Map{}
	}

	// One sequencer owns the shared level3 ring for this whole session. Child
	// sessions forward through it so the ring has a single writer regardless
	// of how the universe is chunked across websocket children.
	if live.l3forward == nil {
		live.l3forward = newLevel3Sequencer(
			live.ingress["level3"],
			8192,
		)
	}

	for groups := range slices.Chunk(symbols, 200) {
		groupKey := strings.Join(groups, "|")

		existing, loaded := live.level3.Load(groupKey)

		if loaded {
			conn, valid := existing.(*Live)

			if valid && conn != nil && conn.client.IsActive() {
				// The child socket is still alive: reuse it and simply
				// resubscribe the symbol group on the existing connection
				// rather than dialing a duplicate level3 socket.
				live.subscribeLevel3Group(conn)
				continue
			}
		}

		conn := NewWithClient(
			live.ctx,
			live.ingress,
			live.simulator,
			live.auth,
			system.Cfg.WebSocket.Endpoints.Level3,
			live.level3ClientFor(),
			live.capture,
		)

		if conn == nil {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"websocket: failed to create level3 child connection",
				nil,
			))

			continue
		}

		conn.l3forward = live.l3forward
		live.level3.Store(groupKey, conn)
		conn.symbols = append([]string{}, groups...)
		live.subscribeLevel3Group(conn)
	}
}

/*
subscribeLevel3Group re-runs the exact paced level3 subscription batch the
startup path uses for one child connection. It is shared by boot and by a
checksum-divergence recovery so the two never diverge: both re-create the local
books and re-request the venue's level3 stream for the child's symbol group on
its already-connected socket.
*/
func (live *Live) subscribeLevel3Group(conn *Live) {
	if conn == nil || len(conn.symbols) == 0 {
		return
	}

	for group := range slices.Chunk(conn.symbols, 40) {
		conn.Client().SubL3(group, viper.GetInt("market.l3_depth"))
		time.Sleep(viper.GetDuration("market.subscribe.pace"))
	}
}

/*
AttachLevel3 installs an already-constructed level3 child connection into the
session's level3 map, so a fixture or injected transport can serve the book
manager without dialing the venue. It is the same registration SubL3 performs,
minus the venue client construction, and keeps the book lookup path unchanged.
*/
func (live *Live) AttachLevel3(groupKey string, conn *Live) {
	if conn == nil || groupKey == "" {
		return
	}

	if live.level3 == nil {
		live.level3 = &sync.Map{}
	}

	live.level3.Store(groupKey, conn)
}

/*
SetLevel3Client overrides the venue client SubL3 constructs for its child
connections. Fixtures set this to the level3 listener's own client so level3
subscriptions complete against the fixture instead of the real venue.
*/
func (live *Live) SetLevel3Client(factory func() *spot.WebSocket) {
	live.level3Client = factory
}

func (live *Live) level3ClientFor() *spot.WebSocket {
	if live != nil && live.level3Client != nil {
		return live.level3Client()
	}

	client := spot.NewWebSocket()
	client.URL = system.Cfg.WebSocket.Endpoints.Level3

	return client
}

func (live *Live) Balance() (map[string]*decimal.Decimal, error) {
	if live.model == "real" {
		response, err := live.client.REST.Balances()

		if err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.IO,
				"balance: failed to fetch",
				err,
			))
		}

		return response.Result, nil
	}

	return live.paper.Balances()
}

func (live *Live) TradesHistory() (spot.TradesHistoryResult, error) {
	if live.model == "real" {
		result := spot.TradesHistoryResult{Trades: map[string]spot.Trade{}}
		offset := 0

		for {
			response, err := live.client.REST.TradesHistory(&spot.TradesHistoryRequest{
				Type:             "all",
				Trades:           true,
				Start:            0,
				End:              0,
				Ofs:              offset,
				ConsolidateTaker: true,
				Ledgers:          true,
			})

			if err != nil {
				return spot.TradesHistoryResult{}, errnie.Error(err)
			}

			maps.Copy(result.Trades, response.Result.Trades)

			result.Count = response.Result.Count

			count, err := strconv.Atoi(response.Result.Count.String())

			if err == nil && len(result.Trades) >= count {
				return result, nil
			}

			if len(response.Result.Trades) == 0 {
				return result, nil
			}

			offset += len(response.Result.Trades)
		}
	}

	return live.paper.TradesHistory()
}

func (live *Live) OpenOrders() (spot.OpenOrdersResult, error) {
	if live.model != "real" {
		return live.paper.OpenOrders()
	}

	response, err := live.client.REST.OpenOrders(&spot.OpenOrdersRequest{Trades: true})

	if err != nil {
		return spot.OpenOrdersResult{}, errnie.Error(err)
	}

	return response.Result, nil
}

func (live *Live) CancelOrder(
	request *spot.CancelOrderRequest,
) (spot.CancelResult, error) {
	if live.model != "real" {
		return live.paper.CancelOrder(request)
	}

	response, err := live.client.REST.CancelOrder(request)

	if err != nil {
		return spot.CancelResult{}, errnie.Error(err)
	}

	return response.Result, nil
}

func (live *Live) TradeBalance() (kraken.TradeBalanceResult, error) {
	if live.model == "real" {
		response, err := live.Post(
			TradeBalanceEndpoint,
			kraken.NewTradeBalanceRequest(live.quote),
		)

		return kraken.NewTradeBalance(response), errnie.Error(err)
	}

	return live.paper.TradeBalance()
}

func (live *Live) TradeVolume(symbols []string) (*kraken.TradeVolumeResult, error) {
	if live.model != "real" {
		return live.paper.TradeVolume(symbols)
	}

	response, err := live.Post(
		TradeVolumeEndpoint,
		kraken.NewTradeVolumeRequest(symbols),
	)

	if len(response) > 0 {
		captureErr := live.captureFrame("trade_volume", TradeVolumeEndpoint, response)

		if captureErr != nil {
			return nil, captureErr
		}
	}

	return kraken.NewTradeVolume(response), errnie.Error(err)
}

func (live *Live) AddOrder(order *spot.AddOrderRequest) (spot.AddOrderResult, error) {
	// Only a real model reaches the venue. The test read the other way round,
	// which sent paper orders to Kraken over REST and routed real ones into
	// the simulator.
	if live.model == "real" {
		response, err := live.client.REST.AddOrder(order)

		if err != nil {
			return spot.AddOrderResult{}, errnie.Error(errnie.Err(
				errnie.IO,
				"add order: failed to submit",
				err,
			))
		}

		return response.Result, nil
	}

	return live.paper.AddOrder(order)
}

func (live *Live) Write(params json.Marshaler, callbacks ...Callback[any]) error {
	for _, callback := range callbacks {
		live.callbacks.Store(callback.Channel, callback.Message)
	}

	raw, err := params.MarshalJSON()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"websocket: write marshal failed",
			err,
		))
	}

	started := time.Now()

	err = live.client.WriteMessage(
		gorillawebsocket.TextMessage, raw,
	)

	if live.simulator != nil {
		live.simulator.Record(WEBSOCKET, time.Since(started))
	}

	return errnie.Error(err)
}

func (live *Live) do(options spot.RequestOptions) ([]byte, error) {
	started := time.Now()

	request, err := live.client.REST.NewRequest(options)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			err.Error(),
			err,
		))
	}

	resp, err := request.Do()

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"Kraken REST request failed",
			err,
		))
	}

	errors := utils.GetStringSlice(resp.Body, "error")

	if len(errors) > 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			errors[0],
			nil,
		))
	}

	if resp.StatusCode != 200 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf(
				"websocket.Live.do[%d]: %s",
				resp.StatusCode,
				resp.Body,
			),
			nil,
		))
	}

	if live.simulator != nil {
		live.simulator.Record(REST, time.Since(started))
	}

	return resp.Body, nil
}

func (live *Live) Post(
	path string, params json.Marshaler,
) ([]byte, error) {
	return live.do(spot.RequestOptions{
		Auth:   live.auth,
		Path:   path,
		Method: "POST",
		Body:   params,
	})
}

func (live *Live) Close() {
	live.cancel()

	if live.l3forward != nil {
		live.l3forward.Close()
	}

	if live.level3 != nil {
		live.level3.Range(func(_, value any) bool {
			child, valid := value.(*Live)

			if valid && child != nil {
				child.Close()
			}

			return true
		})
	}

	if live.client.IsActive() {
		errnie.Error(live.client.Disconnect())
	}
}
