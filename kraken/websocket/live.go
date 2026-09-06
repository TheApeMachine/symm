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
	"github.com/krakenfx/api-go/v2/pkg/book"
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
Live is one required spot websocket session. Operational disconnects replace
the venue connection, authenticate a fresh network session, and restore its
subscriptions; protocol and ingestion failures remain terminal.
*/
type Live struct {
	funding      FundingLedger
	ctx          context.Context
	cancel       context.CancelFunc
	status       *runtime.Status
	err          error
	client       atomic.Pointer[spot.WebSocket]
	endpoint     string
	quote        string
	ingress      map[string]runtime.Ingress[*types.Envelope]
	simulator    *Simulator
	normalizer   *spot.Normalizer
	book         *Book
	level3       *sync.Map
	symbols      []string
	publicMu     sync.RWMutex
	public       map[string][][]string
	auth         bool
	nonce        *AuthNonce
	nonceErr     error
	subscribers  *sync.Map
	callbacks    *sync.Map
	paper        *Paper
	model        string
	capture      CaptureSink
	manifestSink ManifestSink
	failureMu    sync.RWMutex
	failure      func(error)
	observer     atomic.Pointer[func(string, time.Duration)]
	connected    atomic.Bool
	released     atomic.Bool
	reconnecting atomic.Bool
	closing      atomic.Bool
	closeOnce    sync.Once

	// streams owns this session's operational epoch/sequence bookkeeping.
	streams *Streams

	// pinger owns this session's keepalive loop.
	pinger *Pinger

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

	// Level3Observers constructs numeric producers at each child book owner.
	// Only their measurements cross the ingress ring; order arrays do not.
	Level3Observers func() []runtime.Node[*types.Envelope]
	level3Observers []runtime.Node[*types.Envelope]

	// pingReqID is echoed back on the pong reply, so a response can be tied to
	// the request that produced it.
	pingReqID atomic.Int64
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
	ingress   chan *types.Envelope
	done      chan struct{}
	closeOnce sync.Once
}

