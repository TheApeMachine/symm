package client

import (
	"context"
	"fmt"
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/core"
	"github.com/theapemachine/symm/kraken/order"
)

/*
PrivateClient maintains an authenticated Kraken WebSocket v2 session.
It reuses PublicClient for transport and attaches REST-issued tokens to private channels.
*/
type PrivateClient struct {
	ctx        context.Context
	cancel     context.CancelFunc
	err        error
	conn       *PublicClient
	publicKey  string
	privateKey string
	token      errnie.Result[*kraken.Token]
	pending    sync.Map
	fillWaits  sync.Map
	stopWaits  sync.Map
	fillBuffer sync.Map
	readOnce   sync.Once
}

/*
NewPrivateClient creates an authenticated websocket client with API credentials.
*/
func NewPrivateClient(
	ctx context.Context,
	publicKey string,
	privateKey string,
	opts ...PublicClientOption,
) (*PrivateClient, error) {
	ctx, cancel := context.WithCancel(ctx)

	authOpts := append([]PublicClientOption{
		WithWebSocketURL(core.KRAKEN_WS_AUTH_URL),
	}, opts...)

	client := &PrivateClient{
		ctx:        ctx,
		cancel:     cancel,
		conn:       NewPublicClient(ctx, authOpts...),
		publicKey:  publicKey,
		privateKey: privateKey,
		token:      errnie.Result[*kraken.Token]{},
	}

	return client, errnie.Require(map[string]any{
		"ctx":        ctx,
		"cancel":     cancel,
		"publicKey":  publicKey,
		"privateKey": privateKey,
		"conn":       client.conn,
		"token":      client.token,
	})
}

/*
Connect dials the websocket and fetches a session token.
*/
func (privateClient *PrivateClient) Connect() error {
	if err := privateClient.conn.Connect(); err != nil {
		return err
	}

	if err := privateClient.Authenticate(); err != nil {
		return err
	}

	return privateClient.Subscribe(kraken.ChannelTypeExecutions)
}

/*
Authenticate refreshes the REST websocket token used by private channel subscriptions.
*/
func (privateClient *PrivateClient) Authenticate() error {
	privateClient.token = errnie.Does(func() (*kraken.Token, error) {
		return kraken.NewToken(privateClient.publicKey, privateClient.privateKey)
	}).Or(func(err error) {
		privateClient.err = errnie.Error(err)
	})

	return privateClient.token.Err()
}

/*
EnsureToken refreshes the websocket token when expired.
*/
func (privateClient *PrivateClient) EnsureToken() error {
	if privateClient.token.Err() != nil {
		return privateClient.Authenticate()
	}

	if privateClient.token.Value().Expired() {
		return privateClient.Authenticate()
	}

	return nil
}

/*
Close closes the underlying public websocket session.
*/
func (privateClient *PrivateClient) Close() error {
	privateClient.cancel()

	return privateClient.conn.Close()
}

/*
Send writes a JSON frame on the authenticated session socket.
*/
func (privateClient *PrivateClient) Send(message any) error {
	return privateClient.conn.Send(message)
}

/*
Read returns the next websocket text frame payload.
*/
func (privateClient *PrivateClient) Read() ([]byte, error) {
	return privateClient.conn.Read()
}

/*
Ping sends a Kraken v2 heartbeat request.
*/
func (privateClient *PrivateClient) Ping() error {
	return privateClient.conn.Ping()
}

/*
NextReqID returns the next monotonic request id for trading frames.
*/
func (privateClient *PrivateClient) NextReqID() int {
	return privateClient.conn.NextReqID()
}

/*
Subscribe sends a subscribe frame, injecting a token when the channel requires auth.
*/
func (privateClient *PrivateClient) Subscribe(
	channel kraken.ChannelType,
) error {
	if err := privateClient.EnsureToken(); err != nil {
		return err
	}

	return privateClient.conn.Send(&kraken.Subscription{
		Method: kraken.MethodSubscribe,
		Params: map[string]any{
			"channel": channel,
			"token":   privateClient.token.Value().Result.Token,
		},
		ReqID: privateClient.conn.NextReqID(),
	})
}

