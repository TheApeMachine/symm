package trader

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/types"
)

type Instrument struct {
	ctx        context.Context
	cancel     context.CancelFunc
	err        error
	pool       *qpool.Q[any]
	broadcasts *sync.Map
	pairs      structure.Ring[market.InstrumentPair]
	quote      string
	known      *sync.Map
}

func NewInstrument(ctx context.Context, pool *qpool.Q[any]) *Instrument {
	ctx, cancel := context.WithCancel(ctx)

	instrument := &Instrument{
		ctx:        ctx,
		cancel:     cancel,
		pool:       pool,
		broadcasts: &sync.Map{},
		quote:      viper.GetString("market.quote_currency"),
		known:      &sync.Map{},
	}

	for _, channel := range []string{"kraken:public"} {
		instrument.broadcasts.Store(
			channel, pool.CreateBroadcastGroup(channel),
		)
	}

	return instrument
}

func (instrument *Instrument) ensurePairsRing(capacity int) {
	if instrument.pairs != nil {
		return
	}

	if capacity <= 0 {
		capacity = viper.GetInt("market.max_scan_symbols")
	}

	if capacity <= 0 {
		capacity = 1024
	}

	instrument.pairs = structure.NewListRing[market.InstrumentPair](
		capacity,
		datura.Acquire("instrument", datura.Artifact_Type_json),
	)
}

/*
Update ingests the instrument catalog and returns newly discovered quote symbols.
*/
func (instrument *Instrument) Update(update market.InstrumentUpdate) ([]string, error) {
	instrument.ensurePairsRing(len(update.Pairs))

	added := make([]string, 0, len(update.Pairs))

	for _, pair := range update.Pairs {
		if pair.Quote != instrument.quote || pair.Status != "online" {
			continue
		}

		if pair.Symbol == "" {
			continue
		}

		if _, exists := instrument.known.LoadOrStore(pair.Symbol, struct{}{}); exists {
			continue
		}

		if !instrument.pairs.Push(pair) {
			instrument.err = errnie.Error(errnie.Err(
				errnie.Validation,
				"instrument: failed to push pair",
				errors.New("instrument: failed to push pair"),
			))
		}

		added = append(added, pair.Symbol)
	}

	return added, nil
}

func (instrument *Instrument) sendSubscribe(params any) error {
	message, err := types.NewKrakenMessage("subscribe", params, 0)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"instrument: failed to build subscribe message",
			err,
		))
	}

	payload, err := sonic.Marshal(message)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"instrument: failed to marshal subscribe message",
			err,
		))
	}

	artifact := datura.Acquire(
		"instrument", datura.Artifact_Type_json,
	).WithDestination(
		"kraken:public",
	).WithRole(
		"subscribe",
	).WithPayload(
		payload,
	)

	bg, _ := instrument.broadcasts.Load("kraken:public")

	return errnie.Error(bg.(*qpool.BroadcastGroup).Send(artifact))
}

func (instrument *Instrument) Subscribe() error {
	return instrument.sendSubscribe(market.InstrumentParams{
		Channel:  "instrument",
		Snapshot: true,
	})
}

func (instrument *Instrument) SubscribeSymbols() error {
	if instrument.pairs == nil {
		return nil
	}

	symbols := make([]string, 0)

	instrument.pairs.Do(func(pair market.InstrumentPair) {
		if pair.Symbol == "" {
			return
		}

		symbols = append(symbols, pair.Symbol)
	})

	if len(symbols) == 0 {
		return nil
	}

	batchSize := viper.GetInt("market.subscribe_batch")

	if batchSize <= 0 {
		batchSize = 100
	}

	pace := viper.GetDuration("market.subscribe_pace")

	bookDepth := viper.GetInt("market.book.depth")

	if bookDepth <= 0 {
		bookDepth = viper.GetInt("market.book_depth_levels")
	}

	if bookDepth <= 0 {
		bookDepth = 10
	}

	for batchStart := 0; batchStart < len(symbols); batchStart += batchSize {
		batch := symbols[batchStart:min(batchStart+batchSize, len(symbols))]

		for _, trigger := range market.TickerTriggers() {
			if err := instrument.sendSubscribe(market.TickerParams{
				Channel:      "ticker",
				Symbol:       batch,
				Snapshot:     true,
				EventTrigger: trigger,
			}); err != nil {
				return err
			}

			if pace > 0 {
				time.Sleep(pace)
			}
		}

		if err := instrument.sendSubscribe(market.BookParams{
			Channel:  "book",
			Symbol:   batch,
			Depth:    bookDepth,
			Snapshot: true,
		}); err != nil {
			return err
		}

		if pace > 0 {
			time.Sleep(pace)
		}

		if err := instrument.sendSubscribe(market.TradeParams{
			Channel:  "trade",
			Symbol:   batch,
			Snapshot: true,
		}); err != nil {
			return err
		}

		if pace > 0 {
			time.Sleep(pace)
		}
	}

	return nil
}

func (instrument *Instrument) SubscribeAnchorOhlc() error {
	anchor := strings.TrimSpace(viper.GetString("market.anchor_symbol"))

	if anchor == "" {
		return nil
	}

	intervalMinutes := viper.GetInt("market.chart.ohlc_interval")

	if intervalMinutes <= 0 {
		intervalMinutes = 1
	}

	return instrument.sendSubscribe(market.CandleParams{
		Channel:  "ohlc",
		Symbol:   []string{anchor},
		Interval: intervalMinutes,
		Snapshot: true,
	})
}

func (instrument *Instrument) Error() error {
	return instrument.err
}

func (instrument *Instrument) Close() error {
	instrument.cancel()
	return nil
}
