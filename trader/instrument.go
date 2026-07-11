package trader

import (
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

type Instrument struct {
	status types.Status
	api    *websocket.API
	cache  *sync.Map
	ring   *structure.SPSCRing[[]byte]
	quote  string
	uiHub  chan []byte
}

func NewInstrument(api *websocket.API, channel chan []byte) *Instrument {
	return &Instrument{
		status: types.INITIALIZING,
		api:    api,
		cache:  &sync.Map{},
		ring:   structure.NewSPSCRing[[]byte](8*1024, false),
		quote:  viper.GetString("market.quote_currency"),
		uiHub:  channel,
	}
}

func (instrument *Instrument) Status() types.Status {
	return instrument.status
}

/*
Pairs returns the cached instrument pairs, if known.
*/
func (instrument *Instrument) Pairs() []kraken.InstrumentPair {
	pairs := make([]kraken.InstrumentPair, 0)

	instrument.cache.Range(func(key, value any) bool {
		pairs = append(pairs, value.(kraken.InstrumentPair))
		return true
	})

	return pairs
}

/*
Pair returns the cached instrument metadata for the symbol, if known.
*/
func (instrument *Instrument) Pair(symbol string) (kraken.InstrumentPair, error) {
	value, ok := instrument.cache.Load(symbol)

	if !ok {
		return kraken.InstrumentPair{}, errnie.Error(errnie.Err(
			errnie.NotFound,
			"trader: instrument pair not found",
			nil,
		))
	}

	return value.(kraken.InstrumentPair), nil
}

func (instrument *Instrument) On(data []byte) {
	frame := make([]byte, len(data))
	copy(frame, data)

	message := kraken.NewInstrumentData(frame)

	for _, pair := range message.Pairs {
		if pair.Quote != instrument.quote && pair.Status != "online" {
			continue
		}

		instrument.cache.LoadOrStore(pair.Symbol, pair)
	}
}

func (instrument *Instrument) Subscribe() error {
	if instrument.status == types.READY {
		return nil
	}

	count := 0

	instrument.status = types.PENDING
	batchSize := viper.GetInt("market.subscribe_batch")
	pace := viper.GetDuration("market.subscribe_pace")

	pairs := make([][]string, 0)

	instrument.cache.Range(func(key, value any) bool {
		if count%batchSize == 0 {
			pairs = append(pairs, make([]string, 0))
		}

		symbol := key.(string)
		pairs[len(pairs)-1] = append(pairs[len(pairs)-1], symbol)

		count++
		return true
	})

	for index, batch := range pairs {
		for _, subscribe := range []func([]string) error{
			instrument.api.SubscribeTicker,
			instrument.api.SubscribeTrade,
			instrument.api.SubscribeBook,
			instrument.api.SubscribeOHLC,
			instrument.api.SubscribeLevel3,
		} {
			if err := subscribe(batch); err != nil {
				errnie.Error(errnie.Err(
					errnie.Validation,
					err.Error(),
					nil,
				))
			}
		}

		if index < len(pairs)-1 {
			time.Sleep(pace)
		}
	}

	if count > 0 && instrument.status != types.READY {
		instrument.status = types.READY
		errnie.Info("instrument ready")

		if instrument.uiHub != nil {
			instrument.uiHub <- datura.Map[any]{
				"instruments": instrument.Pairs(),
			}.Marshal()
		}
	}

	return nil
}