func newLevel3Sequencer(workload runtime.Ingress[*types.Envelope], capacity int) *level3Sequencer {
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

	sequencer.closeOnce.Do(func() {
		close(sequencer.done)
	})
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
Error returns the first terminal session failure.
*/
func (live *Live) Error() error {
	if live == nil {
		return nil
	}

	live.failureMu.RLock()
	defer live.failureMu.RUnlock()

	return live.err
}

/*
SetFailure binds this session to its owner. Existing failures are replayed so a
constructor failure cannot be lost before API attaches the supervisor.
*/
func (live *Live) SetFailure(handler func(error)) {
	if live == nil {
		return
	}

	live.failureMu.Lock()
	live.failure = handler
	err := live.err
	live.failureMu.Unlock()

	if live.level3 != nil {
		live.level3.Range(func(_, value any) bool {
			child, valid := value.(*Live)

			if valid && child != nil {
				child.SetFailure(live.fail)
			}

			return true
		})
	}

	if err != nil && handler != nil {
		handler(err)
	}
}

func (live *Live) fail(err error) {
	if live == nil || err == nil {
		return
	}

	err = errnie.Error(err)

	live.failureMu.Lock()

	if live.err != nil {
		live.failureMu.Unlock()
		return
	}

	live.err = err
	handler := live.failure
	live.failureMu.Unlock()

	live.status.Transition(runtime.ERROR)
	live.cancel()

	if handler != nil {
		handler(err)
	}
}

func (live *Live) operationalError() error {
	if err := live.Error(); err != nil {
		return err
	}

	select {
	case <-live.ctx.Done():

		if err := live.Error(); err != nil {
			return err
		}

		return live.ctx.Err()
	default:
		return nil
	}
}

/*
New opens a spot websocket session and wires SDK callbacks in the constructor.
*/
func New(
	ctx context.Context,
	workloads map[string]runtime.Ingress[*types.Envelope],
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
	workloads map[string]runtime.Ingress[*types.Envelope],
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

	// The SDK reconnect loop neither observes this session's context nor restores
	// subscriptions. Live owns that lifecycle and replaces the SDK client so a
	// rotated network address never inherits the previous venue session.
	client.Reconnect = nil
	client.OnDisconnected.Reset()

	ctx, cancel := context.WithCancel(ctx)

	viper.SetDefault("market.quote_currency", "USD")

	live := &Live{
		ctx:         ctx,
		cancel:      cancel,
		status:      runtime.NewStatus(),
		simulator:   simulator,
		endpoint:    endpoint,
		normalizer:  spot.NewNormalizer(),
		auth:        auth,
		subscribers: &sync.Map{},
		callbacks:   &sync.Map{},
		public:      make(map[string][][]string),
		paper:       NewPaper(ctx, simulator, workloads),
		ingress:     workloads,
		model:       viper.GetViper().GetString("trading.model"),
		quote:       viper.GetViper().GetString("market.quote_currency"),
		streams:     NewStreams(client.URL),
	}
	live.client.Store(client)

	live.pinger = NewPinger("websocket", func() error {
		if !live.connected.Load() {
			return nil
		}

		ping, err := kraken.NewPing(live.pingReqID.Add(1)).MarshalJSON()

		if err != nil {
			return err
		}

		return live.Client().WriteMessage(gorillawebsocket.TextMessage, ping)
	})

	// A failed ping is the only evidence a half-open socket may produce. Treat it
	// exactly like a read-side disconnect and replace the venue session.
	live.pinger.OnFailed(func(err error) {
		go live.reconnect(err)
	})

	if len(recorders) == 1 {
		live.capture = recorders[0]

		if manifestSink, ok := recorders[0].(ManifestSink); ok {
			live.manifestSink = manifestSink
		}
	}

	if err := live.normalizer.Use(live.Client().REST); err != nil {
		live.fail(errnie.Err(
			errnie.Validation,
			"websocket: failed to initialize normalizer",
			err,
		))

		return live
	}

	if auth {
		nonce, err := processAuthNonce()
		live.nonce = nonce
		live.nonceErr = err
		live.Client().REST.PublicKey = os.Getenv("KRAKEN_API_KEY")
		live.Client().REST.PrivateKey = os.Getenv("KRAKEN_API_SECRET")

		if live.nonceErr != nil || live.nonce == nil {
			live.fail(errnie.Err(
				errnie.Validation,
				"websocket: auth nonce unavailable",
				live.nonceErr,
			))

			return live
		}

		// Private and every Level3 batch authenticate with the same key; they
		// must share one monotonic nonce sequence or concurrent token fetches
		// collide (EAPI:Invalid nonce).
		live.Client().REST.Nonce = live.nonce.Next
	}

	// The socket's receive callback and Book.Update notifications are synchronous.
	// Carry only the current frame's identity into the lightweight notification;
	// order arrays remain inside the transport and its resident book.
	var bookFrame *kraken.Level3
	var bookStream hindsight.StreamRef
	var bookCapture hindsight.CaptureIdentity

	if endpoint == system.Cfg.WebSocket.Endpoints.Level3 {
		live.level3 = &sync.Map{}
		live.book = NewBook(ctx, live.normalizer)
		live.book.SetResync(live.resyncLevel3)
		live.book.SetNotify(func(symbol string, at time.Time) {
			if live.Status() != runtime.READY {
				return
			}

			envelope := types.NewEnvelope(types.EnvelopeLevel3)
			envelope.Level3Data = kraken.Level3Data{Symbol: symbol, Timestamp: at}
			envelope.Stream, envelope.CaptureID = bookStream, bookCapture
			envelope.CaptureOrdinal = uint64(slices.IndexFunc(bookFrame.Data, func(data kraken.Level3Data) bool {
				return data.Symbol == symbol
			}))

			// Observe the accepted delta synchronously at its transport owner.
			// Strip orders before either the sequencer or workload sees the value.
			envelope.Level3Data = bookFrame.Data[envelope.CaptureOrdinal]

			for _, observer := range live.level3Observers {
				observer.Step(envelope)

				if failure, ok := observer.(runtime.ErrorNode); ok && failure.Error() != nil {
					live.fail(failure.Error())
					return
				}
			}

			envelope.Level3Data = kraken.Level3Data{Symbol: symbol, Timestamp: at}

			if live.manifestSink != nil {
				if err := live.manifestSink.WriteManifest(manifestFor(
					envelope, bookCapture, envelope.CaptureOrdinal, "level3", symbol,
				)); err != nil {
					live.fail(errnie.Err(errnie.IO, "websocket: persist book notification manifest", err))
					return
				}
			}

			if live.l3forward != nil {
				live.l3forward.Push(envelope)
				return
			}

			workload := live.ingress["level3"]

			if workload == nil || workload.Status() == nil || workload.Status().Current() != runtime.READY {
				live.fail(errnie.Err(errnie.NotAcceptable, "websocket: book notification ingress is not ready", nil))
				return
			}

			workload.Push(envelope)
		})
	}

	client.OnReceived.Recurring(func(event *callback.Event[*sdkkraken.WebSocketMessage]) {
		if live.operationalError() != nil {
			return
		}

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
		streamRef := live.streams.Next(channel)

		var captureID hindsight.CaptureIdentity

		if live.capture != nil {
			var captureErr error

			captureID, captureErr = live.capture.Capture(
				channel,
				live.Client().URL,
				raw,
				time.Now().UTC(),
				streamRef,
			)

			if captureErr != nil {
				live.fail(errnie.Err(
					errnie.IO,
					fmt.Sprintf("websocket: capture failed for %s frame: %s", channel, captureErr.Error()),
					captureErr,
				))
				return
			}
		}

		// An unsubscribe acknowledgement answers the instrument's paced
		// recovery of a checksum-diverged symbol; there is nothing to
		// dispatch for it.
		if channel == "unsubscribe" {
			if message := utils.GetString(raw, "error"); message != "" {
				live.fail(errnie.Err(errnie.IO, "websocket: unsubscribe rejected: "+message, nil))
			}

			return
		}

		handler, ok := entityMap[channel]

		if !ok {
			live.fail(errnie.Err(
				errnie.NotFound,
				"websocket: unhandled channel "+channel,
				nil,
			))
			return
		}

		out := handler(raw)

		if channel == "subscribe" {
			errMessage := utils.GetString(raw, "error")

			if errMessage != "" {
				live.fail(errnie.Err(
					errnie.IO,
					fmt.Sprintf("websocket: subscription rejected: %s", errMessage),
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

		if channel == "level3" && live.book != nil {
			level3, ok := out.(*kraken.Level3)

			if !ok {
				errnie.Error(errnie.Err(
					errnie.Validation,
					"websocket: unexpected level3 payload type",
					nil,
				))

				return
			}

			bookFrame, bookStream, bookCapture = level3, streamRef, captureID

			if err := live.book.Update(event, level3); err != nil {
				errnie.Error(err)
			}

			bookFrame = nil
			return
		}

		switch channel {
		case "pong":

			if errMsg := utils.GetString(raw, "error"); errMsg != "" {
				live.fail(errnie.Err(
					errnie.IO,
					fmt.Sprintf("websocket: pong error: %s", errMsg),
					nil,
				))

				return
			}

			return
		case "ticker", "trade", "executions":
			// Connected sessions capture but do not feed the pipeline until the
			// complete consumer graph has crossed its READY boundary.
			if live.Status() != runtime.READY {
				return
			}

			envelopes, manifests := IngestEnvelopes(channel, out, captureID)

			for index, envelope := range envelopes {
				// Live trading reads the operational StreamRef; Hindsight's
				// CaptureID records the same fact but is never the source.
				envelope.Stream = streamRef

				if live.manifestSink != nil {
					if err := live.manifestSink.WriteManifest(manifests[index]); err != nil {
						live.fail(errnie.Err(
							errnie.IO,
							"websocket: failed to persist envelope manifest",
							err,
						))

						return
					}
				}

				workload, mounted := live.ingress[channel]

				if !mounted || workload == nil {
					live.fail(errnie.Err(
						errnie.NotFound,
						"websocket: required ingress is not mounted for "+channel,
						nil,
					))

					return
				}

				if workload.Status() == nil || workload.Status().Current() != runtime.READY {
					live.fail(errnie.Err(
						errnie.NotAcceptable,
						"websocket: ingress is not ready for "+channel,
						nil,
					))

					return
				}

				workload.Push(envelope)
			}
		}
	})

	client.OnConnected.Recurring(func(event *callback.Event[any]) {
		if live.operationalError() != nil {
			return
		}

		errnie.Info(fmt.Sprintf("websocket: connected to %s", live.Client().URL))

		live.connected.Store(true)
		live.status.Transition(runtime.BUSY)
		live.pinger.Start(live.ctx)
	})

	client.OnDisconnected.Recurring(func(event *callback.Event[error]) {
		live.connected.Store(false)

		if live.closing.Load() {
			return
		}

		select {
		case <-live.ctx.Done():
			return
		default:
		}

		go live.reconnect(event.Data)
	})

	errnie.Info(fmt.Sprintf("websocket: connecting to %s", live.Client().URL))
	live.status.Transition(runtime.WAITING)

	if err := live.Client().Connect(); err != nil {
		live.fail(errnie.Err(
			errnie.IO,
			"websocket: failed to connect",
			err,
		))
	} else if err := live.resume(); err != nil {
		live.fail(err)
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
		live.streams.Next(kind),
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
	client := live.Client()
	errnie.Info(fmt.Sprintf("websocket[%s]: authenticating", client.URL))

	if live.nonceErr != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"websocket: auth nonce unavailable",
			live.nonceErr,
		))
	}

	if err = client.Authenticate(); err != nil && !strings.Contains(
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

	return client.Authenticate()
}

/*
resume establishes the authenticated identity and subscriptions belonging to a
fresh venue connection. A reconnect fetches a new token instead of carrying the
previous network session across a possible client-IP change.
*/
func (live *Live) resume() error {
	if live.auth {
		if err := live.authenticate(); err != nil {
			return errnie.Err(
				errnie.Unauthorized,
				"websocket: authentication failed",
				err,
			)
		}

		errnie.Info(fmt.Sprintf("websocket: authenticated to %s", live.Client().URL))
	}

	if live.released.Load() {
		live.status.Transition(runtime.READY)
	}

	if live.endpoint == system.Cfg.WebSocket.Endpoints.Private {
		if err := live.subscribeAccount(live.Client().Token); err != nil {
			return errnie.Err(
				errnie.IO,
				"websocket: failed to restore private account subscriptions",
				err,
			)
		}

		return nil
	}

	if !live.released.Load() {
		return nil
	}

	return live.restoreSubscriptions()
}

/*
reconnect replaces the SDK client and retries until a complete venue session is
ready or the owning context is canceled. Every attempt uses the SDK's configured
retry cadence and, for authenticated sockets, obtains a new websocket token.
*/
func (live *Live) reconnect(err error) {
	if live.closing.Load() || live.ctx.Err() != nil {
		return
	}

	if !live.reconnecting.CompareAndSwap(false, true) {
		return
	}

	defer live.reconnecting.Store(false)

	live.connected.Store(false)
	live.pinger.Stop()
	live.status.Transition(runtime.WAITING)
	retryWait := live.Client().ReconnectWait
	errnie.Error(errnie.Err(
		errnie.IO,
		fmt.Sprintf("websocket %s disconnected; reconnecting with a fresh session", live.endpoint),
		err,
	))

	for live.ctx.Err() == nil {
		live.streams.Advance()
		client := live.Client()
		replacement := spot.NewWebSocket()
		replacement.REST = client.REST
		replacement.URL = client.URL
		replacement.Reconnect = nil
		replacement.ReconnectWait = client.ReconnectWait
		replacement.Insecure = client.Insecure
		replacement.OnAuthenticated = client.OnAuthenticated
		replacement.OnConnected = client.OnConnected
		replacement.OnDisconnected = client.OnDisconnected
		replacement.OnSent = client.OnSent
		replacement.OnReceived = client.OnReceived
		live.client.Store(replacement)

		err = replacement.Connect()
		established := err == nil

		if established {
			err = live.resume()
		}

		if err == nil {
			return
		}

		live.connected.Store(false)
		live.pinger.Stop()
		live.status.Transition(runtime.WAITING)
		errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("websocket %s fresh-session reconnect failed", live.endpoint),
			err,
		))

		if established {
			_ = replacement.Disconnect()
		}

		retry := time.NewTimer(retryWait)

		select {
		case <-retry.C:
		case <-live.ctx.Done():
			retry.Stop()

			return
		}
	}
}

/*
subscribeAccount activates Kraken's private wallet and execution streams after
authentication. Kraken closes an authenticated socket that does not submit a
private subscription within its token deadline.

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

	// A replacement session is already READY, so restore executions in this
	// attempt and let reconnect retry the whole fresh session if the write fails.
	if live.Status() == runtime.READY {
		return live.Write(kraken.NewExecutionSubscription(token))
	}

	// Initial boot authenticates before the execution workload is ready. Wait
	// asynchronously there so authentication does not stall the readiness gate.
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
	if err := live.waitReady(); err != nil {
		live.fail(errnie.Err(
			errnie.IO,
			"websocket: execution subscription readiness failed",
			err,
		))

		return
	}

	if err := live.Write(kraken.NewExecutionSubscription(token)); err != nil {
		live.fail(errnie.Err(
			errnie.IO,
			"websocket: failed to subscribe to executions",
			err,
		))
	}
}

/*
MarkReady releases a connected session after the complete consumer graph has
been admitted. Level3 child sessions already attached to this parent cross the
same boundary because they push through the same ingress sequencer.
*/
func (live *Live) MarkReady() {
	if live == nil {
		return
	}

	if live.operationalError() != nil {
		return
	}

	if live.Status() != runtime.BUSY && live.Status() != runtime.READY {
		live.fail(errnie.Err(
			errnie.NotAcceptable,
			"websocket: only a connected session can become ready",
			nil,
		))

		return
	}

	if len(live.ingress) == 0 {
		live.fail(errnie.Err(
			errnie.NotFound,
			"websocket: readiness requires mounted ingress workloads",
			nil,
		))

		return
	}

	for channel, workload := range live.ingress {
		if workload != nil && workload.Status() != nil &&
			workload.Status().Current() == runtime.READY {
			continue
		}

		live.fail(errnie.Err(
			errnie.NotAcceptable,
			"websocket: cannot become ready before ingress "+channel,
			nil,
		))

		return
	}

	if live.level3 != nil {
		live.level3.Range(func(_, value any) bool {
			child, valid := value.(*Live)

			if !valid || child == nil {
				return true
			}

			child.MarkReady()

			if err := child.operationalError(); err != nil {
				live.fail(errnie.Err(
					errnie.IO,
					"websocket: level3 child unavailable at readiness barrier",
					err,
				))

				return false
			}

			return true
		})
	}

	if live.operationalError() != nil {
		return
	}

	live.released.Store(true)
	live.status.Transition(runtime.READY)
}

/*
waitReady blocks until this session is READY, or the session context ends. It
waits on the session's own status rather than polling the shared ingress map,
so it never reads a map another goroutine is writing.
*/
func (live *Live) waitReady() error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		if err := live.Error(); err != nil {
			return err
		}

		if live.Status() == runtime.READY {
			return nil
		}

		select {
		case <-live.ctx.Done():
			return live.operationalError()
		case <-ticker.C:
		}
	}
}

