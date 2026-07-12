package trader

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
InstrumentAPI is the Kraken subscription surface owned by Instrument.
*/
type InstrumentAPI interface {
	SubscribeTicker(pairs []string) error
	SubscribeTrade(pairs []string) error
	SubscribeBook(pairs []string) error
	SubscribeOHLC(pairs []string) error
	SubscribeLevel3(pairs []string) error
	UnsubscribeLevel3(pairs []string) error
}

type Instrument struct {
	status       types.Status
	api          InstrumentAPI
	price        *broker.Price
	cache        *sync.Map
	ring         *structure.SPSCRing[[]byte]
	quote        string
	uiHub        chan []byte
	plan         *SubscriptionPlan
	feesHydrated bool
}

func NewInstrument(
	api InstrumentAPI,
	price *broker.Price,
	channel chan []byte,
) *Instrument {
	return &Instrument{
		status: types.INITIALIZING,
		api:    api,
		price:  price,
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

/*
RefreshLevel3 requests one new authoritative snapshot for symbol by replacing
its current Kraken level3 subscription.
*/
func (instrument *Instrument) RefreshLevel3(symbol string) error {
	symbol = strings.TrimSpace(symbol)

	if symbol == "" {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"trader: level3 refresh symbol required",
			nil,
		))
	}

	pairs := []string{symbol}

	if err := instrument.api.UnsubscribeLevel3(pairs); err != nil {
		return err
	}

	return instrument.api.SubscribeLevel3(pairs)
}

func (instrument *Instrument) On(data []byte) {
	frame := make([]byte, len(data))
	copy(frame, data)

	message := kraken.NewInstrumentData(frame)

	for _, pair := range message.Pairs {
		if pair.Quote != instrument.quote || pair.Status != "online" {
			continue
		}

		instrument.cache.LoadOrStore(pair.Symbol, pair)
	}
}

func (instrument *Instrument) Subscribe() error {
	if instrument.status == types.READY {
		return nil
	}

	instrument.status = types.PENDING

	if instrument.plan == nil {
		return instrument.observe()
	}

	if !instrument.plan.Ranked() {
		rows, missing := instrument.price.Snapshot(instrument.plan.Symbols())

		if len(missing) > 0 {
			return nil
		}

		tradingTierSize := viper.GetInt("market.universe.trading_tier_size")

		if err := instrument.plan.Rank(rows, tradingTierSize); err != nil {
			instrument.status = types.ERROR
			return err
		}
	}

	if !instrument.feesHydrated {
		if err := instrument.price.GetFees(instrument.tradingSymbols()); err != nil {
			instrument.status = types.ERROR
			return err
		}

		instrument.feesHydrated = true
	}

	if err := instrument.subscribeHeavy(); err != nil {
		instrument.status = types.ERROR
		return err
	}

	instrument.status = types.READY
	errnie.Info("instrument ready")

	if instrument.uiHub != nil {
		instrument.uiHub <- datura.Map[any]{
			"instruments": instrument.Pairs(),
		}.Marshal()
	}

	return nil
}

func (instrument *Instrument) observe() error {
	pairs := instrument.Pairs()

	if len(pairs) == 0 {
		return nil
	}

	batchSize := viper.GetInt("market.subscribe_batch")
	pace := viper.GetDuration("market.subscribe_pace")
	plan, err := NewSubscriptionPlan(pairs, batchSize)

	if err != nil {
		instrument.status = types.ERROR
		return err
	}

	if err := instrument.subscribe(plan.Observation(), pace, []func([]string) error{
		instrument.api.SubscribeTicker,
		instrument.api.SubscribeOHLC,
	}); err != nil {
		instrument.status = types.ERROR
		return err
	}

	instrument.plan = plan
	return nil
}

func (instrument *Instrument) subscribeHeavy() error {
	heavy := []func([]string) error{
		instrument.api.SubscribeTrade,
		instrument.api.SubscribeBook,
	}

	if viper.GetBool("market.l3_enabled") {
		heavy = append(heavy, instrument.api.SubscribeLevel3)
	}

	return instrument.subscribe(
		instrument.plan.Trading(),
		viper.GetDuration("market.subscribe_pace"),
		heavy,
	)
}

func (instrument *Instrument) tradingSymbols() []string {
	symbols := make([]string, 0)

	for _, batch := range instrument.plan.Trading() {
		symbols = append(symbols, batch...)
	}

	return symbols
}

func (instrument *Instrument) subscribe(
	batches [][]string,
	pace time.Duration,
	subscriptions []func([]string) error,
) error {
	for index, batch := range batches {
		for _, subscribe := range subscriptions {
			if err := subscribe(batch); err != nil {
				return err
			}
		}

		if index < len(batches)-1 {
			time.Sleep(pace)
		}
	}

	return nil
}
