package market

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/bus"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/market/quote"
	"github.com/theapemachine/symm/market/settings"
)

const instrumentSubscriberID = "market:instrument"

/*
InstrumentParams is the Kraken WebSocket v2 subscribe payload for the instrument channel.
*/
type InstrumentParams struct {
	Channel  string `json:"channel"`
	Snapshot bool   `json:"snapshot"`
}

/*
InstrumentAsset is one tradable asset's precision and margin metadata from the instrument feed.
*/
type InstrumentAsset struct {
	ID               string  `json:"id"`
	Status           string  `json:"status"`
	Precision        int     `json:"precision"`
	PrecisionDisplay int     `json:"precision_display"`
	Borrowable       bool    `json:"borrowable"`
	CollateralValue  float64 `json:"collateral_value"`
	MarginRate       float64 `json:"margin_rate"`
}

/*
InstrumentPair is one market's sizing, status, and increment rules from the instrument feed.
*/
type InstrumentPair struct {
	Symbol             string  `json:"symbol"`
	Base               string  `json:"base"`
	Quote              string  `json:"quote"`
	Status             string  `json:"status"`
	QtyPrecision       int     `json:"qty_precision"`
	QtyIncrement       float64 `json:"qty_increment"`
	PricePrecision     int     `json:"price_precision"`
	CostPrecision      int     `json:"cost_precision"`
	Marginable         bool    `json:"marginable"`
	HasIndex           bool    `json:"has_index"`
	CostMin            float64 `json:"cost_min"`
	MarginInitial      float64 `json:"margin_initial,omitempty"`
	PositionLimitLong  int     `json:"position_limit_long,omitempty"`
	PositionLimitShort int     `json:"position_limit_short,omitempty"`
	PriceIncrement     float64 `json:"price_increment"`
	QtyMin             float64 `json:"qty_min"`
}

/*
InstrumentUpdate is the instrument channel snapshot: the tradable asset and pair catalog.

The exchange's complete tradable catalog pushed live: every asset's precision and
margin terms, and every pair's status, sizing increments, minimums, and limits.
It is the authoritative, self-updating definition of what is currently tradable
and the exact rules for sizing and rounding an order, reflecting halts and
precision changes the moment they happen.
*/
type InstrumentUpdate struct {
	Assets []InstrumentAsset `json:"assets"`
	Pairs  []InstrumentPair  `json:"pairs"`
}

type Instrument struct {
	ctx           context.Context
	cancel        context.CancelFunc
	err           error
	pool          *qpool.Q
	broadcasts    map[string]*qpool.BroadcastGroup
	subscribers   map[string]*qpool.Subscriber
	subscribersMu sync.RWMutex
	Pairs         []string
}

func NewInstrument(ctx context.Context, pool *qpool.Q) *Instrument {
	ctx, cancel := context.WithCancel(ctx)

	instrument := &Instrument{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		Pairs:       make([]string, 0),
	}

	instrument.broadcasts["kraken:public"] = bus.Group(pool, "kraken:public", 10*time.Millisecond)
	instrument.broadcasts["raw"] = bus.Group(pool, "raw", 10*time.Millisecond)
	instrument.subscribers["raw"] = instrument.broadcasts["raw"].Subscribe(
		instrumentSubscriberID, 128,
	)

	return instrument
}

func (instrument *Instrument) Tick() error {
	incoming := instrument.subscribers["raw"].Incoming
	publicBroadcast := instrument.broadcasts["kraken:public"]
	bookDepth, err := settings.RequiredBookDepthLevels()

	if err != nil {
		return err
	}

	currency, err := settings.RequiredQuoteCurrency()

	if err != nil {
		return err
	}

	pace, err := settings.RequiredDuration("market.subscribe_pace")

	if err != nil {
		return err
	}

	scanCap := settings.ScanSymbolCap()

	for {
		select {
		case <-instrument.ctx.Done():
			return instrument.ctx.Err()
		case message, ok := <-incoming:
			if !ok {
				return io.EOF
			}

			if message == nil {
				continue
			}

			sm, ok := instrument.socketMessage(message.Value)

			if !ok {
				continue
			}

			if sm.Channel != public.InstrumentsChannel {
				continue
			}

			var update InstrumentUpdate

			if err := sonic.Unmarshal(sm.Data, &update); err != nil {
				return fmt.Errorf("instrument: decode snapshot: %w", err)
			}

			for _, pair := range update.Pairs {
				if scanCap > 0 && len(instrument.Pairs) >= scanCap {
					break
				}

				if !instrument.tradableQuotePair(pair, currency) {
					continue
				}

				if slices.Contains(instrument.Pairs, pair.Symbol) {
					continue
				}

				instrument.Pairs = append(instrument.Pairs, pair.Symbol)

				publicBroadcast.Send(&qpool.QValue[any]{Value: map[string]any{
					"method": "subscribe",
					"params": map[string]any{
						"channel":  "ticker",
						"symbol":   []string{pair.Symbol},
						"snapshot": true,
					},
				}})

				publicBroadcast.Send(&qpool.QValue[any]{Value: map[string]any{
					"method": "subscribe",
					"params": map[string]any{
						"channel":  "book",
						"depth":    bookDepth,
						"symbol":   []string{pair.Symbol},
						"snapshot": true,
					},
				}})

				publicBroadcast.Send(&qpool.QValue[any]{Value: map[string]any{
					"method": "subscribe",
					"params": map[string]any{
						"channel":  "ohlc",
						"interval": 1,
						"symbol":   []string{pair.Symbol},
						"snapshot": true,
					},
				}})

				publicBroadcast.Send(&qpool.QValue[any]{Value: map[string]any{
					"method": "subscribe",
					"params": map[string]any{
						"channel":  "trade",
						"symbol":   []string{pair.Symbol},
						"snapshot": true,
					},
				}})

				time.Sleep(pace)
			}
		}
	}
}

func (instrument *Instrument) socketMessage(value any) (*public.SocketMessage, bool) {
	switch typed := value.(type) {
	case *public.SocketMessage:
		return typed, typed != nil
	case map[string]any:
		channel, _ := typed["channel"].(string)

		if channel == "" {
			return nil, false
		}

		data, ok := instrument.rawData(typed["data"])

		if !ok {
			return nil, false
		}

		return &public.SocketMessage{Channel: channel, Data: data}, true
	default:
		return nil, false
	}
}

func (instrument *Instrument) rawData(value any) (json.RawMessage, bool) {
	switch typed := value.(type) {
	case json.RawMessage:
		return typed, true
	case []byte:
		return json.RawMessage(typed), true
	case string:
		return json.RawMessage(typed), true
	default:
		return nil, false
	}
}

func (instrument *Instrument) tradableQuotePair(pair InstrumentPair, currency string) bool {
	if pair.Symbol == "" {
		return false
	}

	if pair.Status != "online" {
		return false
	}

	pairQuote := quote.NormalizeCurrency(pair.Quote)

	if pairQuote != "" {
		return pairQuote == currency
	}

	return quote.SymbolMatchesCurrency(pair.Symbol, currency)
}

func (instrument *Instrument) Close() error {
	if instrument.cancel == nil {
		return nil
	}

	instrument.cancel()
	instrument.cancel = nil

	instrument.subscribersMu.Lock()
	defer instrument.subscribersMu.Unlock()

	for channel, subscriber := range instrument.subscribers {
		if subscriber == nil {
			continue
		}

		if broadcast, ok := instrument.broadcasts[channel]; ok && broadcast != nil {
			broadcast.Unsubscribe(subscriber.ID)
		}
	}

	instrument.subscribers = nil
	instrument.broadcasts = nil

	return nil
}