func (live *Live) Client() *spot.WebSocket {
	if live == nil {
		return nil
	}

	return live.client.Load()
}

func (live *Live) SubInstrument(callback chan any) {
	if live.operationalError() != nil {
		return
	}

	errnie.Info("websocket: subscribing to instrument")

	if err := live.Write(kraken.NewInstrumentSubscription(), Callback[any]{
		Channel: "instrument",
		Message: callback,
	}); err != nil {
		live.fail(errnie.Err(
			errnie.IO,
			"websocket: failed to subscribe to instruments",
			err,
		))
	}
}

func (live *Live) SubTicker(symbols []string) {
	if err := live.operationalError(); err != nil {
		return
	}

	if live.Status() != runtime.READY {
		live.fail(errnie.Err(
			errnie.NotAcceptable,
			"websocket: ticker subscription requires a ready session",
			nil,
		))

		return
	}

	if err := live.Client().SubTicker(symbols); err != nil {
		live.fail(errnie.Err(
			errnie.IO,
			"websocket: failed to subscribe to ticker",
			err,
		))

		return
	}

	live.publicMu.Lock()
	live.public["ticker"] = append(live.public["ticker"], slices.Clone(symbols))
	live.publicMu.Unlock()
}

func (live *Live) SubTrades(symbols []string) {
	if err := live.operationalError(); err != nil {
		return
	}

	if live.Status() != runtime.READY {
		live.fail(errnie.Err(
			errnie.NotAcceptable,
			"websocket: trade subscription requires a ready session",
			nil,
		))

		return
	}

	if err := live.Client().SubTrades(symbols); err != nil {
		live.fail(errnie.Err(
			errnie.IO,
			"websocket: failed to subscribe to trades",
			err,
		))

		return
	}

	live.publicMu.Lock()
	live.public["trade"] = append(live.public["trade"], slices.Clone(symbols))
	live.publicMu.Unlock()
}