/*
TokenValue returns the current websocket session token string.
*/
func (privateClient *PrivateClient) TokenValue() string {
	if privateClient.token.Err() != nil {
		return ""
	}

	return privateClient.token.Value().Result.Token
}

/*
PlaceOrder sends add_order and waits for the Kraken ack matching req_id.
*/
func (privateClient *PrivateClient) PlaceOrder(
	ctx context.Context,
	request order.Request,
) (*order.Ack, error) {
	if err := privateClient.EnsureToken(); err != nil {
		return nil, err
	}

	request.Params.Token = privateClient.token.Value().Result.Token
	request.ReqID = privateClient.conn.NextReqID()

	privateClient.startReadPump()

	responseCh := privateClient.registerPending(request.ReqID)
	defer privateClient.dropPending(request.ReqID)

	if err := privateClient.Send(request); err != nil {
		return nil, err
	}

	return privateClient.waitAck(ctx, responseCh, "add_order")
}

/*
AmendOrder sends amend_order and waits for the Kraken ack matching req_id.
*/
func (privateClient *PrivateClient) AmendOrder(
	ctx context.Context,
	request order.AmendRequest,
) (*order.Ack, error) {
	if err := privateClient.EnsureToken(); err != nil {
		return nil, err
	}

	request.Params.Token = privateClient.token.Value().Result.Token
	request.ReqID = privateClient.conn.NextReqID()

	privateClient.startReadPump()

	responseCh := privateClient.registerPending(request.ReqID)
	defer privateClient.dropPending(request.ReqID)

	if err := privateClient.Send(request); err != nil {
		return nil, err
	}

	return privateClient.waitAck(ctx, responseCh, "amend_order")
}

/*
WaitStopOrder blocks until Kraken creates the OTO stop order for one entry fill.
*/
func (privateClient *PrivateClient) WaitStopOrder(
	ctx context.Context,
	parentOrderID string,
	symbol string,
) (string, error) {
	if parentOrderID == "" {
		return "", fmt.Errorf("parent order id is required")
	}

	privateClient.startReadPump()

	stopCh := privateClient.registerStopWait(parentOrderID)
	defer privateClient.dropStopWait(parentOrderID)

	select {
	case stopOrderID := <-stopCh:
		if stopOrderID == "" {
			return "", fmt.Errorf("empty stop order id for parent %s", parentOrderID)
		}

		return stopOrderID, nil
	case <-ctx.Done():
		return "", fmt.Errorf("stop order timeout: %w", ctx.Err())
	}
}

/*
PollFill returns one buffered execution without blocking.
*/
func (privateClient *PrivateClient) PollFill(orderID string) (order.Fill, bool) {
	return privateClient.takeBufferedFill(orderID)
}

func (privateClient *PrivateClient) registerStopWait(parentOrderID string) chan string {
	stopCh := make(chan string, 1)
	privateClient.stopWaits.Store(parentOrderID, stopCh)

	return stopCh
}

func (privateClient *PrivateClient) dropStopWait(parentOrderID string) {
	privateClient.stopWaits.Delete(parentOrderID)
}

func (privateClient *PrivateClient) deliverStopOrder(parentOrderID, stopOrderID string) {
	value, found := privateClient.stopWaits.Load(parentOrderID)

	if !found {
		return
	}

	stopCh, ok := value.(chan string)

	if !ok {
		return
	}

	select {
	case stopCh <- stopOrderID:
	default:
	}
}

