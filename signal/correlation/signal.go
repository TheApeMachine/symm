package correlation

import (
	"context"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
Signal measures whether a symbol is moving with the cohort, against it, beyond
it, or without a stable relation to it. Categories belong in logic; this signal
emits numerical scores only.
*/
type Signal struct {
	ctx     context.Context
	cancel  context.CancelFunc
	ticker  *Ticker
	section *Section
}

func NewSignal(ctx context.Context, api *websocket.API) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:     ctx,
		cancel:  cancel,
		ticker:  NewTicker(ctx, api),
		section: NewSection(),
	}
}

func (signal *Signal) Measure(
	thesis *types.Thesis,
) *types.Thesis {
	rows := signal.ticker.cache
	out := datura.Map[datura.Map[*decimal.Decimal]]{}

	thesis.CrossSection.Observe(rows)

	for _, row := range rows {
		if row.Symbol == "" {
			continue
		}

		scores, ok := signal.section.Scores(row.Symbol, thesis.CrossSection)

		if !ok {
			continue
		}

		metrics := datura.Map[*decimal.Decimal]{
			"correlation":    decimal.NewFromFloat64(scores["correlation"]),
			"signed":         decimal.NewFromFloat64(scores["signed"]),
			"relativeEnergy": decimal.NewFromFloat64(scores["relativeEnergy"]),
			"herdScore":      decimal.NewFromFloat64(scores["herdScore"]),
			"alphaScore":     decimal.NewFromFloat64(scores["alphaScore"]),
			"noiseScore":     decimal.NewFromFloat64(scores["noiseScore"]),
			"stressScore":    decimal.NewFromFloat64(scores["stressScore"]),
			"peakScore":      decimal.NewFromFloat64(scores["peakScore"]),
			"strength":       decimal.NewFromFloat64(scores["strength"]),
		}

		out[row.Symbol] = metrics
	}

	signal.ticker.cache = signal.ticker.cache[:0]

	thesis.Signals.Store("tickers", rows)
	thesis.Measurements.Store("correlation", out)

	return thesis
}

func (signal *Signal) Close() (err error) {
	err = errnie.Error(errnie.Err(
		errnie.Internal,
		"signal: close failed",
		nil,
	))

	signal.cancel()
	return err
}
