package market

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/activate"
	"github.com/theapemachine/symm/kraken/public"
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
	pairSet       map[string]struct{}
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
		pairSet:     make(map[string]struct{}),
	}

	instrument.broadcasts["kraken:public"] = pool.CreateBroadcastGroup("kraken:public", 10*time.Millisecond)
	instrument.broadcasts["raw"] = pool.CreateBroadcastGroup("raw", 10*time.Millisecond)
	instrument.subscribers["raw"] = instrument.broadcasts["raw"].Subscribe(
		instrumentSubscriberID, 128,
	)

	activate.Boot("kraken/instrument ready")

	return instrument
}

func (instrument *Instrument) Tick() error {
	instrument.subscribersMu.RLock()
	incoming := instrument.subscribers["raw"].Incoming
	publicBroadcast := instrument.broadcasts["kraken:public"]
	instrument.subscribersMu.RUnlock()

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

			envelope, envelopeOK := message.Value.(public.SocketMessage)

			if !envelopeOK {
				continue
			}

			if envelope.Channel != public.InstrumentsChannel {
				continue
			}

			instrument.applyCatalogUpdate(publicBroadcast, envelope)
		}
	}
}

func (instrument *Instrument) applyCatalogUpdate(
	publicBroadcast *qpool.BroadcastGroup,
	envelope public.SocketMessage,
) {
	var update InstrumentUpdate

	if err := sonic.Unmarshal(envelope.Data, &update); err != nil {
		errnie.Error(err)
		return
	}

	if len(update.Pairs) > 0 {
		activate.Once("kraken/instrument:catalog")
	}

	maxScan := viper.GetInt("market.max_scan_symbols")

	for _, pair := range update.Pairs {
		if maxScan > 0 && len(instrument.Pairs) >= maxScan {
			break
		}

		if _, known := instrument.pairSet[pair.Symbol]; known {
			continue
		}

		instrument.Pairs = append(instrument.Pairs, pair.Symbol)
		instrument.pairSet[pair.Symbol] = struct{}{}

		if len(instrument.Pairs) == 1 {
			activate.Once("kraken/instrument:first-pair-subscribe")
		}

		publicBroadcast.Send(subscribeFrame("ticker", pair.Symbol, nil))
		publicBroadcast.Send(subscribeFrame("book", pair.Symbol, map[string]any{
			"depth": 1000,
		}))
		publicBroadcast.Send(subscribeFrame("trade", pair.Symbol, nil))
	}
}

func subscribeFrame(
	channel, symbol string, extra map[string]any,
) *qpool.QValue[any] {
	params := map[string]any{
		"channel":  channel,
		"symbol":   []string{symbol},
		"snapshot": true,
	}

	for key, value := range extra {
		params[key] = value
	}

	return &qpool.QValue[any]{Value: map[string]any{
		"method": "subscribe",
		"params": params,
	}}
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