/*
UnsubTicker and UnsubTrades withdraw the ticker/trade streams for the given
symbols. They mirror SubTicker/SubTrades so the instrument — which owns the
market universe — drives teardown through the same batched seam it drives
setup, rather than the transport keeping a second copy of the universe.
*/
func (live *Live) UnsubTicker(symbols []string) {
	live.unsubscribe("ticker", symbols, 100)
}

func (live *Live) UnsubTrades(symbols []string) {
	live.unsubscribe("trade", symbols, 100)
}

/*
UnsubL3 withdraws level3 for the given symbols. Level3 fans out across child
sessions, each holding its own socket and its own slice of the universe, so the
request is routed to whichever child actually holds each symbol.
*/
func (live *Live) UnsubL3(symbols []string) {
	if live == nil || live.level3 == nil || len(symbols) == 0 {
		return
	}

	wanted := make(map[string]struct{}, len(symbols))

	for _, symbol := range symbols {
		wanted[symbol] = struct{}{}
	}

	live.level3.Range(func(_, value any) bool {
		child, valid := value.(*Live)

		if !valid || child == nil {
			return true
		}

		mine := make([]string, 0, len(child.symbols))

		for _, symbol := range child.symbols {
			if _, held := wanted[symbol]; held {
				mine = append(mine, symbol)
			}
		}

		child.unsubscribe("level3", mine, 40)

		return true
	})
}

