package trader

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/kraken/market"
)

type OHLC struct {
	ctx    context.Context
	cancel context.CancelFunc
	bus    *internal.Bus
}

func NewOHLC(
	ctx context.Context, bus *internal.Bus,
) *OHLC {
	ctx, cancel := context.WithCancel(ctx)

	return &OHLC{
		ctx:    ctx,
		cancel: cancel,
		bus:    bus,
	}
}

func (ohlc *OHLC) Tick(message *qpool.QValue[any]) error {
	updates, ok := message.Value.(*market.CandleUpdates)

	if !ok || updates == nil {
		return errnie.Err(
			errnie.Validation,
			"crypto: invalid ohlc",
			errors.New(message.Type),
		)
	}

	for _, candle := range *updates {
		if candle == nil || candle.Symbol == "" || candle.IntervalBegin == "" {
			continue
		}

		eventAt, parseErr := time.Parse(time.RFC3339Nano, candle.IntervalBegin)

		if parseErr != nil {
			eventAt, parseErr = time.Parse(time.RFC3339, candle.IntervalBegin)
		}

		if parseErr != nil {
			errnie.Error(fmt.Errorf("crypto: ohlc interval_begin: %w", parseErr))
			continue
		}

		if err := errnie.Error(ohlc.bus.Send(
			internal.ChannelUI,
			"ohlc",
			map[string]any{
				"symbol": candle.Symbol,
				"sec":    float64(eventAt.Unix()),
				"open":   candle.Open,
				"high":   candle.High,
				"low":    candle.Low,
				"close":  candle.Close,
				"volume": candle.Volume,
			},
		)); errnie.Error(err) != nil {
			return errnie.Err(
				errnie.IO,
				"crypto: failed to send ohlc",
				err,
			)
		}
	}

	return nil
}
