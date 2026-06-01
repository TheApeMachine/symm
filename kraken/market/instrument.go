package market

import (
	"context"
	"slices"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
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

/*
NewInstrumentSubscription opens the instrument channel and forwards snapshots to recv.
It blocks until ctx is canceled or the socket closes.
*/
func NewInstrumentSubscription(ctx context.Context, pool *qpool.Q) <-chan *InstrumentUpdate {
	feed := OpenFeed(ctx, pool, public.InstrumentsChannel, InstrumentParams{
		Channel:  public.InstrumentsChannel,
		Snapshot: true,
	})

	out := make(chan *InstrumentUpdate, 128)

	go func() {
		defer close(out)

		for msg := range feed.Stream {
			if msg == nil {
				continue
			}

			var instrument InstrumentUpdate

			if err := sonic.Unmarshal(msg.Data, &instrument); err != nil {
				errnie.Error(err)
				continue
			}

			select {
			case <-ctx.Done():
				return
			case out <- &instrument:
			}
		}
	}()

	return out
}

type Instrument struct {
	ctx         context.Context
	cancel      context.CancelFunc
	err         error
	pool        *qpool.Q
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	Pairs       []string
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

	instrument.broadcasts["kraken:public"] = pool.CreateBroadcastGroup("kraken:public", 10*time.Millisecond)
	instrument.broadcasts["instrument"] = pool.CreateBroadcastGroup("instrument", 10*time.Millisecond)
	instrument.subscribers["instrument"] = instrument.broadcasts["instrument"].Subscribe(
		instrumentSubscriberID, 128,
	)

	return instrument
}

func (instrument *Instrument) Tick() error {
	incoming := instrument.subscribers["instrument"].Incoming

	for {
		select {
		case <-instrument.ctx.Done():
			return instrument.ctx.Err()
		case message, ok := <-incoming:
			if !ok {
				return instrument.ctx.Err()
			}

			if message == nil {
				continue
			}

			envelope, envelopeOK := message.Value.(public.SocketMessage)

			if !envelopeOK {
				continue
			}

			var update InstrumentUpdate

			if err := sonic.Unmarshal(envelope.Data, &update); err != nil {
				errnie.Error(err)
				continue
			}

			for _, pair := range update.Pairs {
				if slices.Contains(instrument.Pairs, pair.Symbol) {
					continue
				}

				instrument.Pairs = append(instrument.Pairs, pair.Symbol)

				instrument.broadcasts["kraken:public"].Send(&qpool.QValue[any]{Value: map[string]any{
					"method": "subscribe",
					"params": map[string]any{
						"channel":  "ticker",
						"symbol":   []string{pair.Symbol},
						"snapshot": true,
					},
				}})

				instrument.broadcasts["kraken:public"].Send(&qpool.QValue[any]{Value: map[string]any{
					"method": "subscribe",
					"params": map[string]any{
						"channel":  "book",
						"depth":    1000,
						"symbol":   []string{pair.Symbol},
						"snapshot": true,
					},
				}})

				instrument.broadcasts["kraken:public"].Send(&qpool.QValue[any]{Value: map[string]any{
					"method": "subscribe",
					"params": map[string]any{
						"channel":  "ohlc",
						"interval": 1,
						"symbol":   []string{pair.Symbol},
						"snapshot": true,
					},
				}})

				instrument.broadcasts["kraken:public"].Send(&qpool.QValue[any]{Value: map[string]any{
					"method": "subscribe",
					"params": map[string]any{
						"channel":  "trade",
						"symbol":   []string{pair.Symbol},
						"snapshot": true,
					},
				}})
			}
		}
	}
}

func (instrument *Instrument) Close() error {
	if instrument.cancel == nil {
		return nil
	}

	instrument.cancel()
	instrument.cancel = nil

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
