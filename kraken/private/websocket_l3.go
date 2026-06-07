package private

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/fasthttp/websocket"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/bus"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/user"
	"github.com/theapemachine/symm/market/settings"
)

func newDataOnlyWebSocket(
	ctx context.Context,
	pool *qpool.Q[any],
	apiKey string,
	apiSecret string,
) (*WebSocket, error) {
	if strings.TrimSpace(apiKey) == "" || strings.TrimSpace(apiSecret) == "" {
		return nil, fmt.Errorf("kraken/private: L3 requires API credentials")
	}

	ctx, cancel := context.WithCancel(ctx)
	provider, err := NewLiveTokenProvider(ctx, apiKey, apiSecret)

	if err != nil {
		cancel()
		return nil, err
	}

	raw := bus.Group(pool, "raw", 10*time.Millisecond)

	websocketClient := &WebSocket{
		ctx:           ctx,
		cancel:        cancel,
		provider:      provider,
		pool:          pool,
		raw:           raw,
		level3:        bus.Group(pool, "level3", 10*time.Millisecond),
		rawSubscriber: raw.Subscribe("kraken/private:l3-instrument", 1024),
		dialer:        *websocket.DefaultDialer,
		dataOnly:      true,
		l3Symbols:     make(map[string]struct{}),
	}

	return websocketClient, nil
}

func (websocketClient *WebSocket) subscribeLevel3(symbols []string) error {
	if websocketClient.conn == nil || len(symbols) == 0 {
		return nil
	}

	token, err := websocketClient.provider.Token(websocketClient.ctx)

	if err != nil {
		return err
	}

	frame := user.NewLevel3SubscribeFrame(symbols, settings.L3Depth(), token)

	return websocketClient.conn.WriteJSON(frame)
}

func (websocketClient *WebSocket) ensureLevel3Symbols(symbols []string) {
	if websocketClient.conn == nil {
		return
	}

	pending := make([]string, 0, len(symbols))

	for _, symbol := range symbols {
		if symbol == "" {
			continue
		}

		if _, seen := websocketClient.l3Symbols[symbol]; seen {
			continue
		}

		websocketClient.l3Symbols[symbol] = struct{}{}
		pending = append(pending, symbol)
	}

	if len(pending) == 0 {
		return
	}

	if err := websocketClient.subscribeLevel3(pending); err != nil {
		errnie.Error(err, "kraken/private: level3 subscribe")
	}
}

func (websocketClient *WebSocket) seedLevel3Symbols() {
	websocketClient.ensureLevel3Symbols(viper.GetStringSlice("market.default_symbols"))
}

func (websocketClient *WebSocket) watchInstrumentCatalog() {
	if websocketClient.rawSubscriber == nil {
		return
	}

	go func() {
		for {
			message, err := websocketClient.rawSubscriber.Wait(websocketClient.ctx)

			if err != nil {
				return
			}

			if message == nil || message.Value == nil {
				continue
			}

			websocketClient.ingestInstrumentForL3(message.Value)
		}
	}()
}

func (websocketClient *WebSocket) ingestInstrumentForL3(value any) {
	frame, ok := value.(map[string]any)

	if !ok {
		return
	}

	if frame["channel"] != public.InstrumentsChannel {
		return
	}

	rawData, ok := frame["data"].(json.RawMessage)

	if !ok {
		return
	}

	var update struct {
		Pairs []struct {
			Symbol string `json:"symbol"`
			Quote  string `json:"quote"`
			Status string `json:"status"`
		} `json:"pairs"`
	}

	if err := sonic.Unmarshal(rawData, &update); err != nil {
		errnie.Error(err, "kraken/private: instrument decode for L3")
		return
	}

	quoteCurrency, err := settings.RequiredQuoteCurrency()

	if err != nil {
		errnie.Error(err, "kraken/private: L3 quote currency")
		return
	}

	cap := settings.ScanSymbolCap()
	symbols := make([]string, 0, len(update.Pairs))

	for _, pair := range update.Pairs {
		if cap > 0 && len(symbols) >= cap {
			break
		}

		if pair.Status != "" && pair.Status != "online" {
			continue
		}

		if !strings.EqualFold(pair.Quote, quoteCurrency) {
			continue
		}

		if slices.Contains(symbols, pair.Symbol) {
			continue
		}

		symbols = append(symbols, pair.Symbol)
	}

	websocketClient.ensureLevel3Symbols(symbols)
}

func (websocketClient *WebSocket) publishLevel3(frame authFrame) {
	envelope := map[string]any{
		"channel": frame.Channel,
		"type":    frame.Type,
		"data":    append(json.RawMessage(nil), frame.Data...),
	}

	if websocketClient.raw != nil {
		user.PublishRaw(websocketClient.raw, frame.Channel, frame.Type, frame.Data)
	}

	if websocketClient.level3 != nil {
		websocketClient.level3.Send(&qpool.QValue[any]{Value: envelope})
	}
}
