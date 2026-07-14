package trader

import (
	"slices"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
Instrument owns Kraken pair metadata and the subscription universe. Its heavy
tier always includes open holdings so every managed position receives the data
required for continuation and exit decisions.
*/
type Instrument struct {
	status    atomic.Value
	api       *websocket.API
	price     *broker.Price
	cache     *sync.Map
	quote     string
	uiHub     chan []byte
	symbols   []string
	tierReady bool
}

/*
NewInstrument creates the market-instrument registry used by subscriptions and
order validation.
*/
func NewInstrument(
	api *websocket.API,
	price *broker.Price,
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

	for _, pair := range frame.Data.Pairs {
		instrument.cache.Store(pair.Symbol, pair)
	}

	status := instrument.Status()

	if status == types.READY || status == types.ERROR {
		return
	}

	if err := instrument.Subscribe(); err != nil {
		instrument.status.Store(types.ERROR)
		errnie.Error(err)
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
	if instrument.Status() == types.READY {
		return nil
	}

	errnie.Info("subscribing to instruments")

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

	for batch := range slices.Chunk(symbols, batchSize) {
		if err := instrument.api.SubscribeTicker(batch); err != nil {
			return errnie.Error(err)
		}

		if err := instrument.price.GetFees(batch); err != nil {
			return errnie.Error(err)
		}

		time.Sleep(pace)
	}

	instrument.status.Store(types.READY)

	return nil
}

/*
Tier returns the most immediately executable symbols from the observed ticker
cohort. It does not invent liquidity for listed pairs that have emitted no
ticker; the heavy tier becomes valid only when the cohort can fill every slot.
*/
func (instrument *Instrument) Tier(required []string) ([]string, bool, error) {
	rows, _ := instrument.price.Snapshot(instrument.symbols)

	size := viper.GetInt("market.universe.trading_tier_size")

	if size <= 0 || len(rows) < size {
		return nil, false, nil
	}

	requiredSet := make(map[string]struct{}, len(required))
	available := make(map[string]struct{}, len(instrument.symbols))

	for _, symbol := range instrument.symbols {
		available[symbol] = struct{}{}
	}

	for _, symbol := range required {
		if _, exists := available[symbol]; !exists {
			return nil, false, errnie.Err(
				errnie.Validation,
				"open holding has no online instrument: "+symbol,
				nil,
			)
		}

		requiredSet[symbol] = struct{}{}
	}

	if len(requiredSet) > size {
		return nil, false, errnie.Err(
			errnie.Validation,
			"open holdings exceed configured trading tier capacity",
			nil,
		)
	}

	sort.Slice(rows, func(left, right int) bool {
		leftDepth := types.ExecutableDepth(rows[left])
		rightDepth := types.ExecutableDepth(rows[right])

		if leftDepth != rightDepth {
			return leftDepth > rightDepth
		}

		leftNotional := types.QuoteNotional(rows[left])
		rightNotional := types.QuoteNotional(rows[right])

		if leftNotional != rightNotional {
			return leftNotional > rightNotional
		}

		return rows[left].Symbol < rows[right].Symbol
	})

	symbols := make([]string, 0, size)
	seen := make(map[string]struct{}, size)

	for symbol := range requiredSet {
		symbols = append(symbols, symbol)
	}

	sort.Strings(symbols)

	for _, symbol := range symbols {
		seen[symbol] = struct{}{}
	}

	for _, row := range rows {
		if len(symbols) == size {
			break
		}

		if _, exists := seen[row.Symbol]; exists {
			continue
		}

		symbols = append(symbols, row.Symbol)
	}

	return symbols, true, nil
}

/*
Activate subscribes the statistically selected heavy tier exactly once. Ticker
coverage remains universal while trade, book, and level3 compute stays within
the configured solver capacity.
*/
func (instrument *Instrument) Activate(required []string) (bool, error) {
	if instrument.tierReady {
		return true, nil
	}

	symbols, ready, err := instrument.Tier(required)

	if err != nil {
		return false, errnie.Error(err)
	}

	if !ready {
		return false, nil
	}

	subscribers := []func([]string) error{
		instrument.api.SubscribeTrade,
		instrument.api.SubscribeBook,
	}

	if viper.GetBool("market.l3_enabled") {
		subscribers = append(subscribers, instrument.api.SubscribeLevel3)
	}

	batchSize := viper.GetInt("market.subscribe_batch")

	for batch := range slices.Chunk(symbols, batchSize) {
		for _, subscribe := range subscribers {
			if err := subscribe(batch); err != nil {
				instrument.status.Store(types.ERROR)

				return false, errnie.Error(err)
			}
		}
	}

	instrument.tierReady = true

	return true, nil
}
