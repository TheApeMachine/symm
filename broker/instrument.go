package broker

import (
	"slices"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
Instrument owns Kraken pair metadata and the subscription universe. Every
online pair for the configured quote currency remains observable so strategy
can discover opportunities across the whole tradable market.
*/
type Instrument struct {
	status  atomic.Value
	api     *websocket.API
	price   *Price
	cache   *sync.Map
	quote   string
	uiHub   chan []byte
	symbols []string
	active  bool
}

/*
NewInstrument creates the market-instrument registry used by subscriptions and
order validation.
*/
func NewInstrument(
	api *websocket.API,
	price *Price,
	channel chan []byte,
) *Instrument {
	instrument := &Instrument{
		api:   api,
		price: price,
		cache: &sync.Map{},
		quote: viper.GetString("market.quote_currency"),
		uiHub: channel,
	}
	instrument.status.Store(types.INITIALIZING)

	return instrument
}

func (instrument *Instrument) Initialize() error {
	errnie.Info("initializing instrument")
	instrument.status.Store(types.PENDING)

	instrument.api.On("instrument", instrument.On)

	if err := instrument.api.SubscribeInstruments(); err != nil {
		instrument.status.Store(types.ERROR)
		return errnie.Error(err)
	}

	return nil
}

func (instrument *Instrument) On(data []byte) {
	frame := utils.Unmarshal[kraken.Instrument](data)

	if len(frame.Data.Pairs) == 0 {
		return
	}

	for index := range frame.Data.Pairs {
		pair := frame.Data.Pairs[index]
		instrument.cache.Store(pair.Symbol, &pair)
	}

	instrument.Publish()

	if slices.Contains([]types.Status{
		types.READY, types.ERROR,
	}, instrument.Status()) {
		return
	}

	if err := instrument.Subscribe(); err != nil {
		instrument.status.Store(types.ERROR)
		errnie.Error(err)
	}
}

/*
Publish forwards the online quote-currency instrument universe to the terminal
so the command palette can search concrete symbols.
*/
func (instrument *Instrument) Publish() {
	if instrument == nil || instrument.uiHub == nil {
		return
	}

	pairs := make([]datura.Map[any], 0)

	for _, pair := range instrument.Pairs() {
		if pair.Quote != instrument.quote || pair.Status != "online" {
			continue
		}

		pairs = append(pairs, datura.Map[any]{
			"symbol": pair.Symbol,
			"base":   pair.Base,
			"quote":  pair.Quote,
			"status": pair.Status,
		})
	}

	if len(pairs) == 0 {
		return
	}

	frame := datura.Map[any]{
		"instruments": pairs,
	}.Marshal()

	select {
	case instrument.uiHub <- frame:
	default:
		// Hub coalesce owns latest-by-key; retry once so a full channel does
		// not permanently strand the command-palette universe at a tiny snapshot.
		select {
		case instrument.uiHub <- frame:
		default:
		}
	}
}

func (instrument *Instrument) Status() types.Status {
	status := instrument.status.Load()

	if status == nil {
		return types.INITIALIZING
	}

	return status.(types.Status)
}

/*
Pairs returns the cached instrument pairs, if known.
*/
func (instrument *Instrument) Pairs() []kraken.InstrumentPair {
	pairs := make([]kraken.InstrumentPair, 0)

	instrument.cache.Range(func(key, value any) bool {
		pairs = append(pairs, *value.(*kraken.InstrumentPair))
		return true
	})

	sort.Slice(pairs, func(left, right int) bool {
		return pairs[left].Symbol < pairs[right].Symbol
	})

	return pairs
}

/*
Remember stores one pair for paper/fixture sizing without a live instrument feed.
*/
func (instrument *Instrument) Remember(pair *kraken.InstrumentPair) {
	if instrument == nil || pair == nil || pair.Symbol == "" {
		return
	}

	instrument.cache.Store(pair.Symbol, pair)
}

/*
Pair returns the cached instrument metadata for the symbol, if known.
*/
func (instrument *Instrument) Pair(symbol string) (*kraken.InstrumentPair, error) {
	value, ok := instrument.cache.Load(symbol)

	if !ok {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"trader: instrument pair not found",
			nil,
		))
	}

	return value.(*kraken.InstrumentPair), nil
}

func (instrument *Instrument) Subscribe() error {
	if instrument.Status() == types.READY {
		return nil
	}

	errnie.Info("subscribing to instruments")

	symbols := make([]string, 0)
	seen := make(map[string]struct{})

	instrument.cache.Range(func(_, value any) bool {
		pair := value.(*kraken.InstrumentPair)

		if pair.Quote != instrument.quote || pair.Status != "online" {
			return true
		}

		if _, duplicate := seen[pair.Symbol]; duplicate {
			return true
		}

		seen[pair.Symbol] = struct{}{}
		symbols = append(symbols, pair.Symbol)

		return true
	})

	if len(symbols) == 0 {
		instrument.status.Store(types.INITIALIZING)
		return nil
	}

	batchSize := viper.GetInt("market.subscribe_batch")

	if batchSize < 1 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"trader: market.subscribe_batch must be at least 1",
			nil,
		))
	}

	pace := viper.GetDuration("market.subscribe_pace")
	instrument.symbols = symbols

	subscribers := []func([]string) error{
		instrument.api.SubscribeTrade,
		instrument.api.SubscribeBook,
		instrument.api.SubscribeTicker,
		instrument.api.SubscribeLevel3,
	}

	for batch := range slices.Chunk(symbols, batchSize) {
		if err := instrument.price.GetFees(batch); err != nil {
			return errnie.Error(err)
		}

		for _, subscribe := range subscribers {
			if err := subscribe(batch); err != nil {
				return errnie.Error(err)
			}
		}

		time.Sleep(pace)
	}

	instrument.status.Store(types.READY)

	return nil
}
