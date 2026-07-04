package websocket

import (
	"context"
	"os"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/callback"
	"github.com/krakenfx/api-go/v2/pkg/kraken"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
)

type L3 struct {
	ctx        context.Context
	cancel     context.CancelFunc
	client     *spot.WebSocket
	url        string
	publicKey  string
	privateKey string
	symbols    []string
	depth      int
	observers  map[string][]chan []byte
	buffer     int
	auth       bool
	subscribed bool
}

func NewL3(ctx context.Context, symbols []string) *L3 {
	ctx, cancel := context.WithCancel(ctx)

	l3 := &L3{
		ctx:        ctx,
		cancel:     cancel,
		client:     spot.NewWebSocket(),
		url:        os.Getenv("KRAKEN_API_SPOT_WS_L3_URL"),
		publicKey:  os.Getenv("KRAKEN_API_KEY"),
		privateKey: os.Getenv("KRAKEN_API_SECRET"),
		symbols:    symbols,
		depth:      viper.GetViper().GetInt("market.l3_depth"),
		observers:  map[string][]chan []byte{},
		buffer:     viper.GetViper().GetInt("system.websocket.channel.buffer"),
	}

	l3.client.REST.PublicKey = l3.publicKey
	l3.client.REST.PrivateKey = l3.privateKey
	if l3.url != "" {
		l3.client.URL = l3.url
	}

	l3.client.OnSent.Recurring(func(e *callback.Event[*kraken.WebSocketMessage]) {
		l3.checkContext()
	})

	l3.client.OnReceived.Recurring(func(e *callback.Event[*kraken.WebSocketMessage]) {
		l3.checkContext()
		l3.receive(e.Data.Bytes())
	})

	l3.client.OnAuthenticated.Recurring(func(e *callback.Event[string]) {
		l3.checkContext()
		l3.auth = true
		l3.Subscribe(l3.symbols)
	})

	l3.client.OnConnected.Recurring(func(e *callback.Event[any]) {
		l3.checkContext()
		l3.subscribed = false
		errnie.Error(l3.client.Authenticate())
	})

	errnie.Error(l3.client.Connect())

	return l3
}

func (l3 *L3) Observe(channel string) chan []byte {
	out := make(chan []byte, l3.buffer)

	if l3.observers == nil {
		l3.observers = map[string][]chan []byte{}
	}

	l3.observers[channel] = append(l3.observers[channel], out)
	return out
}

func (l3 *L3) receive(raw []byte) {
	channel := l3.channel(raw)
	if channel == "" || len(l3.observers[channel]) == 0 {
		return
	}

	data := l3.data(raw)
	if len(data) == 0 {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"websocket: level3 data required",
			nil,
		))
		return
	}

	for _, observer := range l3.observers[channel] {
		observer <- data
	}
}

func (l3 *L3) channel(raw []byte) string {
	node, err := sonic.Get(raw, "channel")
	if err != nil || !node.Exists() {
		return ""
	}

	channel, err := node.String()
	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"websocket: level3 channel required",
			err,
		))
		return ""
	}

	return strings.TrimSpace(channel)
}

func (l3 *L3) data(raw []byte) []byte {
	node, err := sonic.Get(raw, "data")
	if err != nil || !node.Exists() {
		return nil
	}

	data, err := node.Raw()
	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"websocket: level3 payload required",
			err,
		))
		return nil
	}

	return []byte(data)
}

func (l3 *L3) Subscribe(symbols []string) {
	if len(symbols) == 0 {
		return
	}

	l3.symbols = append([]string(nil), symbols...)
	if !l3.auth || l3.subscribed {
		return
	}

	batchSize := viper.GetViper().GetInt("market.subscribe_batch")
	if batchSize <= 0 {
		batchSize = len(l3.symbols)
	}

	for start := 0; start < len(l3.symbols); start += batchSize {
		end := min(start+batchSize, len(l3.symbols))
		errnie.Error(l3.client.SubL3(
			l3.symbols[start:end],
			l3.depth,
			map[string]any{"params": map[string]any{"depth": l3.depth}},
		))
	}

	l3.subscribed = true
}

func (l3 *L3) checkContext() {
	select {
	case <-l3.ctx.Done():
		l3.Close()
	default:
	}
}

func (l3 *L3) Close() {
	l3.cancel()
	l3.client.Disconnect()
}
