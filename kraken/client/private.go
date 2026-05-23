package client

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
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

	client := &PrivateClient{
		ctx:        ctx,
		cancel:     cancel,
		conn:       NewPublicClient(ctx, opts...),
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

	return privateClient.Authenticate()
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
Close closes the underlying public websocket session.
*/
func (privateClient *PrivateClient) Close() error {
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
Subscribe sends a subscribe frame, injecting a token when the channel requires auth.
*/
func (privateClient *PrivateClient) Subscribe(
	channel kraken.ChannelType,
) error {
	return privateClient.conn.Send(&kraken.Subscription{
		Method: kraken.MethodSubscribe,
		Params: map[string]any{
			"channel": channel,
			"token":   privateClient.token.Value().Result.Token,
		},
		ReqID: privateClient.conn.NextReqID(),
	})
}
