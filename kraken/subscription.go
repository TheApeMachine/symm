package kraken

import (
	"encoding/json"
	"fmt"
)

/*
Method is a Kraken WebSocket v2 request method.
*/
type Method string

const (
	MethodSubscribe   Method = "subscribe"
	MethodUnsubscribe Method = "unsubscribe"
)

type ChannelType string

const (
	ChannelTypeInstrument ChannelType = "instrument"
	ChannelTypeTicker     ChannelType = "ticker"
	ChannelTypeBook       ChannelType = "book"
	ChannelTypeTrades     ChannelType = "trades"
	ChannelTypeExecutions ChannelType = "executions"
)

/*
Subscription is a generic Kraken WebSocket v2 subscribe or unsubscribe request.
*/
type Subscription struct {
	Method Method `json:"method"`
	Params any    `json:"params"`
	ReqID  int    `json:"req_id,omitempty"`

	auth bool `json:"-"`
}

type SubscriptionOption func(*Subscription)

func WithReqID(reqID int) SubscriptionOption {
	return func(subscription *Subscription) {
		subscription.ReqID = reqID
	}
}

func WithAuth() SubscriptionOption {
	return func(subscription *Subscription) {
		subscription.auth = true
	}
}

func NewSubscribe(params any, opts ...SubscriptionOption) *Subscription {
	subscription := &Subscription{
		Method: MethodSubscribe,
		Params: params,
		auth:   requiresAuth(channelFromParams(params)),
	}

	for _, opt := range opts {
		opt(subscription)
	}

	return subscription
}

func NewUnsubscribe(params any, opts ...SubscriptionOption) *Subscription {
	subscription := &Subscription{
		Method: MethodUnsubscribe,
		Params: params,
		auth:   requiresAuth(channelFromParams(params)),
	}

	for _, opt := range opts {
		opt(subscription)
	}

	return subscription
}

func (subscription *Subscription) RequiresAuth() bool {
	return subscription.auth
}

func requiresAuth(channel string) bool {
	_, ok := authenticatedChannels[channel]
	return ok
}

func channelFromParams(params any) string {
	if params == nil {
		return ""
	}

	payload, err := json.Marshal(params)
	if err != nil {
		return ""
	}

	var fields struct {
		Channel string `json:"channel"`
	}
	if err := json.Unmarshal(payload, &fields); err != nil {
		return ""
	}

	return fields.Channel
}

func withToken(params any, token string) (any, error) {
	payload, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("marshal params: %w", err)
	}

	fields := map[string]any{}
	if err := json.Unmarshal(payload, &fields); err != nil {
		return nil, fmt.Errorf("unmarshal params: %w", err)
	}

	fields["token"] = token

	return fields, nil
}

var authenticatedChannels = map[string]struct{}{
	"executions": {},
	"balances":   {},
}
