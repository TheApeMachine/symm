package websocket

import (
	"os"
	"strings"
	"sync"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/callback"
	"github.com/krakenfx/api-go/v2/pkg/kraken"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

/*
Transport owns nonce authentication, callback registration, reconnect handling,
and replay dispatch for a Live websocket so Live stays a thin composition root.
*/
type Transport struct {
	auth         bool
	nonce        *AuthNonce
	nonceErr     error
	reconnectMu  sync.Mutex
	reconnectFns []func() error
}

/*
NewTransport constructs lifecycle state for public or authenticated Live links.
Authenticated transports load the process-wide AuthNonce; load failures are
retained so authenticate refuses to proceed.
*/
func NewTransport(auth bool) *Transport {
	transport := &Transport{auth: auth}

	if !auth {
		return transport
	}

	nonce, err := processAuthNonce()
	transport.nonce = nonce
	transport.nonceErr = err

	return transport
}

/*
wireCredentials installs API keys and the shared nonce generator on REST.
*/
func (transport *Transport) wireCredentials(client *spot.WebSocket) {
	if client == nil || !transport.auth {
		return
	}

	client.REST.PublicKey = os.Getenv("KRAKEN_API_KEY")
	client.REST.PrivateKey = os.Getenv("KRAKEN_API_SECRET")

	if transport.nonceErr != nil || transport.nonce == nil {
		return
	}

	// Private and every Level3 batch authenticate with the same key; they
	// must share one monotonic nonce sequence or concurrent token fetches
	// collide (EAPI:Invalid nonce).
	client.REST.Nonce = transport.nonce.Next
}

/*
bindCallbacks registers connect, receive, and authenticated handlers on Live.
*/
func (transport *Transport) bindCallbacks(live *Live) {
	live.client.OnReceived.Recurring(func(event *callback.Event[*kraken.WebSocketMessage]) {
		raw := event.Data.Bytes()

		// Level3 book application is expensive (checksum + depth enforcement)
		// and must never run on the socket reader. Hand the frame to the FIFO
		// worker and let the reader keep draining subsequent frames.
		if live.isLevel3 {
			live.enqueueLevel3(raw)
		}

		live.route(raw)
	})

	live.client.OnConnected.Recurring(func(event *callback.Event[any]) {
		transport.onConnected(live)
	})

	if !transport.auth {
		return
	}

	live.client.OnAuthenticated.Recurring(func(event *callback.Event[string]) {
		transport.onAuthenticated(live)
	})
}

/*
onConnected authenticates private links or marks public transports ready after
successful subscription replay.
*/
func (transport *Transport) onConnected(live *Live) {
	if !transport.auth {
		if err := transport.fireReconnect(); err != nil {
			live.status.Store(types.ERROR)
			errnie.Error(errnie.Err(
				errnie.Validation,
				"websocket: reconnect replay failed",
				err,
			))

			return
		}

		live.status.Store(types.READY)

		return
	}

	if errnie.Error(transport.authenticate(live.client)) != nil {
		live.status.Store(types.ERROR)
	}
}

/*
onAuthenticated replays private subscriptions after token acquisition and only
marks READY when every required replay succeeds.
*/
func (transport *Transport) onAuthenticated(live *Live) {
	if live.isLevel3 && len(live.symbols) > 0 && live.SubscribeLevel3(
		live.symbols,
		viper.GetInt("market.l3_depth"),
	) != nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"websocket: level3 book subscription failed",
			nil,
		))
		live.status.Store(types.ERROR)

		return
	}

	if err := transport.fireReconnect(); err != nil {
		live.status.Store(types.ERROR)
		errnie.Error(errnie.Err(
			errnie.Validation,
			"websocket: reconnect replay failed",
			err,
		))

		return
	}

	live.status.Store(types.READY)
}

/*
authenticate fetches a websocket token. An Invalid nonce rejection bumps the
persisted high-water and retries once so reconnect storms after a crash do not
leave the transport permanently in ERROR.
*/
func (transport *Transport) authenticate(client *spot.WebSocket) (err error) {
	if transport.nonceErr != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"websocket: auth nonce unavailable",
			transport.nonceErr,
		))
	}

	if err = client.Authenticate(); err != nil && !strings.Contains(err.Error(), "Invalid nonce") {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"websocket: authentication failed",
			err,
		))
	}

	if err == nil {
		return nil
	}

	if transport.nonce != nil {
		transport.nonce.Bump()
	}

	return client.Authenticate()
}

/*
OnReconnect registers a callback invoked after public connect or private
authentication so subscription intent can be replayed.
*/
func (transport *Transport) OnReconnect(fn func() error) {
	if fn == nil {
		return
	}

	transport.reconnectMu.Lock()
	transport.reconnectFns = append(transport.reconnectFns, fn)
	transport.reconnectMu.Unlock()
}

/*
fireReconnect runs registered replay callbacks and returns the first error so
callers can refuse READY when required subscriptions fail.
*/
func (transport *Transport) fireReconnect() error {
	transport.reconnectMu.Lock()
	callbacks := append([]func() error{}, transport.reconnectFns...)
	transport.reconnectMu.Unlock()

	for _, callback := range callbacks {
		if err := callback(); err != nil {
			return err
		}
	}

	return nil
}

/*
configureLevel3 installs the SDK BookManager the way Kraken's official L3
example does: create books from the outbound subscribe (OnSent), then apply
inbound frames through the FIFO worker.
*/
func configureLevel3(live *Live) {
	live.books = spot.NewBookManager()
	live.books.OnCreateBook.Recurring(func(event *callback.Event[*book.Book]) {
		managed := event.Data

		// Kraken frames are atomic, so depth cannot be enforced per order.
		managed.EnableMaxDepth = false
		managed.NoBookCrossing = false
		managed.OnChecksummed.Recurring(
			func(*callback.Event[*book.ChecksumResult]) {
				managed.EnforceDepth()
			},
		)
	})

	live.client.OnSent.Recurring(func(event *callback.Event[*kraken.WebSocketMessage]) {
		live.ingestLevel3Sent(event)
	})

	live.level3Queue = make(chan []byte, level3QueueDepth)
	go live.drainLevel3()
}
