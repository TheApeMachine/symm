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

type Public struct {
	ctx       context.Context
	cancel    context.CancelFunc
	client    *spot.WebSocket
	url       string
	symbols   []string
	depth     int
	quote     string
	observers map[string][]chan []byte
	symbolsCh []chan []string
	buffer    int
}

func NewPublic(ctx context.Context, symbols []string) *Public {
	ctx, cancel := context.WithCancel(ctx)

	public := &Public{
		ctx:     ctx,
		cancel:  cancel,
		client:  spot.NewWebSocket(),
		url:     os.Getenv("KRAKEN_API_SPOT_WS_URL"),
		symbols: symbols,
		depth:   viper.GetViper().GetInt("market.book.depth"),
		quote: strings.ToUpper(strings.TrimSpace(
			viper.GetViper().GetString("market.quote_currency"),
		)),
		observers: map[string][]chan []byte{},
		buffer:    viper.GetViper().GetInt("system.websocket.channel.buffer"),
	}

	public.client.OnSent.Recurring(func(e *callback.Event[*kraken.WebSocketMessage]) {
		public.checkContext()
	})

	public.client.OnReceived.Recurring(func(e *callback.Event[*kraken.WebSocketMessage]) {
		public.checkContext()
		public.receive(e.Data.Bytes())
	})

	public.client.OnConnected.Recurring(func(e *callback.Event[any]) {
		public.checkContext()
		errnie.Error(public.client.SubInstruments())

		if len(public.symbols) == 0 {
			return
		}

		public.Subscribe(public.symbols)
	})

	errnie.Error(public.client.Connect())

	return public
}

func (public *Public) Observe(channel string) chan []byte {
	out := make(chan []byte, public.buffer)

	if public.observers == nil {
		public.observers = map[string][]chan []byte{}
	}

	public.observers[channel] = append(public.observers[channel], out)
	return out
}

func (public *Public) Symbols() chan []string {
	out := make(chan []string, public.buffer)
	public.symbolsCh = append(public.symbolsCh, out)

	if len(public.symbols) > 0 {
		out <- append([]string(nil), public.symbols...)
	}

	return out
}

func (public *Public) receive(raw []byte) {
	channel := public.channel(raw)
	if channel == "" {
		return
	}

	if channel == "instrument" {
		public.subscribe(raw)
	}

	if len(public.observers[channel]) == 0 {
		return
	}

	data := public.data(raw)
	if len(data) == 0 {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"websocket: frame data required",
			nil,
		))
		return
	}

	for _, observer := range public.observers[channel] {
		observer <- data
	}
}

func (public *Public) channel(raw []byte) string {
	node, err := sonic.Get(raw, "channel")
	if err != nil || !node.Exists() {
		return ""
	}

	channel, err := node.String()
	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"websocket: channel string required",
			err,
		))
		return ""
	}

	return strings.TrimSpace(channel)
}

func (public *Public) data(raw []byte) []byte {
	node, err := sonic.Get(raw, "data")
	if err != nil || !node.Exists() {
		return nil
	}

	data, err := node.Raw()
	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"websocket: data payload required",
			err,
		))
		return nil
	}

	return []byte(data)
}

func (public *Public) subscribe(raw []byte) {
	if len(public.symbols) > 0 {
		return
	}

	var frame map[string]any
	if err := sonic.Unmarshal(raw, &frame); err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"websocket: decode instrument frame",
			err,
		))
		return
	}

	if channel, _ := frame["channel"].(string); channel != "instrument" {
		return
	}

	data, _ := frame["data"].(map[string]any)
	pairs, _ := data["pairs"].([]any)
	if len(pairs) == 0 {
		return
	}

	symbols := make([]string, 0, len(pairs))
	for _, item := range pairs {
		pair, _ := item.(map[string]any)
		symbol, _ := pair["symbol"].(string)
		status, _ := pair["status"].(string)
		quote, _ := pair["quote"].(string)

		if strings.TrimSpace(symbol) == "" {
			continue
		}

		if status != "online" {
			continue
		}

		if strings.ToUpper(strings.TrimSpace(quote)) != public.quote {
			continue
		}

		symbols = append(symbols, symbol)
	}

	if len(symbols) == 0 {
		return
	}

	public.symbols = symbols
	public.Subscribe(symbols)

	for _, observer := range public.symbolsCh {
		observer <- append([]string(nil), symbols...)
	}
}

func (public *Public) Subscribe(symbols []string) {
	batchSize := viper.GetViper().GetInt("market.subscribe_batch")
	if batchSize <= 0 {
		batchSize = len(symbols)
	}

	for start := 0; start < len(symbols); start += batchSize {
		end := min(start+batchSize, len(symbols))
		group := symbols[start:end]

		errnie.Error(public.client.SubTicker(group))
		errnie.Error(public.client.SubTrades(group))
		errnie.Error(public.client.SubCandles(group))

		if public.depth > 0 {
			errnie.Error(public.client.SubBook(group, public.depth))
		}
	}
}

func (public *Public) checkContext() {
	select {
	case <-public.ctx.Done():
		public.Close()
	default:
	}
}

func (public *Public) Close() {
	public.cancel()
	public.client.Disconnect()
}