/*
WaitFill blocks until one trade fill arrives for orderID on the executions channel.
*/
func (privateClient *PrivateClient) WaitFill(
	ctx context.Context,
	orderID string,
) (order.Fill, error) {
	if orderID == "" {
		return order.Fill{}, fmt.Errorf("order id is required")
	}

	if buffered, ok := privateClient.takeBufferedFill(orderID); ok {
		return buffered, nil
	}

	privateClient.startReadPump()

	fillCh := privateClient.registerFillWait(orderID)
	defer privateClient.dropFillWait(orderID)

	select {
	case fill := <-fillCh:
		return fill, nil
	case <-ctx.Done():
		return order.Fill{}, fmt.Errorf("execution fill timeout: %w", ctx.Err())
	}
}

func (privateClient *PrivateClient) takeBufferedFill(orderID string) (order.Fill, bool) {
	value, ok := privateClient.fillBuffer.LoadAndDelete(orderID)

	if !ok {
		return order.Fill{}, false
	}

	fill, ok := value.(order.Fill)

	return fill, ok
}

func (privateClient *PrivateClient) bufferFill(fill order.Fill) {
	privateClient.fillBuffer.Store(fill.OrderID, fill)
}

func (privateClient *PrivateClient) waitAck(
	ctx context.Context,
	responseCh chan []byte,
	label string,
) (*order.Ack, error) {
	select {
	case payload := <-responseCh:
		ack, err := order.ParseAck(payload)

		if err != nil {
			return nil, err
		}

		if !ack.Success {
			if ack.Error == "" {
				return nil, fmt.Errorf("%s rejected", label)
			}

			return nil, fmt.Errorf("%s rejected: %s", label, ack.Error)
		}

		return ack, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("%s response timeout: %w", label, ctx.Err())
	}
}

func (privateClient *PrivateClient) registerPending(reqID int) chan []byte {
	responseCh := make(chan []byte, 1)
	privateClient.pending.Store(reqID, responseCh)

	return responseCh
}

func (privateClient *PrivateClient) dropPending(reqID int) {
	privateClient.pending.Delete(reqID)
}

func (privateClient *PrivateClient) registerFillWait(orderID string) chan order.Fill {
	fillCh := make(chan order.Fill, 1)
	privateClient.fillWaits.Store(orderID, fillCh)

	return fillCh
}

func (privateClient *PrivateClient) dropFillWait(orderID string) {
	privateClient.fillWaits.Delete(orderID)
}

func (privateClient *PrivateClient) startReadPump() {
	privateClient.readOnce.Do(func() {
		go privateClient.readPump()
	})
}

func (privateClient *PrivateClient) readPump() {
	for {
		if privateClient.ctx.Err() != nil {
			return
		}

		payload, err := privateClient.Read()

		if err != nil {
			if privateClient.ctx.Err() != nil {
				return
			}

			continue
		}

		privateClient.routeFrame(payload)
	}
}

func (privateClient *PrivateClient) routeFrame(payload []byte) {
	if reqID, ok := order.ReqIDFromFrame(payload); ok {
		if value, found := privateClient.pending.Load(reqID); found {
			responseCh, ok := value.(chan []byte)

			if ok {
				select {
				case responseCh <- payload:
				default:
				}
			}
		}
	}

	fills, err := order.ParseExecutionFills(payload)

	if err == nil {
		for _, fill := range fills {
			if value, found := privateClient.fillWaits.Load(fill.OrderID); found {
				fillCh, ok := value.(chan order.Fill)

				if !ok {
					continue
				}

				select {
				case fillCh <- fill:
				default:
				}

				continue
			}

			privateClient.bufferFill(fill)
		}
	}

	events, err := order.ParseExecutionEvents(payload)

	if err != nil {
		return
	}

	for _, event := range events {
		stopOrderID := order.FindOTOStopOrderID([]order.ExecutionEvent{event}, event.OrdRefID, event.Symbol)

		if stopOrderID == "" || event.OrdRefID == "" {
			continue
		}

		privateClient.deliverStopOrder(event.OrdRefID, stopOrderID)
	}
}