/*
unsubscribe writes the batched unsubscribe requests for one channel. A session
whose socket is already gone has nothing to withdraw, which is not an error:
the venue drops the subscriptions along with the connection.
*/
func (live *Live) unsubscribe(channel string, symbols []string, batch int) {
	if live == nil || live.Client() == nil || len(symbols) == 0 {
		return
	}

	if !live.connected.Load() {
		live.forgetSubscription(channel, symbols)

		return
	}

	for group := range slices.Chunk(symbols, batch) {
		if err := live.Write(kraken.NewChannelUnsubscription(channel, group)); err != nil {
			live.fail(errnie.Err(
				errnie.IO,
				fmt.Sprintf("websocket: unsubscribe %s failed", channel),
				err,
			))
		}
	}

	live.forgetSubscription(channel, symbols)
}

func (live *Live) forgetSubscription(channel string, symbols []string) {
	live.publicMu.Lock()
	defer live.publicMu.Unlock()

	groups := live.public[channel]

	for index := range groups {
		groups[index] = slices.DeleteFunc(groups[index], func(symbol string) bool {
			return slices.Contains(symbols, symbol)
		})
	}

	live.public[channel] = slices.DeleteFunc(groups, func(group []string) bool {
		return len(group) == 0
	})
}

func (live *Live) restoreSubscriptions() error {
	client := live.Client()

	if len(live.symbols) > 0 {
		return live.subscribeLevel3Group(live)
	}

	live.publicMu.RLock()
	defer live.publicMu.RUnlock()

	for channel, groups := range live.public {
		for _, group := range groups {
			var err error

			switch channel {
			case "ticker":
				err = client.SubTicker(group)
			case "trade":
				err = client.SubTrades(group)
			}

			if err != nil {
				return errnie.Err(errnie.IO, "websocket: failed to restore "+channel+" subscription", err)
			}
		}
	}

	return nil
}

