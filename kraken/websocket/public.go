package websocket

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/callback"
	"github.com/krakenfx/api-go/v2/pkg/kraken"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
)

type Public struct {
	ctx              context.Context
	cancel           context.CancelFunc
	client           *spot.WebSocket
	url              string
	symbols          []string
	depth            int
	quote            string
	stream           *Stream
	symbolsCh        []chan []string
	buffer           int
	latestInstrument []byte
	mu               sync.Mutex
}

func NewPublic(ctx context.Context, symbols []string) *Public {
	ctx, cancel := context.WithCancel(ctx)
	buffer := viper.GetViper().GetInt("system.websocket.channel.buffer")

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
		stream: NewStream(buffer),
		buffer: buffer,
	}

	if public.url != "" {
		public.client.URL = public.url
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
		errnie.Info("websocket: public spot client connected, subscribing to instruments")
		errnie.Error(public.client.SubInstruments())

		if len(public.symbols) == 0 {
			return
		}

		errnie.Info(fmt.Sprintf("websocket: re-subscribing to %d cached symbols on reconnect", len(public.symbols)))
		public.Subscribe(public.symbols)
	})

	errnie.Info("websocket: connecting to public spot endpoint")
	errnie.Error(public.client.Connect())

	return public
}

func (public *Public) Observe(channel string) chan []byte {
	if public.stream == nil {
		public.stream = NewStream(public.buffer)
	}

	ch := public.stream.Observe(channel)

	if channel == "instrument" {
		public.mu.Lock()

		if public.latestInstrument != nil {
			select {
			case ch <- public.latestInstrument:
			default:
			}
		}

		public.mu.Unlock()
	}

	return ch
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
	if public.stream == nil {
		public.stream = NewStream(public.buffer)
	}

	channel := public.stream.Receive(raw)

	if channel == "instrument" {
		public.mu.Lock()
		data := public.stream.Data(raw)

		if len(data) > 0 {
			public.latestInstrument = append([]byte(nil), data...)
		}

		public.mu.Unlock()
		public.subscribe(raw)
	}
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

	errnie.Info(fmt.Sprintf("websocket: received instrument update with %d total pairs", len(pairs)))

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
		errnie.Info("websocket: no online pairs matched configured quote currency " + public.quote)
		return
	}

	errnie.Info(fmt.Sprintf("websocket: found %d matching online pairs for quote %s", len(symbols), public.quote))

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

	errnie.Info(fmt.Sprintf("websocket: subscribing to ticker, trades, candles, and book for %d symbols in batches of %d", len(symbols), batchSize))

	for start := 0; start < len(symbols); start += batchSize {
		end := min(start+batchSize, len(symbols))
		group := symbols[start:end]

		errnie.Info(fmt.Sprintf("websocket: subscribing batch: %v", group))

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
