package trader

import (
	"context"
	"errors"
	"strings"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/market"
)

type Instrument struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	pairs  structure.Ring[market.InstrumentPair]
	quote  string
}

func NewInstrument(ctx context.Context) *Instrument {
	ctx, cancel := context.WithCancel(ctx)

	instrument := &Instrument{
		ctx:    ctx,
		cancel: cancel,
		quote:  viper.GetString("market.quote_currency"),
	}

	return instrument
}

func (instrument *Instrument) Update(update market.InstrumentUpdate) {
	if instrument.pairs == nil {
		instrument.pairs = structure.NewListRing[market.InstrumentPair](
			len(update.Pairs),
			datura.Acquire("instrument", datura.Artifact_Type_json),
		)
	}

	var added strings.Builder

	for _, pair := range update.Pairs {
		if pair.Quote != instrument.quote || pair.Status != "online" {
			continue
		}

		if !instrument.pairs.Push(pair) {
			instrument.err = errnie.Error(errnie.Err(
				errnie.Validation,
				"instrument: failed to push pair",
				errors.New("instrument: failed to push pair"),
			))
		}

		added.WriteString(pair.Symbol)
		added.WriteString(" ")
	}

	if added.Len() > 0 {
		errnie.Info("instrument: added pairs:" + added.String())
	}
}

func (instrument *Instrument) Error() error {
	return instrument.err
}

func (instrument *Instrument) Close() error {
	instrument.cancel()
	return nil
}
