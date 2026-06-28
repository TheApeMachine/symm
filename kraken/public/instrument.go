package public

import (
	"context"
	"sync"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/types"
)

type Instrument struct {
	ctx         context.Context
	cancel      context.CancelFunc
	err         error
	pool        *qpool.Q[any]
	broadcasts  *sync.Map
	subscribers *sync.Map
	cache       *sync.Map
	quote       string
}

func NewInstrument(ctx context.Context, pool *qpool.Q[any]) *Instrument {
	ctx, cancel := context.WithCancel(ctx)

	instrument := &Instrument{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  &sync.Map{},
		subscribers: &sync.Map{},
		cache:       &sync.Map{},
		quote:       viper.GetViper().GetString("market.quote_currency"),
	}

	return instrument
}

func (instrument *Instrument) Subscribe() error {
	instrument.cache.Clear()

	bg, _ := instrument.broadcasts.LoadOrStore(
		"kraken:public",
		instrument.pool.CreateBroadcastGroup("kraken:public"),
	)

	return errnie.Error(bg.(*qpool.BroadcastGroup).Send(datura.Acquire(
		"instrument", datura.APPJSON,
	).WithDestination(
		"kraken:public",
	).WithRole(
		"subscribe",
	).WithScope(
		"universe",
	).WithPayload(datura.Map[any]{
		"method": "subscribe",
		"params": datura.Map[any]{
			"channel": "instrument",
		},
		"req_id": 79,
	}.Marshal())))
}

func (instrument *Instrument) Update(artifact *datura.Artifact) {
	pairs := datura.Peek[[]datura.Map[any]](
		artifact, "data", "pairs",
	)

	out := make([][]string, 0)
	idx := -1

	for index, pair := range pairs {
		if index%100 == 0 {
			idx++
			out = append(out, make([]string, 0))
		}

		if pair["status"] == "online" && pair["quote"] == instrument.quote {
			symbol, ok := instrument.cache.LoadOrStore(
				pair["symbol"], datura.Acquire(
					"kraken:public", datura.APPJSON,
				).WithRole(
					"instrument",
				).WithScope(
					pair["symbol"].(string),
				).WithPayload(
					pair.Marshal(),
				),
			)

			if !ok {
				out[idx] = append(out[idx], datura.Peek[string](
					symbol.(*datura.Artifact), "scope"),
				)
			}
		}
	}

	for _, group := range out {
		bg, _ := instrument.broadcasts.LoadOrStore(
			"kraken:public",
			instrument.pool.CreateBroadcastGroup("kraken:public"),
		)

		for _, channel := range []string{"ticker", "trades", "ohlc", "book"} {
			payload := datura.Map[any]{
				"method": "subscribe",
				"params": datura.Map[any]{
					"channel": channel,
					"symbol":  group,
				},
				"req_id": 79,
			}

			if channel == "ohlc" {
				payload["params"].(datura.Map[any])["interval"] = 1
			}

			if channel == "trades" {
				payload["params"].(datura.Map[any])["snapshot"] = true
			}

			errnie.Error(bg.(*qpool.BroadcastGroup).Send(datura.Acquire(
				"instrument", datura.APPJSON,
			).WithDestination(
				"kraken:public",
			).WithRole(
				"subscribe",
			).WithScope(
				"universe",
			).WithPayload(payload.Marshal())))
		}
	}
}

func (instrument *Instrument) Send(message []byte) *types.SocketMessage {
	artifact := datura.Acquire("kraken:public", datura.APPJSON).
		WithPayload(message)
	defer artifact.Release()

	instrument.Update(artifact)

	out := &types.SocketMessage{}
	out.Decode(message)

	return out
}

func (instrument *Instrument) Observe(sockets ...types.Socket) {}
