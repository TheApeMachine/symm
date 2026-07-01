package public

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/bytedance/sonic"
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
		quote: strings.ToUpper(strings.TrimSpace(
			viper.GetViper().GetString("market.quote_currency"),
		)),
	}

	return instrument
}

func (instrument *Instrument) Subscribe() error {
	errnie.Info("subscribing to instruments")

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
	pairs := instrumentPairs(artifact)
	if len(pairs) == 0 {
		return
	}

	out := make([][]string, 0)

	for _, pair := range pairs {
		status, _ := pair["status"].(string)
		quote, _ := pair["quote"].(string)
		symbol, _ := pair["symbol"].(string)

		if status == "online" &&
			strings.ToUpper(strings.TrimSpace(quote)) == instrument.quote &&
			strings.TrimSpace(symbol) != "" {
			if len(out) == 0 || len(out[len(out)-1]) == 100 {
				out = append(out, make([]string, 0))
			}

			cached, ok := instrument.cache.LoadOrStore(
				symbol, datura.Acquire(
					"kraken:public", datura.APPJSON,
				).WithRole(
					"instrument",
				).WithScope(
					symbol,
				).WithPayload(
					pair.Marshal(),
				),
			)

			if !ok {
				out[len(out)-1] = append(out[len(out)-1], datura.Peek[string](
					cached.(*datura.Artifact), "scope"),
				)
			}
		}
	}

	count := 0

	for _, group := range out {
		if len(group) == 0 {
			continue
		}

		count += len(group)

		bg, _ := instrument.broadcasts.LoadOrStore(
			"kraken:public",
			instrument.pool.CreateBroadcastGroup("kraken:public"),
		)

		for _, channel := range []string{
			TickerChannel,
			TradesChannel,
			CandlesChannel,
			BookChannel,
		} {
			payload := datura.Map[any]{
				"method": "subscribe",
				"params": datura.Map[any]{
					"channel": channel,
					"symbol":  group,
				},
				"req_id": 79,
			}

			if channel == CandlesChannel {
				payload["params"].(datura.Map[any])["interval"] = 1
			}

			if channel == TradesChannel {
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

		privateBG, _ := instrument.broadcasts.LoadOrStore(
			"kraken:private",
			instrument.pool.CreateBroadcastGroup("kraken:private"),
		)

		errnie.Error(privateBG.(*qpool.BroadcastGroup).Send(datura.Acquire(
			"instrument", datura.APPJSON,
		).WithDestination(
			"kraken:private",
		).WithRole(
			"level3",
		).WithScope(
			"universe",
		).WithPayload(datura.Map[any]{
			"method": "subscribe",
			"params": datura.Map[any]{
				"channel": "level3",
				"symbol":  group,
			},
			"req_id": 79,
		}.Marshal())))
	}

	errnie.Info(fmt.Sprintf("subscribed to %d instruments", count))
}

func instrumentPairs(artifact *datura.Artifact) []datura.Map[any] {
	if artifact == nil || !artifact.IsValid() {
		return nil
	}

	var frame struct {
		Data struct {
			Pairs []datura.Map[any] `json:"pairs"`
		} `json:"data"`
	}

	if err := sonic.Unmarshal(artifact.DecryptPayload(), &frame); err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"kraken:public: instrument payload decode failed",
			err,
		))
		return nil
	}

	return frame.Data.Pairs
}

func (instrument *Instrument) Send(artifact *datura.Artifact) *datura.Artifact {
	if instrument == nil || artifact == nil || !artifact.IsValid() {
		return nil
	}

	instrument.Update(artifact)

	return artifact
}

func (instrument *Instrument) Observe(sockets ...types.Socket) {}