func (live *Live) SubL3(symbols []string) {
	if live.operationalError() != nil {
		return
	}

	if live.Status() != runtime.READY {
		live.fail(errnie.Err(
			errnie.NotAcceptable,
			"websocket: level3 subscription requires a ready session",
			nil,
		))

		return
	}

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

			if valid && conn != nil && conn.Error() == nil {
				if err := live.subscribeLevel3Group(conn); err != nil {
					live.fail(err)
				}

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

		if conn.Error() != nil {
			return
		}

		conn.l3forward = live.l3forward

		if live.Level3Observers != nil {
			conn.level3Observers = live.Level3Observers()
		}

		conn.symbols = append([]string{}, groups...)
		live.AttachLevel3(groupKey, conn)

		if err := live.subscribeLevel3Group(conn); err != nil {
			live.fail(err)
			return
		}
	}
}

/*
resyncLevel3 replaces one divergent subscription on its owning socket. The
ordered writes withdraw the old stream before requesting an authoritative
snapshot. A failed write is a visible transport failure; it is never ignored.
*/
func (live *Live) resyncLevel3(symbol string) {
	if err := live.Write(kraken.NewChannelUnsubscription("level3", []string{symbol})); err != nil {
		live.fail(errnie.Err(errnie.IO, "websocket: level3 recovery unsubscribe failed", err))
		return
	}

	if err := live.Client().SubPrivate("level3", map[string]any{
		"params": map[string]any{"symbol": []string{symbol}, "depth": viper.GetInt("market.l3_depth"), "snapshot": true},
	}); err != nil {
		live.fail(errnie.Err(errnie.IO, "websocket: level3 recovery snapshot request failed", err))
	}
}

