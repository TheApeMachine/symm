package trader

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/ui"
)

type Instrument struct {
	pool    *qpool.Q[any]
	status  types.Status
	public  websocket.Conn
	private websocket.Conn
	level3  websocket.Conn
	cache   *sync.Map
	ring    *structure.SPSCRing[[]byte]
	quote   string
	uiHub   *ui.Hub
}

func NewInstrument(
	pool *qpool.Q[any],
	public websocket.Conn,
	private websocket.Conn,
	level3 websocket.Conn,
	uiHub *ui.Hub,
) *Instrument {
	return &Instrument{
		pool:    pool,
		status:  types.INITIALIZING,
		public:  public,
		private: private,
		level3:  level3,
		cache:   &sync.Map{},
		ring:    structure.NewSPSCRing[[]byte](8*1024, false),
		quote:   viper.GetString("market.quote_currency"),
		uiHub:   uiHub,
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
		if pair.Quote == instrument.quote && pair.Status == "online" {
			instrument.cache.LoadOrStore(pair.Symbol, pair)
		}
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
		for _, entity := range []json.Marshaler{
			kraken.NewTickerSubscription(batch),
			kraken.NewTradeSubscription(batch),
			kraken.NewBookSubscription(batch),
			kraken.NewOHLCSubscription(batch),
		} {
			errnie.Error(instrument.public.Write(entity))
		}

		for _, entity := range []json.Marshaler{
			kraken.NewLevel3Subscription(batch),
		} {
			errnie.Error(instrument.level3.Write(entity))
		}

		if index < len(pairs)-1 {
			time.Sleep(pace)
		}
	}

	if count > 0 && instrument.status != types.READY {
		instrument.status = types.READY
		errnie.Info("instrument ready")

		if instrument.uiHub != nil && instrument.uiHub.Messages != nil {
			select {
			case instrument.uiHub.Messages <- datura.Map[any]{
				"instruments": instrument.Pairs(),
			}.Marshal():
			default:
			}
		}
	}

	return nil
}
