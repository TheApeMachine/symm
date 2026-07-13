package trader

import (
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

type Instrument struct {
	status types.Status
	api    *websocket.API
	price  *broker.Price
	cache  *sync.Map
	quote  string
	uiHub  chan []byte
}

func NewInstrument(
	api *websocket.API,
	price *broker.Price,
	channel chan []byte,
) *Instrument {
	instrument := &Instrument{
		status: types.INITIALIZING,
		api:    api,
		price:  price,
		cache:  &sync.Map{},
		quote:  viper.GetString("market.quote_currency"),
		uiHub:  channel,
	}

	return instrument
}

func (instrument *Instrument) Initialize() error {
	errnie.Info("initializing instrument")

	instrument.api.On("instrument", instrument.On)

	if err := instrument.api.SubscribeInstruments(); err != nil {
		instrument.status = types.ERROR
		errnie.Error(err)
	}

	instrument.status = types.READY
	return nil
}

func (instrument *Instrument) On(data []byte) {
	frame := utils.Unmarshal[kraken.Instrument](data)

	if len(frame.Data.Pairs) == 0 {
		return
	}

	for _, pair := range frame.Data.Pairs {
		instrument.cache.Store(pair.Symbol, pair)
	}

	if instrument.status == types.READY || instrument.status == types.ERROR {
		return
	}

	if err := instrument.Subscribe(); err != nil {
		instrument.status = types.ERROR
		errnie.Error(err)
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

	sort.Slice(pairs, func(left, right int) bool {
		return pairs[left].Symbol < pairs[right].Symbol
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

func (instrument *Instrument) Subscribe() error {
	if instrument.status == types.READY {
		return nil
	}

	errnie.Info("subscribing to instruments")

	instrument.status = types.PENDING
	symbols := make([]string, 0)
	seen := make(map[string]struct{})

	for _, pair := range instrument.Pairs() {
		if pair.Quote != instrument.quote || pair.Status != "online" {
			continue
		}

		if _, duplicate := seen[pair.Symbol]; duplicate {
			continue
		}

		seen[pair.Symbol] = struct{}{}
		symbols = append(symbols, pair.Symbol)
	}

	if len(symbols) == 0 {
		instrument.status = types.INITIALIZING

		return nil
	}

	tierSize := viper.GetInt("market.universe.trading_tier_size")

	if tierSize > 0 && len(symbols) > tierSize {
		symbols = symbols[:tierSize]
	}

	if err := instrument.price.GetFees(symbols); err != nil {
		return errnie.Error(err)
	}

	subscribers := []func([]string) error{
		instrument.api.SubscribeTicker,
		instrument.api.SubscribeTrade,
		instrument.api.SubscribeBook,
	}

	if viper.GetBool("market.l3_enabled") {
		subscribers = append(subscribers, instrument.api.SubscribeLevel3)
	}

	batchSize := viper.GetInt("market.subscribe_batch")
	pace := viper.GetDuration("market.subscribe_pace")

	for batch := range slices.Chunk(symbols, batchSize) {
		for _, subscribe := range subscribers {
			if err := subscribe(batch); err != nil {
				return errnie.Error(err)
			}
		}

		time.Sleep(pace)
	}

	instrument.status = types.READY

	return nil
}
