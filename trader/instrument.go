package trader

import (
	"context"
	"errors"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/market"
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

/*
Update ingests the instrument catalog and returns newly discovered quote symbols.
*/
func (instrument *Instrument) Update(update market.InstrumentUpdate) error {
	errnie.Debug("instrument: updating instrument catalog")

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

	}

	return nil
}

func (instrument *Instrument) Subscribe() error {
	errnie.Debug("instrument: subscribing to instrument catalog")

	pairs := make([]market.InstrumentPair, 0)

	instrument.pairs.Do(func(pair market.InstrumentPair) {
		pairs = append(pairs, pair)
	})

	// Split pairs into batches of 100
	batches := make([][]market.InstrumentPair, 0)

	for i := 0; i < len(pairs); i += 100 {
		batches = append(batches, pairs[i:min(i+100, len(pairs))])
	}

	for _, batch := range batches {
		payload, err := sonic.Marshal(batch)

		if err != nil {
			return err
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

		bg, _ := instrument.broadcasts.Load(
			"kraken:public",
		)

		errnie.Error(bg.(*qpool.BroadcastGroup).Send(artifact))
	}

	return nil
}

func (instrument *Instrument) Error() error {
	return instrument.err
}

func (instrument *Instrument) Close() error {
	instrument.cancel()
	return nil
}
