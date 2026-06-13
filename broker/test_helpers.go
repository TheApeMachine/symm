package broker

import (
	"context"
	"testing"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	symmmarket "github.com/theapemachine/symm/market"
)

func newTestDesk(
	test *testing.T,
	ctx context.Context,
	pool *qpool.Q[any],
) (*Desk, *symmmarket.TouchRegistry) {
	test.Helper()

	startTestOrderSink(ctx, pool)

	registry := symmmarket.NewTestTouchRegistry(test, ctx, pool)
	desk := NewDesk(ctx, pool, registry)

	return desk, registry
}

func startTestOrderSink(ctx context.Context, pool *qpool.Q[any]) {
	sink := internal.NewBus(
		ctx,
		pool,
		nil,
		[]internal.Subscription{
			internal.Subscribe(internal.ChannelKrakenPrivate, "broker-test-order-sink"),
		},
	)

	go func() {
		for {
			_, receiveErr := sink.Receive(internal.ChannelKrakenPrivate)

			if internal.IsShutdown(receiveErr) {
				return
			}
		}
	}()
}

/*
SeedSpreadHistory primes quote-dynamics baselines for pre-trade risk tests.
*/
func SeedSpreadHistory(
	riskGate *TickerPreTradeRiskGate,
	symbol string,
	bid float64,
	ask float64,
	now time.Time,
) {
	envelope, envelopeErr := symmmarket.LoadDynamicsEnvelope()

	if envelopeErr != nil {
		panic(envelopeErr)
	}

	for range envelope.MinSamples {
		riskGate.RecordQuote(QuoteSnapshot{
			Symbol:     symbol,
			Bid:        bid,
			Ask:        ask,
			Last:       (bid + ask) / 2,
			ObservedAt: now,
		})
	}
}

/*
SeedDeskQuoteReadiness installs touch and spread baselines for integration harnesses.
*/
func SeedDeskQuoteReadiness(
	desk *Desk,
	touchRegistry *symmmarket.TouchRegistry,
	symbol string,
	bid float64,
	ask float64,
	last float64,
) {
	if desk == nil || touchRegistry == nil || symbol == "" {
		return
	}

	now := time.Now().UTC()
	touchRegistry.SeedTouch(symmmarket.TouchSnapshot{
		Symbol:     symbol,
		Bid:        bid,
		Ask:        ask,
		Last:       last,
		ObservedAt: now,
	})
	desk.syncTouchQuote(symbol)

	gate, gateOK := desk.riskGate.(*TickerPreTradeRiskGate)

	if !gateOK {
		return
	}

	SeedSpreadHistory(gate, symbol, bid, ask, now)
}

func seedEntryStopQuote(
	desk *Desk,
	touchRegistry *symmmarket.TouchRegistry,
	symbol string,
	mid float64,
	spreadBps float64,
) {
	halfSpread := mid * (spreadBps / basisPointsPerUnit) / 2

	SeedDeskQuoteReadiness(
		desk,
		touchRegistry,
		symbol,
		mid-halfSpread,
		mid+halfSpread,
		mid,
	)
}
