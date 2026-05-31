package trader

import (
	"sync"
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives"
)

/*
Tick runs one dedicated goroutine per inbound channel so high-volume feeds
(trades, books, measurements) cannot starve order events or the heartbeat.
*/
func (crypto *Crypto) Tick() error {
	var waitGroup sync.WaitGroup

	waitGroup.Go(crypto.runHeartbeatLoop)
	waitGroup.Go(crypto.runTickerLoop)
	waitGroup.Go(crypto.runBookLoop)
	waitGroup.Go(crypto.runTradeLoop)
	waitGroup.Go(crypto.runMeasurementLoop)
	waitGroup.Go(crypto.runFillLoop)
	waitGroup.Go(crypto.runAckLoop)

	waitGroup.Wait()

	return crypto.ctx.Err()
}

func (crypto *Crypto) runHeartbeatLoop() {
	heartbeat := time.NewTicker(config.System.UIHeartbeatInterval)
	defer heartbeat.Stop()

	for {
		select {
		case <-crypto.ctx.Done():
			return
		case <-heartbeat.C:
			crypto.refreshCrossSection()
			crypto.advanceStressMachine()
			crypto.publishEnginePulse()
			crypto.publishDecisionTrace()
			crypto.publishWallet()
			crypto.advanceMakerFallback()
		}
	}
}

func (crypto *Crypto) runTickerLoop() {
	for row := range market.NewTickerSubscription(crypto.ctx, config.System.Symbols...) {
		if row == nil {
			continue
		}

		crypto.quotes.ingestTicker(*row)
	}
}

func (crypto *Crypto) runBookLoop() {
	for update := range market.NewBookSubscription(
		crypto.ctx, config.System.BookDepthLevels, config.System.Symbols...,
	) {
		if update == nil {
			continue
		}

		crypto.quotes.ingestBook(*update)
	}
}

func (crypto *Crypto) runTradeLoop() {
	for trade := range market.NewTradeSubscription(crypto.ctx, config.System.Symbols...) {
		if trade == nil {
			continue
		}

		crypto.makers.observeTrade(*trade)
		crypto.tryPaperMakerFills()
	}
}

func (crypto *Crypto) runMeasurementLoop() {
	if crypto.measurements == nil {
		<-crypto.ctx.Done()

		return
	}

	for {
		select {
		case <-crypto.ctx.Done():
			return
		case value, ok := <-crypto.measurements.Incoming:
			if !ok {
				return
			}

			if value.Value == nil {
				continue
			}

			measurement, measurementOK := value.Value.(perspectives.Measurement)

			if !measurementOK || measurement.Symbol == "" {
				continue
			}

			crypto.record(measurement)
			crypto.evaluate(measurement.Symbol, measurement.Last)
		}
	}
}

func (crypto *Crypto) runFillLoop() {
	fills := crypto.paper.Fills()

	if crypto.live != nil {
		fills = crypto.live.Fills()
	}

	for {
		select {
		case <-crypto.ctx.Done():
			return
		case fill, ok := <-fills:
			if !ok {
				return
			}

			crypto.handleOrderFill(fill)
		}
	}
}

func (crypto *Crypto) runAckLoop() {
	acks := crypto.paper.Acks()

	if crypto.live != nil {
		acks = crypto.live.Acks()
	}

	for {
		select {
		case <-crypto.ctx.Done():
			return
		case ack, ok := <-acks:
			if !ok {
				return
			}

			crypto.handleOrderAck(ack)
		}
	}
}