/*
subscribeLevel3Group re-runs the exact paced level3 subscription batch the
startup path uses for one child connection. Boot and fresh-session reconnects
therefore apply the same venue pacing and request the same symbol group.
*/
func (live *Live) subscribeLevel3Group(conn *Live) error {
	if conn == nil || len(conn.symbols) == 0 {
		return nil
	}

	if conn.Status() != runtime.READY {
		return errnie.Err(
			errnie.NotAcceptable,
			"websocket: level3 child subscription requires a ready session",
			nil,
		)
	}

	for group := range slices.Chunk(conn.symbols, 40) {
		if err := conn.Client().SubPrivate("level3", map[string]any{
			"params": map[string]any{"symbol": group, "depth": viper.GetInt("market.l3_depth"), "snapshot": true},
		}); err != nil {
			err = errnie.Err(
				errnie.IO,
				"websocket: failed to subscribe to level3",
				err,
			)

			return err
		}

		time.Sleep(viper.GetDuration("market.subscribe.pace"))
	}

	return nil
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

	childFailure := live.fail
	conn.SetFailure(childFailure)
	live.level3.Store(groupKey, conn)

	if live.Status() == runtime.READY {
		conn.MarkReady()
	}
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

func (live *Live) Books() *sync.Map {
	out := &sync.Map{}

	if live.level3 == nil {
		return out
	}

	live.level3.Range(func(key, value any) bool {
		if conn, ok := value.(*Live); ok && conn.book != nil {
			conn.book.SnapshotInto(out)
		}

		return true
	})

	return out
}

func (live *Live) Book(symbol string, read func(*book.Book)) {
	if live.level3 == nil {
		read(nil)
		return
	}

	found := false
	live.level3.Range(func(_, value any) bool {
		conn, ok := value.(*Live)

		if !ok || conn.book == nil {
			return true
		}

		conn.book.Book(symbol, func(managed *book.Book) {
			found = true
			read(managed)
		})
		return !found
	})

	if !found {
		read(nil)
	}
}

func (live *Live) Balance() (map[string]*decimal.Decimal, error) {
	if live.model == "real" {
		response, err := live.Client().REST.Balances()

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
			response, err := live.Client().REST.TradesHistory(&spot.TradesHistoryRequest{
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

	response, err := live.Client().REST.OpenOrders(&spot.OpenOrdersRequest{Trades: true})

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

	response, err := live.Client().REST.CancelOrder(request)

	if err != nil {
		return spot.CancelResult{}, errnie.Error(err)
	}

	return response.Result, nil
}

func (live *Live) TradeBalance() (kraken.TradeBalanceResult, error) {
	if live.model == "real" {
		before, _, err := live.funding.Observe(live.Post, live.normalizer.Name, live.quote, time.Now().UTC())

		if err != nil {
			errnie.Error(err)
		}
		response, err := live.Post(
			TradeBalanceEndpoint,
			kraken.NewTradeBalanceRequest(live.quote),
		)

		if err != nil {
			return kraken.TradeBalanceResult{}, errnie.Error(err)
		}
		result := kraken.NewTradeBalance(response)
		complete := result.EquivalentBalance != nil
		result.ValuationComplete = &complete
		balances, err := live.Post("/0/private/BalanceEx", json.RawMessage(`{}`))

		if err != nil {
			return result, errnie.Error(err)
		}

		extended, err := kraken.NewExtendedBalance(balances)

		if err != nil {
			return result, errnie.Error(err)
		}

		result.AvailableCash, err = extended.Available(live.quote, live.normalizer.Name)

		if err != nil {
			return result, errnie.Error(err)
		}
		result.NetFunding, result.FundingReason, err = live.funding.Observe(live.Post, live.normalizer.Name, live.quote, time.Now().UTC())

		if err != nil {
			errnie.Error(err)
		}

		if before == nil || (result.NetFunding != nil && before.Cmp(result.NetFunding) != 0) {
			result.NetFunding = nil
			result.FundingReason = "funding changed or was unavailable during valuation"
		}
		return result, nil
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
		response, err := live.Client().REST.AddOrder(order)

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
	if err := live.operationalError(); err != nil {
		return err
	}

	for _, callback := range callbacks {
		live.callbacks.Store(callback.Channel, callback.Message)
	}

	raw, err := params.MarshalJSON()

	if err != nil {
		err = errnie.Err(
			errnie.Validation,
			"websocket: write marshal failed",
			err,
		)
		live.fail(err)

		return err
	}

	started := time.Now()

	err = live.Client().WriteMessage(
		gorillawebsocket.TextMessage, raw,
	)

	if live.simulator != nil {
		live.simulator.Record(WEBSOCKET, time.Since(started))
	}

	if err != nil {
		err = errnie.Err(
			errnie.IO,
			"websocket: write failed",
			err,
		)
	}

	return err
}

func (live *Live) do(options spot.RequestOptions) ([]byte, error) {
	started := time.Now()

	request, err := live.Client().REST.NewRequest(options)

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
	if live == nil {
		return
	}

	live.closeOnce.Do(func() {
		live.closing.Store(true)
		live.cancel()

		if live.pinger != nil {
			live.pinger.Stop()
		}

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

		if live.connected.Load() && live.Error() == nil && live.Client() != nil {
			errnie.Error(live.Client().Disconnect())
		}

		if live.Error() == nil && live.status != nil {
			live.status.Transition(runtime.DONE)
		}
	})
}
